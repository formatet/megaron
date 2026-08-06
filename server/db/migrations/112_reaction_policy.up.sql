-- Migration 112: reaction_policy — the avsiktslagret's relation×avsikt table
-- (megaron_plan_avsiktslagret.md). Encoding decided by Timothy 2026-08-06:
-- a single JSONB column, not separate intent_foreign/intent_own columns —
-- gates in SQL as `reaction_policy->>'foreign' = 'intercept'`.
--
-- Default reproduces today's HARDCODED sentry behaviour exactly
-- (foreign->intercept, own->ignore, ally->ignore) — see
-- transport/intercept.go and combat/stance_set.go. Postgres applies a
-- constant DEFAULT to existing rows too (no separate UPDATE needed), so this
-- migration is behaviour-neutral for every sentry that exists today.
ALTER TABLE units
  ADD COLUMN reaction_policy JSONB NOT NULL
    DEFAULT '{"foreign":"intercept","own":"ignore","ally":"ignore"}'::jsonb;
