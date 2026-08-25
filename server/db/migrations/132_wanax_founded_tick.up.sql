-- Migration 132: nådefrist för nygrundade Wanaxer (megaron_todo.md §NU,
-- "kult-brunnen"; beslut Timothy 2026-08-25, alternativ A).
--
-- Varför en egen kolumn: nådefristen mäts i TICK, aldrig i väggklocka
-- (CLAUDE.md — framsteg mäts i speldygn). `joined_at` finns redan men är en
-- timestamptz och duger därför inte, och `kharis_calc_tick` skrivs om varje
-- dygn av kharis-ticket. Det behövdes ett tal som står still.
--
-- DEFAULT 0 är avsiktligt för BEFINTLIGA rader: en levande värld står på
-- tusentals tick, så current_world_tick() - 0 passerar fristen med marginal
-- och ingen redan spelande Wanax får en retroaktiv fredad period.
ALTER TABLE player_world_records
    ADD COLUMN founded_tick INTEGER NOT NULL DEFAULT 0;
