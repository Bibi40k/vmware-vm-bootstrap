package main

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const talosClusterPlanDefaultPath = "configs/talos.cluster.sops.yaml"

type talosClusterPlanFile struct {
	Cluster struct {
		Name    string `yaml:"name"`
		Network struct {
			CIDR    string `yaml:"cidr"`
			StartIP string `yaml:"start_ip"`
			Gateway string `yaml:"gateway"`
			DNS     string `yaml:"dns"`
			DNS2    string `yaml:"dns2,omitempty"`
		} `yaml:"network"`
		Defaults struct {
			Datastore    string `yaml:"datastore,omitempty"`
			NetworkName  string `yaml:"network_name,omitempty"`
			Folder       string `yaml:"folder,omitempty"`
			ResourcePool string `yaml:"resource_pool,omitempty"`
			TimeoutMins  int    `yaml:"timeout_minutes,omitempty"`
		} `yaml:"defaults,omitempty"`
		NodeTypes map[string]talosNodeTypeSpec `yaml:"node_types"`
		Layout    []talosNodeLayoutSpec        `yaml:"layout"`
	} `yaml:"cluster"`
}

type talosNodeTypeSpec struct {
	CPUs         int    `yaml:"cpus"`
	MemoryMB     int    `yaml:"memory_mb"`
	DiskSizeGB   int    `yaml:"disk_size_gb"`
	Datastore    string `yaml:"datastore,omitempty"`
	NetworkName  string `yaml:"network_name,omitempty"`
	Folder       string `yaml:"folder,omitempty"`
	ResourcePool string `yaml:"resource_pool,omitempty"`
	TalosVersion string `yaml:"talos_version"`
	SchematicID  string `yaml:"schematic_id"`
}

type talosNodeLayoutSpec struct {
	Type   string `yaml:"type"`
	Count  int    `yaml:"count"`
	Prefix string `yaml:"prefix,omitempty"`
}

func runTalosGeneratePrompt() error {
	fmt.Printf("\n\033[1mTalos\033[0m — Node Config Generator\n")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()
	planPath := strings.TrimSpace(readLine("Plan config", talosClusterPlanDefaultPath))
	if wasPromptInterrupted() {
		fmt.Println("  Cancelled.")
		return nil
	}
	if planPath == "" {
		planPath = talosClusterPlanDefaultPath
	}
	planWasCreated := false
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		if err := createTalosPlanInteractive(planPath, latestDraftForTarget(planPath)); err != nil {
			return err
		}
		planWasCreated = true
	}
	force := false
	if !planWasCreated {
		force = readYesNoDanger("Overwrite existing generated VM configs?")
	}
	return runTalosGenerate(planPath, force)
}

