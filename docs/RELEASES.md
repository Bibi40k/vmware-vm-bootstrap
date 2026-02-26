# Release Notes

## Unreleased

Highlights:
- TBD

Notes:
- TBD

## v0.1.5 (2026-02-26)

Highlights:
- Bootstrap result export now includes SSH host fingerprint for downstream automation.
- CLI auto-writes a bootstrap result file to `tmp/` by default (configurable).

Notes:
- Bootstrap result enables strict host key verification in downstream tools (no prompt required).

## v0.1.4 (2026-02-25)

Highlights:
- Configurable guest NIC name (no longer hard-coded `ens192`).
- Smoke test improvements (reuse/recreate, SSH key handling, SSH port support, better feedback).
- ISO autoinstall cache invalidation via metadata.
- Added smoke test doc and automated release notes flow.

Notes:
- Ubuntu 20.04 autoinstall now patches ISOLINUX `append` lines.
- Release notes generated from `docs/RELEASES.md` via `scripts/release-notes.sh`.
- `--debug` writes to `tmp/vmbootstrap-debug.log`.

## v0.1.3 (2026-02-25)

Highlights:
- Configurable guest NIC name (no longer hard-coded `ens192`).
- Smoke test improvements (reuse/recreate, SSH key handling, SSH port support, better feedback).
- ISO autoinstall cache invalidation via metadata.
- Added smoke test doc and automated release notes flow.

Notes:
- Ubuntu 20.04 autoinstall now patches ISOLINUX `append` lines.
- Release notes generated from `docs/RELEASES.md` via `scripts/release-notes.sh`.
- `--debug` writes to `tmp/vmbootstrap-debug.log`.

## v0.1.1 (2026-02-24)

Highlights:
- First public alpha release.
- CI pipeline with tests, linting, and vuln checks.
- Coverage badge generation.
- Expanded simulator-based test coverage.
- VM post-creation operations: verify, power on/off, delete.
- Release workflow now attaches prebuilt binaries.
- Example configs for vCenter/VM.

Notes:
- NoCloud ISO creation uses a pure Go ISO9660 writer.
- `make test-cover` excludes `cmd/` packages to avoid toolchain issues.
- `vcenter.NewClient` accepts full `https://.../sdk` URLs (https only).
- `VM.Verify` requires VMware Tools running + SSH access.

## v0.1.0 (2026-02-24)

Highlights:
- First public alpha release.
- CI pipeline with tests, linting, and vuln checks.
- Coverage badge generation.
- Expanded simulator-based test coverage.
- VM post-creation operations: verify, power on/off, delete.
- Release workflow now attaches prebuilt binaries.
- Example configs for vCenter/VM.

Notes:
- NoCloud ISO creation uses a pure Go ISO9660 writer.
- `make test-cover` excludes `cmd/` packages to avoid toolchain issues.
- `vcenter.NewClient` accepts full `https://.../sdk` URLs (https only).
- `VM.Verify` requires VMware Tools running + SSH access.
