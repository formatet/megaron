#!/usr/bin/env bash
set -euo pipefail
report_dir="$(cd "$(dirname "$0")" && pwd)"
repo_dir="$(git -C "$report_dir" rev-parse --show-toplevel)"
study_dir="$(mktemp -d "$repo_dir/server/cmd/silver-study-XXXXXX")"
trap 'rm -rf "$study_dir"' EXIT
cp "$report_dir/mapgen.go.txt" "$study_dir/main.go"
(cd "$repo_dir/server" && go run "./cmd/$(basename "$study_dir")")
