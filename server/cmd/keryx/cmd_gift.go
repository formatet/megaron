package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
)

func resolveTransferDestination(c *Client, world, source, name string) (string, error) {
	if _, err := uuid.Parse(name); err == nil {
		return name, nil
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/trade/destinations", world, source))
	if err != nil {
		return "", err
	}
	var rows []struct {
		ID   string `json:"settlement_id"`
		Name string `json:"name"`
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		return "", err
	}
	found := ""
	for _, r := range rows {
		if strings.EqualFold(r.Name, name) {
			if found != "" {
				return "", fmt.Errorf("more than one city named %q; use its settlement ID", name)
			}
			found = r.ID
		}
	}
	if found == "" {
		// Destinations exclude the sending city. A selected colony is still a
		// valid source: explain a self-transfer rather than pretending it is absent.
		data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", world))
		if err != nil {
			return "", fmt.Errorf("could not identify the sending city: %w", err)
		}
		var markers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Own  bool   `json:"own"`
		}
		if err := json.Unmarshal(data, &markers); err != nil {
			return "", err
		}
		for _, marker := range markers {
			if marker.Own && marker.ID == source && strings.EqualFold(marker.Name, name) {
				return "", fmt.Errorf("sending city %q is also the destination — choose another source with --from <city> (see keryx settlements)", marker.Name)
			}
		}
		return "", fmt.Errorf("no own or contacted city named %q", name)
	}
	return found, nil
}

func printGiftOutcome(n notificationItem) {
	var b struct {
		SenderName string  `json:"sender_name"`
		Good       string  `json:"good_key"`
		Credited   float64 `json:"credited_quantity"`
		Returned   float64 `json:"returned_quantity"`
		Lost       float64 `json:"lost_quantity"`
		Reason     string  `json:"reason"`
		Changed    bool    `json:"owner_changed"`
		Actual     string  `json:"actual_recipient_name"`
		Intended   string  `json:"recipient_name"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return
	}
	fmt.Printf("    Gift: %s → %s · %g %s received; %g lost", giftWanax(b.SenderName), giftWanax(b.Intended), b.Credited, b.Good, b.Lost)
	if b.Returned > 0 {
		fmt.Printf(" · %g sent home aboard the damaged ship (return can still be intercepted)", b.Returned)
	}
	if b.Reason != "" {
		fmt.Printf(" · %s", strings.ReplaceAll(b.Reason, "_", " "))
	}
	if b.Changed {
		fmt.Printf(" · now held by %s, intended for %s", b.Actual, b.Intended)
	}
	fmt.Println()
}

func giftWanax(name string) string {
	if name == "" {
		return "a Wanax"
	}
	return name
}

func printManifestOutcome(n notificationItem) {
	var b struct {
		Sender    string `json:"sender_name"`
		Recipient string `json:"recipient_name"`
		Actual    string `json:"actual_recipient_name"`
		Changed   bool   `json:"owner_changed"`
		Reason    string `json:"reason"`
		Items     []struct {
			Good     string  `json:"good_key"`
			Quantity float64 `json:"quantity"`
			Credited float64 `json:"credited_quantity"`
			Lost     float64 `json:"lost_quantity"`
			Returned float64 `json:"returned_quantity"`
		} `json:"items"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return
	}
	label := "Transfer delivered"
	if strings.HasPrefix(n.Kind, "Gift") {
		label = "Gift delivered"
	}
	if strings.Contains(n.Kind, "Lost") {
		label = strings.Replace(label, "delivered", "lost", 1)
	}
	if n.Kind == "GiftDispatchedV2" {
		label = "Gift sent"
	}
	fmt.Printf("      %s: ", label)
	if strings.HasPrefix(n.Kind, "Gift") {
		fmt.Printf("%s → %s · ", giftWanax(b.Sender), giftWanax(b.Recipient))
	}
	for i, item := range b.Items {
		if i > 0 {
			fmt.Print(", ")
		}
		if n.Kind == "GiftDispatchedV2" {
			fmt.Printf("%g %s", item.Quantity, item.Good)
		} else {
			fmt.Printf("%g %s received; %g lost", item.Credited, item.Good, item.Lost)
			if item.Returned > 0 {
				fmt.Printf("; %g sent home aboard the damaged ship (return can still be intercepted)", item.Returned)
			}
		}
	}
	if b.Reason != "" {
		fmt.Printf(" · %s", strings.ReplaceAll(b.Reason, "_", " "))
	}
	if b.Changed {
		fmt.Printf(" · now held by %s, intended for %s", giftWanax(b.Actual), giftWanax(b.Recipient))
	}
	fmt.Println()
}
