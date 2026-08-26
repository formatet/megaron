package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// noisyNotificationKinds are the notification kinds collapsed to a summary
// by default (both the default-view exclusion and the ×N grouping below) —
// an explicit allowlist, not "anything repetitive", so a future high-signal
// kind (e.g. SubsistenceWarning) is never silently swallowed just because
// it happens to fire often. Today: only Sitos' routine noise (~99% of the
// feed — see megaron_ekonomi_legibilitet_plan.md DEL B).
var noisyNotificationKinds = []string{"SitosIntervention", "SitosFundLow"}

// subsistenceWarningKind is the own-city grain-subsistence warning (DEL D,
// megaron_ekonomi_legibilitet_plan.md). It is deliberately NOT in
// noisyNotificationKinds — it must never be collapsed or hidden, and its
// critical tier floats to the very top of the feed (see notificationsCmd).
const subsistenceWarningKind = "SubsistenceWarning"

// subsistenceTier extracts the tier ("yellow"/"red"/"critical") from a
// SubsistenceWarning's body; "" for anything else or an unparseable body.
func subsistenceTier(n notificationItem) string {
	if n.Kind != subsistenceWarningKind || len(n.Body) == 0 {
		return ""
	}
	var b struct {
		Tier string `json:"tier"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return ""
	}
	return b.Tier
}

// subsistenceTierLabel renders the tier marker shown in the keryx feed.
func subsistenceTierLabel(tier string) string {
	switch tier {
	case "critical":
		return "KRITISK"
	case "red":
		return "röd"
	case "yellow":
		return "gul"
	default:
		return tier
	}
}

type notificationItem struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Level     int             `json:"level"`
	Body      json.RawMessage `json:"body"`
	CreatedAt string          `json:"created_at"`
	ReadAt    *string         `json:"read_at"`
}

func isNoisyNotificationKind(kind string) bool {
	for _, k := range noisyNotificationKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func notificationAge(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	ago := time.Since(t)
	switch {
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	case ago < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(ago.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(ago.Hours()/24))
	}
}

func printNotificationRow(c *Client, n notificationItem) {
	marker := " "
	if n.ReadAt == nil {
		marker = "*"
	}
	kind := n.Kind
	if tier := subsistenceTier(n); tier != "" {
		kind = n.Kind + " [" + subsistenceTierLabel(tier) + "]"
	}
	fmt.Printf("%s[%s]  %-20s  %s\n", marker, notificationAge(n.CreatedAt), kind, string(n.Body))
	printNotificationDetail(c, n)
}

// printNotificationDetail prints the kind-specific actionable follow-up
// line(s) for a notification's body — broken out of printNotificationRow so
// `keryx watch` (cmd_watch.go) can render the exact same detail line for a
// live push message, which carries a kind+payload but none of the
// listing-only fields (id, created_at, read_at). Two renderers of the same
// notification kind is exactly the fifth-copy fate megaron_plan_keryx_strom.md
// §3 point 3 calls out — this is the single shared function both call.
// Takes c only to hand to printExploreReturnedLine's game-day ETA (rad K) —
// every other line below is pure formatting of the notification body.
func printNotificationDetail(c *Client, n notificationItem) {
	if n.Kind == "ColonyFounded" {
		printColonyFoundedGrainLine(n)
	}
	if n.Kind == "ScoutReport" {
		printScoutReportLine(n)
	}
	if n.Kind == "UnitExploreReturned" {
		printExploreReturnedLine(c, n)
	}
	if n.Kind == "UpkeepUnpaid" {
		printUpkeepUnpaidLine(n)
	}
	if n.Kind == "BattleWon" || n.Kind == "BattleLost" {
		printBattleReportLine(n)
	}
	if n.Kind == "ShipDamaged" {
		printShipDamagedLine(n)
	}
	if n.Kind == "ShipRepaired" {
		printShipRepairedLine(n)
	}
	if n.Kind == "ForeignMarchSighted" {
		printForeignMarchSightedLine(n)
	}
	if n.Kind == "DivinePunishment" {
		printDivinePunishmentLine(n)
	}
	switch n.Kind {
	case "CityOccupied", "OccupationDefended", "CityAnnexReady", "SettlementLooted", "SettlementBurned":
		printOccupationLine(n)
	}
}

// printForeignMarchSightedLine renders the human-readable follow-up to a
// ForeignMarchSighted notification — the notice that starts the clock in the
// asynchronicity gate. The three facts a Wanax must be able to act on are WHO,
// WHERE IT IS GOING and WHEN IT LANDS, and the landing is given as a TICK because
// that is the unit the player plans in (speldygn, never wall clock).
//
// The threatened-city clause is the whole point of the urgent level: "something is
// moving out there" and "an army is walking into Pylos" are the two ends of the
// irreversibility gradient (Timothy 2026-08-03) and must not read alike.
func printForeignMarchSightedLine(n notificationItem) {
	var body struct {
		Owner         string `json:"owner"`
		UnitType      string `json:"unit_type"`
		Size          int    `json:"size"`
		Stance        string `json:"stance"`
		TargetQ       int    `json:"target_q"`
		TargetR       int    `json:"target_r"`
		ArriveTick    int    `json:"arrive_tick"`
		ThreatensName string `json:"threatens_name"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	owner := body.Owner
	if owner == "" {
		owner = "An unknown Wanax"
	} else {
		owner += "'"
		if !strings.HasSuffix(body.Owner, "s") {
			owner += "s"
		}
	}
	unitType := body.UnitType
	if unitType == "" {
		unitType = "force"
	}
	stance := ""
	if body.Stance != "" {
		stance = ", " + body.Stance
	}
	threat := ""
	if body.ThreatensName != "" {
		threat = " — YOUR CITY " + body.ThreatensName
	}
	fmt.Printf("      %s %s (%d%s) marching to (%d,%d)%s — lands tick %d\n",
		owner, unitType, body.Size, stance, body.TargetQ, body.TargetR, threat, body.ArriveTick)
}

// printOccupationLine renders the erövring notification family
// (megaron_plan_erovring.md S6): a city falling under occupation, its
// annex countdown maturing or resetting, or the outcome of a sack/burn
// choice — the raw JSON body above turned into an actionable line, same
// pattern as printBattleReportLine.
func printOccupationLine(n notificationItem) {
	var body struct {
		SettlementID           string `json:"settlement_id"`
		Name                   string `json:"name"`
		Role                   string `json:"role"`
		OccupationTicksToAnnex int    `json:"occupation_ticks_to_annex"`
		PopLost                int    `json:"pop_lost"`
		BuildingHit            string `json:"building_hit"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	name := body.Name
	if name == "" {
		name = body.SettlementID
	}
	switch n.Kind {
	case "CityOccupied":
		if body.Role == "attacker" {
			fmt.Printf("      %s föll — din här håller den under ockupation. Väljer du inget stannar den ockuperad; annektera erbjuds efter %d obestridda tick. `keryx occupation order --settlement %s --action sack|burn|annex`\n",
				name, body.OccupationTicksToAnnex, body.SettlementID)
		} else {
			fmt.Printf("      %s har fallit under ockupation — inte förlorad än. Undsättning inom %d tick nollar fiendens räknare.\n",
				name, body.OccupationTicksToAnnex)
		}
	case "OccupationDefended":
		fmt.Printf("      Ockupationen av %s höll mot ett angrepp — annekteringsräknaren har nollställts, %d nya tick krävs.\n",
			name, body.OccupationTicksToAnnex)
	case "CityAnnexReady":
		fmt.Printf("      %s har varit obestridd länge nog — annektera med `keryx occupation order --settlement %s --action annex`\n",
			name, body.SettlementID)
	case "SettlementLooted":
		if body.Role == "attacker" {
			fmt.Printf("      %s sackad — bytet är på väg hem som en karavan (kan avskäras).\n", name)
		} else {
			extra := ""
			if body.BuildingHit != "" {
				extra = fmt.Sprintf(", %s nedslagen en nivå", body.BuildingHit)
			}
			fmt.Printf("      %s sackad — befolkning −⅓%s. Staden är kvar, men länsad.\n", name, extra)
		}
	case "SettlementBurned":
		if body.Role == "attacker" {
			fmt.Printf("      %s sackad och bränd — bytet är på väg hem, ruinen kan inte återkoloniseras på en tid.\n", name)
		} else {
			fmt.Printf("      %s sackad och bränd — staden är en ruin.\n", name)
		}
	}
}

// printBattleReportLine renders the human-readable follow-up to a
// BattleWon/BattleLost notification (megaron_plan_stridsrapport.md §4/S4) —
// BattleTickHandler.notifyBattleEnded's payload. Both sides of a KR3 battle
// get one of these; before this slice the defender got nothing at all and
// the attacker only got raw {q,r}.
func printBattleReportLine(n notificationItem) {
	var body struct {
		Role     string `json:"role"`
		Outcome  string `json:"outcome"`
		Opponent string `json:"opponent_name"`
		OwnUnit  struct {
			Type       string `json:"type"`
			SizeBefore int    `json:"size_before"`
			SizeAfter  int    `json:"size_after"`
			PopLost    int    `json:"pop_lost"`
		} `json:"own_unit"`
		EnemyUnit struct {
			Type       string `json:"type"`
			SizeBefore int    `json:"size_before"`
			SizeAfter  int    `json:"size_after"`
		} `json:"enemy_unit"`
		Q     int     `json:"q"`
		R     int     `json:"r"`
		Place *string `json:"place"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	place := fmt.Sprintf("(%d,%d)", body.Q, body.R)
	if body.Place != nil && *body.Place != "" {
		place = *body.Place
	}
	outcomeWord := "Seger"
	if n.Kind == "BattleLost" {
		outcomeWord = "Förlust"
	}
	trailer := ""
	switch {
	case body.Outcome == "mutual_wipe":
		trailer = " Inga överlevande på endera sidan."
	case body.Outcome == "attacker_wins" && body.Role == "attacker":
		trailer = " Fältet togs."
	case body.Outcome == "attacker_wins" && body.Role == "defender":
		trailer = " Fältet föll."
	case body.Outcome == "defender_holds" && body.Role == "defender":
		trailer = " Fältet höll."
	case body.Outcome == "defender_holds" && body.Role == "attacker":
		trailer = " Anfallet slogs tillbaka."
	}
	fmt.Printf("      %s vid %s — din %s (%d→%d, −%d döda) mot %s's %s (%d→%d).%s\n",
		outcomeWord, place, body.OwnUnit.Type, body.OwnUnit.SizeBefore, body.OwnUnit.SizeAfter, body.OwnUnit.PopLost,
		body.Opponent, body.EnemyUnit.Type, body.EnemyUnit.SizeBefore, body.EnemyUnit.SizeAfter, trailer)
}

// printShipDamagedLine renders the human-readable follow-up to a ShipDamaged
// notification (megaron_plan_skeppsreparation.md Slice B point 6) —
// BattleTickHandler.notifyShipDamaged's payload.
func printShipDamagedLine(n notificationItem) {
	var body struct {
		UnitType      string `json:"unit_type"`
		Hull          int    `json:"hull"`
		HullMax       int    `json:"hull_max"`
		Sunk          bool   `json:"sunk"`
		ReturningHome bool   `json:"returning_home"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	switch {
	case body.Sunk:
		fmt.Printf("      Din %s sänktes i striden.\n", body.UnitType)
	case body.ReturningHome:
		fmt.Printf("      Din %s tog skada (skrov %d/%d) och linkar hem för reparation.\n",
			body.UnitType, body.Hull, body.HullMax)
	default:
		fmt.Printf("      Din %s tog skada (skrov %d/%d) men behåller sina order.\n",
			body.UnitType, body.Hull, body.HullMax)
	}
}

// printShipRepairedLine renders the human-readable follow-up to a
// ShipRepaired notification (megaron_plan_skeppsreparation.md Slice C point
// 4) — ShipRepairCompleteHandler's payload.
func printShipRepairedLine(n notificationItem) {
	var body struct {
		UnitType string `json:"unit_type"`
		Hull     int    `json:"hull"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	fmt.Printf("      Din %s är reparerad (skrov %d/5) och redo att segla igen.\n", body.UnitType, body.Hull)
}

// printUpkeepUnpaidLine renders the human-readable follow-up to an UpkeepUnpaid
// notification (SLICE A, megaron_todo.md 2026-07-31): the forewarning fired the
// period a unit's silver upkeep goes unpaid, before desertion actually starts —
// previously recordUnpaid's else-branch bumped unpaid_periods with zero signal
// until desertion itself fired. Without this, the raw JSON body above is all a
// Wanax would see; this turns it into an actionable line.
func printUpkeepUnpaidLine(n notificationItem) {
	var body struct {
		UnitType              string  `json:"unit_type"`
		UnpaidPeriods         int     `json:"unpaid_periods"`
		PeriodsUntilDesertion int     `json:"periods_until_desertion"`
		SilverUnpaid          float64 `json:"silver_unpaid"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	unitType := body.UnitType
	if unitType == "" {
		unitType = "unit"
	}
	urgency := fmt.Sprintf("%d periods left before desertion", body.PeriodsUntilDesertion)
	if body.PeriodsUntilDesertion == 1 {
		urgency = "ONE more unpaid period and they desert"
	}
	fmt.Printf("      %s unpaid (period %d) — %.0f silver short — %s\n",
		unitType, body.UnpaidPeriods, body.SilverUnpaid, urgency)
}

// printScoutReportLine renders the human-readable follow-up to a ScoutReport
// notification (temenos_todo.md "Explore-order kommer hem utan rapport"): the
// raw JSON body above already carries q/r/terrain/deposits, but a Wanax
// shouldn't have to parse JSON to learn what a scout found. "Nothing of
// value" is the common case and must read as a clean report, not a blank one.
func printScoutReportLine(n notificationItem) {
	var body struct {
		Q             int    `json:"q"`
		R             int    `json:"r"`
		Terrain       string `json:"terrain"`
		CopperDeposit bool   `json:"copper_deposit"`
		TinDeposit    bool   `json:"tin_deposit"`
		SilverDeposit bool   `json:"silver_deposit"`
		CedarDeposit  bool   `json:"cedar_deposit"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	var deposits []string
	if body.CopperDeposit {
		deposits = append(deposits, "copper")
	}
	if body.TinDeposit {
		deposits = append(deposits, "tin")
	}
	if body.SilverDeposit {
		deposits = append(deposits, "silver")
	}
	if body.CedarDeposit {
		deposits = append(deposits, "cedar")
	}
	found := "nothing of value"
	if len(deposits) > 0 {
		found = strings.Join(deposits, ", ")
	}
	fmt.Printf("      explored (%d,%d) — %s, %s\n", body.Q, body.R, body.Terrain, found)
}

// printExploreReturnedLine renders the human-readable follow-up to a
// UnitExploreReturned notification (Rad G, megaron_plan_cli_sanning.md). The
// bare kind name printNotificationRow already prints reads as "the scout
// is home" — it isn't: EventUnitExploreReturned fires the moment the unit
// reaches its explore target and TURNS for home (internal/unit/events.go's
// doc comment on the const; dispatchReturnHome in unit_arrival.go). The
// event type and its payload are untouched — arrives_at was always the
// return leg's ETA, this only makes that legible. Mirrors web's notifText
// 'UnitExploreReturned' case (web/static/js/megaron/ui/format.js) so both
// surfaces say the same thing.
func printExploreReturnedLine(c *Client, n notificationItem) {
	var body struct {
		Q         int    `json:"q"`
		R         int    `json:"r"`
		ArrivesAt string `json:"arrives_at"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	eta := ""
	if t, err := time.Parse(time.RFC3339, body.ArrivesAt); err == nil {
		eta = ", home " + gameETA(c, t)
	}
	fmt.Printf("      scout reached (%d,%d) and turned for home%s\n", body.Q, body.R, eta)
}

// printColonyFoundedGrainLine renders the founding grain balance carried in a
// ColonyFounded notification (DEL B, megaron_koloni_legibilitet_plan.md). A colony
// does NOT feed itself automatically, so a negative net grain rate at founding is
// surfaced immediately — in the Lawagetas voice, per tick — with how long the
// seed lasts and the two remedies (build a farm if the catchment bears it, else
// send grain by internal transfer). A self-sustaining colony gets one short
// positive line. Additive/back-compatible: an older ColonyFounded body without the
// grain_* fields prints nothing extra.
func printColonyFoundedGrainLine(n notificationItem) {
	var body struct {
		Name            string   `json:"name"`
		GrainAmount     *float64 `json:"grain_amount"`
		GrainNetPerTick *float64 `json:"grain_net_per_tick"`
		GrainTicks      *float64 `json:"grain_ticks"`
		// GrainDays is the pre-rename key (2026-08-06): the server keeps writing
		// it alongside grain_ticks so old persisted notifications stay readable.
		// Fall back to it only when grain_ticks is absent.
		GrainDays *float64 `json:"grain_days"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil || body.GrainNetPerTick == nil {
		return
	}
	name := body.Name
	if name == "" {
		name = "Kolonin"
	}
	grainTicks := body.GrainTicks
	if grainTicks == nil {
		grainTicks = body.GrainDays
	}
	perTick := *body.GrainNetPerTick
	if perTick < 0 {
		ticks := ""
		if grainTicks != nil {
			ticks = fmt.Sprintf(" — grain räcker ~%.0f tick", *grainTicks)
		}
		fmt.Printf("      %s föder inte sig själv (~%.0f grain/tick i underskott)%s. Bygg farm om catchment bär det, annars sänd grain: keryx transfer --good grain --qty <n> --dest %s\n",
			name, -perTick, ticks, name)
	} else {
		fmt.Printf("      %s försörjer sig själv (~%+.0f grain/tick).\n", name, perTick)
	}
}

// notificationsCmd surfaces the persistent notifications feed (server since
// mig 045/06-10) — previously invisible in keryx entirely: arrivals, colony
// foundings, build/train completions, trade events etc. fired server-side
// with nowhere to see them in the CLI (Fas 2h/keryx-surface rule: everything
// in temenos must be visible AND actionable in keryx).
//
// Fas 2026-07-12 (DEL B, megaron_ekonomi_legibilitet_plan.md): the default
// view was ~99% SitosIntervention noise burying real events (TradeDelivery,
// UnitArrived, ...). Default now excludes noisyNotificationKinds and prints
// a one-line pointer to them; --kind/--exclude give explicit control.
func notificationsCmd() *cobra.Command {
	var unreadOnly, markRead bool
	var kindFilter, excludeFilter string
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Show your notification feed (arrivals, completions, trade events, ...)",
		Example: `  keryx notifications
  keryx notifications --unread
  keryx notifications --kind SitosIntervention
  keryx notifications --exclude SitosIntervention
  keryx notifications --mark-read`,
		// --kind and --exclude are both plausible for a stray positional —
		// no single one is the obvious guess.
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			if markRead {
				if _, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/notifications/read-all", cfg.WorldID), nil); err != nil {
					return err
				}
				fmt.Println("All notifications marked read.")
				return nil
			}

			basePath := fmt.Sprintf("/api/v1/worlds/%s/notifications", cfg.WorldID)

			// Default (no --kind/--exclude given): hide the noisy kinds so
			// they don't crowd real signal out of the server's LIMIT 100
			// window. Explicit --kind/--exclude always win outright.
			usingDefaultNoiseFilter := kindFilter == "" && excludeFilter == ""

			params := url.Values{}
			if unreadOnly {
				params.Set("unread", "true")
			}
			switch {
			case kindFilter != "":
				params.Set("kind", kindFilter)
			case excludeFilter != "":
				params.Set("exclude", excludeFilter)
			default:
				params.Set("exclude", strings.Join(noisyNotificationKinds, ","))
			}

			path := basePath
			if enc := params.Encode(); enc != "" {
				path += "?" + enc
			}
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Notifications []notificationItem `json:"notifications"`
				Unread        int                `json:"unread"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}

			// Best-effort count of what the default filter hid, for the
			// "+N ... --kind X för alla" pointer. Skipped entirely when the
			// caller already asked for a specific --kind/--exclude.
			hiddenCounts := map[string]int{}
			if usingDefaultNoiseFilter {
				for _, kind := range noisyNotificationKinds {
					countParams := url.Values{}
					if unreadOnly {
						countParams.Set("unread", "true")
					}
					countParams.Set("kind", kind)
					countData, err := c.get(basePath + "?" + countParams.Encode())
					if err != nil {
						continue
					}
					var countResp struct {
						Notifications []json.RawMessage `json:"notifications"`
					}
					if json.Unmarshal(countData, &countResp) == nil {
						hiddenCounts[kind] = len(countResp.Notifications)
					}
				}
			}

			if len(resp.Notifications) == 0 && len(hiddenCounts) == 0 {
				fmt.Println("No notifications.")
				return nil
			}

			fmt.Printf("%d notification(s), %d unread\n", len(resp.Notifications), resp.Unread)
			fmt.Println("────────────────────────────────────────────────────────────")

			// Critical SubsistenceWarnings float to the very top (DEL D,
			// Sparta-forensiken): a starving capital must never scroll past.
			for _, n := range resp.Notifications {
				if n.Kind == subsistenceWarningKind && subsistenceTier(n) == "critical" {
					printNotificationRow(c, n)
				}
			}

			// Non-noisy notifications shown in full, at the top, unabridged.
			grouped := map[string][]notificationItem{}
			for _, n := range resp.Notifications {
				if isNoisyNotificationKind(n.Kind) {
					grouped[n.Kind] = append(grouped[n.Kind], n)
					continue
				}
				if n.Kind == subsistenceWarningKind && subsistenceTier(n) == "critical" {
					continue // already printed at the very top
				}
				printNotificationRow(c, n)
			}

			// Noisy kinds present in this response (e.g. explicit --kind
			// SitosIntervention) collapse to one "×N" line instead of
			// flooding the terminal.
			for _, kind := range noisyNotificationKinds {
				occ := grouped[kind]
				if len(occ) == 0 {
					continue
				}
				latest := occ[0] // server orders created_at DESC
				marker := " "
				for _, n := range occ {
					if n.ReadAt == nil {
						marker = "*"
						break
					}
				}
				fmt.Printf("%s[%s]  %-20s  ×%d (senaste: %s)\n", marker, notificationAge(latest.CreatedAt), kind, len(occ), string(latest.Body))
			}

			// Default-view summary for kinds excluded from the fetch entirely.
			for _, kind := range noisyNotificationKinds {
				if n := hiddenCounts[kind]; n > 0 {
					fmt.Printf("+%d %s — `keryx notifications --kind %s` för alla\n", n, kind, kind)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&unreadOnly, "unread", false, "show only unread notifications")
	cmd.Flags().BoolVar(&markRead, "mark-read", false, "mark all notifications as read and exit")
	cmd.Flags().StringVar(&kindFilter, "kind", "", "only show these notification kinds (comma-separated, e.g. SitosIntervention)")
	cmd.Flags().StringVar(&excludeFilter, "exclude", "", "hide these notification kinds (comma-separated)")
	return cmd
}

// printDivinePunishmentLine renders what the gods actually took.
//
// P4 (megaron_plan_tysta_forluster hål 1) gave the event and the notification a
// real `amount` — before that the mutation ran through Exec and the result was
// thrown away, so no notification was sent at all. But the notification landed
// here as bare JSON, so what a Wanax saw for an involuntary loss of a fifth of
// their garrison was {"type":"garrison_plague","amount":20,...} (acceptance
// sweep 2026-08-24). The plan's own acceptance criterion was a notice that
// NAMES what was taken and how much; this is that notice.
//
// Unknown types fall through to the raw JSON above rather than inventing a
// sentence — a new punishment must be added here deliberately, not described
// wrongly by a default.
func printDivinePunishmentLine(n notificationItem) {
	var body struct {
		Type   string  `json:"type"`
		Amount float64 `json:"amount"`
	}
	if err := json.Unmarshal(n.Body, &body); err != nil {
		return
	}
	amount := body.Amount
	if amount <= 0 {
		return
	}
	var line string
	switch body.Type {
	case "chariot_loss":
		line = fmt.Sprintf("Gudarna skingrade dina stridsvagnar i natten — %.0f man borta.", amount)
	case "ship_loss":
		line = fmt.Sprintf("En gudasänd storm tog %.0f fartyg ur din hamn.", amount)
	case "harvest_failure":
		line = fmt.Sprintf("Fälten låg i träda på gudarnas vilja — %s spannmål ruttnade.", resource(amount))
	case "garrison_plague":
		line = fmt.Sprintf("En pest gick genom baracken — %.0f man föll.", amount)
	default:
		return
	}
	fmt.Printf("      %s\n", line)
}
