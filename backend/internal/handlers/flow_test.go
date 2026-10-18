package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/auth"
	"github.com/bajajsakshi738/leaderboard-backend/internal/middleware"
	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/bajajsakshi738/leaderboard-backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// end to end handler tests against a real Postgres and Redis. they need
//
//	TEST_DATABASE_URL (a database with the migrations applied)
//	TEST_REDIS_ADDR
//
// skipped otherwise. they create users named "flowtest_..." and delete them
// at the end. Redis database 15 is used and flushed.
func flowStores(t *testing.T) (*pgxpool.Pool, *redis.Client) {
	t.Helper()
	dsn, addr := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_ADDR must both be set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres not usable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable: %v", err)
	}
	rdb.FlushDB(ctx)
	t.Cleanup(func() { rdb.FlushDB(ctx); rdb.Close() })
	return pool, rdb
}

func flowRouter(pool *pgxpool.Pool, rdb *redis.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	userRepo := repository.NewUserRepository(pool)
	scoreRepo := repository.NewScoreRepository(pool)
	boards := repository.NewLeaderboardRepository(rdb)
	tm := auth.NewTokenManager("test-secret-test-secret-test-secret", time.Hour)

	denylist := auth.NewDenylist(rdb)
	authH := NewAuthHandler(userRepo, boards, tm, denylist)
	lbH := NewLeaderboardHandler(boards, services.NewScoreService(scoreRepo, boards), scoreRepo)

	r := gin.New()
	r.POST("/register", authH.Register)
	r.POST("/login", authH.Login)
	r.POST("/logout", middleware.RequireAuth(tm, denylist), authH.Logout)
	r.POST("/scores", middleware.RequireAuth(tm, denylist), lbH.SubmitScore)
	r.GET("/leaderboard/:gameCode/me", middleware.RequireAuth(tm, denylist), lbH.GetMyRank)
	r.GET("/history", middleware.RequireAuth(tm, denylist), lbH.GetMyHistory)
	return r
}

func sendAuth(r *gin.Engine, method, path, token, body string) (int, map[string]any) {
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestRegisterLoginSubmitFlow(t *testing.T) {
	pool, rdb := flowStores(t)
	ctx := context.Background()
	r := flowRouter(pool, rdb)

	name := fmt.Sprintf("flowtest_%d", time.Now().UnixNano())
	email := name + "@example.com"
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE username = $1`, name) })

	creds := fmt.Sprintf(`{"username":%q,"email":%q,"password":"secret-pass-1"}`, name, email)

	// register
	code, body := sendAuth(r, http.MethodPost, "/register", "", creds)
	if code != http.StatusCreated {
		t.Fatalf("register: %d %v", code, body)
	}
	token, _ := body["token"].(string)
	user, _ := body["user"].(map[string]any)
	userID, _ := user["id"].(string)
	if token == "" || userID == "" {
		t.Fatalf("register response missing token or user id: %v", body)
	}

	// registering the same account again is a conflict.
	if code, _ := sendAuth(r, http.MethodPost, "/register", "", creds); code != http.StatusConflict {
		t.Fatalf("duplicate register: got %d, want 409", code)
	}

	// login works. a wrong password and an unknown email give the same message,
	// so it doesn't reveal which emails exist.
	if code, _ := sendAuth(r, http.MethodPost, "/login", "", fmt.Sprintf(`{"email":%q,"password":"secret-pass-1"}`, email)); code != http.StatusOK {
		t.Fatalf("login: got %d, want 200", code)
	}
	codeWrong, bodyWrong := sendAuth(r, http.MethodPost, "/login", "", fmt.Sprintf(`{"email":%q,"password":"wrong-password"}`, email))
	codeNone, bodyNone := sendAuth(r, http.MethodPost, "/login", "", `{"email":"nobody-here@example.com","password":"whatever1"}`)
	if codeWrong != http.StatusUnauthorized || codeNone != http.StatusUnauthorized {
		t.Fatalf("bad login: got %d and %d, want 401 and 401", codeWrong, codeNone)
	}
	if bodyWrong["error"] != bodyNone["error"] {
		t.Fatalf("login errors differ: %v vs %v", bodyWrong["error"], bodyNone["error"])
	}

	// submitting needs a token.
	if code, _ := sendAuth(r, http.MethodPost, "/scores", "", `{"game_code":"speed-run","points":100}`); code != http.StatusUnauthorized {
		t.Fatalf("submit without token: got %d, want 401", code)
	}

	// submit two scores.
	for _, pts := range []int{100, 50} {
		code, body := sendAuth(r, http.MethodPost, "/scores", token, fmt.Sprintf(`{"game_code":"speed-run","points":%d}`, pts))
		if code != http.StatusCreated {
			t.Fatalf("submit %d: %d %v", pts, code, body)
		}
	}

	// Postgres has both rows, Redis has the totals on the game and global boards.
	var rows int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM scores WHERE user_id = $1`, userID).Scan(&rows)
	if rows != 2 {
		t.Errorf("postgres score rows = %d, want 2", rows)
	}
	for _, board := range []string{"speed-run", "global"} {
		got, err := rdb.ZScore(ctx, "leaderboard:"+board, userID).Result()
		if err != nil || got != 150 {
			t.Errorf("redis %s score = %v (err %v), want 150", board, got, err)
		}
	}

	// the global board can't be written to directly.
	if code, _ := sendAuth(r, http.MethodPost, "/scores", token, `{"game_code":"global","points":9999}`); code != http.StatusBadRequest {
		t.Errorf("submit to global: got %d, want 400", code)
	}
	if got, _ := rdb.ZScore(ctx, "leaderboard:global", userID).Result(); got != 150 {
		t.Errorf("global score changed after a rejected submit: %v", got)
	}

	// my rank reads back from Redis.
	code, body = sendAuth(r, http.MethodGet, "/leaderboard/speed-run/me", token, "")
	if code != http.StatusOK || body["rank"] != float64(1) || body["score"] != float64(150) {
		t.Errorf("my rank: %d %v, want rank 1 score 150", code, body)
	}
}

