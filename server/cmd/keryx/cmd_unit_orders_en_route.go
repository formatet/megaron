package main

import (
	"fmt"
	"time"
)

// ordersEnRouteLine renders one order the player sent whose Runner has not yet
// delivered it: "Order on its way: march to (47,23) — the runner reaches it in
// 3 days". The player sent it and knows where it goes (no FOW concern).
func ordersEnRouteLine(c *Client, o orderEnRoute) string {
	line := "Order on its way: " + orderVerbText(o)
	if o.FollowingUnit {
		return line + " — " + runnerPursuitText(o.PursuitLegs, arrivalETA(c, o.ArrivesAt), o.FollowingWait)
	}
	if t, err := time.Parse(time.RFC3339, o.ArrivesAt); err == nil {
		if eta := gameETA(c, t); eta != "arrival time unavailable" {
			line += " — the runner reaches it " + eta
		}
	}
	return line
}

func orderVerbText(o orderEnRoute) string {
	to := ""
	if o.TargetQ != nil && o.TargetR != nil {
		to = fmt.Sprintf(" to (%d,%d)", *o.TargetQ, *o.TargetR)
	}
	switch o.Verb {
	case "march":
		return "march" + to
	case "redirect":
		return "redirect" + to
	case "recall":
		return "recall home"
	case "stance", "stance_pursuit":
		return "stance change"
	case "occupy_action":
		return "occupation order"
	case "standing_orders":
		return "retreat orders"
	case "build_road":
		return "build road" + to
	}
	return o.Verb
}

// runnerPursuitText describes the current physical leg, never a delivery promise.
func runnerPursuitText(legs int, eta string, waiting bool) string {
	if waiting {
		return "Runner waiting to continue following the unit"
	}
	text := "Runner following the unit"
	if legs > 0 {
		text += " on a new route"
	}
	if eta == "" || eta == "arrival time unavailable" {
		return text + " — meeting time unavailable"
	}
	return text + " — next meeting " + eta
}

func pursuitDispatchText(c *Client, resp map[string]any) string {
	if resp["following_unit"] != true {
		return ""
	}
	at, _ := resp["courier_arrives_at"].(string)
	waiting, _ := resp["following_wait"].(bool)
	legs, _ := resp["pursuit_legs"].(float64)
	if resp["passage_status"] == "awaiting_passage" {
		return "Runner following the unit — waiting for a ship"
	}
	return runnerPursuitText(int(legs), arrivalETA(c, at), waiting)
}
