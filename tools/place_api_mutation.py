#!/usr/bin/env python3
"""Physical mutation of the authoritative place-name permission gate."""
import pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/flod-doparratt'
f=ROOT/'server/api/handlers/place_names.go';original=f.read_bytes()
try:
 source=original.decode();guard='if claimedBy == nil || *claimedBy != playerID {';assert guard in source
 f.write_text(source.replace(guard,'if false {',1))
 run=subprocess.run(['tools/gotest.sh','./api/handlers/','-run','TestPlaceNameOnlyFirstToReachAndOnce'],cwd=ROOT,capture_output=True,text=True)
 text=run.stdout+run.stderr;(OUT/'mutation-api-right.log').write_text(text)
 assert run.returncode and 'nonclaimant naming' in text and 'want403' in text,text
 print('place permission: named red')
finally:f.write_bytes(original)
with (OUT/'restored-api-right.log').open('w') as log:
 subprocess.run(['tools/gotest.sh','./api/handlers/','-run','TestPlaceName|TestPlaces|TestMapRiverNamesFOW'],cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,check=True)
print('place permission: restored green')
