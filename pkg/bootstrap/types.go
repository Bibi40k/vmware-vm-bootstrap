// Package bootstrap provides the main public API for VM bootstrapping.
package bootstrap

import (
	"fmt"

	"github.com/Bibi40k/vmware-vm-bootstrap/configs"
	"github.com/Bibi40k/vmware-vm-bootstrap/internal/utils"
	"github.com/vmware/govmomi/vim25/types"
)

// VMConfig defines the complete configuration for VM bootstrap.
type VMConfig struct {
	// === vCenter Connection ===
	VCenterHost     string // vCenter hostname or IP (e.g., "vcenter.example.com")
	VCenterUsername string // vCenter username (e.g., "administrator@vsphere.local")
	VCenterPassword string // vCenter password (encrypted/plain - user's responsibility)
	VCenterPort     int    // vCenter port (default: 443)
	VCenterInsecure bool   // Skip TLS verification (not recommended for production)

	// === VM Specifications ===
	Name              string // VM name (e.g., "web-server-01")
	CPUs              int    // Number of CPUs (e.g., 4)
	MemoryMB          int    // Memory in MB (e.g., 8192)
	DiskSizeGB        int    // OS disk size in GB (e.g., 40)
	DataDiskSizeGB    *int   // Optional data disk size in GB (e.g., 500) - nil = not created
	DataDiskMountPath string // Mount point for data disk (e.g., "/data") - required if DataDiskSizeGB set

	// === Network Configuration ===
	NetworkName      string   // Network name (e.g., "LAN_Management")
	NetworkInterface string   // Guest NIC name (e.g., "ens192")
	IPAddress        string   // Static IP address (e.g., "192.168.1.10")
	Netmask          string   // Network mask (e.g., "255.255.255.0")
	Gateway          string   // Default gateway (e.g., "192.168.1.1")
	DNS              []string // DNS servers (e.g., ["8.8.8.8", "8.8.4.4"])

	// === VM Placement ===
	Datacenter   string // Datacenter name (e.g., "DC1")
	Folder       string // VM folder path (e.g., "Production/WebServers")
	ResourcePool string // Resource pool path (e.g., "WebTier")
	Datastore    string // VM datastore name (e.g., "VMwareSSD01")
	ISODatastore string // Datastore for ISO uploads (e.g., "VMwareStorage01"); falls back to Datastore if empty

	// === OS & User Configuration ===
	// OS profile used for VM provisioning (default: "ubuntu").
	Profile string
	// Profile-specific options (Phase 1 keeps Ubuntu as active implementation).
	Profiles      VMProfiles
	UbuntuVersion string   // Ubuntu version (e.g., "24.04" - supported: 22.04, 24.04)
	Username      string   // SSH user to create (e.g., "sysadmin")
	SSHPublicKeys []string // SSH public keys (one or more)
	Password      string   // Optional plain text password (auto-hashed with bcrypt before use)
	PasswordHash  string   // Optional pre-computed password hash (bcrypt); overrides Password if both set
	// Allow SSH password authentication (default: false). Requires Password or PasswordHash.
	AllowPasswordSSH bool
	// Skip SSH verification during bootstrap (default: false).
	SkipSSHVerify bool
	// Keep VM/ISO on bootstrap failure for debugging (default: false).
	SkipCleanupOnError bool

	// === Advanced Options ===
	Timezone   string // System timezone (default: "UTC")
	Locale     string // System locale (default: "en_US.UTF-8")
	SwapSizeGB *int   // Swap size in GB (default from configs/defaults.yaml)
	Firmware   string // Firmware type: "bios" or "efi" (default: "bios")
}

// VMProfiles contains profile-specific settings.
type VMProfiles struct {
	Ubuntu UbuntuProfile
}

// UbuntuProfile contains Ubuntu-specific settings for profile mode.
type UbuntuProfile struct {
	Version string
}

// VM represents a bootstrapped virtual machine.
type VM struct {
	Name          string                       // VM name
	IPAddress     string                       // Assigned IP address
	ManagedObject types.ManagedObjectReference // govmomi VM reference
	SSHReady      bool                         // SSH port 22 accessible
	Hostname      string                       // Configured hostname
	// vCenter connection data for post-create operations (Verify/PowerOn/PowerOff/Delete).
	// These fields are intentionally not serialized.
	VCenterHost     string `json:"-"`
	VCenterPort     int    `json:"-"`
	VCenterUser     string `json:"-"`
	VCenterPass     string `json:"-"`
	VCenterInsecure bool   `json:"-"`
}

