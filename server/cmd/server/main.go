package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/poleia/server/api/handlers"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/chronicle"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/kharis"
	"github.com/poleia/server/internal/loyalty"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/notify"
	"github.com/poleia/server/internal/world"
)

func main() {
	_ = godotenv.Load()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dbURL := mustEnv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dbURL)
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

	// GameClock — single source of time for all game logic.
	// On startup, check for downtime since last heartbeat and absorb it.
	gameClock := clock.NewWallClock()
	absorbStartupDowntime(ctx, pool, gameClock)
	go runHeartbeat(ctx, pool)

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
	arrivalH := combat.NewArrivalHandler(pool, eventStore, hub, gameClock, scheduler)
	buildH := combat.NewBuildCompleteHandler(pool, eventStore, hub)
	trainH := combat.NewTrainCompleteHandler(pool, eventStore, hub)
	decayH := loyalty.NewDecayHandler(pool, scheduler, eventStore)
	colonyH := loyalty.NewColonyPenaltyHandler(pool, scheduler, eventStore)
	borrowedH := loyalty.NewBorrowedArmyPenaltyHandler(pool, scheduler, eventStore, gameClock)
	messengerArrivalH := messenger.NewArrivalHandler(pool, scheduler, eventStore)
	messengerReturnH := messenger.NewReturnHandler(pool, eventStore)
	kharisH := kharis.NewTickHandler(pool, scheduler, eventStore)
	tradeH := economy.NewDeliveryHandler(pool, eventStore, hub, scheduler)
	tradeReturnH := economy.NewTradeReturnHandler(pool, eventStore, hub)
	respawnH := handlers.NewRespawnHandler(pool)
	recallH := messenger.NewRecallArrivalHandler(pool, scheduler, gameClock)
	worker.Register(events.ScheduledArmyArrival, arrivalH.Handle)
	worker.Register(events.ScheduledBuildComplete, buildH.Handle)
	worker.Register(events.ScheduledTrainComplete, trainH.Handle)
	worker.Register(events.ScheduledLoyaltyDecayTick, decayH.Handle)
	worker.Register(events.ScheduledColonyPenaltyTick, colonyH.Handle)
	worker.Register(events.ScheduledBorrowedArmyTick, borrowedH.Handle)
	worker.Register(events.ScheduledMessengerArrival, messengerArrivalH.Handle)
	worker.Register(events.ScheduledMessengerReturn, messengerReturnH.Handle)
	worker.Register(events.ScheduledKharisTick, kharisH.Handle)
	worker.Register(events.ScheduledTradeDelivery, tradeH.Handle)
	worker.Register(events.ScheduledTradeReturn, tradeReturnH.Handle)
	worker.Register(events.ScheduledRespawn, respawnH.Handle)
	worker.Register(events.ScheduledRecallArrival, recallH.Handle)
	logisticsH := handlers.NewLogisticsArrivalHandler(pool)
	worker.Register(events.ScheduledLogisticsArrival, logisticsH.Handle)
	go worker.Run(ctx)
	go seedDailyTicks(ctx, pool, scheduler)
	go healDispossessed(ctx, pool, scheduler)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

	// Static files and HTML templates.
	staticDir := getEnv("STATIC_DIR", "../../web/static")
	templateDir := getEnv("TEMPLATE_DIR", "../../web/templates")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	webH, err := handlers.NewWebHandler(pool, authSvc, templateDir, gameClock, serverWorldID)
	if err != nil {
		slog.Error("load templates", "err", err)
		os.Exit(1)
	}

	wsH := handlers.NewWSHandler(hub)
	r.Get("/ws/{worldID}", wsH.Connect)

	// Web (HTML) routes.
	r.Get("/", webH.Index)
	r.With(auth.WebMiddleware(authSvc)).Get("/play", webH.Play)
	r.With(auth.WebMiddleware(authSvc)).Route("/world/{worldID}", func(r chi.Router) {
		r.Get("/join", webH.JoinView)
		r.Get("/megaron", webH.MegaronView)
		r.Get("/", webH.Province)
		r.Get("/map", webH.MapView)
		r.Get("/rawaketa", webH.RawakView)
		r.Get("/kingdom", webH.KingdomView)
		r.Get("/messages", webH.MessagesView)
		r.Get("/market", webH.MarketView)
		r.Get("/resource-bar", webH.ResourceBar)
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
	ph := handlers.NewProvinceHandler(pool, scheduler, gameClock)
	sh := handlers.NewSettlementHandler(pool, eventStore, scheduler, gameClock)
	mh := handlers.NewMessengerHandler(pool, scheduler, gameClock)
	jh := handlers.NewJoinHandler(pool)

	r.Route("/api/v1", func(r chi.Router) {
		// World endpoints — list/get/map are public; create requires auth.
		r.Get("/worlds", wh.List)
		r.With(auth.Middleware(authSvc)).Post("/worlds", wh.Create)
		r.Get("/worlds/{worldID}", wh.Get)
		// Map and province list use OptionalMiddleware: fog-of-war when authenticated.
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/map", wh.Map)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/provinces", wh.Provinces)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/marches", wh.Marches)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/messengers", wh.MapMessengers)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/trades", wh.MapTrades)
		r.With(auth.OptionalMiddleware(authSvc)).Get("/worlds/{worldID}/wanaxes", wh.Wanaxes)

		// Province and kingdom endpoints require authentication.
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(authSvc))

			r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/army", ph.GetArmy)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/buildings", ph.Buildings)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/goods", ph.Goods)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/march", ph.March)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/build", ph.Build)
			r.Delete("/worlds/{worldID}/provinces/{provinceID}/build-queue/{queueID}", ph.CancelBuild)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/recruit", ph.Recruit)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/marches", ph.Marches)
			r.Delete("/worlds/{worldID}/provinces/{provinceID}/marches/{marchID}", ph.RecallMarch)
			r.Delete("/worlds/{worldID}/provinces/{provinceID}/outpost", ph.RecallOutpost)
			r.Get("/worlds/{worldID}/provinces/{provinceID}/trade", ph.TradeRoutes)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/trade", ph.Trade)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/craft", ph.Craft)
			r.Post("/worlds/{worldID}/provinces/{provinceID}/disband", ph.Disband)
			r.Put("/worlds/{worldID}/provinces/{provinceID}/labor", ph.LaborAlloc)

			r.Get("/worlds/{worldID}/outpost-flows", ph.OutpostFlows)

			r.Post("/worlds/{worldID}/join", jh.Join)

			r.Get("/worlds/{worldID}/kingdoms", kh.List)
			r.Post("/worlds/{worldID}/kingdoms", kh.Found)
			r.Get("/worlds/{worldID}/kingdoms/invitations", kh.Invitations)
			r.Post("/worlds/{worldID}/kingdoms/{kingdomID}/invite", kh.Invite)
			r.Post("/worlds/{worldID}/kingdoms/{kingdomID}/join", kh.Join)
			r.Delete("/worlds/{worldID}/kingdoms/{kingdomID}/leave", kh.Leave)
			r.Get("/worlds/{worldID}/kingdoms/{kingdomID}/council", kh.Council)
			r.Patch("/worlds/{worldID}/kingdoms/{kingdomID}/council/{role}", kh.AssignRole)
			r.Post("/worlds/{worldID}/kingdoms/{kingdomID}/borrow-army", kh.BorrowArmy)
			r.Post("/worlds/{worldID}/kingdoms/{kingdomID}/election", kh.CallElection)
			r.Post("/worlds/{worldID}/kingdoms/{kingdomID}/vote", kh.Vote)

			r.Get("/worlds/{worldID}/settlements", sh.List)
			r.Get("/worlds/{worldID}/settlements/{settlementID}", sh.Get)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/gift", sh.Gift)
			r.Get("/worlds/{worldID}/settlements/{settlementID}/loyalty-log", sh.LoyaltyLog)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/return-army", sh.ReturnArmy)
			r.Patch("/worlds/{worldID}/settlements/{settlementID}/cult-level", sh.SetCultLevel)
			r.Post("/worlds/{worldID}/settlements/{settlementID}/rite", sh.Rite)
			r.Get("/worlds/{worldID}/gossip", sh.Gossip)

			r.Post("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.Send)
			r.Get("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.ListSent)
			r.Get("/worlds/{worldID}/messengers/inbox", mh.Inbox)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/reply", mh.Reply)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/trade-accept", mh.TradeAccept)
			r.Post("/worlds/{worldID}/messengers/{messengerID}/trade-decline", mh.TradeDecline)

			r.Get("/worlds/{worldID}/kingdoms/{kingdomID}/election", kh.ElectionStatus)
			r.Get("/worlds/{worldID}/kingdoms/{kingdomID}/borrowed-armies", kh.BorrowedArmiesList)
			r.Post("/worlds/{worldID}/kingdoms/{kingdomID}/treasury/deposit", kh.TreasuryDeposit)
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
		slog.Info("poleia server starting", "addr", addr)
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

// seedDailyTicks ensures each active world has exactly one queued instance of
// each daily tick type. Safe to call on every startup — INSERT is skipped when
// a pending (unprocessed) tick already exists.
func seedDailyTicks(ctx context.Context, pool *pgxpool.Pool, sched *events.Scheduler) {
	rows, err := pool.Query(ctx, `SELECT id FROM worlds WHERE state = 'active'`)
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
		events.ScheduledColonyPenaltyTick,
		events.ScheduledBorrowedArmyTick,
		events.ScheduledKharisTick,
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
			if err := sched.EnqueueAfter(ctx, wid, tt, struct{}{}, 24*time.Hour); err != nil {
				slog.Error("seed daily tick", "world", wid, "type", tt, "err", err)
			}
		}
	}
	slog.Info("daily ticks seeded", "worlds", len(worldIDs))
}

