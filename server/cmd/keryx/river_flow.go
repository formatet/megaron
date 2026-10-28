package main

import "strings"

type riverMapInfo struct {
	RiverEdges           *[]int `json:"river_edges,omitempty"`
	RiverFlowKnown       *bool  `json:"river_flow_known,omitempty"`
	RiverUpstreamEdges   []int  `json:"river_upstream_edges,omitempty"`
	RiverDownstreamEdges []int  `json:"river_downstream_edges,omitempty"`
}

var riverOffsets = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
var riverCompass = []string{"south-east", "north-west", "south", "north", "north-east", "south-west"}

func riverDirections(edges []int) string {
	names := []string{}
	seen := map[int]bool{}
	for _, i := range edges {
		if i >= 0 && i < len(riverCompass) && !seen[i] {
			names = append(names, riverCompass[i])
			seen[i] = true
		}
	}
	return strings.Join(names, " and ")
}
func riverMapSummary(meta riverMapInfo, q, r int, known map[[2]int]bool) string {
	if meta.RiverEdges == nil {
		return ""
	}
	lines := []string{}
	if meta.RiverFlowKnown != nil && *meta.RiverFlowKnown {
		if to := riverDirections(meta.RiverDownstreamEdges); to != "" {
			from := riverDirections(meta.RiverUpstreamEdges)
			flow := "flows "
			if from != "" {
				flow += "from " + from + " "
			}
			lines = append(lines, flow+"toward "+to)
		}
	} else if meta.RiverFlowKnown != nil {
		lines = append(lines, "flow direction unavailable")
	}
	fog := []int{}
	for _, i := range *meta.RiverEdges {
		if i >= 0 && i < len(riverOffsets) {
			d := riverOffsets[i]
			if !known[[2]int{q + d[0], r + d[1]}] {
				fog = append(fog, i)
			}
		}
	}
	if len(fog) > 0 {
		lines = append(lines, "continues into fog toward "+riverDirections(fog))
	}
	if len(lines) == 0 {
		return ""
	}
	return " · " + strings.Join(lines, " · ")
}