func TestLogoutBlocksOnlyThatToken(t *testing.T) {
	pool, rdb := flowStores(t)
	ctx := context.Background()
	r := flowRouter(pool, rdb)

	name := fmt.Sprintf("flowtest_%d", time.Now().UnixNano())
	email := name + "@example.com"
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE username = $1`, name) })

	creds := fmt.Sprintf(`{"username":%q,"email":%q,"password":"secret-pass-1"}`, name, email)
	if code, body := sendAuth(r, http.MethodPost, "/register", "", creds); code != http.StatusCreated {
		t.Fatalf("register: %d %v", code, body)
	}

	// two logins, like two browsers.
	login := fmt.Sprintf(`{"email":%q,"password":"secret-pass-1"}`, email)
	_, bodyA := sendAuth(r, http.MethodPost, "/login", "", login)
	_, bodyB := sendAuth(r, http.MethodPost, "/login", "", login)
	tokenA, _ := bodyA["token"].(string)
	tokenB, _ := bodyB["token"].(string)
	if tokenA == "" || tokenB == "" || tokenA == tokenB {
		t.Fatalf("expected two different tokens")
	}

	if code, _ := sendAuth(r, http.MethodGet, "/history", tokenA, ""); code != http.StatusOK {
		t.Fatalf("history before logout: got %d, want 200", code)
	}

	// logout with token A.
	if code, _ := sendAuth(r, http.MethodPost, "/logout", tokenA, ""); code != http.StatusNoContent {
		t.Fatalf("logout: got %d, want 204", code)
	}

	// A is dead on every logged in route, B still works.
	if code, _ := sendAuth(r, http.MethodGet, "/history", tokenA, ""); code != http.StatusUnauthorized {
		t.Errorf("history with logged out token: got %d, want 401", code)
	}
	if code, _ := sendAuth(r, http.MethodPost, "/scores", tokenA, `{"game_code":"speed-run","points":10}`); code != http.StatusUnauthorized {
		t.Errorf("submit with logged out token: got %d, want 401", code)
	}
	if code, _ := sendAuth(r, http.MethodGet, "/history", tokenB, ""); code != http.StatusOK {
		t.Errorf("history with the other token: got %d, want 200", code)
	}

	// logging out again with A is refused like any dead token.
	if code, _ := sendAuth(r, http.MethodPost, "/logout", tokenA, ""); code != http.StatusUnauthorized {
		t.Errorf("second logout: got %d, want 401", code)
	}

	// the block is removed by Redis when the token would have expired.
	keys, _ := rdb.Keys(ctx, "leaderboard:revoked:*").Result()
	if len(keys) != 1 {
		t.Fatalf("revoked keys = %v, want exactly 1", keys)
	}
	if ttl := rdb.TTL(ctx, keys[0]).Val(); ttl <= 0 || ttl > time.Hour {
		t.Errorf("revoked key ttl = %v, want between 0 and the 1h token lifetime", ttl)
	}
}
