package main

import "fmt"

type expeditionMission struct {
	AreaQ       int     `json:"area_q"`
	AreaR       int     `json:"area_r"`
	LengthTicks int     `json:"length_ticks"`
	TurnTick    int     `json:"turn_tick"`
	HomeByTick  int     `json:"home_by_tick"`
	Homeward    bool    `json:"homeward"`
	TurnReason  *string `json:"turn_reason,omitempty"`
}

func expeditionMissionText(e *expeditionMission) string {
	state := fmt.Sprintf("turns home by %s", formatDay(e.TurnTick, "%d"))
	if e.Homeward {
		state = "returning home"
		if e.TurnReason != nil {
			state += " — " + expeditionReason(*e.TurnReason)
		}
	}
	return fmt.Sprintf("Expedition around (%d,%d), %s; %s; home by %s", e.AreaQ, e.AreaR, formatDays(e.LengthTicks, "%d"), state, formatDay(e.HomeByTick, "%d"))
}
func expeditionReason(reason string) string {
	switch reason {
	case "half_time":
		return "half the time reached"
	case "area_known":
		return "area explored"
	case "no_path":
		return "no reachable unexplored ground"
	default:
		return reason
	}
}
