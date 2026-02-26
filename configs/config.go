// Package configs provides library defaults loaded from embedded YAML files.
// All hardcoded values live in defaults.yaml and ubuntu-releases.yaml.
package configs

import (
	_ "embed"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var defaultsYAML []byte

//go:embed ubuntu-releases.yaml
var releasesYAML []byte

// Defaults holds all library default values (loaded from defaults.yaml at startup).
var Defaults LibDefaults

// UbuntuReleases holds Ubuntu release download info (loaded from ubuntu-releases.yaml).
var UbuntuReleases ReleasesConfig

func init() {
	if err := yaml.Unmarshal(defaultsYAML, &Defaults); err != nil {
		panic("vmware-vm-bootstrap: invalid defaults.yaml: " + err.Error())
	}
	if err := yaml.Unmarshal(releasesYAML, &UbuntuReleases); err != nil {
		panic("vmware-vm-bootstrap: invalid ubuntu-releases.yaml: " + err.Error())
	}
}

// LibDefaults holds all configurable library defaults.
type LibDefaults struct {
	VCenter   VCenterDefaults   `yaml:"vcenter"`
	VM        VMDefaults        `yaml:"vm"`
	Network   NetworkDefaults   `yaml:"network"`
	CloudInit CloudInitDefaults `yaml:"cloudinit"`
	Timeouts  TimeoutDefaults   `yaml:"timeouts"`
	ISO       ISODefaults       `yaml:"iso"`
	Output    OutputDefaults    `yaml:"output"`
}

// VCenterDefaults holds vCenter connection defaults.
type VCenterDefaults struct {
	Port int `yaml:"port"`
}

// VMDefaults holds VM hardware defaults.
type VMDefaults struct {
	Firmware string `yaml:"firmware"`
	GuestOS  string `yaml:"guest_os"`
}

// NetworkDefaults holds network configuration defaults.
type NetworkDefaults struct {
	Interface string `yaml:"interface"`
}

// CloudInitDefaults holds cloud-init configuration defaults.
type CloudInitDefaults struct {
	Locale         string   `yaml:"locale"`
	Timezone       string   `yaml:"timezone"`
	KeyboardLayout string   `yaml:"keyboard_layout"`
	SwapSizeGB     int      `yaml:"swap_size_gb"`
	Packages       []string `yaml:"packages"`
	UserGroups     string   `yaml:"user_groups"`
	UserShell      string   `yaml:"user_shell"`
}

// TimeoutDefaults holds all timeout and retry values.
type TimeoutDefaults struct {
	InstallationMinutes    int `yaml:"installation_minutes"`
	PollingSeconds         int `yaml:"polling_seconds"`
	HostnameChecks         int `yaml:"hostname_checks"`
	ServiceStartupSeconds  int `yaml:"service_startup_seconds"`
	SSHRetries             int `yaml:"ssh_retries"`
	SSHConnectSeconds      int `yaml:"ssh_connect_seconds"`
	SSHRetryDelaySeconds   int `yaml:"ssh_retry_delay_seconds"`
	HardwareInitSeconds    int `yaml:"hardware_init_seconds"`
	DownloadMinutes        int `yaml:"download_minutes"`
	UploadProgressSeconds  int `yaml:"upload_progress_seconds"`
	ExtractProgressSeconds int `yaml:"extract_progress_seconds"`
}

// As time.Duration convenience methods.

func (t TimeoutDefaults) Installation() time.Duration {
	return time.Duration(t.InstallationMinutes) * time.Minute
}
func (t TimeoutDefaults) Polling() time.Duration {
	return time.Duration(t.PollingSeconds) * time.Second
}
func (t TimeoutDefaults) ServiceStartup() time.Duration {
	return time.Duration(t.ServiceStartupSeconds) * time.Second
}
func (t TimeoutDefaults) SSHConnect() time.Duration {
	return time.Duration(t.SSHConnectSeconds) * time.Second
}
func (t TimeoutDefaults) SSHRetryDelay() time.Duration {
	return time.Duration(t.SSHRetryDelaySeconds) * time.Second
}
func (t TimeoutDefaults) HardwareInit() time.Duration {
	return time.Duration(t.HardwareInitSeconds) * time.Second
}
func (t TimeoutDefaults) Download() time.Duration {
	return time.Duration(t.DownloadMinutes) * time.Minute
}
func (t TimeoutDefaults) UploadProgress() time.Duration {
	return time.Duration(t.UploadProgressSeconds) * time.Second
}
func (t TimeoutDefaults) ExtractProgress() time.Duration {
	return time.Duration(t.ExtractProgressSeconds) * time.Second
}

// ISODefaults holds ISO-related defaults.
type ISODefaults struct {
	NoCloudVolumeID      string `yaml:"nocloud_volume_id"`
	GRUBTimeoutSeconds   int    `yaml:"grub_timeout_seconds"`
	CacheDir             string `yaml:"cache_dir"`
	UbuntuModifiedSuffix string `yaml:"ubuntu_modified_suffix"`
	ExtractDirName       string `yaml:"extract_dir_name"`
	UbuntuVolumeID       string `yaml:"ubuntu_volume_id"`
}

// OutputDefaults holds CLI output defaults.
type OutputDefaults struct {
	Enable           bool   `yaml:"enable"`
	BootstrapResultPath string `yaml:"bootstrap_result_path"`
}

// UbuntuRelease holds download info for a single Ubuntu release.
type UbuntuRelease struct {
	URL      string `yaml:"url"`
	Checksum string `yaml:"checksum"`
}

// ReleasesConfig holds all known Ubuntu releases.
type ReleasesConfig struct {
	Releases map[string]UbuntuRelease `yaml:"releases"`
}