func createTalosPlanInteractive(planPath, draftPath string) error {
	fmt.Printf("\n\033[1mCreate Talos Cluster Plan\033[0m\n")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	var plan talosClusterPlanFile
	session := NewWizardSession(planPath, draftPath, &plan, func() bool {
		return strings.TrimSpace(plan.Cluster.Name) == "" &&
			strings.TrimSpace(plan.Cluster.Network.CIDR) == "" &&
			len(plan.Cluster.NodeTypes) == 0 &&
			len(plan.Cluster.Layout) == 0
	})
	if loaded, err := session.LoadDraft(); err == nil && loaded {
		fmt.Printf("\033[33m⚠ Resuming draft: %s\033[0m\n\n", filepath.Base(draftPath))
	}

	session.Start()
	defer session.Stop()

	dsDefault := ""
	netDefault := ""
	folderDefault := ""
	poolDefault := ""
	cidrDefault, startIPDefault, gatewayDefault, dnsDefault := suggestNodeNetworkDefaults()
	var (
		cat    *VCenterCatalog
		catErr error
	)
	if vcCfg, err := loadVCenterConfig(vcenterConfigFile); err == nil {
		dsDefault = vcCfg.VCenter.ISODatastore
		netDefault = vcCfg.VCenter.Network
		folderDefault = vcCfg.VCenter.Folder
		poolDefault = vcCfg.VCenter.ResourcePool

		fmt.Print("  Connecting to vCenter... ")
		cat, err = fetchVCenterCatalog(vcCfg, 60*time.Second)
		if err != nil {
			catErr = err
			fmt.Printf("\033[33m⚠ %v\033[0m (pickers unavailable)\n", err)
		} else {
			fmt.Printf("\033[32m✓\033[0m  (%d datastores, %d networks, %d folders, %d pools)\n",
				len(cat.Datastores), len(cat.Networks), len(cat.Folders), len(cat.Pools))
		}
	}

	clusterName := strings.TrimSpace(readLine("Cluster name", strOrDefault(plan.Cluster.Name, "dev")))
	if clusterName == "" {
		clusterName = "dev"
	}
	plan.Cluster.Name = clusterName

	cidr := strings.TrimSpace(readLine("VMware node network CIDR (real LAN/VLAN, not K8s pod/service)", strOrDefault(plan.Cluster.Network.CIDR, cidrDefault)))
	for {
		if _, err := netip.ParsePrefix(cidr); err == nil {
			break
		}
		fmt.Println("  Invalid CIDR")
		cidr = strings.TrimSpace(readLine("VMware node network CIDR (real LAN/VLAN, not K8s pod/service)", strOrDefault(plan.Cluster.Network.CIDR, cidrDefault)))
	}
	plan.Cluster.Network.CIDR = cidr

	gateway := readIPLine("Gateway", strOrDefault(plan.Cluster.Network.Gateway, gatewayDefault))
	startIP := readIPLine("Start IP for first node", strOrDefault(plan.Cluster.Network.StartIP, startIPDefault))
	dns := readIPLine("DNS", strOrDefault(plan.Cluster.Network.DNS, dnsDefault))
	dns2 := readLine("Secondary DNS (optional)", plan.Cluster.Network.DNS2)
	plan.Cluster.Network.Gateway = gateway
	plan.Cluster.Network.StartIP = startIP
	plan.Cluster.Network.DNS = dns
	plan.Cluster.Network.DNS2 = dns2

	currentCP := 3
	currentWK := 2
	for _, l := range plan.Cluster.Layout {
		switch l.Type {
		case "controlplane":
			currentCP = l.Count
		case "worker":
			currentWK = l.Count
		}
	}
	cpCount := readInt("Controlplane count", currentCP, 0, 99)
	workerCount := readInt("Worker count", currentWK, 0, 999)
	if cpCount+workerCount == 0 {
		return fmt.Errorf("at least one node is required")
	}

	fmt.Println()
	currentTalosVersion := "v1.12.4"
	currentSchematic := ""
	if t, ok := plan.Cluster.NodeTypes["controlplane"]; ok {
		if strings.TrimSpace(t.TalosVersion) != "" {
			currentTalosVersion = strings.TrimSpace(t.TalosVersion)
		}
		currentSchematic = strings.TrimSpace(t.SchematicID)
	}
	talosVersion := selectTalosVersion(currentTalosVersion)
	schematicID := currentSchematic
	for {
		schematicID = strings.TrimSpace(selectTalosSchematicID(schematicID))
		if schematicID != "" {
			break
		}
		fmt.Println("  Talos schematic ID is required")
	}

	fmt.Println()
	cpDefault := plan.Cluster.NodeTypes["controlplane"]
	cpCPU := readInt("Controlplane CPU cores", intOrDefault(cpDefault.CPUs, 4), 1, 128)
	cpRAMGB := readInt("Controlplane RAM (GB)", intOrDefault(cpDefault.MemoryMB/1024, 8), 1, 2048)
	cpDiskGB := readInt("Controlplane OS disk (GB)", intOrDefault(cpDefault.DiskSizeGB, 60), 10, 4096)

	workerDefault := plan.Cluster.NodeTypes["worker"]
	workerCPU := readInt("Worker CPU cores", intOrDefault(workerDefault.CPUs, 8), 1, 128)
	workerRAMGB := readInt("Worker RAM (GB)", intOrDefault(workerDefault.MemoryMB/1024, 16), 1, 2048)
	workerDiskGB := readInt("Worker OS disk (GB)", intOrDefault(workerDefault.DiskSizeGB, 100), 10, 4096)

	fmt.Println()
	defaultDatastore := pickDatastoreFromCatalog(catalogIfReady(cat, catErr), strOrDefault(plan.Cluster.Defaults.Datastore, dsDefault))
	defaultNetwork := pickNetworkFromCatalog(catalogIfReady(cat, catErr), strOrDefault(plan.Cluster.Defaults.NetworkName, netDefault))
	defaultFolder := pickFolderFromCatalog(catalogIfReady(cat, catErr), strOrDefault(plan.Cluster.Defaults.Folder, folderDefault))
	defaultPool := pickResourcePoolFromCatalog(catalogIfReady(cat, catErr), strOrDefault(plan.Cluster.Defaults.ResourcePool, poolDefault))
	timeoutMinutes := readInt("Node timeout (minutes)", intOrDefault(plan.Cluster.Defaults.TimeoutMins, 45), 1, 240)

	plan.Cluster.Defaults.Datastore = defaultDatastore
	plan.Cluster.Defaults.NetworkName = defaultNetwork
	plan.Cluster.Defaults.Folder = defaultFolder
	plan.Cluster.Defaults.ResourcePool = defaultPool
	plan.Cluster.Defaults.TimeoutMins = timeoutMinutes
	plan.Cluster.NodeTypes = map[string]talosNodeTypeSpec{
		"controlplane": {
			CPUs:         cpCPU,
			MemoryMB:     cpRAMGB * 1024,
			DiskSizeGB:   cpDiskGB,
			TalosVersion: talosVersion,
			SchematicID:  schematicID,
		},
		"worker": {
			CPUs:         workerCPU,
			MemoryMB:     workerRAMGB * 1024,
			DiskSizeGB:   workerDiskGB,
			TalosVersion: talosVersion,
			SchematicID:  schematicID,
		},
	}
	if cpCount > 0 {
		plan.Cluster.Layout = append(plan.Cluster.Layout, talosNodeLayoutSpec{
			Type: "controlplane", Count: cpCount, Prefix: "cp",
		})
	}
	if workerCount > 0 {
		plan.Cluster.Layout = append(plan.Cluster.Layout, talosNodeLayoutSpec{
			Type: "worker", Count: workerCount, Prefix: "wk",
		})
	}

	data, err := yaml.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal cluster plan: %w", err)
	}
	if err := sopsEncrypt(planPath, data); err != nil {
		return err
	}
	_ = session.Finalize()

	fmt.Printf("\n\033[32m✓ Saved and encrypted:\033[0m %s\n", filepath.Base(planPath))
	return nil
}

