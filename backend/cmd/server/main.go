package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/bajajsakshi738/leaderboard-backend/internal/auth"
	"github.com/bajajsakshi738/leaderboard-backend/internal/config"
	"github.com/bajajsakshi738/leaderboard-backend/internal/database"
	"github.com/bajajsakshi738/leaderboard-backend/internal/handlers"
	"github.com/bajajsakshi738/leaderboard-backend/internal/middleware"
	"github.com/bajajsakshi738/leaderboard-backend/internal/redisclient"
	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/bajajsakshi738/leaderboard-backend/internal/services"
	"github.com/bajajsakshi738/leaderboard-backend/internal/websocket"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// postgres
	pool, err := database.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()
	log.Println("connected to postgres")

	// redis
	rdb, err := redisclient.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisUseTLS)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	defer rdb.Close()
	log.Println("connected to redis")

	// repositories
	userRepo := repository.NewUserRepository(pool)
	scoreRepo := repository.NewScoreRepository(pool)
	leaderboardRepo := repository.NewLeaderboardRepository(rdb)

	// services
	scoreService := services.NewScoreService(scoreRepo, leaderboardRepo)

	// if Redis lost its data, refill the boards and usernames from Postgres
	// before serving. a failure is logged, not fatal; the next start tries again.
	rebuildService := services.NewRebuildService(scoreRepo, userRepo, leaderboardRepo)
	rebuildCtx, cancelRebuild := context.WithTimeout(context.Background(), 2*time.Minute)
	rebuilt, rebuildStats, err := rebuildService.RebuildIfNeeded(rebuildCtx)
	cancelRebuild()
	switch {
	case err != nil:
		log.Printf("redis rebuild failed, continuing with the data Redis has: %v", err)
	case rebuilt:
		log.Printf("rebuilt Redis from Postgres: %d board entries, %d usernames", rebuildStats.BoardEntries, rebuildStats.Usernames)
	default:
		log.Println("redis boards already in sync with postgres")
	}
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpiry)

	// websocket hub and broadcaster
	hub := websocket.NewHub(cfg.AllowedOrigins)
	go hub.Run()

	broadcaster := services.NewLeaderboardBroadcaster(leaderboardRepo, hub, 1*time.Second, 20, "global")
	broadcastCtx, cancelBroadcast := context.WithCancel(context.Background())
	go broadcaster.Run(broadcastCtx)

	// handlers
	denylist := auth.NewDenylist(rdb)
	authHandler := handlers.NewAuthHandler(userRepo, leaderboardRepo, tokenManager, denylist)
	leaderboardHandler := handlers.NewLeaderboardHandler(leaderboardRepo, scoreService, scoreRepo)

	// router
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// don't trust X-Forwarded-For by default, or anyone could fake their IP and
	// dodge the limits. the limiter reads it itself when TRUST_PROXY_HEADER=true.
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatalf("failed to set trusted proxies: %v", err)
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Retry-After"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// rate limiting
	limiter := middleware.NewRateLimiter(rdb, cfg.RateLimitBypassToken, cfg.TrustProxyHeader)
	// register and login share one budget per IP. fails closed: if Redis is
	// down, login returns 503. score submits stay fail-open.
	authLimit := limiter.LimitFailClosed("auth", 10, time.Minute, limiter.ByIP)
	// one score per user every 3 seconds.
	scoreLimit := limiter.Limit("score", 1, 3*time.Second, limiter.ByUser)

	router.GET("/health", handlers.HealthCheck)
	router.GET("/ws/leaderboard", hub.ServeWS)

	// logged in routes. also refuses tokens that were logged out.
	requireAuth := middleware.RequireAuth(tokenManager, denylist)

	api := router.Group("/api")
	// bodies here are tiny, so cap them at 16 KiB.
	api.Use(middleware.BodyLimit(16 << 10))
	{
		authGroup := api.Group("/auth")
		authGroup.POST("/register", authLimit, authHandler.Register)
		authGroup.POST("/login", authLimit, authHandler.Login)
		authGroup.GET("/me", requireAuth, authHandler.Me)
		authGroup.POST("/logout", requireAuth, authHandler.Logout)

		lb := api.Group("/leaderboard")
		lb.GET("/:gameCode/top", leaderboardHandler.GetTop)
		lb.GET("/:gameCode/rank/:userID", leaderboardHandler.GetRankForUser)
		lb.GET("/:gameCode/me", requireAuth, leaderboardHandler.GetMyRank)

		scores := api.Group("/scores")
		scores.POST("", requireAuth, scoreLimit, leaderboardHandler.SubmitScore)
		scores.GET("/history", requireAuth, leaderboardHandler.GetMyHistory)

		reports := api.Group("/reports")
		reports.GET("/top-players", leaderboardHandler.GetTopPlayersReport)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
		// timeouts so slow clients can't hold connections open. they stop applying
		// to a WebSocket once it is upgraded (gorilla handles those).
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server listening on port %s (env=%s)", cfg.ServerPort, cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	cancelBroadcast()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server exited cleanly")
}
