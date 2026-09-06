package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"formatet/megaron/server/api/handlers"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/chronicle"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/loyalty"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/world"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dbURL := mustEnv("DATABASE_URL")
	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		slog.Error("parse database config", "err", err)
		os.Exit(1)
	}
	// pgx defaults MaxConns to max(4, nproc) = 4 on CT 126 — the fan-out
	// bottleneck: a burst of WS events queues behind 4 connections and trips the
	// 5 s handler timeouts on /units, /provinces (UnitArrival). Lift the ceiling;
	// Postgres max_connections default 100 leaves ample headroom.
	poolCfg.MaxConns = 16
	poolCfg.MinConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		slog.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := runMigrations(dbURL); err != nil {
		slog.Error("run migrations", "err", err)
		os.Exit(1)
	}

	redisURL := mustEnv("REDIS_URL")
	rdb := redis.NewClient(&redis.Options{Addr: redisURL})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("connect to redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	jwtSecret := mustEnv("JWT_SECRET")
	authSvc := auth.NewService(pool, jwtSecret)

	hub := notify.New()
	hub.SetPool(pool)

	// GameClock — single source of time for all game logic.
	// On startup, check for downtime since last heartbeat and absorb it.
	gameClock := clock.NewWallClock()
	absorbStartupDowntime(ctx, pool, gameClock)
	go runHeartbeat(ctx, pool)

	// Retention for the three operational tables that grow without bound and
	// are not cleaned by a reseed (see runRetention's doc comment). Config is
	// loaded eagerly, like ensureWorld's envMapDim, so a malformed *_RETENTION
	// value fails the boot instead of silently running with a bad window.
	retentionCfg, err := loadRetentionConfig()
	if err != nil {
		slog.Error("load retention config", "err", err)
		os.Exit(1)
	}
	go runRetention(ctx, pool, retentionCfg)

	serverWorldID, err := ensureWorld(ctx, pool, gameClock)
	if err != nil {
		slog.Error("ensure world", "err", err)
		os.Exit(1)
	}
	slog.Info("world ready", "id", serverWorldID)

	// Chronicle: append-only world log + daily prose Markdown.
	// Disabled when CHRONICLE_DIR is empty.
	chronicler, err := chronicle.Open(ctx, getEnv("CHRONICLE_DIR", "/var/lib/poleia/chronicles"), pool, serverWorldID)
	if err != nil {
		slog.Error("open chronicle", "err", err)
		os.Exit(1)
	}
	defer chronicler.Close()

	// Event worker — processes timed game events.
	eventStore := events.NewStore(pool, chronicler)
	scheduler := events.NewScheduler(pool, gameClock)
	worker := events.NewWorker(pool, gameClock)
	// Poll at least as often as ticks advance so due events don't wait >1 tick.
	// On the production cadence (minutes/tick) the 10 s default already wins; only
	// a sub-minute TICK_SECONDS dev cadence tightens it. (Set here, not in
	// internal/events, to avoid the events→tick import cycle.)
	if tickDur := time.Duration(tick.TickSeconds) * time.Second; tickDur < 10*time.Second {
		worker.SetPollInterval(tickDur)
	}
	buildH := combat.NewBuildCompleteHandler(pool, eventStore, hub)
	trainH := combat.NewTrainCompleteHandler(pool, eventStore, hub)
	shipRepairH := combat.NewShipRepairCompleteHandler(pool, eventStore, hub)
	decayH := loyalty.NewDecayHandler(pool, scheduler, eventStore)
	welfareH := loyalty.NewWelfareHandler(pool, scheduler, eventStore)
	colonyH := loyalty.NewColonyPenaltyHandler(pool, scheduler, eventStore)
	borrowedH := loyalty.NewBorrowedArmyPenaltyHandler(pool, scheduler, eventStore, gameClock)
	messengerArrivalH := messenger.NewArrivalHandler(pool, scheduler, eventStore)
	messengerReturnH := messenger.NewReturnHandler(pool, eventStore)
	kharisH := kharis.NewTickHandler(pool, scheduler, eventStore, hub)
	sitosCfg := economy.LoadSitosConfig()
	sitosH := economy.NewSitosTickHandler(pool, scheduler, eventStore, hub, sitosCfg)
	foodTickH := economy.NewFoodTickHandler(pool, scheduler, eventStore, hub)
	tradeH := economy.NewDeliveryHandler(pool, eventStore, hub, scheduler)
	tradeReturnH := economy.NewTradeReturnHandler(pool, eventStore, hub)
	recallH := messenger.NewRecallArrivalHandler(pool, scheduler, hub, gameClock)
	marchRecallH := messenger.NewMarchRecallHandler(pool, scheduler, eventStore, hub, gameClock)
	orderDeliveryH := messenger.NewOrderDeliveryHandler(pool, scheduler, eventStore, hub, gameClock)
	worker.Register(events.ScheduledBuildComplete, buildH.Handle)
	worker.Register(events.ScheduledTrainComplete, trainH.Handle)
	worker.Register(events.ScheduledShipRepairComplete, shipRepairH.Handle)
	worker.Register(events.ScheduledLoyaltyDecayTick, decayH.Handle)
	worker.Register(events.ScheduledLoyaltyWelfareTick, welfareH.Handle)
	worker.Register(events.ScheduledColonyPenaltyTick, colonyH.Handle)
	worker.Register(events.ScheduledBorrowedArmyTick, borrowedH.Handle)
	worker.Register(events.ScheduledOrderDelivery, orderDeliveryH.Handle)
	// P3 soak fix (2026-07-19): notify the owner if a march order's courier
	// delivery keeps failing until dead-lettered, instead of it silently
	// vanishing after the dispatch's 202 promised it was en route.
	worker.RegisterDeadLetterHook(events.ScheduledOrderDelivery, orderDeliveryH.NotifyDeadLetter)
	worker.Register(events.ScheduledMessengerArrival, messengerArrivalH.Handle)
	worker.Register(events.ScheduledMessengerReturn, messengerReturnH.Handle)
	worker.Register(events.ScheduledKharisTick, kharisH.Handle)
	worker.Register(events.ScheduledSitosTick, sitosH.Handle)
	worker.Register(events.ScheduledTradeDelivery, tradeH.Handle)
	worker.Register(events.ScheduledTradeReturn, tradeReturnH.Handle)
	worker.Register(events.ScheduledRecallArrival, recallH.Handle)
	worker.Register(events.ScheduledMarchRecall, marchRecallH.Handle)
	logisticsH := handlers.NewLogisticsArrivalHandler(pool)
	worker.Register(events.ScheduledLogisticsArrival, logisticsH.Handle)
	transportH := transport.NewArrivalHandler(pool, hub)
	worker.Register(events.ScheduledTransportArrival, transportH.Handle)
	interceptH := transport.NewInterceptScanHandler(pool, scheduler, eventStore, hub, gameClock)
	worker.Register(events.ScheduledInterceptScan, interceptH.Handle)
	unitInterceptH := combat.NewUnitInterceptScanHandler(pool, scheduler, eventStore, gameClock, hub)
	worker.Register(events.ScheduledUnitInterceptScan, unitInterceptH.Handle)
	marchSightH := combat.NewMarchSightingHandler(pool, scheduler, hub, gameClock)
	worker.Register(events.ScheduledMarchSightingScan, marchSightH.Handle)
	marchEncounterH := combat.NewMarchEncounterHandler(pool, scheduler, eventStore, gameClock, hub)
	worker.Register(events.ScheduledMarchEncounterScan, marchEncounterH.Handle)
	unitArrivalH := combat.NewUnitArrivalHandler(pool, eventStore, hub, scheduler, gameClock, sitosCfg)
	worker.Register(events.ScheduledUnitArrival, unitArrivalH.Handle)
	// P3 soak fix (2026-07-19): same reasoning as the order-delivery hook above —
	// a marching unit's arrival that dead-letters must tell its owner, not just
	// stay silently "marching" forever with no player-facing signal.
	worker.RegisterDeadLetterHook(events.ScheduledUnitArrival, unitArrivalH.NotifyDeadLetter)
	worker.Register(events.ScheduledSentryReturn, unitArrivalH.HandleSentryReturn)
	battleTickH := combat.NewBattleTickHandler(pool, eventStore, scheduler, hub, gameClock)
	worker.Register(events.ScheduledBattleTick, battleTickH.Handle)
	occupationCheckH := combat.NewOccupationCheckHandler(pool, scheduler, hub)
	worker.Register(events.ScheduledOccupationCheck, occupationCheckH.Handle)
	siegeCapitulationH := combat.NewSiegeCapitulationHandler(pool, eventStore, scheduler, hub)
	worker.Register(events.ScheduledSiegeCapitulation, siegeCapitulationH.Handle)
	collapseH := combat.NewCollapseSettlementHandler(pool, eventStore, scheduler, hub)
	worker.Register(events.ScheduledCollapseSettlement, collapseH.Handle)
	upkeepH := combat.NewUpkeepHandler(pool, scheduler, eventStore, hub, unitArrivalH)
	worker.Register(events.ScheduledUpkeepTick, upkeepH.Handle)
	standingOrderH := combat.NewStandingOrderTickHandler(pool, scheduler, gameClock, hub)
	worker.Register(events.ScheduledStandingOrderTick, standingOrderH.Handle)
	worker.Register(events.ScheduledFoodTick, foodTickH.Handle)
	offerExpiryH := economy.NewOfferExpiryHandler(pool, scheduler, hub)
	worker.Register(events.ScheduledOfferExpiry, offerExpiryH.Handle)
	go worker.Run(ctx)
	tickWorker := tick.New(pool, gameClock, eventStore)
	go tickWorker.Run(ctx)
	go seedDailyTicks(ctx, pool, scheduler)

	r := chi.NewRouter()
	installMiddleware(r)

	// Liveness/readiness probe for deploy verification and monitoring. Public,
	// no auth. Pings the DB with a short deadline so a 200 means the server can
	// actually serve (DB reachable), not merely that the process is up; 503
	// signals a live process that can't reach its database.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, pingCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer pingCancel()
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Static files and HTML templates.
	staticDir := getEnv("STATIC_DIR", "../../web/static")
	templateDir := getEnv("TEMPLATE_DIR", "../../web/templates")
	// no-cache: the ES modules under /static are deployed by git pull + restart
	// with no cache-busting in their import paths, so a browser that cached them
	// keeps running PRE-deploy code — every founder-phase affordance looked
	// missing on 2026-07-15 because the client was executing last week's JS.
	// no-cache still allows conditional 304s; it only forces revalidation.
	staticFS := http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir)))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		staticFS.ServeHTTP(w, req)
	}))

	webH, err := handlers.NewWebHandler(pool, authSvc, templateDir, staticDir, gameClock)
	if err != nil {
		slog.Error("load templates", "err", err)
		os.Exit(1)
	}

	wsH := handlers.NewWSHandler(hub)
	// OptionalMiddleware (not Middleware): a client without a valid token must
	// still connect and receive world-wide broadcasts (map/spectator views),
	// it just won't be a NotifyPlayer target. See notify.Hub.Register.
	r.With(auth.OptionalMiddleware(authSvc)).Get("/ws/{worldID}", wsH.Connect)

	// Web (HTML) routes.
	r.Get("/", webH.Index)
	r.Get("/logout", webH.Logout)
	r.With(auth.WebMiddleware(authSvc)).Get("/play", webH.Play)
	r.With(auth.WebMiddleware(authSvc)).Route("/world/{worldID}", func(r chi.Router) {
		r.Get("/join", webH.JoinView)
		r.Get("/epitaph", webH.EpitaphView)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			worldID := chi.URLParam(r, "worldID")
			http.Redirect(w, r, "/world/"+worldID+"/map", http.StatusSeeOther)
		})
		r.Get("/map", webH.MapView)
	})

	// Auth routes (public).
	ah := handlers.NewAuthHandler(authSvc)
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", ah.Register)
		r.Post("/login", ah.Login)
		r.Post("/refresh", ah.Refresh)
		r.With(auth.Middleware(authSvc)).Get("/me", ah.Me)
	})

	// Game routes (authenticated).
	wh := handlers.NewWorldHandler(pool, authSvc, gameClock)
	kh := handlers.NewKingdomHandler(pool, scheduler, gameClock)
	ph := handlers.NewProvinceHandler(pool, scheduler, gameClock, sitosCfg, eventStore, hub)
	soh := handlers.NewStandingOrderHandler(pool)
	sh := handlers.NewSettlementHandler(pool, eventStore, scheduler, gameClock)
	mh := handlers.NewMessengerHandler(pool, scheduler, gameClock, hub)
	jh := handlers.NewJoinHandler(pool, eventStore, sitosCfg, gameClock, hub)
	nh := handlers.NewNotificationsHandler(pool)
	dph := handlers.NewDispatchPreferencesHandler(pool)
	uh := handlers.NewUnitHandler(pool, scheduler, eventStore, gameClock)
	godH := handlers.NewGodHandler(pool)
	rh := handlers.NewReportsHandler(pool)

	r.Route("/api/v1", func(r chi.Router) {
		// Admin routes — no JWT, keyed by X-Admin-Key header.
		r.Get("/admin/worlds/{worldID}/god-view", godH.View)
		r.Get("/admin/worlds/{worldID}/reports", rh.List)
		r.Post("/admin/worlds/{worldID}/backfill-placements", ph.BackfillPlacements)
		// Reference catalogue — no auth, static data.
		r.Get("/buildings", ph.BuildingCatalogue)
		r.Get("/units", ph.UnitCatalogue)
		r.Get("/recipes", ph.RecipeCatalogue)
		// Tradeable goods — the same set messenger.go's offer validation
		// accepts (silver is the price side, cult is temple labor), read from
		// one shared helper so the offer form and the offer validator can
		// never drift apart.
		r.Get("/goods", handlers.NewGoodsHandler(pool).TradeableCatalogue)

		// Dispatch preferences — per-player, not per-world (players is a global
		// account, and the preference is about the KIND of event, the same
		// across every world). megaron_plan_dispatches.md §2/§6:4.
		r.With(auth.Middleware(authSvc)).Get("/notification-preferences", dph.List)
		r.With(auth.Middleware(authSvc)).Put("/notification-preferences/{kind}", dph.Mute)
		r.With(auth.Middleware(authSvc)).Delete("/notification-preferences/{kind}", dph.Unmute)

		// World endpoints — list/get/map are public; create requires auth.
		r.Get("/worlds", wh.List)
		r.With(auth.Middleware(authSvc)).Post("/worlds", wh.Create)
		r.Get("/worlds/{worldID}", wh.Get)
		// Map and province list use OptionalMiddleware: fog-of-war when authenticated.
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/map", wh.Map)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/colonize-preview", wh.ColonizePreview)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/provinces", wh.Provinces)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/marches", wh.Marches)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/messengers", wh.MapMessengers)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/trades", wh.MapTrades)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/foreign-units", wh.ForeignUnits)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/rural-projections", wh.RuralProjections)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/wanaxes", wh.Wanaxes)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/cities", wh.Cities)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/diplomacy", wh.Diplomacy)

		// Province and kingdom endpoints require authentication.
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(authSvc))
			// Single-world enforcement: reject writes aimed at an archived world
			// (a stale client otherwise gets writes accepted but never ticked).
			r.Use(handlers.RequireActiveWorld(pool))

			r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/actions", ph.Actions)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/army", ph.GetArmy)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/buildings", ph.Buildings)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/goods", ph.Goods)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/ticklog", ph.Ticklog)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/build", ph.Build)
			r.Delete("/worlds/{worldID}/provinces/{provinceID}/build-queue/{queueID}", ph.CancelBuild)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/recruit", ph.Recruit)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/trade", ph.TradeRoutes)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/trade", ph.Trade)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/disband", ph.Disband)
			r.Put("/worlds/{worldID}/provinces/{provinceID}/labor", ph.LaborAlloc)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/placement-options", ph.PlacementOptions)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/placements", ph.Placements)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/placements", ph.PlaceGubbe)
			r.Delete("/worlds/{worldID}/provinces/{provinceID}/placements/{ordinal}", ph.UnplaceGubbe)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/slaughter-livestock", ph.SlaughterLivestock)

			r.Post("/worlds/{worldID}/standing-orders", soh.Create)
			r.Get("/worlds/{worldID}/standing-orders", soh.List)
			r.Post("/worlds/{worldID}/standing-orders/{orderID}/pause", soh.Pause)
			r.Post("/worlds/{worldID}/standing-orders/{orderID}/resume", soh.Resume)
			r.Delete("/worlds/{worldID}/standing-orders/{orderID}", soh.Delete)

			r.Get("/worlds/{worldID}/market/wants", ph.MarketWants)

			r.Post("/worlds/{worldID}/join", jh.Join)
			// Founder phase: the Nomadic Host becomes a metropolis where it stands.
			r.Post("/worlds/{worldID}/founding/settle", jh.Settle)
			// Founder phase: messengers from the wandering host (mig 087).
			r.Post("/worlds/{worldID}/founding/messengers", mh.SendFromHost)
			r.Get("/worlds/{worldID}/founding/messengers", mh.ListFromHost)
			r.Get("/worlds/{worldID}/founding/status", jh.FoundingStatus)

			r.Route("/worlds/{worldID}/kingdoms", func(r chi.Router) {
				r.Use(requireKingdomsEnabled)
				r.Get("/", kh.List)
				r.Post("/", kh.Found)
				r.Get("/invitations", kh.Invitations)
				r.Post("/{kingdomID}/invite", kh.Invite)
				r.Post("/{kingdomID}/join", kh.Join)
				r.Delete("/{kingdomID}/leave", kh.Leave)
				r.Get("/{kingdomID}/council", kh.Council)
				r.Patch("/{kingdomID}/council/{role}", kh.AssignRole)
				r.Post("/{kingdomID}/borrow-army", kh.BorrowArmy)
				r.Post("/{kingdomID}/election", kh.CallElection)
				r.Post("/{kingdomID}/vote", kh.Vote)
				r.Get("/{kingdomID}/election", kh.ElectionStatus)
				r.Get("/{kingdomID}/borrowed-armies", kh.BorrowedArmiesList)
				r.Post("/{kingdomID}/treasury/deposit", kh.TreasuryDeposit)
			})

			// Unit endpoints (C3/C4/C5/C6).
			r.Get("/worlds/{worldID}/units", uh.ListUnits)
			r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
			r.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)
			r.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)
			r.Post("/worlds/{worldID}/units/{unitID}/standing-orders", uh.SetStandingOrders)
			r.Post("/worlds/{worldID}/units/{unitID}/load", uh.Load)
			r.Post("/worlds/{worldID}/units/{unitID}/unload", uh.Unload)
			r.Post("/worlds/{worldID}/units/{unitID}/reinforce", uh.Reinforce)
			r.Post("/worlds/{worldID}/units/{unitID}/repair", uh.Repair)

			r.Get("/worlds/{worldID}/settlements", sh.List)
			r.Get("/worlds/{worldID}/settlements/{settlementID}", sh.Get)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/occupation-order", uh.OccupationOrder)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/gift", sh.Gift)
			r.Get("/worlds/{worldID}/settlements/{settlementID}/loyalty-log", sh.LoyaltyLog)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/return-army", sh.ReturnArmy)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/rite", sh.Rite)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/abandon", sh.Abandon)
			r.Get("/worlds/{worldID}/gossip", sh.Gossip)

			r.Post("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.Send)
			r.Get("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.ListSent)
			r.Get("/worlds/{worldID}/messengers/inbox", mh.Inbox)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/reply", mh.Reply)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/trade-accept", mh.TradeAccept)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/trade-decline", mh.TradeDecline)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/trade-cancel", mh.CancelOffer)

			r.Get("/worlds/{worldID}/notifications", nh.List)
			r.Post("/worlds/{worldID}/notifications/read-all", nh.ReadAll)
			r.Delete("/worlds/{worldID}/notifications", nh.DeleteAll)

			r.Post("/worlds/{worldID}/reports", rh.Create)
		})
	})

	addr := fmt.Sprintf(":%s", getEnv("PORT", "8080"))
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("keryx server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// seedDailyTicks ensures each world has exactly one queued instance of each
// daily tick type. Safe to call on every startup — INSERT is skipped when a
// pending (unprocessed) tick already exists.
//
// 'forming' is included, and that is the whole point. A world is born forming
// (worlds.state DEFAULT) and only flips to active on the FIRST join
// (api/handlers/join.go). Seeding ran at startup over active worlds only, so a
// freshly created world was seeded never: it existed before this ran, it was
// not active yet when it ran, and nothing re-seeds at the transition. Every
// daily tick — upkeep, sitos, kharis, loyalty decay and welfare, colony
// penalty, borrowed army, intercept scan — stayed dead for the world's entire
// life until someone happened to restart the process. On the dev server `air`
// restarts constantly and hid it; the acceptance world, which restarts for
// nothing, exposed it (2026-08-02). Ticking a forming world is harmless: every
// handler iterates settlements or non-founder units, and a forming world has
// neither.
func seedDailyTicks(ctx context.Context, pool *pgxpool.Pool, sched *events.Scheduler) {
	rows, err := pool.Query(ctx, `SELECT id FROM worlds WHERE state IN ('forming', 'active')`)
	if err != nil {
		slog.Error("seed daily ticks: query worlds", "err", err)
		return
	}
	defer rows.Close()

	var worldIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			worldIDs = append(worldIDs, id)
		}
	}

	tickTypes := []events.ScheduledEventType{
		events.ScheduledLoyaltyDecayTick,
		events.ScheduledLoyaltyWelfareTick,
		events.ScheduledColonyPenaltyTick,
		events.ScheduledBorrowedArmyTick,
		events.ScheduledKharisTick,
		events.ScheduledUpkeepTick,
		events.ScheduledFoodTick,
		events.ScheduledSitosTick,
		events.ScheduledInterceptScan,
		events.ScheduledUnitInterceptScan,
		events.ScheduledMarchSightingScan,
		events.ScheduledMarchEncounterScan,
		events.ScheduledStandingOrderTick,
	}

	for _, wid := range worldIDs {
		for _, tt := range tickTypes {
			var exists bool
			_ = pool.QueryRow(ctx,
				`SELECT EXISTS (
				     SELECT 1 FROM scheduled_events
				     WHERE world_id = $1 AND event_type = $2
				       AND processed_at IS NULL AND failed_at IS NULL
				 )`,
				wid, string(tt),
			).Scan(&exists)
			if exists {
				continue
			}
			if err := sched.EnqueueTick(ctx, wid, tt, struct{}{}, 1); err != nil {
				slog.Error("seed daily tick", "world", wid, "type", tt, "err", err)
			}
		}
	}
	slog.Info("daily ticks seeded", "worlds", len(worldIDs))
}

