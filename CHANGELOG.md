# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- VM post-creation operations: `Verify`, `PowerOn`, `PowerOff`, `Delete`.
- vCenter simulator tests for VM operations and vCenter list/find behavior.
- Release workflow now attaches prebuilt binaries to GitHub Releases.
- Example configs: `configs/vcenter.example.yaml`, `configs/vm.example.yaml`.

### Changed
- `vcenter.NewClient` now accepts full `https://` URLs with scheme.
- `VM.Verify` is strict: VMware Tools running + SSH access required.
- SOPS encryption pipes plaintext to SOPS (no plaintext written to disk).

### Fixed
- Prevented deletion from wrong datastore when `ISODatastore` differs from `Datastore`.
- Datastore file existence checks no longer swallow non-NotFound errors.
- Removed sensitive/local config files from the repository.

## [0.1.0] - 2026-02-23

### Added
- CI pipeline (tests, lint, vulncheck).
- Coverage badge generation in CI.
- vCenter simulator-based tests for `vcenter` and `vm`.
- Extended bootstrap error-path tests.

### Changed
- NoCloud ISO creation uses pure Go ISO9660 writer.
- `make test-cover` excludes `cmd/` packages from coverage.
- README clarified library vs CLI dependencies and SOPS usage.

### Fixed
- `make test-cover` no longer fails on `covdata` in CLI package.
