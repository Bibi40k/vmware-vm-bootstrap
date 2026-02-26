# Release Notes

## Unreleased

Highlights:
- TBD

Notes:
- TBD

## v0.1.10 (2026-02-26)

Highlights:
- Destructive confirmations now default to No (`[y/N]`) and are highlighted in red.

Notes:
- Applied to overwrite/delete/cleanup prompts in `config`, `create`, and `smoke` flows.

## v0.1.9 (2026-02-26)

Highlights:
- Fixes VM selector lint regression introduced in v0.1.8.

Notes:
- No functional changes beyond lint cleanup.

## v0.1.8 (2026-02-26)

Highlights:
- VM bootstrap menu now uses the same interactive selector style as config manager.

Notes:
- Fixes selector UI alignment issues in some terminals.

## v0.1.7 (2026-02-26)

Highlights:
- Autoinstall no-swap now works when `swap_size_gb=0` (swap partition omitted).
- VM selection supports a clean Exit option in the bootstrap menu.

Notes:
- Ctrl+C in the VM selector exits without triggering a bootstrap prompt.

## v0.1.6 (2026-02-26)

Highlights:
- Bootstrap result terminology (no legacy naming) across CLI/docs/config.
- Default bootstrap result output is enabled and configurable via `output.*`.

Notes:
- New flag `--bootstrap-result` replaces the previous output flag name.
- Default output path: `tmp/bootstrap-result.{vm}.yaml` (can be disabled).

## v0.1.5 (2026-02-26)

Highlights:
- Bootstrap result export includes SSH host fingerprint for downstream automation.
- Optional CLI flag writes a normalized bootstrap contract.

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