func runTalosGenerate(planPath string, force bool) error {
	planPath = strings.TrimSpace(planPath)
	if planPath == "" {
		planPath = talosClusterPlanDefaultPath
	}
	if _, err := os.Stat(planPath); err != nil {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			if err := createTalosPlanInteractive(planPath, latestDraftForTarget(planPath)); err != nil {
				return err
			}
		} else {
			return &userError{
				msg:  fmt.Sprintf("plan config not found: %s", planPath),
				hint: "Create configs/talos.cluster.sops.yaml (or pass --config).",
			}
		}
	}

	raw, err := sopsDecrypt(planPath)
	if err != nil {
		return err
	}

	var plan talosClusterPlanFile
	if err := yaml.Unmarshal(raw, &plan); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(planPath), err)
	}

	clusterName := strings.TrimSpace(plan.Cluster.Name)
	if clusterName == "" {
		return fmt.Errorf("cluster.name is required")
	}
	cidr := strings.TrimSpace(plan.Cluster.Network.CIDR)
	startIPRaw := strings.TrimSpace(plan.Cluster.Network.StartIP)
	gateway := strings.TrimSpace(plan.Cluster.Network.Gateway)
	dns := strings.TrimSpace(plan.Cluster.Network.DNS)
	if cidr == "" || startIPRaw == "" || gateway == "" || dns == "" {
		return fmt.Errorf("cluster.network.{cidr,start_ip,gateway,dns} are required")
	}

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid cluster.network.cidr %q: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return fmt.Errorf("only IPv4 CIDR is supported (got %q)", cidr)
	}

	startIP, err := netip.ParseAddr(startIPRaw)
	if err != nil {
		return fmt.Errorf("invalid cluster.network.start_ip %q: %w", startIPRaw, err)
	}
	if !prefix.Contains(startIP) {
		return fmt.Errorf("start_ip %s is not in cidr %s", startIP, cidr)
	}

	if len(plan.Cluster.NodeTypes) == 0 || len(plan.Cluster.Layout) == 0 {
		return fmt.Errorf("cluster.node_types and cluster.layout are required")
	}

	netmask := ipv4MaskFromPrefix(prefix.Bits())
	nextOffset := 0
	created := 0
	updated := 0

	fmt.Printf("\n\033[1mGenerate Talos Node Configs\033[0m — %s\n", filepath.Base(planPath))
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Cluster:   %s\n", clusterName)
	fmt.Printf("  CIDR:      %s\n", cidr)
	fmt.Printf("  Start IP:  %s\n", startIP)
	fmt.Printf("  Force:     %v\n", force)
	fmt.Println()

	for _, item := range plan.Cluster.Layout {
		typ := strings.TrimSpace(item.Type)
		if typ == "" {
			return fmt.Errorf("layout item missing type")
		}
		if item.Count <= 0 {
			return fmt.Errorf("layout %q has invalid count %d", typ, item.Count)
		}
		spec, ok := plan.Cluster.NodeTypes[typ]
		if !ok {
			return fmt.Errorf("layout references unknown node type %q", typ)
		}
		if err := validateTalosNodeTypeSpec(typ, spec); err != nil {
			return err
		}

		prefixName := strings.TrimSpace(item.Prefix)
		if prefixName == "" {
			prefixName = typ
		}

		for i := 1; i <= item.Count; i++ {
			ip, err := addIPv4(startIP, nextOffset)
			if err != nil {
				return err
			}
			if !prefix.Contains(ip) {
				return fmt.Errorf("generated IP %s escaped CIDR %s", ip, cidr)
			}
			nextOffset++

			nodeName := fmt.Sprintf("%s-%s-%02d", clusterName, prefixName, i)
			outPath := filepath.Join("configs", fmt.Sprintf("vm.%s.sops.yaml", nodeName))
			_, existedErr := os.Stat(outPath)
			existed := existedErr == nil

			vmCfg := buildTalosVMWizardOutput(nodeName, ip.String(), netmask, gateway, dns, strings.TrimSpace(plan.Cluster.Network.DNS2), spec, plan.Cluster.Defaults)

			if existed && !force {
				fmt.Printf("  \033[33m~ skip\033[0m  %s (already exists)\n", filepath.Base(outPath))
				continue
			}

			data, err := yaml.Marshal(vmCfg)
			if err != nil {
				return fmt.Errorf("marshal %s: %w", outPath, err)
			}
			if err := sopsEncrypt(outPath, data); err != nil {
				return err
			}
			if existed {
				updated++
				fmt.Printf("  \033[36m↺ update\033[0m %s\n", filepath.Base(outPath))
			} else {
				created++
				fmt.Printf("  \033[32m+ create\033[0m %s\n", filepath.Base(outPath))
			}
		}
	}

	fmt.Println()
	fmt.Printf("\033[32m✓ Generated Talos node configs\033[0m\n")
	fmt.Printf("  Created: %d\n", created)
	fmt.Printf("  Updated: %d\n", updated)
	fmt.Printf("  Next IP: %s\n", mustAddIPv4(startIP, nextOffset))
	return nil
}

