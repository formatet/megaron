# Physical vision

Contract: a marching unit's live eye follows the same saved weighted tick route as its marker, using the world's tick anchor. Move the saved route decoder into low-level movement; province may consume movement but never combat/tick. Four upper consumers supply anchor.MilliAt(now). Legacy absent/stale routes retain explicit wall-clock fallback. No new sight radius, city relocation, founding/marker layout change or courier movement changes. FUNCTION gate; supports geographic discovery. Proof: unequal-cost saved-route fixture with deliberately misleading wall timestamps and map route; fresh baseline/fix, mutation, complete Go/JS and vet.


Implementation: `movement.StoredRoute` and `LoadActiveRoute` own the persistent decoder; combat retains its public adapters. Province uses movement directly. The HTTP adapter and the three domain consumers load the world anchor and supply its millitick. The transaction adapter supports both pool and TX without nested readers.

Proof: `TestMarchEyeUsesSavedTickRouteAtMarkerPosition` checks land/naval eyes against marker positions at five tick boundaries on a weighted detour whose wall-clock departure is still in the future. Old interpolation is physically reintroduced by `tools/physical_eyes_mutation.py`: named RED, restoration GREEN. Existing single-connection/TX regression GREEN. Relevant vet GREEN; all 684 JS GREEN. Full Go result follows below.

Live observation (read only): Lord of the Deep was marching 27,16 →20,15 at ticks768–772. By tick772 it had already turned home, and21,15 was remembered forest/olive grove. That confirms the reported eventual reveal, without proving its precise historical frame. No player order or DB row was changed.

Gates: FUNCTION; no new layout, text, radius, movement semantics or city coordinates. Legacy absent/stale routes retain their previous fallback. Founding and approaching/arrived presentation remain separate work.

Full fresh193 Go suite GREEN (world144.477s), relevant vet GREEN,684/684JS GREEN. No schema/static changes.