func runMigrations(dbURL string) error {
	m, err := migrate.New("file://db/migrations", dbURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		// Dirty state: force back to the previous clean version and retry once.
		var dirtyErr migrate.ErrDirty
		if errors.As(err, &dirtyErr) && dirtyErr.Version > 0 {
			slog.Warn("dirty migration state — forcing to previous version", "version", dirtyErr.Version-1)
			if fErr := m.Force(dirtyErr.Version - 1); fErr != nil {
				return fmt.Errorf("force migration version: %w", fErr)
			}
			if err2 := m.Up(); err2 != nil && err2 != migrate.ErrNoChange {
				return fmt.Errorf("migrate up (after force): %w", err2)
			}
		} else {
			return fmt.Errorf("migrate up: %w", err)
		}
	}
	slog.Info("migrations applied")
	return nil
}

// installMiddleware wires the root router's middleware chain, in order.
// Pulled out of main() (which can't run in a test — it needs a live DB/Redis
// pool and blocks forever) so cmd/server/compress_test.go can build a router
// via the same function instead of a hand-copied mirror of this list — a
// mirror can't fail when the real chain changes.
//
// Compress: /map and friends go out uncompressed today (6.8 MB measured on a
// 230x230 world, gzip -9 gets it to 18% of that). chi's compressResponseWriter
// forwards Hijack() to the underlying ResponseWriter whenever nothing
// compressible has been written yet (WriteHeader not yet called ->
// compressible defaults false), so gorilla/websocket's Upgrade — which does
// its own w.(http.Hijacker) assertion in /ws/{worldID} — still gets a real
// Hijacker. Static files' Cache-Control (set where /static/* is registered)
// and conditional 304s are untouched: this only gates the body encoding, not
// the header map. See compress_test.go, which pins this ordering.
func installMiddleware(r chi.Router) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)
	r.Use(middleware.Compress(5))
}

