// Water classification — client mirror of server/internal/province/water.go
// (IsSea, IsNavigable, IsWaterBarrier). The one place the client lists water
// terrains; a new one is added here and in the server owners only.

// Open water: horizon, storms.
export function isSea(terrain) {
  return terrain === 'coastal_sea' || terrain === 'deep_sea';
}

// A ship floats here; never worked as land.
export function isNavigable(terrain) {
  return isSea(terrain) || terrain === 'river' || terrain === 'river_ford';
}

// Water a land unit cannot step into (the ford is the gap).
export function isWaterBarrier(terrain) {
  return isSea(terrain) || terrain === 'river';
}
