# BILD · Doom fire över en bränd stad (`claude/doom-fire`)

En stad som plundras och bränns brinner på kartan under just den ticken (ett speldygn),
bara för den som har levande sikt över hexen. Sedan står ruinen kvar som förut.

- `firefox-1to1.png` — Firefox, 1280×800, acceptansriggen @ `0652520`, mig 164. Brandvakts
  Argos (47,47) och den brända staden Midea (44,47), avstånd 3, i levande sikt.
- `firefox-crop-3-frames-x3.png` — samma ställe, tre bildrutor ~180 ms isär, förstorade 3× (nearest).

Fixtur: Midea är insatt med SQL (`state='razed'`, `burned_tick = current_tick`), inte bränd via
ordervägen. Ordervägens stämpel bevisas av `TestExecuteOccupyAction_Burn`.

Att döma: storlek mot ruin och stad (16×18 px), palett, höjd på lågorna, att träden
ritas över eldens högerkant.
