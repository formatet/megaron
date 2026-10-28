package world

// RiverEdgeIndices decodes saved masks into the public six-edge protocol.
// It never derives topology or consults neighbouring tiles.
func RiverEdgeIndices(mask int16) []int {
	out := make([]int, 0, 6)
	for i := 0; i < 6; i++ {
		if mask&(1<<i) != 0 {
			out = append(out, i)
		}
	}
	return out
}
func riverEdgeMask(edges []int) int16 {
	var mask int16
	for _, i := range edges {
		mask |= 1 << i
	}
	return mask
}

// PopulateRiverMetadata is called once after the map's final terrain passes.
// The same owner serves a one-time legacy backfill from the saved geography.
func PopulateRiverMetadata(tiles []MapTile, width, height int) {
	edges := RiverEdges(tiles)
	flows := RiverFlows(tiles, width, height)
	for i := range tiles {
		xy := [2]int{tiles[i].Q, tiles[i].R}
		e, ok := edges[xy]
		if !ok {
			continue
		}
		f := flows[xy]
		tiles[i].RiverMetadataReady = true
		tiles[i].RiverEdgesMask = riverEdgeMask(e)
		tiles[i].RiverUpstreamMask = riverEdgeMask(f.Upstream)
		tiles[i].RiverDownstreamMask = riverEdgeMask(f.Downstream)
		tiles[i].RiverFlowKnown = f.Known
	}
}

// RiverMetadataValues lets each tile writer persist the generated values in
// the same INSERT. Non-river tiles retain NULLs; empty river edges are mask 0.
func (t MapTile) RiverMetadataValues() (*int16, *int16, *int16, *bool) {
	if !t.RiverMetadataReady {
		return nil, nil, nil, nil
	}
	return &t.RiverEdgesMask, &t.RiverUpstreamMask, &t.RiverDownstreamMask, &t.RiverFlowKnown
}
