package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/Bibi40k/vmware-vm-bootstrap/configs"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vcenter"
	"github.com/chzyer/readline"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// VMWizardOutput is the YAML structure for vm.*.sops.yaml files.
type VMWizardOutput struct {
	VM struct {
		Name              string `yaml:"name"`
		CPUs              int    `yaml:"cpus"`
		MemoryMB          int    `yaml:"memory_mb"`
		DiskSizeGB        int    `yaml:"disk_size_gb"`
		DataDiskSizeGB    int    `yaml:"data_disk_size_gb,omitempty"`
		DataDiskMountPath string `yaml:"data_disk_mount_path,omitempty"`
		SwapSizeGB        *int   `yaml:"swap_size_gb,omitempty"`
		UbuntuVersion     string `yaml:"ubuntu_version"`
		Username          string `yaml:"username"`
		SSHKeyPath        string `yaml:"ssh_key_path,omitempty"`
		SSHKey            string `yaml:"ssh_key,omitempty"`
		Password          string `yaml:"password,omitempty"`
		AllowPasswordSSH  bool   `yaml:"allow_password_ssh,omitempty"`
		SSHPort           int    `yaml:"ssh_port,omitempty"`
		IPAddress         string `yaml:"ip_address"`
		Netmask           string `yaml:"netmask"`
		Gateway           string `yaml:"gateway"`
		DNS               string `yaml:"dns"`
		DNS2              string `yaml:"dns2,omitempty"`
		Datastore         string `yaml:"datastore,omitempty"`
		NetworkName       string `yaml:"network_name,omitempty"`
		NetworkInterface  string `yaml:"network_interface,omitempty"`
		Folder            string `yaml:"folder,omitempty"`
		ResourcePool      string `yaml:"resource_pool,omitempty"`
		TimeoutMinutes    int    `yaml:"timeout_minutes"`
	} `yaml:"vm"`
}

// vcenterFileConfig is the YAML structure for vcenter.sops.yaml.
type vcenterFileConfig struct {
	VCenter struct {
		Host         string `yaml:"host"`
		Username     string `yaml:"username"`
		Password     string `yaml:"password"`
		Datacenter   string `yaml:"datacenter"`
		ISODatastore string `yaml:"iso_datastore"`
		Folder       string `yaml:"folder"`        // default VM folder for new VMs
		ResourcePool string `yaml:"resource_pool"` // default resource pool for new VMs
		Network      string `yaml:"network"`       // default network for new VMs
		Port         int    `yaml:"port"`
		Insecure     bool   `yaml:"insecure"`
	} `yaml:"vcenter"`
}

// datastoreCandidate holds a scored datastore for recommendations.
type datastoreCandidate struct {
	Info         vcenter.DatastoreInfo
	FreeAfterGB  float64
	FreePctAfter float64
	LatencyMs    float64
	Score        float64
	Rationale    string
}

// ─── Edit existing configs ───────────────────────────────────────────────────

func editVCenterConfig(path string) error {
	fmt.Printf("\nEdit: %s\n", filepath.Base(path))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	data, err := sopsDecrypt(path)
	if err != nil {
		return err
	}

	var cfg vcenterFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}

	// Connect to vCenter upfront to fetch datastores + folders for pickers.
	fmt.Print("  Connecting to vCenter... ")
	type vcCatalog struct {
		datastores []vcenter.DatastoreInfo
		networks   []vcenter.NetworkInfo
		folders    []vcenter.FolderInfo
		pools      []vcenter.ResourcePoolInfo
	}
	cat, catErr := func() (*vcCatalog, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		vclient, err := vcenter.NewClient(ctx, &vcenter.Config{
			Host:     cfg.VCenter.Host,
			Username: cfg.VCenter.Username,
			Password: cfg.VCenter.Password,
			Port:     cfg.VCenter.Port,
			Insecure: cfg.VCenter.Insecure,
		})
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = vclient.Disconnect()
		}()
		ds, _ := vclient.ListDatastores(cfg.VCenter.Datacenter)
		nets, _ := vclient.ListNetworks(cfg.VCenter.Datacenter)
		fl, _ := vclient.ListFolders(cfg.VCenter.Datacenter)
		pl, _ := vclient.ListResourcePools(cfg.VCenter.Datacenter)
		return &vcCatalog{datastores: ds, networks: nets, folders: fl, pools: pl}, nil
	}()
	if catErr != nil {
		fmt.Printf("\033[33m⚠ %v\033[0m (will use manual input)\n", catErr)
	} else {
		fmt.Printf("\033[32m✓\033[0m  (%d datastores, %d networks, %d folders, %d pools)\n",
			len(cat.datastores), len(cat.networks), len(cat.folders), len(cat.pools))
	}
	fmt.Println()

	v := &cfg.VCenter
	v.Host = readLine("Host", v.Host)
	v.Username = readLine("Username", v.Username)
	if pw := readPassword("Password (blank = keep current)"); pw != "" {
		v.Password = pw
	}
	v.Datacenter = readLine("Datacenter", v.Datacenter)

	if catErr != nil {
		v.ISODatastore = readLine("ISO datastore (where Ubuntu + seed ISOs are stored)", v.ISODatastore)
		v.Folder = readLine("Default VM folder", v.Folder)
		v.ResourcePool = readLine("Default resource pool", v.ResourcePool)
		v.Network = readLine("Default network", v.Network)
	} else {
		fmt.Println("  ISO datastore (where Ubuntu + seed ISOs are stored):")
		v.ISODatastore = selectISODatastore(cat.datastores, v.ISODatastore)
		v.Folder = selectFolder(cat.folders, v.Folder, "Default VM folder:")
		v.ResourcePool = selectResourcePool(cat.pools, v.ResourcePool, "Default resource pool:")
		if len(cat.networks) > 0 {
			var netNames []string
			for _, n := range cat.networks {
				parts := strings.Split(n.Name, "/")
				netNames = append(netNames, parts[len(parts)-1])
			}
			v.Network = interactiveSelect(netNames, v.Network, "Default network:")
		} else {
			v.Network = readLine("Default network", v.Network)
		}
	}

	fmt.Println()
	if !readYesNo("Save and re-encrypt?", true) {
		fmt.Println("  Cancelled — no changes saved.")
		return nil
	}

	if err := saveAndEncrypt(path, cfg, ""); err != nil {
		return err
	}

	fmt.Printf("\n\033[32m✓ Saved and encrypted: %s\033[0m\n", filepath.Base(path))
	return nil
}

func createVCenterConfig(path string) error {
	return createVCenterConfigWithSeed(path, vcenterFileConfig{}, "")
}