// corsMiddleware allows cross-origin requests for the Bearer-auth API.
// Needed for WKWebView (iOS) and any future native client. Stateless — no credentials.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// minMapWidth and minMapHeight are the smallest dimensions the map generator
// is actually exercised at (internal/world/mapgen_test.go, TestGenerateMap_*
// size tables use {30,20} as their smallest case) — below that, GenerateMap's
// own rejection-sampling has no proven track record of finding a valid map
// and would just burn maxMapAttempts before panicking. Catching it here turns
// that panic into a clean boot-time error instead.
const (
	minMapWidth  = 30
	minMapHeight = 20
)

// envMapDim parses a map-size env var (MAP_WIDTH / MAP_HEIGHT), returning def
// when unset. Refuses (rather than clamping or defaulting) on a non-integer
// value or one below min — see the ensureWorld doc comment for why a bad
// value fails loud instead of quietly falling back.
func envMapDim(key string, def, min int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: not an integer", key, v)
	}
	if n < min {
		return 0, fmt.Errorf("%s=%d is below the minimum map dimension (%d) — refusing to seed an unusably small world", key, n, min)
	}
	return n, nil
}

// requireKingdomsEnabled gates the /kingdoms subtree behind KINGDOMS_ENABLED
// (default off — kingdoms are post-MVP, Timothy 2026-07-08). Handlers stay
// registered per temenos_arkitektur Fas 6 (endpoints always exist, they just
// answer disabled) — this middleware is the only thing that changes.
func requireKingdomsEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("KINGDOMS_ENABLED") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "kingdoms_disabled",
				"hint":  "Kingdoms är avstängt i MVP — riken återkommer efter grundmekanik-beviset",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// absorbStartupDowntime reads the most recent server heartbeat and, if the gap
