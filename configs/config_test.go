package configs

import (
	"testing"
	"time"
)

func TestDefaultsLoaded(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"VCenter.Port", Defaults.VCenter.Port, 443},
		{"VM.Firmware", Defaults.VM.Firmware, "bios"},
		{"VM.GuestOS", Defaults.VM.GuestOS, "ubuntu64Guest"},
		{"Network.Interface", Defaults.Network.Interface, "ens192"},
		{"CloudInit.Locale", Defaults.CloudInit.Locale, "en_US.UTF-8"},
		{"CloudInit.Timezone", Defaults.CloudInit.Timezone, "UTC"},
		{"ISO.NoCloudVolumeID", Defaults.ISO.NoCloudVolumeID, "CIDATA"},
		{"ISO.CacheDir", Defaults.ISO.CacheDir, "cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestTimeoutDurationsPositive(t *testing.T) {
	d := Defaults.Timeouts

	durations := []struct {
		name string
		got  time.Duration
	}{
		{"Installation", d.Installation()},
		{"Polling", d.Polling()},
		{"ServiceStartup", d.ServiceStartup()},
		{"SSHConnect", d.SSHConnect()},
		{"SSHRetryDelay", d.SSHRetryDelay()},
		{"HardwareInit", d.HardwareInit()},
		{"Download", d.Download()},
		{"UploadProgress", d.UploadProgress()},
		{"ExtractProgress", d.ExtractProgress()},
	}

	for _, tt := range durations {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got <= 0 {
				t.Errorf("%s() = %v, want > 0", tt.name, tt.got)
			}
		})
	}
}

func TestTimeoutDurationsConversion(t *testing.T) {
	d := Defaults.Timeouts

	if d.Installation() != time.Duration(d.InstallationMinutes)*time.Minute {
		t.Error("Installation() conversion mismatch")
	}
	if d.Download() != time.Duration(d.DownloadMinutes)*time.Minute {
		t.Error("Download() conversion mismatch")
	}
	if d.Polling() != time.Duration(d.PollingSeconds)*time.Second {
		t.Error("Polling() conversion mismatch")
	}
	if d.ServiceStartup() != time.Duration(d.ServiceStartupSeconds)*time.Second {
		t.Error("ServiceStartup() conversion mismatch")
	}
}

func TestUbuntuReleasesLoaded(t *testing.T) {
	if len(UbuntuReleases.Releases) == 0 {
		t.Fatal("ubuntu-releases.yaml loaded no releases")
	}

	for _, version := range []string{"24.04", "22.04"} {
		t.Run("Ubuntu "+version, func(t *testing.T) {
			r, ok := UbuntuReleases.Releases[version]
			if !ok {
				t.Fatalf("Ubuntu %s not found in releases", version)
			}
			if r.URL == "" {
				t.Errorf("Ubuntu %s has no download URL", version)
			}
		})
	}
}

func TestTalosReleasesLoaded(t *testing.T) {
	if len(TalosReleases.Versions) == 0 {
		t.Fatal("talos-releases.yaml loaded no versions")
	}
	for i, v := range TalosReleases.Versions {
		if v == "" {
			t.Fatalf("TalosReleases.Versions[%d] is empty", i)
		}
		if v[0] != 'v' {
			t.Fatalf("Talos version must start with 'v': %q", v)
		}
	}
}