func buildTalosVMWizardOutput(name, ip, netmask, gateway, dns, dns2 string, spec talosNodeTypeSpec, defaults struct {
	Datastore    string `yaml:"datastore,omitempty"`
	NetworkName  string `yaml:"network_name,omitempty"`
	Folder       string `yaml:"folder,omitempty"`
	ResourcePool string `yaml:"resource_pool,omitempty"`
	TimeoutMins  int    `yaml:"timeout_minutes,omitempty"`
}) VMWizardOutput {
	var out VMWizardOutput
	out.VM.Name = name
	out.VM.Profile = "talos"
	out.VM.CPUs = spec.CPUs
	out.VM.MemoryMB = spec.MemoryMB
	out.VM.DiskSizeGB = spec.DiskSizeGB
	out.VM.IPAddress = ip
	out.VM.Netmask = netmask
	out.VM.Gateway = gateway
	out.VM.DNS = dns
	out.VM.DNS2 = dns2
	out.VM.NetworkInterface = "ens192"
	out.VM.Datastore = firstNonEmpty(spec.Datastore, defaults.Datastore)
	out.VM.NetworkName = firstNonEmpty(spec.NetworkName, defaults.NetworkName)
	out.VM.Folder = firstNonEmpty(spec.Folder, defaults.Folder)
	out.VM.ResourcePool = firstNonEmpty(spec.ResourcePool, defaults.ResourcePool)
	out.VM.TimeoutMinutes = defaults.TimeoutMins
	if out.VM.TimeoutMinutes == 0 {
		out.VM.TimeoutMinutes = 45
	}
	out.VM.Profiles.Talos.Version = strings.TrimSpace(spec.TalosVersion)
	out.VM.Profiles.Talos.SchematicID = strings.TrimSpace(spec.SchematicID)
	return out
}

