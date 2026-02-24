# Smoke Test

This is a minimal, practical validation to run before a public release.

## Prerequisites

- `configs/vcenter.sops.yaml` exists and can be decrypted
- `configs/vm.<name>.sops.yaml` exists (VM config)
- `make install-requirements` already run

## Run

```bash
make smoke VM=configs/vm.myvm.sops.yaml
```

If the VM already exists, choose:

- **Reuse existing VM** (checks only)
- **Create new VM** (delete and recreate)

## What It Checks

- SSH connectivity (with retries)
- Data disk mount (if configured)
- Swap enabled (if configured)
- `open-vm-tools` running

## Expected Outcome

- "✓ Smoke checks passed"
- Optional cleanup (delete VM)

If it fails:

- Keep the VM for inspection
- Use the printed SSH command to debug