func createVCenterConfigWithDraft(path, draftPath string) error {
	data, err := os.ReadFile(draftPath)
	if err != nil {
		return err
	}
	var cfg vcenterFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse draft: %w", err)
	}
	fmt.Printf("\n\033[33m⚠ Resuming draft: %s\033[0m\n", filepath.Base(draftPath))
	return createVCenterConfigWithSeed(path, cfg, draftPath)
}

func createVCenterConfigWithSeed(path string, seed vcenterFileConfig, draftPath string) error {
	fmt.Printf("\nCreate: %s\n", filepath.Base(path))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	cfg := seed
	v := &cfg.VCenter

	stopDraftHandler := startDraftInterruptHandler(path, draftPath, func() ([]byte, bool) {
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, false
		}
		if strings.TrimSpace(string(data)) == "vcenter: {}\n" {
			return nil, false
		}
		return data, true
	})
	defer stopDraftHandler()

	v.Host = readLine("Host", v.Host)
	v.Username = readLine("Username", v.Username)
	if v.Password != "" {
		if readYesNo("Use saved password?", true) {
			// keep existing
		} else {
			v.Password = readPassword("Password")
		}
	} else {
		v.Password = readPassword("Password")
	}
	v.Datacenter = readLine("Datacenter", v.Datacenter)
	v.Port = readInt("Port", intOrDefault(v.Port, configs.Defaults.VCenter.Port), 1, 65535)
	v.Insecure = readYesNo("Skip TLS verification? (not recommended)", v.Insecure)

	// Try to connect and fetch resource pickers.
	fmt.Print("  Connecting to vCenter... ")
	type vcCatalog struct {
		datastores []vcenter.DatastoreInfo
		networks   []vcenter.NetworkInfo
		folders    []vcenter.FolderInfo
		pools      []vcenter.ResourcePoolInfo
	}
	cat, catErr := func() (*vcCatalog, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		vclient, err := vcenter.NewClient(ctx, &vcenter.Config{
			Host:     v.Host,
			Username: v.Username,
			Password: v.Password,
			Port:     v.Port,
			Insecure: v.Insecure,
		})
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = vclient.Disconnect()
		}()
		ds, _ := vclient.ListDatastores(v.Datacenter)
		nets, _ := vclient.ListNetworks(v.Datacenter)
		fl, _ := vclient.ListFolders(v.Datacenter)
		pl, _ := vclient.ListResourcePools(v.Datacenter)
		return &vcCatalog{datastores: ds, networks: nets, folders: fl, pools: pl}, nil
	}()
	if catErr != nil {
		fmt.Printf("\033[33m⚠ %v\033[0m (will use manual input)\n", catErr)
	} else {
		fmt.Printf("\033[32m✓\033[0m  (%d datastores, %d networks, %d folders, %d pools)\n",
			len(cat.datastores), len(cat.networks), len(cat.folders), len(cat.pools))
	}
	fmt.Println()

	if catErr != nil {
		v.ISODatastore = readLine("ISO datastore (where Ubuntu + seed ISOs are stored)", v.ISODatastore)
		v.Folder = readLine("Default VM folder", v.Folder)
		v.ResourcePool = readLine("Default resource pool", v.ResourcePool)
		v.Network = readLine("Default network", v.Network)
	} else {
		fmt.Println("  ISO datastore (where Ubuntu + seed ISOs are stored):")
		v.ISODatastore = selectISODatastore(cat.datastores, v.ISODatastore)
		v.Folder = selectFolder(cat.folders, v.Folder, "Default VM folder:")
		v.ResourcePool = selectResourcePool(cat.pools, v.ResourcePool, "Default resource pool:")
		if len(cat.networks) > 0 {
			var netNames []string
			for _, n := range cat.networks {
				parts := strings.Split(n.Name, "/")
				netNames = append(netNames, parts[len(parts)-1])
			}
			v.Network = interactiveSelect(netNames, v.Network, "Default network:")
		} else {
			v.Network = readLine("Default network", v.Network)
		}
	}

	fmt.Println()
	if !readYesNo("Save and encrypt?", true) {
		fmt.Println("  Cancelled — no changes saved.")
		return nil
	}

	if err := saveAndEncrypt(path, cfg, draftPath); err != nil {
		return err
	}

	fmt.Printf("\n\033[32m✓ Saved and encrypted: %s\033[0m\n", filepath.Base(path))
	return nil
}

