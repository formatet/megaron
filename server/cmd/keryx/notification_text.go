package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// The archive also contains retired kinds. Keep their summaries readable;
// current producers and the Codex client's archive kinds are checked together.
func printNotificationSummary(c *Client, n notificationItem) {
	defer printLetterReplyHint(n)
	switch n.Kind {
	case "GiftDispatchedV2", "GiftDeliveredV2", "GiftLostV2", "TransferDeliveredV2", "TransferLostV2":
		printManifestOutcome(n)
		return
	}
	if n.Kind == "BattleBegun" {
		var b struct {
			Opponent string `json:"opponent_name"`
			Place    string `json:"place"`
			Q        int    `json:"q"`
			R        int    `json:"r"`
			At       string `json:"occurred_at"`
			Joined   bool   `json:"joined"`
		}
		if json.Unmarshal(n.Body, &b) != nil {
			return
		}
		if b.Opponent == "" {
			b.Opponent = "an enemy"
		}
		if b.Place == "" {
			b.Place = fmt.Sprintf("(%d, %d)", b.Q, b.R)
		}
		action := "battle has begun"
		if b.Joined {
			action = "your forces joined the battle"
		}
		fmt.Printf("      %s — %s at %s · %s\n", b.Opponent, action, b.Place, b.At)
		return
	}
	if n.Kind == "TempleDemolitionWrath" {
		var b demolitionKharis
		if json.Unmarshal(n.Body, &b) == nil {
			fmt.Printf("      %s\n", demolitionKharisText(b))
			return
		}
	}
	if n.Kind == "RiteAnswered" {
		var b struct {
			PrayerName string `json:"prayer_name"`
			Name       string `json:"name"`
			Message    string `json:"message"`
		}
		if json.Unmarshal(n.Body, &b) == nil {
			place := ""
			if b.Name != "" {
				place = " at " + b.Name
			}
			fmt.Printf("      %s%s: %s\n", b.PrayerName, place, b.Message)
			return
		}
	}
	if n.Kind == "KharisDeclining" {
		var b struct {
			Kharis    float64 `json:"kharis"`
			Net       float64 `json:"net_per_tick"`
			Days      int     `json:"days_to_15"`
			Threshold float64 `json:"wrath_threshold"`
		}
		if json.Unmarshal(n.Body, &b) == nil {
			fmt.Printf("      Kharis is falling (%.1f, net %+.2f per game day). At this rate it reaches %.0f in %s. Below that the gods may punish you, with or without a temple. Staff and feed your temples.\n", b.Kharis, b.Net, b.Threshold, formatDays(b.Days))
			return
		}
	}
	if printStorageNotification(n) {
		return
	}
	if n.Kind == "GarrisonStarvationStarted" || n.Kind == "GarrisonStarvationEnded" || n.Kind == "CivilianStarvationStarted" || n.Kind == "CivilianStarvationEnded" || n.Kind == "KharisBandChanged" {
		var b struct {
			FoodCategory bool     `json:"food_category"`
			Name         string   `json:"name"`
			FromBand     string   `json:"from_band"`
			ToBand       string   `json:"to_band"`
			Kharis       float64  `json:"kharis"`
			Unmet        *float64 `json:"unmet"`
			Reason       string   `json:"reason"`
		}
		if json.Unmarshal(n.Body, &b) != nil {
			return
		}
		switch n.Kind {
		case "GarrisonStarvationStarted":
			if b.FoodCategory {
				fmt.Printf("      %s: the garrison is losing men to hunger. Soldiers share the city’s grain, fish, oil and wine. Produce or transfer more food here (`keryx transfer`).\n", b.Name)
				break
			}
			fmt.Printf("      %s: the garrison is losing men to hunger. Soldiers need grain even when the city has a food surplus. Place citizens on grain (`keryx place`) or transfer grain here (`keryx transfer`).\n", b.Name)
		case "GarrisonStarvationEnded":
			fmt.Printf("      %s: the garrison is no longer losing men to hunger.\n", b.Name)
		case "CivilianStarvationStarted":
			amount := "unknown"
			if b.Unmet != nil {
				amount = notificationAmount(*b.Unmet)
				if *b.Unmet > 0 && *b.Unmet < 0.1 {
					amount = "<0.1"
				}
			}
			if b.FoodCategory {
				fmt.Printf("      %s: people are going hungry — the meal fell short by %s food. Produce grain, fish, oil or wine, or transfer food here (`keryx transfer`).\n", b.Name, amount)
				break
			}
			fmt.Printf("      %s: people are going hungry — the meal fell short by %s grain. Place citizens on grain or fish (`keryx place`), or transfer food here (`keryx transfer`).\n", b.Name, amount)
		case "CivilianStarvationEnded":
			if b.Reason == "no_longer_owned_or_active" {
				fmt.Printf("      %s: civilian hunger warning closed — this city is no longer yours or active.\n", b.Name)
			} else {
				fmt.Printf("      %s: people were fed again.\n", b.Name)
			}
		case "KharisBandChanged":
			fmt.Printf("      Kharis: %s → %s (%.1f). Check `keryx status` for decay and temple support; staff and feed your temples.\n", b.FromBand, b.ToBand, b.Kharis)
		}
		return
	}

	var b struct {
		Title             string             `json:"title"`
		ArrivesAt         string             `json:"arrives_at"`
		SilverArrivesAt   string             `json:"silver_arrives_at"`
		GoodsArrivesAt    string             `json:"goods_arrives_at"`
		SilverArrivalTick *float64           `json:"silver_arrival_tick"`
		GoodsArrivalTick  *float64           `json:"goods_arrival_tick"`
		Name              string             `json:"name"`
		Type              string             `json:"type"`
		UnitType          string             `json:"unit_type"`
		BuildingType      string             `json:"building_type"`
		Temple            string             `json:"temple"` // a temple's name, "Temple of Potnia"
		OutputKey         string             `json:"output_key"`
		Produced          float64            `json:"produced"`
		Consumed          map[string]float64 `json:"consumed"`
		GoodKey           string             `json:"good_key"`
		Quantity          float64            `json:"quantity"`
		Silver            float64            `json:"silver"`
		Kind              string             `json:"kind"`
		Count             int                `json:"count"`
		Level             int                `json:"level"`
		Q                 int                `json:"q"`
		R                 int                `json:"r"`
		TargetQ           int                `json:"target_q"`
		TargetR           int                `json:"target_r"`
		Workers           int                `json:"workers"`
		Lost              int                `json:"lost"`
		Disbanded         bool               `json:"disbanded"`
		LastSettlement    bool               `json:"last_settlement"`
		Reason            string             `json:"reason"`
		Verb              string             `json:"verb"`
		Outcome           string             `json:"outcome"`
		Role              string             `json:"role"`
		Note              string             `json:"note"`
		Status            string             `json:"status"`
		Stance            string             `json:"stance"`
		DestName          string             `json:"dest_name"`
		ForeignOwner      string             `json:"foreign_owner"`
		ForeignType       string             `json:"foreign_type"`
		Leg               string             `json:"leg"`
		Goods             []struct {
			GoodKey  string  `json:"good_key"`
			Quantity float64 `json:"quantity"`
		} `json:"goods"`
		Looted       map[string]float64 `json:"looted"`
		PerGood      map[string]float64 `json:"per_good"`
		FoodReleased float64            `json:"food_released"`
		CoverageDays float64            `json:"coverage_days"`
		GranaryEmpty bool               `json:"granary_empty"` // Historical only: legacy food-reserve payload.
		KnownHexes   *int               `json:"known_hexes"`
		UnknownHexes int                `json:"unknown_hexes"`
		PoseidonGift json.RawMessage    `json:"poseidon_gift"`
		Tier         string             `json:"tier"`
		NetPerTick   *float64           `json:"net_per_tick"`
		NetPerDay    float64            `json:"net_per_day"`
		TicksLeft    *float64           `json:"ticks_left"`
		DaysLeft     float64            `json:"days_left"`
		PopLoss      int                `json:"pop_loss"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return
	}
	subject := firstNotificationText(b.Name, b.UnitType, b.Type, "A unit")
	city := firstNotificationText(b.Name, "The city")
	at := fmt.Sprintf("(%d, %d)", b.Q, b.R)
	cargo := []string{}
	for _, g := range b.Goods {
		cargo = append(cargo, fmt.Sprintf("%s %s", notificationAmount(g.Quantity), g.GoodKey))
	}
	goods := strings.Join(cargo, ", ")
	reason := notificationWords(b.Reason)
	var line string
	switch n.Kind {
	case "OpeningDispatch":
		line = firstNotificationText(b.Title, "Opening dispatch")
	case "BuildComplete":
		what := notificationWords(b.BuildingType)
		if b.Temple != "" {
			what = b.Temple
		}
		line = fmt.Sprintf("%s completed %s", city, what)
		if b.Level > 0 {
			line += fmt.Sprintf(" (level %d)", b.Level)
		}
	case "GoodsCrafted":
		line = fmt.Sprintf("Cast %s %s", notificationAmount(b.Produced), b.OutputKey)
		if from := notificationGoodAmounts(b.Consumed); from != "" {
			line += " from " + from
		}
	case "TrainComplete":
		line = fmt.Sprintf("Training complete: %d %s", b.Count, notificationWords(b.UnitType))
	case "ArmyArrival":
		line = "Your army arrived"
		if b.Outcome != "" {
			line += " — " + notificationWords(b.Outcome)
		}
	case "ColonyFounded":
		line = "Colony founded: " + city
	case "MetropolisFounded":
		line = "Your metropolis is founded: " + city
		if b.KnownHexes != nil {
			line += fmt.Sprintf(" — %d/%d catchment hexes known", *b.KnownHexes, *b.KnownHexes+b.UnknownHexes)
		}
		if len(b.PoseidonGift) > 0 && string(b.PoseidonGift) != "false" && string(b.PoseidonGift) != "null" {
			line += " — Poseidon grants a galley"
		}
	case "OutpostEstablished":
		line = "Your outpost was established"
	case "OutpostCaptured":
		line = "Your army captured an enemy outpost"
	case "TradeDelivery", "TradeReturn":
		verb := "delivered"
		if n.Kind == "TradeReturn" {
			verb = "returned"
		}
		prefix := "Delivery"
		if n.Kind == "TradeDelivery" {
			verb = "arrived"
		}
		if n.Kind == "TradeReturn" {
			prefix = "Trade"
		}
		line = fmt.Sprintf("%s %s: %s %s", prefix, verb, notificationAmount(b.Quantity), b.GoodKey)
	case "TradeLost":
		line = "Your caravan was lost"
		if reason != "" {
			line += " — " + reason
		}
	case "OfferAccepted":
		line = fmt.Sprintf("Offer accepted: %s %s for %s silver", notificationAmount(b.Quantity), b.GoodKey, notificationAmount(b.Silver))
	case "OfferDeclined", "OfferExpired":
		verb := "declined"
		if n.Kind == "OfferExpired" {
			verb = "expired"
		}
		refund := fmt.Sprintf("%s %s", notificationAmount(b.Quantity), b.GoodKey)
		if b.Kind == "buy" {
			refund = notificationAmount(b.Silver) + " silver"
		}
		line = fmt.Sprintf("Offer %s — %s escrow back in your stores", verb, strings.TrimSpace(refund))
	case "UnitAttrition":
		line = fmt.Sprintf("%s starving — lost %d to hunger", subject, b.Lost)
		if b.Disbanded {
			category := "grain"
			if b.Reason == "food_shortage" {
				category = "food"
			}
			line = subject + " starved to nothing — no " + category
		}
	case "UnitDeserted":
		line = fmt.Sprintf("%s deserting — %d left unpaid", subject, b.Lost)
		if b.Disbanded {
			line = subject + " deserted — unpaid, unit lost"
		}
	case "UnitLostAtSea":
		cause := "the ship starved"
		if b.Reason == "silver_shortage" {
			cause = "the ship went unpaid"
		}
		line = fmt.Sprintf("%s lost at sea — %s, %d men gone", subject, cause, b.Lost)
	case "UnitArrived":
		line = fmt.Sprintf("%s arrived at %s — %s", subject, at, notificationWords(firstNotificationText(b.Status, "positioned")))
		if b.Stance != "" {
			line += ", standing " + notificationWords(b.Stance)
		}
	case "UnitRecalled", "UnitRedirected":
		verb := "recalled"
		if n.Kind == "UnitRedirected" {
			verb = "redirected"
		}
		line = fmt.Sprintf("%s %s — new course to (%d, %d)", subject, verb, b.TargetQ, b.TargetR)
	case "RoadEdgeBuilt":
		line = fmt.Sprintf("Road finished between (%d, %d) and (%d, %d) — everyone may use it", b.Q, b.R, b.TargetQ, b.TargetR)
	case "MarchStalled":
		line = firstNotificationText(reason, "A march could not be processed")
	case "OrderFailed":
		line = "Order failed"
		verbs := map[string]string{"march": "march", "stance": "stance", "stance_pursuit": "pursuit", "recall": "recall", "redirect": "redirect", "standing_orders": "standing orders", "occupy_action": "occupation", "build_road": "road"}
		if v := verbs[b.Verb]; v != "" {
			line += " (" + v + ")"
		}
		if b.Name != "" {
			line = b.Name + " — " + line
		}
		if reason != "" {
			line += ": " + reason
		}
	case "PickupWaiting":
		line = firstNotificationText(b.Note, "A ship waits off the shore for a unit it was sent to fetch")
	case "UnitFetched":
		line = firstNotificationText(b.Note, "A unit is aboard, sailing home")
	case "PickupTimedOut":
		line = firstNotificationText(b.Note, "A ship gave up waiting and is sailing home")
	case "ShipSweptFromSea":
		line = subject + " — " + firstNotificationText(reason, "ships at sea take no orders") + " — returning to port"
	case "ShipCaptured":
		line = "You captured an enemy ship at " + at + " — it is under way to your nearest port"
	case "ShipLost":
		line = "Your ship was captured at " + at + " and is gone"
	case "ShipStranded":
		line = "Your ship has nowhere left to dock and sits stranded at " + at + " — give it a new order"
	case "SentryAlerted":
		line = fmt.Sprintf("Sentry spotted %s's %s at %s", firstNotificationText(b.ForeignOwner, "an unknown Wanax"), firstNotificationText(b.ForeignType, "unit"), at)
	case "StandingOrderDispatched":
		leg := "setting out"
		if b.Leg == "return" {
			leg = "returning"
		}
		line = "Standing delivery " + leg
		if goods != "" {
			line += " — " + goods
		}
	case "StandingOrderPaused":
		line = "Standing delivery paused — " + firstNotificationText(reason, "reason unknown")
	case "TransferDelivered":
		line = "Transfer delivered to " + firstNotificationText(b.DestName, "destination")
		if goods != "" {
			line += ": " + goods
		}
	// Historical only: render archived food-reserve notifications unchanged.
	case "SitosIntervention":
		line = "Food support was given to the city"
	case "SitosFundLow":
		line = "The city's food reserve needs attention"
	case "SitosGranaryRelease":
		released := notificationGoodAmounts(b.PerGood)
		if released == "" {
			released = notificationAmount(b.FoodReleased) + " food"
		}
		line = fmt.Sprintf("Granary released %s (%.1f game days of coverage)", released, b.CoverageDays)
		if b.GranaryEmpty {
			line += " — granary now empty"
		}
	case "SubsistenceWarning":
		net := b.NetPerDay
		if b.NetPerTick != nil {
			net = *b.NetPerTick
		}
		left := b.DaysLeft
		if b.TicksLeft != nil {
			left = *b.TicksLeft
		}
		if b.Tier == "critical" {
			line = fmt.Sprintf("%s is STARVING — %s lost. Grain net %.0f per game day.", city, citizenCount(b.PopLoss), net)
		} else {
			line = fmt.Sprintf("%s: grain net %+.0f per game day", city, net)
			if left > 0 {
				line += fmt.Sprintf(" — grain lasts ~%.0f game days", left)
			}
		}
	case "HexBlockaded":
		line = fmt.Sprintf("%s's hex %s is blockaded — %d workers stopped", city, at, b.Workers)
	case "HexUnblockaded":
		line = fmt.Sprintf("%s's hex %s is free again — %d workers resumed", city, at, b.Workers)
	case "CityCollapsed":
		line = city + " has collapsed"
		if b.LastSettlement {
			line += " — that was your last settlement"
		}
	case "SettlementCaptured":
		line = "One of your settlements has fallen to conquest"
		if b.Role == "attacker" {
			line = "Your army has taken an enemy settlement"
		}
	case "SettlementDefended":
		line = "Your settlement held — the attack was repelled"
		if b.Role == "attacker" {
			line = "Your assault failed — the attack was repelled"
		}
	case "SettlementSacked":
		line = "Your settlement was sacked and razed"
		if b.Role == "attacker" {
			line = "You sacked and razed a settlement"
			if loot := notificationGoodAmounts(b.Looted); loot != "" {
				line += " — looted " + loot
			}
		}
	default:
		return
	}
	switch n.Kind {
	case "UnitRecalled", "UnitRedirected", "ShipSweptFromSea":
		if b.ArrivesAt != "" {
			line += ", arrives " + arrivalETA(c, b.ArrivesAt)
		}
	case "OfferAccepted":
		at, day, leg := b.GoodsArrivesAt, b.GoodsArrivalTick, "goods"
		if b.Kind == "sell" {
			at, day, leg = b.SilverArrivesAt, b.SilverArrivalTick, "silver"
		}
		if day != nil {
			line += " — " + leg + " arrives " + formatWorldTime(c, *day)
		} else if at != "" {
			line += " — " + leg + " arrives " + arrivalETA(c, at)
		}
	}
	fmt.Println("      " + line)
}

func firstNotificationText(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func notificationWords(s string) string { return strings.ReplaceAll(s, "_", " ") }
func notificationGoodAmounts(goods map[string]float64) string {
	keys := make([]string, 0, len(goods))
	for k := range goods {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, notificationAmount(goods[k])+" "+k)
	}
	return strings.Join(parts, ", ")
}

// Resource's whole-unit display must not turn a real small transfer into zero.
func notificationAmount(n float64) string {
	if n != 0 && n > -1 && n < 1 {
		return fmt.Sprintf("%.3g", n)
	}
	return resource(n)
}

// Letter and trade-offer notices name the Wanax to answer (byggplan A.1).
// Rows written before the id existed print nothing extra.
func printLetterReplyHint(n notificationItem) {
	switch n.Kind {
	case "MessengerArrival", "MessengerReturned", "OfferAccepted", "OfferDeclined":
	default:
		return
	}
	var b struct {
		Sender      string `json:"sender_id"`
		Counterpart string `json:"counterpart_wanax_id"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return
	}
	id := b.Sender
	if id == "" {
		id = b.Counterpart
	}
	if id != "" {
		fmt.Printf("      Read and reply: keryx letters %s\n", id)
	}
}
