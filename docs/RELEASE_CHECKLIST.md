# Release Checklist

## 1) Pre-flight

- Working tree is clean (`git status`).
- Release notes are updated in `docs/RELEASES.md`:
  - add a new section `## vX.Y.Z (YYYY-MM-DD)`
  - complete `Highlights` and `Notes`
  - leave `Unreleased` placeholder at top
- Dependency versions are pinned and reviewed.

## 2) Quality Gate

Run locally:

```bash
make fmt
make vet
make lint
make test
make build
make vulncheck
```

Optional but recommended:

```bash
make test-cover
```

## 3) Tag and Push

```bash
git checkout <release-branch>
git pull --ff-only
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

## 4) GitHub Release Verification

- Confirm `Release` workflow succeeded.
- Confirm release body was generated from `docs/RELEASES.md`.
- Confirm attached artifacts and checksums are present and downloadable.
- Keep the release as draft until manual validation is complete.

## 5) Post-release

- Validate install path from release artifact.
- Run smoke checks on a representative environment.
- Announce release with key highlights and upgrade notes.
