#!/usr/bin/env bash
# Convert all ABC notation files to MIDI.
# Requires: abcmidi (sudo pacman -S abcmidi)
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p midi

CULTURES=(akhaier khemetiu knaani thrakes pelasger hatti)
THEMES=(war victory doom love)

ok=0
fail=0

for culture in "${CULTURES[@]}"; do
  for theme in "${THEMES[@]}"; do
    src="${culture}_${theme}.abc"
    out="midi/${culture}_${theme}.mid"
    if [[ -f "$src" ]]; then
      if abc2midi "$src" -o "$out" 2>/dev/null; then
        echo "  ✓  $out"
        ok=$((ok+1))
      else
        echo "  ✗  $src — conversion failed"
        fail=$((fail+1))
      fi
    else
      echo "  ?  $src not found"
      fail=$((fail+1))
    fi
  done
done

echo ""
echo "$ok MIDI files written to midi/"
[[ $fail -gt 0 ]] && echo "$fail files failed"
