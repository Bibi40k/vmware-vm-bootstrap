package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/bootstrap"
	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/vcenter"
	"gopkg.in/yaml.v3"
)

// smokeVM bootstraps a VM and runs a minimal validation, then optionally cleans up.
func smokeVM(vmConfigPath string, cleanup bool) error {
	fmt.Printf("\033[1mSmoke Test\033[0m — %s\n", vmConfigPath)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	// Load vCenter config
	vcCfg, err := loadVCenterConfig(vcenterConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load vCenter config: %w", err)
	}

	// Decrypt VM config
	vmOut, err := sopsDecrypt(vmConfigPath)
	if err != nil {
		return err
	}

	var vmFile vmFileConfig
	if err := yaml.Unmarshal(vmOut, &vmFile); err != nil {
		return fmt.Errorf("failed to parse VM config: %w", err)
	}

	v := vmFile.VM

	// Load SSH key
	sshKey, err := loadSSHKey(v.SSHKeyPath, v.SSHKey)
	if err != nil {
		return err
	}

	cfg := &bootstrap.VMConfig{
		VCenterHost:     vcCfg.VCenter.Host,
		VCenterUsername: vcCfg.VCenter.Username,
		VCenterPassword: vcCfg.VCenter.Password,
		VCenterPort:     vcCfg.VCenter.Port,
		VCenterInsecure: vcCfg.VCenter.Insecure,

		Name:             v.Name,
		CPUs:             v.CPUs,
		MemoryMB:         v.MemoryMB,
		DiskSizeGB:       v.DiskSizeGB,
		UbuntuVersion:    v.UbuntuVersion,
		Username:         v.Username,
		SSHPublicKeys:    []string{sshKey},
		Password:         v.Password,
		AllowPasswordSSH: v.AllowPasswordSSH,

		NetworkName: v.NetworkName,
		IPAddress:   v.IPAddress,
		Netmask:     v.Netmask,
		Gateway:     v.Gateway,
		DNS:         buildDNS(v.DNS, v.DNS2),

		Datacenter:   vcCfg.VCenter.Datacenter,
		Folder:       v.Folder,
		ResourcePool: v.ResourcePool,
		Datastore:    v.Datastore,
		ISODatastore: vcCfg.VCenter.ISODatastore,
	}

	if v.DataDiskSizeGB > 0 {
		size := v.DataDiskSizeGB
		cfg.DataDiskSizeGB = &size
		cfg.DataDiskMountPath = v.DataDiskMountPath
	}
	if v.SwapSizeGB > 0 {
		size := v.SwapSizeGB
		cfg.SwapSizeGB = &size
	}

	// Bootstrap
	logger := newPrettyLogger(os.Stdout)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(45)*time.Minute)
	defer cancel()

	// Handle Ctrl+C locally for cleanup
	signal.Stop(mainSigCh)
	localSigCh := make(chan os.Signal, 1)
	signal.Notify(localSigCh, os.Interrupt)
	go func() {
		select {
		case <-localSigCh:
			fmt.Println("\n\n\033[33m⚠ Interrupted — stopping smoke test...\033[0m")
			cancel()
		case <-ctx.Done():
		}
	}()

	vm, err := bootstrap.BootstrapWithLogger(ctx, cfg, logger)

	// Restore global handler
	signal.Stop(localSigCh)
	signal.Notify(mainSigCh, os.Interrupt)

	if err != nil {
		if ctx.Err() == context.Canceled {
			if cleanup {
				offerVMCleanup(cfg)
			}
			fmt.Println("\nCancelled.")
			os.Exit(0)
		}
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	fmt.Println()
	fmt.Println("\033[32m✓ VM bootstrapped successfully!\033[0m")
	fmt.Printf("  Name:      %s\n", vm.Name)
	fmt.Printf("  IP:        %s\n", vm.IPAddress)
	fmt.Printf("  SSH ready: %v\n", vm.SSHReady)

	// Minimal post-checks via SSH
	if err := smokeSSHChecks(cfg.Username, vm.IPAddress, v.DataDiskMountPath, v.SwapSizeGB); err != nil {
		fmt.Printf("\033[31m✗ Smoke checks failed: %v\033[0m\n", err)
	} else {
		fmt.Println("\033[32m✓ Smoke checks passed\033[0m")
	}

	if !cleanup {
		cleanup = readYesNo("Cleanup (delete VM)?", false)
	}
	if cleanup {
		fmt.Println()
		fmt.Println("Cleanup")
		return cleanupVM(cfg, vm.Name)
	}

	fmt.Printf("\n  Connect: \033[36mssh %s@%s\033[0m\n\n", cfg.Username, vm.IPAddress)
	return nil
}

func loadSSHKey(path, raw string) (string, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read SSH key %s: %w", path, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if raw == "" {
		return "", fmt.Errorf("either vm.ssh_key or vm.ssh_key_path is required")
	}
	return raw, nil
}

func cleanupVM(cfg *bootstrap.VMConfig, name string) error {
	fmt.Print("  Connecting to vCenter... ")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	vclient, err := vcenter.NewClient(ctx, &vcenter.Config{
		Host:     cfg.VCenterHost,
		Username: cfg.VCenterUsername,
		Password: cfg.VCenterPassword,
		Port:     cfg.VCenterPort,
		Insecure: cfg.VCenterInsecure,
	})
	if err != nil {
		fmt.Printf("\033[31m✗ %v\033[0m\n", err)
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = vclient.Disconnect() }()
	fmt.Println("\033[32m✓\033[0m")

	vmObj, err := vclient.FindVM(cfg.Datacenter, name)
	if err != nil || vmObj == nil {
		fmt.Println("  VM not found in vCenter (may already be deleted).")
		return nil
	}

	fmt.Print("  Powering off... ")
	if task, err := vmObj.PowerOff(ctx); err == nil {
		_ = task.Wait(ctx)
	}
	fmt.Println("\033[32m✓\033[0m")

	fmt.Print("  Deleting VM... ")
	task, err := vmObj.Destroy(ctx)
	if err != nil {
		fmt.Printf("\033[31m✗ %v\033[0m\n", err)
		return fmt.Errorf("destroy: %w", err)
	}
	if err := task.Wait(ctx); err != nil {
		fmt.Printf("\033[31m✗ %v\033[0m\n", err)
		return fmt.Errorf("destroy wait: %w", err)
	}
	fmt.Println("\033[32m✓ VM deleted.\033[0m")
	return nil
}