func validateTalosNodeTypeSpec(name string, spec talosNodeTypeSpec) error {
	if spec.CPUs <= 0 || spec.MemoryMB <= 0 || spec.DiskSizeGB < 10 {
		return fmt.Errorf("node_type %q has invalid resources (cpus/memory_mb/disk_size_gb)", name)
	}
	if strings.TrimSpace(spec.TalosVersion) == "" {
		return fmt.Errorf("node_type %q missing talos_version", name)
	}
	if strings.TrimSpace(spec.SchematicID) == "" {
		return fmt.Errorf("node_type %q missing schematic_id", name)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func addIPv4(base netip.Addr, offset int) (netip.Addr, error) {
	if !base.Is4() {
		return netip.Addr{}, fmt.Errorf("base IP is not IPv4")
	}
	if offset < 0 {
		return netip.Addr{}, fmt.Errorf("invalid negative offset")
	}
	b := base.As4()
	u := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	u += uint32(offset)
	return netip.AddrFrom4([4]byte{byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)}), nil
}

func mustAddIPv4(base netip.Addr, offset int) string {
	ip, err := addIPv4(base, offset)
	if err != nil {
		return "n/a"
	}
	return ip.String()
}

func ipv4MaskFromPrefix(bits int) string {
	if bits <= 0 {
		return "0.0.0.0"
	}
	if bits >= 32 {
		return "255.255.255.255"
	}
	mask := ^uint32(0) << (32 - bits)
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(mask>>24),
		byte(mask>>16),
		byte(mask>>8),
		byte(mask),
	)
}

func suggestNodeNetworkDefaults() (cidr string, startIP string, gateway string, dns string) {
	const (
		fallbackCIDR    = "192.168.110.0/24"
		fallbackStartIP = "192.168.110.20"
		fallbackGateway = "192.168.110.1"
	)

	vmFiles, _ := filepath.Glob("configs/vm.*.sops.yaml")
	type subnetInfo struct {
		count   int
		maxHost int
	}
	subnets := map[string]subnetInfo{}
	for _, p := range vmFiles {
		vmFile, err := loadVMConfig(p)
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(vmFile.VM.IPAddress))
		if ip == nil {
			continue
		}
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		key := fmt.Sprintf("%d.%d.%d", v4[0], v4[1], v4[2])
		host := int(v4[3])
		info := subnets[key]
		info.count++
		if host > info.maxHost {
			info.maxHost = host
		}
		subnets[key] = info
	}
	if len(subnets) == 0 {
		return fallbackCIDR, fallbackStartIP, fallbackGateway, fallbackGateway
	}

	type cand struct {
		key     string
		count   int
		maxHost int
	}
	var list []cand
	for k, v := range subnets {
		list = append(list, cand{key: k, count: v.count, maxHost: v.maxHost})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count == list[j].count {
			return list[i].key < list[j].key
		}
		return list[i].count > list[j].count
	})
	chosen := list[0]
	next := chosen.maxHost + 1
	if next < 20 || next > 250 {
		next = 20
	}
	return chosen.key + ".0/24", fmt.Sprintf("%s.%d", chosen.key, next), chosen.key + ".1", chosen.key + ".1"
}
