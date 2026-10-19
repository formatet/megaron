package main

import (
	"encoding/json"
	"fmt"
	"math"
)

// Storage forecasts are resolved by the server, including known deliveries.
func storageWarningLines(sett map[string]any) []string {
	rows, _ := sett["storage"].([]any)
	lines := []string{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		warning, _ := row["warning"].(bool)
		days, ok := row["full_in_ticks"].(float64)
		if !warning || !ok || math.IsNaN(days) || math.IsInf(days, 0) || days < 0 {
			continue
		}
		good, _ := row["good_key"].(string)
		amount, _ := row["amount"].(float64)
		cap, _ := row["cap"].(float64)
		lines = append(lines, fmt.Sprintf("Storage: %s %s/%s — full in %s. Send goods away or trade.", good, notificationAmount(amount), notificationAmount(cap), formatDays(days, "%g")))
	}
	return lines
}

func storageReason(reason string) string {
	switch reason {
	case "production":
		return "production"
	case "refining":
		return "refining"
	case "transfer_delivery":
		return "an internal transfer"
	case "trade_delivery":
		return "a trade delivery"
	case "trade_return":
		return "returned trade goods"
	case "gift_delivery":
		return "a gift delivery"
	case "seized_cargo":
		return "seized cargo"
	case "returned_provisions":
		return "returned provisions"
	case "returned_purse":
		return "a returned purse"
	case "build_refund":
		return "a building refund"
	case "escrow_refund":
		return "an offer refund"
	case "founder_stores":
		return "founding stores"
	default:
		return "incoming goods"
	}
}

func printStorageNotification(n notificationItem) bool {
	if n.Kind != "StorageCapacityWarning" && n.Kind != "StorageOverflow" {
		return false
	}
	var b struct {
		Name   string   `json:"name"`
		Good   string   `json:"good_key"`
		Cap    float64  `json:"cap"`
		FullIn *float64 `json:"full_in_ticks"`
		Lost   *float64 `json:"lost_quantity"`
		Reason string   `json:"reason"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return true
	}
	if n.Kind == "StorageCapacityWarning" {
		duration := "forecast unavailable"
		if b.FullIn != nil {
			duration = "full in " + formatDays(*b.FullIn, "%g")
		}
		fmt.Printf("      %s: %s storage %s (capacity %s). Send goods away or trade.\n", b.Name, b.Good, duration, notificationAmount(b.Cap))
	} else {
		quantity := "unknown amount of"
		if b.Lost != nil {
			quantity = notificationAmount(*b.Lost)
		}
		fmt.Printf("      %s: storage full — lost %s %s from %s (capacity %s). Send goods away or trade.\n", b.Name, quantity, b.Good, storageReason(b.Reason), notificationAmount(b.Cap))
	}
	return true
}