func editVMConfig(path string) error {
	fmt.Printf("\nEdit: %s\n", filepath.Base(path))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	data, err := sopsDecrypt(path)
	if err != nil {
		return err
	}

	var cfg VMWizardOutput
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}

	// Connect to vCenter upfront to fetch pickers (same pattern as runCreateWizard).
	fmt.Print("  Connecting to vCenter... ")
	vcCfg, vcfgErr := loadVCenterConfig(vcenterConfigFile)

	var datastores []vcenter.DatastoreInfo
	var networks []vcenter.NetworkInfo
	var folders []vcenter.FolderInfo
	var pools []vcenter.ResourcePoolInfo
	var dsErr, netErr, folderErr, poolErr error

	if vcfgErr != nil {
		fmt.Printf("\033[33m⚠ %v\033[0m (pickers unavailable)\n", vcfgErr)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		vclient, vcErr := vcenter.NewClient(ctx, &vcenter.Config{
			Host:     vcCfg.VCenter.Host,
			Username: vcCfg.VCenter.Username,
			Password: vcCfg.VCenter.Password,
			Port:     vcCfg.VCenter.Port,
			Insecure: vcCfg.VCenter.Insecure,
		})
		if vcErr != nil {
			cancel()
			fmt.Printf("\033[33m⚠ %v\033[0m (pickers unavailable)\n", vcErr)
		} else {
			datastores, dsErr = vclient.ListDatastores(vcCfg.VCenter.Datacenter)
			networks, netErr = vclient.ListNetworks(vcCfg.VCenter.Datacenter)
			folders, folderErr = vclient.ListFolders(vcCfg.VCenter.Datacenter)
			pools, poolErr = vclient.ListResourcePools(vcCfg.VCenter.Datacenter)
			_ = vclient.Disconnect()
			cancel()
			fmt.Printf("\033[32m✓\033[0m  (%d datastores, %d networks, %d folders, %d pools)\n",
				len(datastores), len(networks), len(folders), len(pools))
		}
	}
	fmt.Println()

	v := &cfg.VM

	// === [1] VM Specs ===
	fmt.Println("[1/5] VM Specs")
	v.Name = readLine("VM name", v.Name)
	v.CPUs = readInt("CPU cores", intOrDefault(v.CPUs, 4), 1, 64)
	ramGB := intOrDefault(v.MemoryMB/1024, 16)
	v.MemoryMB = readInt("RAM (GB)", ramGB, 1, 512) * 1024
	v.DiskSizeGB = readInt("OS disk (GB)", intOrDefault(v.DiskSizeGB, 50), 10, 2000)
	if v.DataDiskSizeGB > 0 {
		v.DataDiskSizeGB = readInt("Data disk (GB)", v.DataDiskSizeGB, 10, 2000)
		v.DataDiskMountPath = readLine("Mount point", strOrDefault(v.DataDiskMountPath, "/data"))
	} else if readYesNo("Add separate data disk?", false) {
		v.DataDiskSizeGB = readInt("Data disk (GB)", 500, 10, 2000)
		v.DataDiskMountPath = readLine("Mount point", "/data")
	}
	defaultSwap := configs.Defaults.CloudInit.SwapSizeGB
	if v.SwapSizeGB != nil {
		defaultSwap = *v.SwapSizeGB
	}
	swap := readInt("Swap size (GB, 0 = no swap)", defaultSwap, 0, 64)
	v.SwapSizeGB = &swap
	fmt.Println()

	// === [2] OS Version ===
	fmt.Println("[2/5] OS Version")
	ubuntuOptions := buildUbuntuOptions()
	defaultUbuntu := ubuntuOptions[0]
	for _, opt := range ubuntuOptions {
		if strings.HasPrefix(opt, v.UbuntuVersion) {
			defaultUbuntu = opt
			break
		}
	}
	var ubuntuChoice string
	surveySelect(&survey.Select{Message: "Ubuntu version:", Options: ubuntuOptions, Default: defaultUbuntu}, &ubuntuChoice)
	v.UbuntuVersion = strings.Split(ubuntuChoice, " ")[0]
	fmt.Println()

	// === [3] Placement & Storage ===
	fmt.Println("[3/5] Placement & Storage")
	if folderErr != nil || vcfgErr != nil {
		v.Folder = readLine("VM folder", v.Folder)
	} else {
		v.Folder = selectFolder(folders, v.Folder, "VM folder:")
	}
	if poolErr != nil || vcfgErr != nil {
		v.ResourcePool = readLine("Resource pool", v.ResourcePool)
	} else {
		v.ResourcePool = selectResourcePool(pools, v.ResourcePool, "Resource pool:")
	}
	fmt.Println()

	requiredGB := float64(v.DiskSizeGB + v.DataDiskSizeGB)
	if dsErr != nil || vcfgErr != nil {
		v.Datastore = readLine("Datastore", v.Datastore)
	} else {
		v.Datastore = selectDatastore(datastores, requiredGB, v.Datastore)
	}
	fmt.Println()

	// === [4] Network ===
	fmt.Println("[4/5] Network")
	if netErr != nil || vcfgErr != nil || len(networks) == 0 {
		v.NetworkName = readLine("Network name", v.NetworkName)
	} else {
		var netNames []string
		for _, n := range networks {
			parts := strings.Split(n.Name, "/")
			netNames = append(netNames, parts[len(parts)-1])
		}
		v.NetworkName = interactiveSelect(netNames, v.NetworkName, "Network:")
	}
	v.NetworkInterface = readLine("Guest NIC name", strOrDefault(v.NetworkInterface, configs.Defaults.Network.Interface))
	v.IPAddress = readIPLine("IP address", v.IPAddress)
	v.Netmask = readIPLine("Netmask", strOrDefault(v.Netmask, "255.255.255.0"))
	v.Gateway = readIPLine("Gateway", v.Gateway)
	v.DNS = readLine("DNS", v.DNS)
	v.DNS2 = readLine("Secondary DNS (optional, Enter to skip)", v.DNS2)
	fmt.Println()

	// === [5] User & SSH ===
	fmt.Println("[5/5] User & SSH")
	v.Username = readLine("Username", strOrDefault(v.Username, "sysadmin"))
	v.SSHKeyPath = readFilePath("SSH public key file", v.SSHKeyPath)
	v.SSHPort = readInt("SSH port", intOrDefault(v.SSHPort, 22), 1, 65535)
	pwStatus := "not set"
	if v.Password != "" {
		pwStatus = "set"
	}
	if readYesNo(fmt.Sprintf("Change password? (currently %s)", pwStatus), false) {
		v.Password = readPassword("New password (blank = remove)")
	}
	if v.Password != "" {
		v.AllowPasswordSSH = readYesNo("Allow SSH password authentication?", v.AllowPasswordSSH)
	} else {
		v.AllowPasswordSSH = false
	}
	fmt.Println()

	fmt.Println("Summary")
	printSummary(cfg)

	if !readYesNo("Save and re-encrypt?", true) {
		fmt.Println("  Cancelled — no changes saved.")
		return nil
	}

	if err := saveAndEncrypt(path, cfg, ""); err != nil {
		return err
	}

	fmt.Printf("\n\033[32m✓ Saved and encrypted: %s\033[0m\n", filepath.Base(path))
	return nil
}

// saveAndEncrypt marshals v to YAML, writes to path, and encrypts in-place with SOPS.
// If the file already exists, it is backed up and restored on failure.
func saveAndEncrypt(path string, v interface{}, draftPath string) error {
	backup, backupExisted := func() ([]byte, bool) {
		data, err := os.ReadFile(path)
		return data, err == nil
	}()

	plaintext, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}

	if err := sopsEncrypt(path, plaintext); err != nil {
		pathOverride := draftPath
		if pathOverride != "" {
			_ = os.MkdirAll("tmp", 0700)
			_ = os.WriteFile(pathOverride, plaintext, 0600)
		}
		dp := pathOverride
		if dp == "" {
			dp, _ = writeDraft(path, plaintext)
		}
		if dp != "" {
			return &userError{
				msg:  err.Error(),
				hint: fmt.Sprintf("Progress saved (plaintext): %s (delete after use)", dp),
			}
		}
		if backupExisted {
			if restoreErr := os.WriteFile(path, backup, 0600); restoreErr != nil {
				return fmt.Errorf("encrypt failed (%v) and restore failed: %w", err, restoreErr)
			}
		} else {
			if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
				return fmt.Errorf("encrypt failed (%v) and cleanup failed: %w", err, rmErr)
			}
		}
		return err
	}
	if err := cleanupDrafts(path); err != nil {
		return fmt.Errorf("cleanup drafts: %w", err)
	}
	return nil
}

func writeDraft(targetPath string, plaintext []byte) (string, error) {
	if err := os.MkdirAll("tmp", 0700); err != nil {
		return "", err
	}
	base := filepath.Base(targetPath)
	ts := time.Now().Format("20060102-150405")
	draftPath := filepath.Join("tmp", fmt.Sprintf("%s.draft.%s.yaml", base, ts))
	if err := os.WriteFile(draftPath, plaintext, 0600); err != nil {
		return "", err
	}
	return draftPath, nil
}