// since then exceeds clock.PauseThreshold, tells the WallClock to adjust.
func absorbStartupDowntime(ctx context.Context, pool *pgxpool.Pool, clk *clock.WallClock) {
	var lastBeat time.Time
	err := pool.QueryRow(ctx,
		`SELECT beat_at FROM server_heartbeats ORDER BY beat_at DESC LIMIT 1`,
	).Scan(&lastBeat)
	if err != nil {
		// Table may not exist yet (first boot) — that's fine.
		return
	}
	gap := time.Since(lastBeat)
	if gap > clock.PauseThreshold {
		clk.RecordDowntime(gap)
		slog.Info("server downtime absorbed into game clock", "gap", gap.Round(time.Second))
	}
}

// ensureWorld returns the single world this server hosts. If no world exists it
// creates one named WORLD_NAME (env; default "The Thalassa") sized MAP_WIDTH ×
// MAP_HEIGHT (env; default 56×40 — the locked map spec, see megaron_todo.md
// "Kartstorlek HARD 56×40"). MAP_WIDTH/MAP_HEIGHT are honoured here using the
// same env names and default-on-unset behaviour as the standalone
// cmd/create-world seeding tool (cmd/create-world/main.go, envInt) — this is a
// separate small parser rather than a shared one because create-world lives in
// its own package and importing across cmd/ packages would tangle two
// independent entrypoints together for four lines of logic. Unlike
// create-world, an out-of-range value here is refused with an error instead of
// silently used or log.Fatal'd: this path runs during normal server boot, and
// a server that boots an unusably small world (too small to fit the 12 spawn
// locations, or too small for world.GenerateMap's own invariants) is worse
// than one that says why it didn't. The world ID is stable across restarts —
// it lives in the database.
func ensureWorld(ctx context.Context, pool *pgxpool.Pool, clk *clock.WallClock) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM worlds ORDER BY created_at DESC LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}

	// No world yet — create one.
	name := getEnv("WORLD_NAME", "The Thalassa")
	width, err := envMapDim("MAP_WIDTH", 56, minMapWidth)
	if err != nil {
		return uuid.Nil, err
	}
	height, err := envMapDim("MAP_HEIGHT", 40, minMapHeight)
	if err != nil {
		return uuid.Nil, err
	}
	seed := clk.Now().UnixNano()

	err = pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_seed, map_width, map_height)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		name, seed, width, height,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create world: %w", err)
	}

	// Generate and store map tiles. GenerateMap may reseed to satisfy map
	// invariants — persist the seed that actually produced the stored map.
	tiles, effSeed := world.GenerateMap(id, seed, width, height)
	if effSeed != seed {
		if _, err := pool.Exec(ctx,
			`UPDATE worlds SET map_seed = $1 WHERE id = $2`, effSeed, id); err != nil {
			return uuid.Nil, fmt.Errorf("persist effective map seed: %w", err)
		}
		seed = effSeed
	}
	// landmass_id (migration 124, megaron_plan_spawn_landmassa.md Slice 1):
	// computed once for the whole world; sea tiles insert NULL.
	comp := world.LandComponents(tiles)
	for _, t := range tiles {
		var landmassID *int
		if lid, ok := comp[[2]int{t.Q, t.R}]; ok {
			landmassID = &lid
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal, fertility, mineral,
			                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit, landmass_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (world_id, q, r) DO NOTHING`,
			id, t.Q, t.R, string(t.Terrain), t.Coastal, t.Fertility, t.Mineral,
			t.CopperDeposit, t.TinDeposit, t.SilverDeposit, t.CedarDeposit, landmassID,
		); err != nil {
			return uuid.Nil, fmt.Errorf("store map tile: %w", err)
		}
	}
	slog.Info("world created", "name", name, "id", id, "seed", seed)
	return id, nil
}

// runHeartbeat writes a row to server_heartbeats every 10 seconds so that the
// next startup can detect how long the server was down.
func runHeartbeat(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := pool.Exec(ctx,
				`INSERT INTO server_heartbeats (beat_at) VALUES (now())`,
			); err != nil {
				slog.Warn("heartbeat write failed", "err", err)
			}
		}
	}
}

// --- Retention: server_heartbeats, processed_sitos_ticks, scheduled_events ---
//
// Three tables grow without bound during ordinary play and are NOT cleaned by
// a reseed:
//   - server_heartbeats (mig 007): no world_id column at all. One row per 10s,
//     forever. Measured on CT 126 2026-08-02: 529 053 rows / 56 MB, the
//     largest table in the DB after the mig-104 orphan cleanup.
//   - processed_sitos_ticks (mig 097): no world_id column. One row per
//     (SitosTick event, settlement) — the idempotency claim itself (see the
//     migration's doc comment). Measured: 556 828 rows / 54 MB, ALL from a
//     world that no longer exists (its rows outlived it because there is no
//     world_id to cascade on). Growth resumes, harder, as soon as the current
//     (empty) world gets settlements — the granary tick (2026-08-03) touches
//     every settlement every tick, same as the fund it replaced.
//   - scheduled_events (mig 001) DOES carry a world_id FK with ON DELETE
//     CASCADE (added in mig 104) — so it IS cleaned the moment a world ROW is
//     deleted. But mig 104's own comment calls this table out separately:
//     "tabellen har noll föräldralösa rader idag (bara historiskt kvarhållna
//     behandlade rader), en annan rot som hör till en egen framtida slice" —
//     i.e. the cascade doesn't help a world that simply keeps running: a
//     persistent, long-lived world (the whole point of Megaron) accumulates
//     processed/failed rows forever with zero time-based pruning. That is the
//     root this slice actually closes for this table. Measured: 156 306 rows
//     / 23 MB, ~26 400 rows/day, in what is currently an EMPTY world.
//
// Invariant (never violated, whatever the window): no row is deleted that a
// pending or possible re-run still needs.
//   - server_heartbeats: the single most recent row is NEVER deleted,
//     regardless of its age — it's the only thing absorbStartupDowntime reads.
//   - processed_sitos_ticks: a claim is only deleted once its window has
//     passed AND NOT EXISTS a scheduled_events row for the same event_id with
//     processed_at IS NULL — i.e. the event that produced the claim is no
//     longer capable of being re-run. The window itself only needs to be
//     bigger than the worker's own retry ceiling (events.Worker: 5s handler
//     timeout, DeadLetterAttempts=3, poll interval ≤ 10s) — the NOT EXISTS
//     check is the real safety net, the window is a wide margin on top of it.
//   - scheduled_events: only processed_at IS NOT NULL or failed_at IS NOT NULL
//     rows are ever candidates — a row with both NULL (pending, still
//     claimable by events.Worker per scheduler.go's claim query) is never
//     touched by either delete statement, at any age.
//
// Retention is drift, not game time — cmd/server/main.go holding a wall clock
// here is a sanctioned exception (CLAUDE.md §Time); this does not use
// clock.Clock and must not start to.

// retentionConfig is loaded once at boot (loadRetentionConfig) and threaded
// through runRetention — mirrors economy.LoadSitosConfig's "load once, pass
// down" convention.
type retentionConfig struct {
	interval                    time.Duration
	heartbeatWindow             time.Duration
	sitosTickWindow             time.Duration
	scheduledEventsWindow       time.Duration
	scheduledEventsFailedWindow time.Duration
	batchSize                   int
	maxBatchesPerTable          int
}

// getEnvDuration parses a duration env var ("1h", "30m", "0" to disable
// pruning for that window), returning def when the var is unset. Fails loud
// (mirrors envMapDim) instead of silently falling back on a malformed value —
// a mistyped retention window either deletes everything or nothing, silently,
// which is exactly the kind of mistake a quiet fallback would hide.
func getEnvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: not a valid duration (e.g. \"1h\", \"30m\", \"0\" to disable): %w", key, v, err)
	}
	return d, nil
}

// getEnvPositiveInt parses an int env var, returning def when unset. Refuses
// (rather than clamping) a non-integer or non-positive value.
func getEnvPositiveInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: not an integer: %w", key, v, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s=%d: must be positive", key, n)
	}
	return n, nil
}

// loadRetentionConfig reads RETENTION_* / *_RETENTION env vars once at boot.
//
// Defaults, and why:
//   - RETENTION_INTERVAL = 1h: how often the job wakes up. Sparse on purpose —
//     this is housekeeping, not a hot path; <= 0 disables the whole job.
//   - HEARTBEAT_RETENTION = 168h (7 days): comfortably longer than any outage
//     we'd actually want absorbStartupDowntime to measure — a server that has
//     been down for a week has bigger problems than a stale downtime figure.
//   - SITOS_TICK_RETENTION = 24h: the NOT EXISTS guard is the real safety net
//     (see doc comment above); 24h is a wide margin over the worker's actual
//     retry ceiling (seconds), kept long enough to be useful for debugging a
//     recent tick without becoming a second unbounded table.
//   - SCHEDULED_EVENTS_RETENTION = 72h: processed rows — long enough to
//     inspect "what fired yesterday" from the DB directly, short enough that
//     an idle world's ~26k rows/day doesn't re-accumulate past a few days.
//   - SCHEDULED_EVENTS_FAILED_RETENTION = 720h (30 days): failed_at rows are
//     dead-letters — diagnostic evidence of a handler that gave up, and rare
//     (0 in production today). They get a much longer window than routine
//     processed rows on purpose: the whole point of keeping them is to look
//     at them later.
//   - RETENTION_BATCH_SIZE = 5000, RETENTION_MAX_BATCHES = 200: bounds a
//     single pass to at most 1,000,000 rows deleted per table — generous
//     against the measured growth rates above, while keeping every individual
//     DELETE small enough not to hold a lock on a live table for long (see
//     the EXPLAIN plans in the proof package: ~5-50ms per 5000-row batch
//     against 200k-600k row tables).
func loadRetentionConfig() (retentionConfig, error) {
	var cfg retentionConfig
	var err error
	if cfg.interval, err = getEnvDuration("RETENTION_INTERVAL", time.Hour); err != nil {
		return cfg, err
	}
	if cfg.heartbeatWindow, err = getEnvDuration("HEARTBEAT_RETENTION", 168*time.Hour); err != nil {
		return cfg, err
	}
	if cfg.sitosTickWindow, err = getEnvDuration("SITOS_TICK_RETENTION", 24*time.Hour); err != nil {
		return cfg, err
	}
	if cfg.scheduledEventsWindow, err = getEnvDuration("SCHEDULED_EVENTS_RETENTION", 72*time.Hour); err != nil {
		return cfg, err
	}
	if cfg.scheduledEventsFailedWindow, err = getEnvDuration("SCHEDULED_EVENTS_FAILED_RETENTION", 720*time.Hour); err != nil {
		return cfg, err
	}
	if cfg.batchSize, err = getEnvPositiveInt("RETENTION_BATCH_SIZE", 5000); err != nil {
		return cfg, err
	}
	if cfg.maxBatchesPerTable, err = getEnvPositiveInt("RETENTION_MAX_BATCHES", 200); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// runRetention is the background job. Follows the runHeartbeat pattern:
// time.NewTicker + select on ctx.Done(), slog on failure, never a crash.
func runRetention(ctx context.Context, pool *pgxpool.Pool, cfg retentionConfig) {
	if cfg.interval <= 0 {
		slog.Info("retention job disabled (RETENTION_INTERVAL <= 0)")
		return
	}
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runRetentionPass(ctx, pool, cfg)
		}
	}
}

// runRetentionPass runs one prune of each of the three tables. Exported (via
// package-level visibility, not literally exported) for retention_test.go to
// call directly without waiting on the ticker.
func runRetentionPass(ctx context.Context, pool *pgxpool.Pool, cfg retentionConfig) {
	pruneHeartbeats(ctx, pool, cfg)
	pruneSitosTicks(ctx, pool, cfg)
	pruneScheduledEvents(ctx, pool, cfg)
}

// heartbeatDeleteSQL deletes rows older than the cutoff ($1), EXCEPT the
// single most recent row (by beat_at) — computed fresh on every batch via the
// idx_server_heartbeats_beat_at (beat_at DESC) index (mig 007), so the
// invariant holds even mid-pass and even if beat writes race the prune.
//
// The "SELECT ctid ... LIMIT $2" + "DELETE ... USING victims" shape (used by
// all three prune queries below) batches a DELETE without needing a
// table-specific composite key: ctid is universal, and wrapping the selector
// in a CTE materializes the LIMIT before the join so Postgres can't rewrite
// the LIMIT away. See the EXPLAIN plans in the proof package.
const heartbeatDeleteSQL = `
WITH victims AS (
    SELECT ctid FROM server_heartbeats
    WHERE beat_at < $1
      AND id <> (SELECT id FROM server_heartbeats ORDER BY beat_at DESC LIMIT 1)
    LIMIT $2
)
DELETE FROM server_heartbeats t USING victims v WHERE t.ctid = v.ctid
`

func pruneHeartbeats(ctx context.Context, pool *pgxpool.Pool, cfg retentionConfig) {
	if cfg.heartbeatWindow <= 0 {
		return
	}
	cutoff := time.Now().Add(-cfg.heartbeatWindow)
	pruneBatched(ctx, pool, "server_heartbeats", heartbeatDeleteSQL, cutoff, cfg.batchSize, cfg.maxBatchesPerTable)
}

// sitosTickDeleteSQL deletes a claim only once its window has passed AND the
// scheduled_events row that produced it is no longer pending (processed_at
// IS NULL means "still claimable" per events.Worker's claim query in
// internal/events/scheduler.go) — see the invariant doc comment above.
const sitosTickDeleteSQL = `
WITH victims AS (
    SELECT p.ctid FROM processed_sitos_ticks p
    WHERE p.processed_at < $1
      AND NOT EXISTS (
          SELECT 1 FROM scheduled_events se
          WHERE se.id = p.event_id AND se.processed_at IS NULL
      )
    LIMIT $2
)
DELETE FROM processed_sitos_ticks t USING victims v WHERE t.ctid = v.ctid
`

func pruneSitosTicks(ctx context.Context, pool *pgxpool.Pool, cfg retentionConfig) {
	if cfg.sitosTickWindow <= 0 {
		return
	}
	cutoff := time.Now().Add(-cfg.sitosTickWindow)
	pruneBatched(ctx, pool, "processed_sitos_ticks", sitosTickDeleteSQL, cutoff, cfg.batchSize, cfg.maxBatchesPerTable)
}

// scheduledEventsProcessedDeleteSQL / scheduledEventsFailedDeleteSQL are two
// separate statements, not one OR'd together, because failed_at (dead-letter)
// rows are diagnostic evidence and get their own, much longer window
// (SCHEDULED_EVENTS_FAILED_RETENTION) — see loadRetentionConfig's doc comment.
// Neither statement ever matches a pending row (processed_at IS NULL AND
// failed_at IS NULL): both conditions require their respective column to be
// NOT NULL first.
const scheduledEventsProcessedDeleteSQL = `
WITH victims AS (
    SELECT ctid FROM scheduled_events
    WHERE processed_at IS NOT NULL AND processed_at < $1
    LIMIT $2
)
DELETE FROM scheduled_events t USING victims v WHERE t.ctid = v.ctid
`

const scheduledEventsFailedDeleteSQL = `
WITH victims AS (
    SELECT ctid FROM scheduled_events
    WHERE failed_at IS NOT NULL AND failed_at < $1
    LIMIT $2
)
DELETE FROM scheduled_events t USING victims v WHERE t.ctid = v.ctid
`

func pruneScheduledEvents(ctx context.Context, pool *pgxpool.Pool, cfg retentionConfig) {
	if cfg.scheduledEventsWindow > 0 {
		cutoff := time.Now().Add(-cfg.scheduledEventsWindow)
		pruneBatched(ctx, pool, "scheduled_events(processed)", scheduledEventsProcessedDeleteSQL, cutoff, cfg.batchSize, cfg.maxBatchesPerTable)
	}
	if cfg.scheduledEventsFailedWindow > 0 {
		cutoff := time.Now().Add(-cfg.scheduledEventsFailedWindow)
		pruneBatched(ctx, pool, "scheduled_events(failed)", scheduledEventsFailedDeleteSQL, cutoff, cfg.batchSize, cfg.maxBatchesPerTable)
	}
}

// pruneBatched runs sql (one of the *DeleteSQL statements above, all shaped
// "... WHERE <col> < $1 ... LIMIT $2") repeatedly in batches of batchSize,
// stopping as soon as a batch deletes fewer than batchSize rows (nothing left
// to do) or after maxBatches (a safety cap so one retention pass can never
// run unbounded — see the "hit max batches" warning below). Each batch is its
// own statement/transaction, so no single DELETE holds a lock on a live table
// for longer than one small batch takes.
func pruneBatched(ctx context.Context, pool *pgxpool.Pool, table, sql string, cutoff time.Time, batchSize, maxBatches int) {
	start := time.Now()
	total := 0
	ranBatches := 0
	for ranBatches < maxBatches {
		tag, err := pool.Exec(ctx, sql, cutoff, batchSize)
		if err != nil {
			slog.Warn("retention: batch delete failed", "table", table, "err", err, "batches_done", ranBatches, "rows_deleted_so_far", total)
			return
		}
		ranBatches++
		n := int(tag.RowsAffected())
		total += n
		if n < batchSize {
			break
		}
	}
	if total > 0 {
		slog.Info("retention pass", "table", table, "rows_deleted", total, "batches", ranBatches, "elapsed", time.Since(start).Round(time.Millisecond))
	}
	if ranBatches == maxBatches && total > 0 {
		slog.Warn("retention: hit max batches per pass — more rows may remain, will continue next pass", "table", table, "max_batches", maxBatches, "batch_size", batchSize)
	}
}
