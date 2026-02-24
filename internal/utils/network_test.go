package utils

import (
	"testing"
)

func TestNetmaskToCIDR(t *testing.T) {
	tests := []struct {
		name     string
		netmask  string
		expected int
		wantErr  bool
	}{
		{"Class C", "255.255.255.0", 24, false},
		{"Class B", "255.255.0.0", 16, false},
		{"Class A", "255.0.0.0", 8, false},
		{"Single host", "255.255.255.255", 32, false},
		{"Subnet /25", "255.255.255.128", 25, false},
		{"Invalid format", "invalid", 0, true},
		{"IPv6 format", "ffff:ffff:ffff::", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NetmaskToCIDR(tt.netmask)
			if (err != nil) != tt.wantErr {
				t.Errorf("NetmaskToCIDR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("NetmaskToCIDR() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCIDRToNetmask(t *testing.T) {
	tests := []struct {
		name     string
		cidr     int
		expected string
		wantErr  bool
	}{
		{"Class C", 24, "255.255.255.0", false},
		{"Class B", 16, "255.255.0.0", false},
		{"Class A", 8, "255.0.0.0", false},
		{"Single host", 32, "255.255.255.255", false},
		{"Zero", 0, "0.0.0.0", false},
		{"Invalid negative", -1, "", true},
		{"Invalid > 32", 33, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CIDRToNetmask(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("CIDRToNetmask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("CIDRToNetmask() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateIPv4(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"Valid IP", "192.168.1.1", false},
		{"Valid IP 2", "10.0.0.1", false},
		{"Loopback", "127.0.0.1", false},
		{"Broadcast", "255.255.255.255", false},
		{"Invalid format", "invalid", true},
		{"IPv6", "2001:db8::1", true},
		{"Out of range", "256.256.256.256", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPv4(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPv4() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateNetworkConfig(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		netmask string
		gateway string
		dns     []string
		wantErr bool
	}{
		{
			"Valid config",
			"192.168.1.10",
			"255.255.255.0",
			"192.168.1.1",
			[]string{"8.8.8.8", "8.8.4.4"},
			false,
		},
		{
			"Invalid IP",
			"invalid",
			"255.255.255.0",
			"192.168.1.1",
			[]string{"8.8.8.8"},
			true,
		},
		{
			"Invalid netmask",
			"192.168.1.10",
			"invalid",
			"192.168.1.1",
			[]string{"8.8.8.8"},
			true,
		},
		{
			"Invalid gateway",
			"192.168.1.10",
			"255.255.255.0",
			"invalid",
			[]string{"8.8.8.8"},
			true,
		},
		{
			"Invalid DNS",
			"192.168.1.10",
			"255.255.255.0",
			"192.168.1.1",
			[]string{"invalid"},
			true,
		},
		{
			"Empty DNS",
			"192.168.1.10",
			"255.255.255.0",
			"192.168.1.1",
			[]string{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkConfig(tt.ip, tt.netmask, tt.gateway, tt.dns)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNetworkConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePortRange(t *testing.T) {
	tests := []struct {
		name      string
		portRange string
		wantStart int
		wantEnd   int
		wantErr   bool
	}{
		{"Single port", "80", 80, 80, false},
		{"Port range", "8000-9000", 8000, 9000, false},
		{"Invalid format", "invalid", 0, 0, true},
		{"Invalid range", "9000-8000", 0, 0, true},
		{"Too many parts", "80-90-100", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := ParsePortRange(tt.portRange)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePortRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if start != tt.wantStart {
					t.Errorf("ParsePortRange() start = %v, want %v", start, tt.wantStart)
				}
				if end != tt.wantEnd {
					t.Errorf("ParsePortRange() end = %v, want %v", end, tt.wantEnd)
				}
			}
		})
	}
}

// Benchmark tests
func BenchmarkNetmaskToCIDR(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := NetmaskToCIDR("255.255.255.0"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateIPv4(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if err := ValidateIPv4("192.168.1.1"); err != nil {
			b.Fatal(err)
		}
	}
}