func cleanupDrafts(targetPath string) error {
	base := filepath.Base(targetPath)
	pattern := filepath.Join("tmp", fmt.Sprintf("%s.draft.*.yaml", base))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, p := range matches {
		if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
			return rmErr
		}
	}
	return nil
}

// startDraftInterruptHandler saves plaintext drafts on Ctrl+C and asks whether to keep them.
func startDraftInterruptHandler(targetPath, draftPath string, dataFn func() ([]byte, bool)) func() {
	localSigCh := make(chan os.Signal, 1)
	signal.Stop(mainSigCh)
	signal.Notify(localSigCh, os.Interrupt)
	go func() {
		<-localSigCh
		if data, ok := dataFn(); ok {
			path := draftPath
			if path == "" {
				path, _ = writeDraft(targetPath, data)
			} else {
				_ = os.MkdirAll("tmp", 0700)
				_ = os.WriteFile(path, data, 0600)
			}
			if path != "" {
				fmt.Printf("\n\033[33m⚠ Interrupted\033[0m\n")
				fmt.Printf("  Draft saved (plaintext): %s (delete after use)\n", path)
			}
		}
		fmt.Println("\nCancelled.")
		restoreTTYOnExit()
		os.Exit(0)
	}()
	return func() {
		signal.Stop(localSigCh)
		signal.Notify(mainSigCh, os.Interrupt)
	}
}

// ─── Create new VM wizard ─────────────────────────────────────────────────────

func runCreateWizard() error {
	return runCreateWizardWithSeed("", "")
}

func runCreateWizardWithDraft(outputFile, draftPath string) error {
	data, err := os.ReadFile(draftPath)
	if err != nil {
		return err
	}
	var out VMWizardOutput
	if err := yaml.Unmarshal(data, &out); err != nil {
		return fmt.Errorf("parse draft: %w", err)
	}
	fmt.Printf("\n\033[33m⚠ Resuming draft: %s\033[0m\n", filepath.Base(draftPath))
	return runCreateWizardWithSeed(outputFile, draftPath)
}

