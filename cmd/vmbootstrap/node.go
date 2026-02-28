package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bibi40k/vmware-vm-bootstrap/pkg/bootstrap"
	"gopkg.in/yaml.v3"
)

func selectOrUseVMConfigPath(path, title, prompt string) (string, string, error) {
	if strings.TrimSpace(path) != "" {
		return path, filepath.Base(path), nil
	}
	return selectVMConfig(title, prompt)
}

func loadVMConfig(path string) (*vmFileConfig, error) {
	vmOut, err := sopsDecrypt(path)
	if err != nil {
		return nil, err
	}
	var vmFile vmFileConfig
	if err := yaml.Unmarshal(vmOut, &vmFile); err != nil {
		return nil, fmt.Errorf("failed to parse VM config: %w", err)
	}
	return &vmFile, nil
}

func buildNodeConfig(vmPath string) (*bootstrap.VMConfig, *vmFileConfig, error) {
	vcCfg, err := loadVCenterConfig(vcenterConfigFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load vCenter config: %w", err)
	}
	vmFile, err := loadVMConfig(vmPath)
	if err != nil {
		return nil, nil, err
	}

	v := vmFile.VM
	cfg := &bootstrap.VMConfig{
		VCenterHost:     vcCfg.VCenter.Host,
		VCenterUsername: vcCfg.VCenter.Username,
		VCenterPassword: vcCfg.VCenter.Password,
		VCenterPort:     vcCfg.VCenter.Port,
		VCenterInsecure: vcCfg.VCenter.Insecure,

		Name:       v.Name,
		Profile:    v.Profile,
		IPAddress:  v.IPAddress,
		Datacenter: vcCfg.VCenter.Datacenter,
		Datastore:  v.Datastore,

		NetworkName:  v.NetworkName,
		Folder:       v.Folder,
		ResourcePool: v.ResourcePool,
		ISODatastore: vcCfg.VCenter.ISODatastore,
	}
	if cfg.Profile == "" {
		cfg.Profile = "ubuntu"
	}
	cfg.Profiles.Ubuntu.Version = v.Profiles.Ubuntu.Version
	cfg.Profiles.Talos.Version = v.Profiles.Talos.Version
	cfg.Profiles.Talos.SchematicID = v.Profiles.Talos.SchematicID
	cfg.SetDefaults()
	return cfg, vmFile, nil
}

func nodeCreate(vmPath string) error {
	selectedPath, selectedLabel, err := selectOrUseVMConfigPath(vmPath, "\033[1mvmbootstrap\033[0m — Node Create", "Select VM config to create:")
	if err != nil {
		return err
	}
	if selectedPath == "" {
		return nil
	}
	if !readYesNo(fmt.Sprintf("Create node from %s?", selectedLabel), true) {
		fmt.Println("  Cancelled.")
		return nil
	}
	return bootstrapVM(selectedPath, bootstrapResultPath)
}

func nodeDelete(vmPath string) error {
	selectedPath, selectedLabel, err := selectOrUseVMConfigPath(vmPath, "\033[1mvmbootstrap\033[0m — Node Delete", "Select VM config to delete:")
	if err != nil {
		return err
	}
	if selectedPath == "" {
		return nil
	}
	cfg, _, err := buildNodeConfig(selectedPath)
	if err != nil {
		return err
	}
	if !readYesNoDanger(fmt.Sprintf("Delete node %s (%s)?", cfg.Name, selectedLabel)) {
		fmt.Println("  Cancelled.")
		return nil
	}
	return cleanupVM(cfg, cfg.Name)
}

func nodeRecreate(vmPath string) error {
	selectedPath, selectedLabel, err := selectOrUseVMConfigPath(vmPath, "\033[1mvmbootstrap\033[0m — Node Recreate", "Select VM config to recreate:")
	if err != nil {
		return err
	}
	if selectedPath == "" {
		return nil
	}

	cfg, _, err := buildNodeConfig(selectedPath)
	if err != nil {
		return err
	}

	if !readYesNoDanger(fmt.Sprintf("Recreate node %s (%s)? This will delete and create again.", cfg.Name, selectedLabel)) {
		fmt.Println("  Cancelled.")
		return nil
	}

	if exists, err := vmExists(cfg); err == nil && exists {
		if err := cleanupVM(cfg, cfg.Name); err != nil {
			return err
		}
	}
	return bootstrapVM(selectedPath, bootstrapResultPath)
}

func nodeUpdateTalos(vmPath, toVersion, talosconfig, endpoint string, preserve, insecure bool) error {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return &userError{
			msg:  "'talosctl' not found in PATH",
			hint: "Install talosctl before running node update.",
		}
	}

	selectedPath, selectedLabel, err := selectOrUseVMConfigPath(vmPath, "\033[1mvmbootstrap\033[0m — Talos Update", "Select Talos node config to update:")
	if err != nil {
		return err
	}
	if selectedPath == "" {
		return nil
	}

	cfg, _, err := buildNodeConfig(selectedPath)
	if err != nil {
		return err
	}
	if cfg.EffectiveProfile() != "talos" {
		return &userError{
			msg:  fmt.Sprintf("vm profile for %s is %q, expected talos", selectedLabel, cfg.EffectiveProfile()),
			hint: "Set vm.profile=talos in VM config before running node update.",
		}
	}

	version := strings.TrimSpace(toVersion)
	if version == "" {
		version = cfg.EffectiveTalosVersion()
	}
	if version == "" {
		return &userError{
			msg:  "missing Talos target version",
			hint: "Set vm.profiles.talos.version in config or pass --to-version.",
		}
	}

	nodeIP := strings.TrimSpace(cfg.IPAddress)
	if nodeIP == "" {
		return &userError{
			msg:  "missing vm.ip_address for Talos node update",
			hint: "Set vm.ip_address in VM config.",
		}
	}

	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		ep = nodeIP
	}

	if !readYesNoDanger(fmt.Sprintf("Upgrade Talos node %s to %s (%s)?", cfg.Name, version, selectedLabel)) {
		fmt.Println("  Cancelled.")
		return nil
	}

	fmt.Printf("\n\033[1mTalos Update\033[0m — %s\n", selectedLabel)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Node:      %s\n", nodeIP)
	fmt.Printf("  Endpoint:  %s\n", ep)
	fmt.Printf("  Version:   %s\n", version)
	if talosconfig != "" {
		fmt.Printf("  Config:    %s\n", talosconfig)
	}
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	return bootstrap.UpgradeTalosNode(ctx, &bootstrap.TalosNodeUpdateConfig{
		NodeIP:      nodeIP,
		Endpoint:    ep,
		Version:     version,
		Talosconfig: talosconfig,
		Preserve:    preserve,
		Insecure:    insecure,
	})
}
