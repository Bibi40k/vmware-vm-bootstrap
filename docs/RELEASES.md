# Release Notes

## Unreleased

Highlights:
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

## v0.1.0 (2026-02-23)

Highlights:
- First public alpha release.
- CI pipeline (tests, lint, vuln checks) and coverage badge.
- Expanded simulator-based tests.

Notes:
- NoCloud ISO creation uses a pure Go ISO9660 writer.
- `make test-cover` excludes `cmd/` packages to avoid toolchain issues.
