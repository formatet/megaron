#!/usr/bin/env python3
"""Change the shared ceiling once; all database consumer pins must fail."""
from pathlib import Path
import os
import subprocess

root=Path(__file__).resolve().parents[3]
source=root/"server/internal/province/storage.go"
original=source.read_text()
old="const DefaultGoodStorageCap = 1_000_000.0"
assert original.count(old)==1
try:
 source.write_text(original.replace(old,"const DefaultGoodStorageCap = 500_000.0"))
 result=subprocess.run([str(root/"tools/gotest.sh"),"./api/handlers","./internal/combat","./internal/transport","./internal/economy","-run","TestDefaultGoodStorageCap|TestTradeDelivery_StaleCap|TestTradeReturn_StaleCap","-v"],cwd=root,env={k:os.environ[k] for k in ("HOME","PATH")},capture_output=True,text=True)
 print(result.stdout+result.stderr)
 assert result.returncode!=0
 for name in ["TestDefaultGoodStorageCapMetropolis","TestDefaultGoodStorageCapLogistics","TestDefaultGoodStorageCapColony","TestDefaultGoodStorageCapArrival","TestDefaultGoodStorageCapLoot","TestTradeDelivery_StaleCapTruncatesSecondDelivery","TestTradeReturn_StaleCapTruncatesSecondDelivery","TestDefaultGoodStorageCapEconomy"]:
  assert "--- FAIL: "+name in result.stdout, "missing assertion failure: "+name
 print("VALID MUTATION: one owner changed; all consumers failed assertions")
finally:
 source.write_text(original)
print("Original production source restored; run full fresh suite next.")