// healDispossessed queues a Respawn event for every dispossessed player that has
// no capital and no pending respawn already in the queue. Runs once at startup
// to recover players that were defeated before the respawn scheduler existed.
func healDispossessed(ctx context.Context, pool *pgxpool.Pool, sched *events.Scheduler) {
	rows, err := pool.Query(ctx, `
		SELECT pwr.player_id, pwr.world_id,
		       COALESCE(
		           (SELECT s.culture_id FROM settlements s
		            WHERE s.world_id = pwr.world_id AND s.owner_id = pwr.player_id
		            LIMIT 1),
		           'akhaier'
		       ) AS culture
		FROM player_world_records pwr
		WHERE pwr.status = 'dispossessed'
		  AND NOT EXISTS (
		      SELECT 1 FROM settlements
		      WHERE world_id = pwr.world_id AND owner_id = pwr.player_id AND is_capital = true
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM scheduled_events
		      WHERE world_id = pwr.world_id AND event_type = 'Respawn'
		        AND payload->>'player_id' = pwr.player_id::text
		        AND processed_at IS NULL AND failed_at IS NULL
		  )`)
	if err != nil {
		slog.Error("healDispossessed: query", "err", err)
		return
	}
	defer rows.Close()

	type victim struct {
		playerID uuid.UUID
		worldID  uuid.UUID
		culture  string
	}
	var victims []victim
	for rows.Next() {
		var v victim
		if err := rows.Scan(&v.playerID, &v.worldID, &v.culture); err == nil {
			victims = append(victims, v)
		}
	}

	for _, v := range victims {
		if err := sched.EnqueueAfter(ctx, v.worldID, events.ScheduledRespawn,
			map[string]any{"player_id": v.playerID, "world_id": v.worldID, "culture": v.culture},
			5*time.Second,
		); err != nil {
			slog.Error("healDispossessed: enqueue", "player", v.playerID, "err", err)
		} else {
			slog.Info("healDispossessed: queued respawn", "player", v.playerID)
		}
	}
	if len(victims) > 0 {
		slog.Info("healDispossessed: scheduled respawns", "count", len(victims))
	}
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
// creates one using WORLD_NAME / MAP_WIDTH / MAP_HEIGHT from the environment.
// The world ID is stable across restarts — it lives in the database.
func ensureWorld(ctx context.Context, pool *pgxpool.Pool, clk *clock.WallClock) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM worlds LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}

	// No world yet — create one.
	name := getEnv("WORLD_NAME", "The Thalassa")
	width := 40
	height := 30
	seed := clk.Now().UnixNano()

	err = pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_seed, map_width, map_height)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		name, seed, width, height,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create world: %w", err)
	}

	// Generate and store map tiles.
	tiles := world.GenerateMap(id, seed, width, height)
	for _, t := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, fertility, mineral,
			                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (world_id, q, r) DO NOTHING`,
			id, t.Q, t.R, string(t.Terrain), t.Fertility, t.Mineral,
			t.CopperDeposit, t.TinDeposit, t.SilverDeposit, t.CedarDeposit,
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