func runCreateWizardWithSeed(outputFile, draftPath string) error {
	if _, err := os.Stat(vcenterConfigFile); os.IsNotExist(err) {
		return &userError{
			msg:  "vCenter config not found",
			hint: "Run: make config → Create vcenter.sops.yaml",
		}
	}
	fmt.Printf("\n\033[1mCreate new VM\033[0m\n")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	var out VMWizardOutput
	if draftPath != "" {
		data, err := os.ReadFile(draftPath)
		if err == nil {
			_ = yaml.Unmarshal(data, &out)
		}
	}

	if outputFile == "" {
		// Config file slug
		var slug string
		for {
			slug = strings.TrimSpace(readLine("Config name (e.g. 'vm1' → configs/vm.vm1.sops.yaml)", ""))
			if slug != "" {
				break
			}
			fmt.Println("  Config name is required")
		}
		outputFile = fmt.Sprintf("configs/vm.%s.sops.yaml", slug)
	}

	if _, err := os.Stat(outputFile); err == nil {
		if !readYesNoDanger(fmt.Sprintf("%s already exists. Overwrite?", outputFile)) {
			fmt.Println("  Cancelled.")
			return nil
		}
	}

	stopDraftHandler := startDraftInterruptHandler(outputFile, draftPath, func() ([]byte, bool) {
		data, err := yaml.Marshal(out)
		if err != nil {
			return nil, false
		}
		return data, true
	})
	defer stopDraftHandler()

	// Connect to vCenter and fetch all resources upfront (before wizard questions).
	// This avoids context deadline issues caused by the user taking time to answer prompts.
	fmt.Print("  Connecting to vCenter... ")
	vcCfg, err := loadVCenterConfig(vcenterConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load vCenter config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	vclient, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     vcCfg.VCenter.Host,
		Username: vcCfg.VCenter.Username,
		Password: vcCfg.VCenter.Password,
		Port:     vcCfg.VCenter.Port,
		Insecure: vcCfg.VCenter.Insecure,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("vCenter connection failed: %w", err)
	}

	// Fetch datastores, networks, and folders while context is still fresh.
	datastores, dsErr := vclient.ListDatastores(vcCfg.VCenter.Datacenter)
	networks, netErr := vclient.ListNetworks(vcCfg.VCenter.Datacenter)
	folders, folderErr := vclient.ListFolders(vcCfg.VCenter.Datacenter)
	pools, poolErr := vclient.ListResourcePools(vcCfg.VCenter.Datacenter)
	_ = vclient.Disconnect()
	cancel()
	fmt.Println("\033[32m✓\033[0m")
	fmt.Println()

	if out.VM.Name == "" {
		out.VM.Name = strings.TrimSuffix(filepath.Base(outputFile), ".sops.yaml")
		out.VM.Name = strings.TrimPrefix(out.VM.Name, "vm.")
	}
	vmName := readLine("VM name in vCenter", out.VM.Name)
	out.VM.Name = vmName

	// === [1] VM Specs ===
	fmt.Println()
	fmt.Println("[1/5] VM Specs")

	defaultCPU := intOrDefault(out.VM.CPUs, 4)
	out.VM.CPUs = readInt("CPU cores", defaultCPU, 1, 64)
	defaultRAM := 16
	if out.VM.MemoryMB > 0 {
		defaultRAM = out.VM.MemoryMB / 1024
	}
	ramGB := readInt("RAM (GB)", defaultRAM, 1, 512)
	out.VM.MemoryMB = ramGB * 1024
	out.VM.DiskSizeGB = readInt("OS disk (GB)", intOrDefault(out.VM.DiskSizeGB, 50), 10, 2000)

	if readYesNo("Add separate data disk?", out.VM.DataDiskSizeGB > 0) {
		defaultData := intOrDefault(out.VM.DataDiskSizeGB, 500)
		out.VM.DataDiskSizeGB = readInt("Data disk (GB)", defaultData, 10, 2000)
		out.VM.DataDiskMountPath = readLine("Mount point", strOrDefault(out.VM.DataDiskMountPath, "/data"))
	}
	defaultSwap := configs.Defaults.CloudInit.SwapSizeGB
	if out.VM.SwapSizeGB != nil {
		defaultSwap = *out.VM.SwapSizeGB
	}
	swap := readInt("Swap size (GB, 0 = no swap)", defaultSwap, 0, 64)
	out.VM.SwapSizeGB = &swap
	fmt.Println()

	// === [2] OS Version ===
	fmt.Println("[2/5] OS Version")
	ubuntuOptions := buildUbuntuOptions()
	var ubuntuChoice string
	surveySelect(&survey.Select{Message: "Ubuntu version:", Options: ubuntuOptions}, &ubuntuChoice)
	out.VM.UbuntuVersion = strings.Split(ubuntuChoice, " ")[0]
	fmt.Println()

	// === [3] Placement & Storage ===
	fmt.Println("[3/5] Placement & Storage")
	if folderErr != nil {
		out.VM.Folder = readLine("VM folder", strOrDefault(out.VM.Folder, vcCfg.VCenter.Folder))
	} else {
		out.VM.Folder = selectFolder(folders, strOrDefault(out.VM.Folder, vcCfg.VCenter.Folder), "VM folder:")
	}
	if poolErr != nil {
		out.VM.ResourcePool = readLine("Resource pool", strOrDefault(out.VM.ResourcePool, vcCfg.VCenter.ResourcePool))
	} else {
		out.VM.ResourcePool = selectResourcePool(pools, strOrDefault(out.VM.ResourcePool, vcCfg.VCenter.ResourcePool), "Resource pool:")
	}
	fmt.Println()

	requiredGB := float64(out.VM.DiskSizeGB + out.VM.DataDiskSizeGB)
	if dsErr != nil {
		fmt.Printf("  ⚠ Could not list datastores: %v\n", dsErr)
		out.VM.Datastore = readLine("Datastore", out.VM.Datastore)
	} else {
		out.VM.Datastore = selectDatastore(datastores, requiredGB, out.VM.Datastore)
	}
	fmt.Println()

	// === [4] Network (cached from upfront fetch) ===
	fmt.Println("[4/5] Network")
	if netErr != nil || len(networks) == 0 {
		if netErr != nil {
			fmt.Printf("  ⚠ Could not list networks: %v\n", netErr)
		}
		out.VM.NetworkName = readLine("Network name", strOrDefault(out.VM.NetworkName, vcCfg.VCenter.Network))
	} else {
		var netNames []string
		for _, n := range networks {
			parts := strings.Split(n.Name, "/")
			netNames = append(netNames, parts[len(parts)-1])
		}
		out.VM.NetworkName = interactiveSelect(netNames, strOrDefault(out.VM.NetworkName, vcCfg.VCenter.Network), "Network:")
	}

	out.VM.NetworkInterface = readLine("Guest NIC name", strOrDefault(out.VM.NetworkInterface, configs.Defaults.Network.Interface))
	out.VM.IPAddress = readIPLine("IP address", out.VM.IPAddress)
	out.VM.Netmask = readIPLine("Netmask", strOrDefault(out.VM.Netmask, "255.255.255.0"))
	out.VM.Gateway = readIPLine("Gateway", strOrDefault(out.VM.Gateway, autoGateway(out.VM.IPAddress)))
	out.VM.DNS = readLine("DNS", strOrDefault(out.VM.DNS, autoFirstDNS(out.VM.Gateway)))
	out.VM.DNS2 = readLine("Secondary DNS (optional, Enter to skip)", out.VM.DNS2)
	fmt.Println()

	// === [5] User & SSH ===
	fmt.Println("[5/5] User & SSH")

	out.VM.Username = readLine("Username", strOrDefault(out.VM.Username, "sysadmin"))
	out.VM.SSHKeyPath = readFilePath("SSH public key file", strOrDefault(out.VM.SSHKeyPath, os.ExpandEnv("$HOME/.ssh/id_ed25519.pub")))
	out.VM.SSHPort = readInt("SSH port", intOrDefault(out.VM.SSHPort, 22), 1, 65535)

	if readYesNo("Set password?", true) {
		out.VM.Password = readPassword("Password")
	}
	if out.VM.Password != "" {
		out.VM.AllowPasswordSSH = readYesNo("Allow SSH password authentication?", false)
	}
	fmt.Println()

	out.VM.TimeoutMinutes = 45

	// Summary
	fmt.Println("Summary")
	printSummary(out)

	if !readYesNo(fmt.Sprintf("Save to %s?", outputFile), true) {
		fmt.Println("\033[33mCancelled — configuration not saved.\033[0m")
		return nil
	}

	if err := saveAndEncrypt(outputFile, out, draftPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n\033[32m✓ Saved and encrypted: %s\033[0m\n", outputFile)
	fmt.Printf("\n  To bootstrap this VM:\n")
	fmt.Printf("    make run VM=%s\n\n", outputFile)
	return nil
}

// ─── Datastore scoring (matches Python resource_selector.py) ─────────────────

func scoreDatastores(datastores []vcenter.DatastoreInfo, requiredGB float64) []datastoreCandidate {
	const minFreePct = 20.0
	const weightSpace = 0.6
	const weightLatency = 0.4

	var eligible []datastoreCandidate
	for _, ds := range datastores {
		if ds.Type != "SSD" || !ds.Accessible || ds.CapacityGB == 0 {
			continue
		}
		freeAfter := ds.FreeSpaceGB - requiredGB
		freePctAfter := freeAfter / ds.CapacityGB * 100
		if freePctAfter < minFreePct {
			continue
		}
		eligible = append(eligible, datastoreCandidate{
			Info:         ds,
			FreeAfterGB:  math.Round(freeAfter*100) / 100,
			FreePctAfter: math.Round(freePctAfter*100) / 100,
			LatencyMs:    2.0, // SSD heuristic
		})
	}

	for i := range eligible {
		ds := eligible[i].Info
		freePct := ds.FreeSpaceGB / ds.CapacityGB * 100
		spaceScore := math.Min(freePct, 100)
		latencyScore := math.Max(0, math.Min(100, (10-eligible[i].LatencyMs)*11.11))
		eligible[i].Score = math.Round((spaceScore*weightSpace+latencyScore*weightLatency)*100) / 100
		eligible[i].Rationale = buildRationale(eligible[i])
	}

	sort.Slice(eligible, func(i, j int) bool { return eligible[i].Score > eligible[j].Score })
	if len(eligible) > 3 {
		eligible = eligible[:3]
	}
	return eligible
}

func buildRationale(c datastoreCandidate) string {
	var parts []string
	switch {
	case c.FreePctAfter > 50:
		parts = append(parts, fmt.Sprintf("plenty of free space (%.1f%% after allocation)", c.FreePctAfter))
	case c.FreePctAfter > 30:
		parts = append(parts, fmt.Sprintf("adequate free space (%.1f%% after allocation)", c.FreePctAfter))
	default:
		parts = append(parts, fmt.Sprintf("sufficient free space (%.1f%% after allocation)", c.FreePctAfter))
	}
	switch {
	case c.LatencyMs < 3:
		parts = append(parts, fmt.Sprintf("excellent performance (%.1fms)", c.LatencyMs))
	case c.LatencyMs < 5:
		parts = append(parts, fmt.Sprintf("good performance (%.1fms)", c.LatencyMs))
	default:
		parts = append(parts, fmt.Sprintf("acceptable performance (%.1fms)", c.LatencyMs))
	}
	return strings.Join(parts, ", ")
}

// selectDatastore shows a unified survey.Select with top-scored datastores marked ★.
// The score and free-space info are embedded directly in the option labels so
// recommendations and the selection list are shown only once (no duplicate display).
func selectDatastore(datastores []vcenter.DatastoreInfo, requiredGB float64, defaultDS string) string {
	recs := scoreDatastores(datastores, requiredGB)
	recSet := make(map[string]datastoreCandidate, len(recs))
	for _, r := range recs {
		recSet[r.Info.Name] = r
	}

	var opts []string
	var dsNames []string

	// Top-scored datastores first, with ★ prefix and score.
	for _, r := range recs {
		label := fmt.Sprintf("★ %s  (score: %.0f · %.0f/%.0f GB free)",
			r.Info.Name, r.Score, r.Info.FreeSpaceGB, r.Info.CapacityGB)
		opts = append(opts, label)
		dsNames = append(dsNames, r.Info.Name)
	}

	// Remaining SSD datastores (not in top-scored list).
	for _, ds := range datastores {
		if _, isRec := recSet[ds.Name]; isRec {
			continue
		}
		if ds.Type == "SSD" && ds.Accessible {
			label := fmt.Sprintf("  %s  (%.0f/%.0f GB free)", ds.Name, ds.FreeSpaceGB, ds.CapacityGB)
			opts = append(opts, label)
			dsNames = append(dsNames, ds.Name)
		}
	}

	// Fallback: any accessible datastore if no SSD found.
	if len(opts) == 0 {
		for _, ds := range datastores {
			if ds.Accessible {
				opts = append(opts, fmt.Sprintf("  %s  (%.0f GB free)", ds.Name, ds.FreeSpaceGB))
				dsNames = append(dsNames, ds.Name)
			}
		}
	}

	if len(opts) == 0 {
		return defaultDS
	}

	// Pre-select the current datastore if it appears in the list.
	defaultOpt := opts[0]
	for i, name := range dsNames {
		if name == defaultDS {
			defaultOpt = opts[i]
			break
		}
	}

	var choice string
	surveySelect(&survey.Select{Message: "Select datastore:", Options: opts, Default: defaultOpt}, &choice)
	for i, opt := range opts {
		if opt == choice {
			return dsNames[i]
		}
	}
	return defaultDS
}

// selectFolder shows all VM folders with survey.Select. Returns empty string for root.
func selectFolder(folders []vcenter.FolderInfo, defaultFolder, message string) string {
	const rootLabel = "  / (root vm folder)"
	opts := []string{rootLabel}
	names := []string{""}

	defaultOpt := rootLabel
	for _, f := range folders {
		opts = append(opts, f.Name)
		names = append(names, f.Name)
		if f.Name == defaultFolder {
			defaultOpt = f.Name
		}
	}

	var choice string
	surveySelect(&survey.Select{
		Message: message,
		Options: opts,
		Default: defaultOpt,
	}, &choice)
	for i, opt := range opts {
		if opt == choice {
			return names[i]
		}
	}
	return defaultFolder
}

// selectResourcePool shows all resource pools with survey.Select.
func selectResourcePool(pools []vcenter.ResourcePoolInfo, defaultPool, message string) string {
	if len(pools) == 0 {
		return defaultPool
	}

	var opts []string
	defaultOpt := pools[0].Name
	for _, p := range pools {
		opts = append(opts, p.Name)
		if p.Name == defaultPool {
			defaultOpt = p.Name
		}
	}

	var choice string
	surveySelect(&survey.Select{
		Message: message,
		Options: opts,
		Default: defaultOpt,
	}, &choice)
	return choice
}

// interactiveSelect renders a navigable list in raw terminal mode.
// ↑/↓ arrows move the selection; Enter confirms.
// Does NOT send cursor-position queries (no \033[6n), so it leaves no CPR bytes
// in stdin — immune to the issue that affects consecutive survey.Select calls.
func interactiveSelect(items []string, defaultItem, message string) string {
	if len(items) == 0 {
		return defaultItem
	}

	sel := 0
	for i, item := range items {
		if item == defaultItem {
			sel = i
			break
		}
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback: numbered list with readline input.
		return selectFromList(items, defaultItem, message)
	}

	const maxVisible = 10
	nVis := len(items)
	if nVis > maxVisible {
		nVis = maxVisible
	}
	offset := 0

	clamp := func() {
		if sel < offset {
			offset = sel
		} else if sel >= offset+nVis {
			offset = sel - nVis + 1
		}
	}

	// Lines rendered: 1 header + nVis items + 1 footer = nVis+2
	total := nVis + 2

	draw := func(initial bool) {
		if !initial {
			fmt.Printf("\033[%dA", total) // move cursor back to top of block
		}
		clamp()
		fmt.Printf("\r  \033[1m%s\033[0m\033[K\r\n", message)
		for i := offset; i < offset+nVis; i++ {
			if i == sel {
				fmt.Printf("\r  \033[36m❯ %s\033[0m\033[K\r\n", items[i])
			} else {
				fmt.Printf("\r    %s\033[K\r\n", items[i])
			}
		}
		if len(items) > nVis {
			fmt.Printf("\r  \033[2m%d/%d · ↑↓ arrows · Enter\033[0m\033[K\r\n", sel+1, len(items))
		} else {
			fmt.Printf("\r  \033[2m↑↓ arrows · Enter\033[0m\033[K\r\n")
		}
	}

	draw(true)

	buf := make([]byte, 8)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
		if n == 1 {
			switch buf[0] {
			case '\r', '\n': // Enter
				result := items[sel]
				_ = term.Restore(fd, oldState)
				stdinReader.Reset(os.Stdin)
				fmt.Printf("\033[%dA", total)
				fmt.Printf("\r  \033[32m❯\033[0m %s \033[36m%s\033[0m\r\n", message, result)
				fmt.Printf("\033[J") // clear everything below
				return result
			case 3: // Ctrl+C
				_ = term.Restore(fd, oldState)
				stdinReader.Reset(os.Stdin)
				fmt.Printf("\r\n")
				return defaultItem
			}
		} else if n >= 3 && buf[0] == '\033' && buf[1] == '[' {
			switch buf[2] {
			case 'A': // up
				if sel > 0 {
					sel--
				} else {
					sel = len(items) - 1
				}
				draw(false)
			case 'B': // down
				if sel < len(items)-1 {
					sel++
				} else {
					sel = 0
				}
				draw(false)
			}
		}
	}

	_ = term.Restore(fd, oldState)
	stdinReader.Reset(os.Stdin)
	return items[sel]
}

// selectFromList is the fallback when raw mode is unavailable.
// Shows a numbered list and reads the selection via readline.
func selectFromList(items []string, defaultItem, label string) string {
	if len(items) == 0 {
		return defaultItem
	}
	fmt.Printf("  %s\n", label)
	defaultIdx := 1
	for i, item := range items {
		marker := "  "
		if item == defaultItem {
			marker = "» "
			defaultIdx = i + 1
		}
		fmt.Printf("   %s%d. %s\n", marker, i+1, item)
	}

	prompt := fmt.Sprintf("  Select [1-%d] [\033[36m%d\033[0m]: ", len(items), defaultIdx)
	rl, err := readline.NewEx(&readline.Config{Prompt: prompt})
	if err != nil {
		n := readInt(fmt.Sprintf("Select [1-%d]", len(items)), defaultIdx, 1, len(items))
		return items[n-1]
	}
	defer func() {
		_ = rl.Close()
		stdinReader.Reset(os.Stdin)
	}()

	for {
		line, err := rl.Readline()
		if err != nil {
			return items[defaultIdx-1]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return items[defaultIdx-1]
		}
		v, err := strconv.Atoi(line)
		if err != nil || v < 1 || v > len(items) {
			fmt.Printf("  Must be a number between 1 and %d\n", len(items))
			continue
		}
		return items[v-1]
	}
}

// selectISODatastore shows all datastores with ★ for the top HDD candidates
// (ISOs are written once and rarely accessed, so HDD is the right choice).
func selectISODatastore(datastores []vcenter.DatastoreInfo, defaultDS string) string {
	// Collect and rank HDD datastores by free space.
	var hddByFree []vcenter.DatastoreInfo
	for _, ds := range datastores {
		if ds.Type == "HDD" && ds.Accessible {
			hddByFree = append(hddByFree, ds)
		}
	}
	sort.Slice(hddByFree, func(i, j int) bool {
		return hddByFree[i].FreeSpaceGB > hddByFree[j].FreeSpaceGB
	})
	topN := 2
	if len(hddByFree) < topN {
		topN = len(hddByFree)
	}
	topHDD := make(map[string]bool, topN)
	for _, ds := range hddByFree[:topN] {
		topHDD[ds.Name] = true
	}

	var opts []string
	var dsNames []string

	// Top HDD datastores first (★).
	for _, ds := range hddByFree[:topN] {
		label := fmt.Sprintf("★ %s  [HDD] (%.0f/%.0f GB free)",
			ds.Name, ds.FreeSpaceGB, ds.CapacityGB)
		opts = append(opts, label)
		dsNames = append(dsNames, ds.Name)
	}

	// Remaining datastores (lower-ranked HDD + SSD).
	for _, ds := range datastores {
		if !ds.Accessible || topHDD[ds.Name] {
			continue
		}
		label := fmt.Sprintf("  %s  [%s] (%.0f/%.0f GB free)",
			ds.Name, ds.Type, ds.FreeSpaceGB, ds.CapacityGB)
		opts = append(opts, label)
		dsNames = append(dsNames, ds.Name)
	}

	if len(opts) == 0 {
		return defaultDS
	}

	var choice string
	surveySelect(&survey.Select{Message: "Select ISO datastore:", Options: opts}, &choice)
	for i, opt := range opts {
		if opt == choice {
			return dsNames[i]
		}
	}
	return defaultDS
}

func buildUbuntuOptions() []string {
	releases := configs.UbuntuReleases.Releases
	var vers []string
	for ver := range releases {
		vers = append(vers, ver)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(vers)))
	names := map[string]string{
		"24.04": "24.04 LTS (Noble)",
		"22.04": "22.04 LTS (Jammy)",
		"20.04": "20.04 LTS (Focal)",
	}
	var out []string
	for _, v := range vers {
		if n, ok := names[v]; ok {
			out = append(out, n)
		} else {
			out = append(out, v)
		}
	}
	return out
}

func autoGateway(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return strings.Join(parts[:3], ".") + ".1"
	}
	return ""
}

