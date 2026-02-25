# Release Notes

## Unreleased

Highlights:
- TBD

Notes:
- TBD

## v0.1.5 (2026-02-26)

Highlights:
- Stage 1 result export now includes SSH host fingerprint for downstream automation.
- `vmbootstrap run --stage1-result` writes a normalized Stage 1 contract file.

Notes:
- Stage 1 output enables strict host key verification in downstream tools (no prompt required).

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
