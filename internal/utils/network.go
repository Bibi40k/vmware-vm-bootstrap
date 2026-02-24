package utils

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// NetmaskToCIDR converts a netmask to CIDR notation.
// Example: "255.255.255.0" -> 24
func NetmaskToCIDR(netmask string) (int, error) {
	ip := net.ParseIP(netmask)
	if ip == nil {
		return 0, fmt.Errorf("invalid netmask: %s", netmask)
	}

	mask := net.IPMask(ip.To4())
	if mask == nil {
		return 0, fmt.Errorf("invalid IPv4 netmask: %s", netmask)
	}

	ones, _ := mask.Size()
	return ones, nil
}

// CIDRToNetmask converts CIDR notation to netmask.
// Example: 24 -> "255.255.255.0"
func CIDRToNetmask(cidr int) (string, error) {
	if cidr < 0 || cidr > 32 {
		return "", fmt.Errorf("invalid CIDR: %d (must be 0-32)", cidr)
	}

	mask := net.CIDRMask(cidr, 32)
	return net.IP(mask).String(), nil
}

// ValidateIPv4 validates an IPv4 address.
func ValidateIPv4(ip string) error {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	if parsed.To4() == nil {
		return fmt.Errorf("not an IPv4 address: %s", ip)
	}

	return nil
}

// ValidateNetworkConfig validates network configuration.
func ValidateNetworkConfig(ip, netmask, gateway string, dns []string) error {
	// Validate IP
	if err := ValidateIPv4(ip); err != nil {
		return fmt.Errorf("invalid IP: %w", err)
	}

	// Validate netmask
	if _, err := NetmaskToCIDR(netmask); err != nil {
		return fmt.Errorf("invalid netmask: %w", err)
	}

	// Validate gateway
	if err := ValidateIPv4(gateway); err != nil {
		return fmt.Errorf("invalid gateway: %w", err)
	}

	// Validate DNS servers
	for i, server := range dns {
		if err := ValidateIPv4(server); err != nil {
			return fmt.Errorf("invalid DNS server %d: %w", i+1, err)
		}
	}

	return nil
}

// ParsePortRange parses a port range string (e.g., "80", "8000-9000").
func ParsePortRange(portRange string) (start, end int, err error) {
	parts := strings.Split(portRange, "-")
	switch len(parts) {
	case 1:
		// Single port
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port: %s", parts[0])
		}
		return port, port, nil
	case 2:
		// Port range
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start port: %s", parts[0])
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end port: %s", parts[1])
		}
		if start > end {
			return 0, 0, fmt.Errorf("start port > end port: %d > %d", start, end)
		}
		return start, end, nil
	default:
		return 0, 0, fmt.Errorf("invalid port range format: %s", portRange)
	}
}

// IsPortOpen checks if a TCP port is accessible within the given timeout.
func IsPortOpen(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
