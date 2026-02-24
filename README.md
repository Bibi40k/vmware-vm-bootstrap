# vmware-vm-bootstrap

![CI](https://github.com/Bibi40k/vmware-vm-bootstrap/actions/workflows/ci.yml/badge.svg)
![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/Bibi40k/vmware-vm-bootstrap/master/docs/coverage/coverage.json)

**Go library for automated VM deployment in VMware vSphere with cloud-init.**

Bootstrap complete Ubuntu VMs in vCenter: from bare metal to SSH-ready in minutes.

## Features

- Core library in Go (ISO tooling still uses system binaries)
- VMware vSphere 7.0+ support
- Ubuntu 22.04/24.04 automated installation
- Cloud-init configuration (network, users, SSH keys)
- Password hashing (bcrypt; SHA-512 planned)
- Context-aware operations (timeout/cancel support)
- Comprehensive error handling

## Installation

```bash
go get github.com/Bibi40k/vmware-vm-bootstrap
```

## Quick Start (Library)

```go
package main

import (
    "context"
    "log"
    "github.com/Bibi40k/vmware-vm-bootstrap/pkg/bootstrap"
)

func main() {
    dataDiskSize := 500 // optional data disk (GB)
    swapSize := 4       // optional swap (GB)

    cfg := &bootstrap.VMConfig{
        // Required vCenter
        VCenterHost:     "vcenter.example.com",
        VCenterUsername: "administrator@vsphere.local",
        VCenterPassword: "secret",

        // Optional vCenter (defaults from configs/defaults.yaml)
        VCenterPort:     443,
        VCenterInsecure: false,

        // Required VM specs
        Name:            "web-server-01",
        CPUs:            4,
        MemoryMB:        8192,
        DiskSizeGB:      40,

        // Optional data disk (set both or leave both empty)
        DataDiskSizeGB:    &dataDiskSize,
        DataDiskMountPath: "/data",

        // Required network
        NetworkName:     "LAN_Management",
        NetworkInterface: "ens192",
        IPAddress:       "192.168.1.10",
        Netmask:         "255.255.255.0",
        Gateway:         "192.168.1.1",
        DNS:             []string{"8.8.8.8"},

        // Required placement
        Datacenter:      "DC1",
        Folder:          "Production",
        Datastore:       "SSD-Storage-01",

        // Optional placement
        ResourcePool:    "WebTier",
        ISODatastore:    "ISO-Storage-01",

        // Required OS/user
        UbuntuVersion:   "24.04",
        Username:        "sysadmin",
        SSHPublicKeys:   []string{"ssh-ed25519 AAAA..."},

        // Optional auth (use keys OR password OR password hash)
        Password:        "",
        PasswordHash:    "",
        AllowPasswordSSH: false,

        // Optional OS tweaks (defaults in configs/defaults.yaml)
        Timezone:        "UTC",
        Locale:          "en_US.UTF-8",
        SwapSizeGB:      &swapSize,
        Firmware:        "bios",
    }

    vm, err := bootstrap.Bootstrap(context.Background(), cfg)
    if err != nil {
        log.Fatalf("Bootstrap failed: %v", err)
    }

    log.Printf("VM %s ready at %s", vm.Name, vm.IPAddress)
}
```

Post-creation operations:

```go
if err := vm.Verify(context.Background()); err != nil {
    log.Fatalf("Verify failed: %v", err)
}
// vm.PowerOff(...), vm.PowerOn(...), vm.Delete(...)
```

Note: `Verify` requires VMware Tools running and SSH access. `VCenterHost` accepts a hostname or a full `https://.../sdk` URL (https only).

Auth options (choose one):

- SSH keys:
```go
SSHPublicKeys: []string{"ssh-ed25519 AAAA..."},
```

- Password (plaintext; will be bcrypt-hashed):
```go
Password: "strong-password",
AllowPasswordSSH: true,
```

- Pre-hashed bcrypt password:
```go
PasswordHash: "$2a$10$...",
AllowPasswordSSH: true,
```

Note: Using `Password` means the plaintext password exists in memory (and possibly in code/config). For better security, prefer `PasswordHash`, and load secrets from a secure source (SOPS via the CLI, a secrets manager, or environment variables) before building the config.

Disable optional fields:

```go
DataDiskSizeGB: nil,
DataDiskMountPath: "",
SwapSizeGB: nil,
```

Default values (from `configs/defaults.yaml`):

- vCenter port: `443`
- Firmware: `bios`
- Network interface: `ens192`
- Locale: `en_US.UTF-8`
- Timezone: `UTC`
- Swap size: `2` GB
- Packages: `open-vm-tools`
- User groups: `sudo,adm,dialout,cdrom,audio,video,plugdev,users`
- User shell: `/bin/bash`
- Timeouts: see `configs/defaults.yaml`
- ISO defaults: see `configs/defaults.yaml`

## CLI Tool

```bash
# Install CLI
go install github.com/Bibi40k/vmware-vm-bootstrap/cmd/vmbootstrap@latest

# Create VM
vmbootstrap create --config vm.yaml
```

Note: The library API consumes an in-memory `bootstrap.VMConfig` and has no SOPS dependency. SOPS is used only by the CLI for encrypted config files.

Config files:

```bash
# Example configs
cp configs/vcenter.example.yaml configs/vcenter.sops.yaml
cp configs/vm.example.yaml configs/vm.myvm.sops.yaml

# SOPS config (edit the AGE key before use)
cp .sops.yaml.example .sops.yaml
cp .sopsrc.example .sopsrc

# Encrypt configs
sops -e -i configs/vcenter.sops.yaml
sops -e -i configs/vm.myvm.sops.yaml
```

## Requirements

Library:
- Go 1.26+
- VMware vCenter 7.0+
- Ubuntu 22.04 or 24.04 Server ISO

CLI (in addition to library requirements):
- `govc` (vSphere CLI)
- `genisoimage`
- `xorriso`
- `sops` (only for encrypted config files)

## Development

Common targets:

```bash
# Install external tools (golangci-lint, govulncheck, govc, sops, genisoimage, xorriso)
make install-requirements

# Build & verify
make build
make build-cli
make test
make lint
make vulncheck
make test-cover

# Maintenance
make fmt
make vet
make deps
make verify
make clean
```

VM management (CLI):

```bash
make config   # interactive config wizard
make run      # bootstrap a VM
make smoke VM=configs/vm.myvm.sops.yaml  # bootstrap + smoke test (+ cleanup)
```

## Releases

Release automation (safe mode):

- Pushing a tag `v*` creates a **Draft Release** with binaries and checksums.
- Manually running the "Release" workflow **publishes** a release for an existing tag.

Example:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Documentation

See [pkg.go.dev](https://pkg.go.dev/github.com/Bibi40k/vmware-vm-bootstrap) for full API documentation.

Ubuntu support matrix: see [docs/UBUNTU_SUPPORT.md](docs/UBUNTU_SUPPORT.md).

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## Versioning

Semantic Versioning is documented in [docs/VERSIONING.md](docs/VERSIONING.md).

## License

MIT - see [LICENSE](LICENSE) file.

## Status

**v0.1.0 Alpha** - Core library functional (VM creation, cloud-init, ISO, SSH verification). API may change before v1.0.0.