// Validate checks if the VM configuration is valid.
func (cfg *VMConfig) Validate() error {
	if cfg.VCenterHost == "" {
		return fmt.Errorf("VCenterHost is required")
	}
	if cfg.VCenterUsername == "" {
		return fmt.Errorf("VCenterUsername is required")
	}
	if cfg.VCenterPassword == "" {
		return fmt.Errorf("VCenterPassword is required")
	}
	if cfg.Name == "" {
		return fmt.Errorf("name is required")
	}
	if cfg.Username == "" {
		return fmt.Errorf("username is required")
	}
	if len(cfg.SSHPublicKeys) == 0 && cfg.Password == "" && cfg.PasswordHash == "" {
		return fmt.Errorf("at least one of SSHPublicKeys, Password, or PasswordHash is required")
	}
	if cfg.AllowPasswordSSH && cfg.Password == "" && cfg.PasswordHash == "" {
		return fmt.Errorf("AllowPasswordSSH requires Password or PasswordHash")
	}
	if cfg.IPAddress == "" {
		return fmt.Errorf("IPAddress is required")
	}
	if cfg.Netmask == "" {
		return fmt.Errorf("netmask is required")
	}
	if cfg.Gateway == "" {
		return fmt.Errorf("gateway is required")
	}
	if len(cfg.DNS) == 0 {
		return fmt.Errorf("at least one DNS server is required")
	}
	if cfg.DiskSizeGB < 10 {
		return fmt.Errorf("DiskSizeGB must be at least 10 (got %d)", cfg.DiskSizeGB)
	}
	if cfg.DataDiskSizeGB != nil && cfg.DataDiskMountPath == "" {
		return fmt.Errorf("DataDiskMountPath is required when DataDiskSizeGB is set")
	}
	profile := cfg.Profile
	if profile == "" {
		profile = "ubuntu"
	}
	if profile != "ubuntu" {
		return fmt.Errorf("unsupported Profile %q (supported: ubuntu)", profile)
	}
	if cfg.effectiveUbuntuVersion() == "" {
		return fmt.Errorf("UbuntuVersion is required (or Profiles.Ubuntu.Version)")
	}
	if cfg.Datacenter == "" {
		return fmt.Errorf("datacenter is required")
	}
	if cfg.Datastore == "" {
		return fmt.Errorf("datastore is required")
	}
	if cfg.NetworkName == "" {
		return fmt.Errorf("NetworkName is required")
	}
	// Validate network config using utils
	if err := utils.ValidateNetworkConfig(cfg.IPAddress, cfg.Netmask, cfg.Gateway, cfg.DNS); err != nil {
		return err
	}
	return nil
}

// SetDefaults sets default values for optional fields from configs/defaults.yaml.
func (cfg *VMConfig) SetDefaults() {
	d := configs.Defaults
	if cfg.Profile == "" {
		cfg.Profile = "ubuntu"
	}
	// Backward compatibility: map legacy UbuntuVersion into profile field.
	if cfg.Profiles.Ubuntu.Version == "" && cfg.UbuntuVersion != "" {
		cfg.Profiles.Ubuntu.Version = cfg.UbuntuVersion
	}
	// Keep legacy field in sync for older call sites.
	if cfg.UbuntuVersion == "" && cfg.Profiles.Ubuntu.Version != "" {
		cfg.UbuntuVersion = cfg.Profiles.Ubuntu.Version
	}
	if cfg.VCenterPort == 0 {
		cfg.VCenterPort = d.VCenter.Port
	}
	if cfg.Timezone == "" {
		cfg.Timezone = d.CloudInit.Timezone
	}
	if cfg.Locale == "" {
		cfg.Locale = d.CloudInit.Locale
	}
	if cfg.Firmware == "" {
		cfg.Firmware = d.VM.Firmware
	}
	if cfg.NetworkInterface == "" {
		cfg.NetworkInterface = d.Network.Interface
	}
}

// EffectiveUbuntuVersion returns Ubuntu version using profile-first fallback.
func (cfg *VMConfig) EffectiveUbuntuVersion() string {
	return cfg.effectiveUbuntuVersion()
}

func (cfg *VMConfig) effectiveUbuntuVersion() string {
	if cfg.Profiles.Ubuntu.Version != "" {
		return cfg.Profiles.Ubuntu.Version
	}
	return cfg.UbuntuVersion
}
