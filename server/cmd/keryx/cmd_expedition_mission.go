package main

import "fmt"

type expeditionMission struct {
	FirstToArea bool    `json:"first_to_area"`
	AreaQ       int     `json:"area_q"`
	AreaR       int     `json:"area_r"`
	LengthTicks int     `json:"length_ticks"`
	TurnTick    int     `json:"turn_tick"`
	HomeByTick  int     `json:"home_by_tick"`
	Homeward    bool    `json:"homeward"`
	TurnReason  *string `json:"turn_reason,omitempty"`
}

func expeditionMissionText(e *expeditionMission, clients ...*Client) string {
	c := firstTimeClient(clients)
	state := fmt.Sprintf("turns home by %s", formatWorldTime(c, e.TurnTick))
	if e.Homeward {
		state = "returning home"
		if e.TurnReason != nil {
			state += " — " + expeditionReason(*e.TurnReason)
		}
	}
	destination := fmt.Sprintf("around (%d,%d)", e.AreaQ, e.AreaR)
	if e.FirstToArea {
		destination = fmt.Sprintf("first to (%d,%d), then explores around it", e.AreaQ, e.AreaR)
	}
	return fmt.Sprintf("Expedition %s, %s; %s; home by %s", destination, formatDays(e.LengthTicks, "%d"), state, formatWorldTime(c, e.HomeByTick))
}
func expeditionReason(reason string) string {
	switch reason {
	case "half_time":
		return "half the time reached"
	case "area_known":
		return "area explored"
	case "no_path":
		return "no reachable unexplored ground"
	case "recalled":
		return "recalled"
	default:
		return reason
	}
}