func autoFirstDNS(gateway string) string {
	return gateway
}

// ─── Output helpers ───────────────────────────────────────────────────────────

func printSummary(out VMWizardOutput) {
	v := out.VM
	fmt.Printf("  %-20s %s\n", "Name:", v.Name)
	fmt.Printf("  %-20s %d cores / %d MB RAM\n", "Specs:", v.CPUs, v.MemoryMB)
	fmt.Printf("  %-20s OS: %d GB", "Disks:", v.DiskSizeGB)
	if v.DataDiskSizeGB > 0 {
		fmt.Printf("  Data: %d GB", v.DataDiskSizeGB)
	}
	fmt.Println()
	if v.SwapSizeGB != nil {
		fmt.Printf("  %-20s %d GB\n", "Swap:", *v.SwapSizeGB)
	}
	fmt.Printf("  %-20s %s\n", "Ubuntu:", v.UbuntuVersion)
	fmt.Printf("  %-20s %s\n", "Datastore:", v.Datastore)
	fmt.Printf("  %-20s %s\n", "Network:", v.NetworkName)
	if v.NetworkInterface != "" {
		fmt.Printf("  %-20s %s\n", "NIC name:", v.NetworkInterface)
	}
	if v.Folder != "" {
		fmt.Printf("  %-20s %s\n", "Folder:", v.Folder)
	}
	if v.ResourcePool != "" {
		fmt.Printf("  %-20s %s\n", "Resource pool:", v.ResourcePool)
	}
	dns := v.DNS
	if v.DNS2 != "" {
		dns += ", " + v.DNS2
	}
	fmt.Printf("  %-20s %s / %s / gw %s / dns %s\n", "Network config:", v.IPAddress, v.Netmask, v.Gateway, dns)
	fmt.Printf("  %-20s %s\n", "User:", v.Username)
	if v.SSHKeyPath != "" {
		fmt.Printf("  %-20s %s\n", "SSH key:", v.SSHKeyPath)
	}
	if v.SSHPort > 0 {
		fmt.Printf("  %-20s %d\n", "SSH port:", v.SSHPort)
	}
	if v.Password != "" {
		fmt.Printf("  %-20s (set)\n", "Password:")
		fmt.Printf("  %-20s %v\n", "SSH password auth:", v.AllowPasswordSSH)
	}
	fmt.Println()
}

