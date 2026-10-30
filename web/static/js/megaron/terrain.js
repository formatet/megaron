// Water classification — client mirror of server/internal/province/water.go
// (IsSea, IsNavigable, IsWaterBarrier). The one place the client lists water
// terrains; a new one is added here and in the server owners only.

// The open sea itself; storms live only here.
export function isThalassa(terrain) {
  return terrain === 'coastal_sea' || terrain === 'deep_sea';
}

// Open water: horizon, shore. A lake counts (Timothy 2026-10-30).
export function isSea(terrain) {
  return isThalassa(terrain) || terrain === 'lake';
}

// Shallow open water: coastal water or a lake.
export function isShallowWater(terrain) {
  return terrain === 'coastal_sea' || terrain === 'lake';
}

// A ship floats here; never worked as land.
export function isNavigable(terrain) {
  return isSea(terrain) || terrain === 'river' || terrain === 'river_ford';
}

// Water a land unit cannot step into (the ford is the gap).
export function isWaterBarrier(terrain) {
  return isSea(terrain) || terrain === 'river';
}
