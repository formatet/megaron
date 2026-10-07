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
	state := fmt.Sprintf("turns home by game day %d", e.TurnTick)
	if e.Homeward {
		state = "returning home"
		if e.TurnReason != nil {
			state += " — " + expeditionReason(*e.TurnReason)
		}
	}
	return fmt.Sprintf("Expedition around (%d,%d), %d game days; %s; home by game day %d", e.AreaQ, e.AreaR, e.LengthTicks, state, e.HomeByTick)
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