// ─── vCenter config loading ───────────────────────────────────────────────────

func loadVCenterConfig(path string) (*vcenterFileConfig, error) {
	data, err := sopsDecrypt(path)
	if err != nil {
		return nil, err
	}
	var cfg vcenterFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return &cfg, nil
}

// ─── File path input with Tab completion ─────────────────────────────────────

// filePathCompleter implements readline.AutoCompleter for filesystem paths.
// Handles ~ expansion for display and supports Tab completion of file/dir names.
type filePathCompleter struct{}

func (c *filePathCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	typed := string(line[:pos])

	// Expand ~ for filesystem lookup only (keep original form for display).
	expanded := typed
	if strings.HasPrefix(typed, "~/") {
		home, _ := os.UserHomeDir()
		expanded = home + typed[1:]
	} else if typed == "~" {
		home, _ := os.UserHomeDir()
		expanded = home
	}

	var dir, partial string
	if strings.HasSuffix(expanded, "/") || expanded == "" {
		dir = expanded
		if dir == "" {
			dir = "."
		}
		partial = ""
	} else {
		dir = filepath.Dir(expanded)
		partial = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0
	}

	var matches [][]rune
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, partial) {
			continue
		}
		// readline appends the returned string at the cursor — return only the
		// suffix after what the user already typed, not the full name.
		suffix := name[len(partial):]
		if e.IsDir() {
			suffix += "/"
		}
		matches = append(matches, []rune(suffix))
	}
	return matches, len([]rune(partial))
}

