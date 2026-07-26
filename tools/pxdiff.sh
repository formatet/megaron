#!/usr/bin/env bash
# Antal FAKTISKT ändrade pixlar mellan två skärmdumpar, plus en diffbild.
#
#   ./pxdiff.sh före.png efter.png [diff.png]
#
# Varför inte `compare -metric AE`: i ImageMagick 7.1.2 Q16-HDRI returnerar AE
# inte ett pixelantal (den gav 7,49e7 för en bild med 702 000 pixlar, där det
# sanna antalet var 10 210). AE duger bara för frågan "är det exakt 0?" —
# determinismkontrollen. Varje påstående om HUR MYCKET som ändrats måste räknas
# så här i stället.
#
# Per-kanal: differensen separeras och maxas över kanalerna innan tröskling, så
# en pixel som bara ändrats i en kanal räknas som en hel pixel och inte som 1/3.
set -euo pipefail

A=$1; B=$2; OUT=${3:-}

# Trailing \n krävs: utan den returnerar `read` 1 vid EOF och `set -e` dödar
# skriptet innan något räknats.
read -r W H < <(magick identify -format '%w %h\n' "$A")
N=$(magick "$A" "$B" -compose difference -composite \
      -separate -evaluate-sequence Max -threshold 0 \
      -format "%[fx:int(mean*w*h)]" info:)

if [ -n "$OUT" ]; then
  # Rött = ändrat, blekt gråskaleoriginal under: visar VAR ändringen ligger.
  # Tre bilder till -composite = dst, src, MASK (annars täcker src allt).
  magick "$A" -colorspace Gray -evaluate multiply 0.55 -colorspace sRGB \
      \( -clone 0 -fill red -colorize 100 \) \
      \( "$A" "$B" -compose difference -composite \
         -separate -evaluate-sequence Max -threshold 0 \) \
      -compose over -composite "$OUT"
fi

awk -v n="$N" -v tot="$((W * H))" \
  'BEGIN { printf "%d ändrade pixlar av %d (%.2f %%)\n", n, tot, 100 * n / tot }'
