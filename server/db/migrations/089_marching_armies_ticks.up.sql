-- Tick-anchored march timing on marching_armies (tid & kalender Fas B-rest,
-- samma mönster som mig 085 på units): depart_tick/arrive_tick är sanningen —
-- upplösningen schemaläggs redan på due_tick (EnqueueTickTx), departs_at/arrives_at
-- är wall-clock-snapshots som läser fel efter tempo-byte. Nullable: äldre rader
-- saknar tick → API:t utelämnar fälten och klienten faller tillbaka på ISO
-- (ui/time.js msUntil). Tabellen är legacy på väg mot units-konsolidering —
-- kolumnerna följer med i den migreringen.
ALTER TABLE marching_armies ADD COLUMN depart_tick INT;
ALTER TABLE marching_armies ADD COLUMN arrive_tick INT;
