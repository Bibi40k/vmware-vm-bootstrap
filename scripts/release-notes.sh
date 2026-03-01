#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
notes_file="${2:-docs/RELEASES.md}"

if [[ -z "${version}" ]]; then
  echo "usage: $0 <version> [notes_file]" >&2
  exit 1
fi

extract_section() {
  local header="$1"
  awk -v h="$header" '
    $0 ~ "^## " h "([[:space:]]*\\(.*\\))?$" {in_section=1; print; next}
    $0 ~ "^## " {if (in_section) exit}
    in_section {print}
  ' "$notes_file"
}

if extract_section "$version" | grep -q .; then
  extract_section "$version"
  exit 0
fi

if extract_section "Unreleased" | grep -q .; then
  extract_section "Unreleased"
  exit 0
fi

echo "No release notes found for $version and no Unreleased section in $notes_file" >&2
exit 1