// readFilePath reads a file path with Tab completion support.
// Falls back to plain readLine if readline cannot be initialized.
func readFilePath(field, current string) string {
	prompt := fmt.Sprintf("  %s: ", field)
	if current != "" {
		prompt = fmt.Sprintf("  %s [\033[36m%s\033[0m]: ", field, current)
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:       prompt,
		AutoComplete: &filePathCompleter{},
	})
	if err != nil {
		return readLine(field, current) // fallback to plain input
	}
	defer func() {
		_ = rl.Close()
		stdinReader.Reset(os.Stdin) // resync bufio reader after readline
	}()

	line, err := rl.Readline()
	if err != nil {
		return current // Ctrl+C or EOF → keep current
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	// Expand ~ in result so callers receive a usable path.
	if strings.HasPrefix(line, "~/") {
		home, _ := os.UserHomeDir()
		line = home + line[1:]
	}
	return line
}

// ─── Plain I/O helpers (no survey — avoids terminal cursor-position queries) ──

// stdinReader is the single shared buffered reader over os.Stdin.
// One instance is required — multiple buffered readers over the same fd
// would each buffer ahead and consume each other's input.
var stdinReader = bufio.NewReader(os.Stdin)
var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var caretEscapeRE = regexp.MustCompile(`\^\[\[[0-9;?]*[ -/]*[@-~]`)

// readLine prints "  Field [current]: " and reads a line.
// Returns current if the user presses Enter without typing anything.
func readLine(field, current string) string {
	prompt := ""
	if current != "" {
		prompt = fmt.Sprintf("  %s [\033[36m%s\033[0m]: ", field, current)
	} else {
		prompt = fmt.Sprintf("  %s: ", field)
	}
	s := readPromptLine(prompt)
	if s == "" {
		return current
	}
	return s
}

// readIPLine reads a line and validates it as an IPv4 address.
func readIPLine(field, current string) string {
	for {
		s := readLine(field, current)
		if isValidIP(s) {
			return s
		}
		fmt.Println("  Invalid IP address — use dotted decimal (e.g. 192.168.1.10)")
	}
}

// readPassword reads a password without echoing. Returns empty string if blank.
func readPassword(field string) string {
	fmt.Printf("  %s: ", field)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return ""
	}
	return string(pw)
}

// readInt prints the prompt and reads a validated integer.
func readInt(field string, current, min, max int) int {
	for {
		s := readPromptLine(fmt.Sprintf("  %s [\033[36m%d\033[0m]: ", field, current))
		if s == "" {
			return current
		}
		v, err := parseInt(s)
		if err != nil || v < min || v > max {
			fmt.Printf("  Must be a number between %d and %d\n", min, max)
			continue
		}
		return v
	}
}

// readYesNo prints "  msg [Y/n]: " and returns true for y/yes, false for n/no.
func readYesNo(msg string, defaultYes bool) bool {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	for {
		s := strings.ToLower(readPromptLine(fmt.Sprintf("  %s %s: ", msg, hint)))
		if s == "" {
			return defaultYes
		}
		if s == "y" || s == "yes" {
			return true
		}
		if s == "n" || s == "no" {
			return false
		}
		fmt.Println("  Enter y or n")
	}
}

// readYesNoDanger is for destructive actions.
// It highlights the prompt in red and defaults to No.
func readYesNoDanger(msg string) bool {
	return readYesNo("\033[31m"+msg+"\033[0m", false)
}

func readPromptLine(prompt string) string {
	rl, err := readline.NewEx(&readline.Config{Prompt: prompt})
	if err == nil {
		cleanup := func() {
			_ = rl.Close()
			stdinReader.Reset(os.Stdin)
		}
		line, err := rl.Readline()
		if err == nil {
			cleanup()
			return strings.TrimSpace(line)
		}
		if errors.Is(err, readline.ErrInterrupt) {
			// Restore terminal before signal handler (it may os.Exit immediately).
			cleanup()
			if p, findErr := os.FindProcess(os.Getpid()); findErr == nil {
				_ = p.Signal(os.Interrupt)
			}
			return ""
		}
		cleanup()
		return ""
	}

	fmt.Print(prompt)
	line, _ := stdinReader.ReadString('\n')
	return sanitizeConsoleInput(line)
}

func sanitizeConsoleInput(raw string) string {
	raw = ansiEscapeRE.ReplaceAllString(raw, "")
	raw = caretEscapeRE.ReplaceAllString(raw, "")
	raw = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)
	return strings.TrimSpace(raw)
}

// surveySelect wraps survey.AskOne for a Select prompt and calls drainStdin()
// afterward to discard any CPR responses (\033[row;colR) that the terminal
// may have queued in stdin in response to survey's \033[6n cursor queries.
// Without this drain, those responses appear as garbage in subsequent readLine calls.
func surveySelect(q *survey.Select, response *string) {
	_ = survey.AskOne(q, response)
	drainStdin()
}

// ─── Small helpers ────────────────────────────────────────────────────────────

func intOrDefault(v, d int) int {
	if v != 0 {
		return v
	}
	return d
}

func strOrDefault(v, d string) string {
	if v != "" {
		return v
	}
	return d
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func isValidIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || v > 255 {
			return false
		}
	}
	return true
}
