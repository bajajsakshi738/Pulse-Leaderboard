package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// these need a Postgres with the migrations applied, and a Redis:
//
//	TEST_DATABASE_URL=postgres://leaderboard:leaderboard@localhost:5432/leaderboard?sslmode=disable
//	TEST_REDIS_ADDR=localhost:6379
//
// skipped if either is missing. they add users named "rebuildtest_..." and
// delete them after. Redis database 15 is used and flushed, so don't point
// this at a Redis you care about.
func testStores(t *testing.T) (*pgxpool.Pool, *redis.Client) {
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

func seedUser(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	unique := fmt.Sprintf("rebuildtest_%s_%d", name, time.Now().UnixNano())
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		unique, unique+"@example.com").Scan(&id)
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

func seedScore(t *testing.T, pool *pgxpool.Pool, user uuid.UUID, game string, points int) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO scores (user_id, game_code, points) VALUES ($1, $2, $3)`, user, game, points)
	if err != nil {
		t.Fatalf("seeding score: %v", err)
	}
}

func score(t *testing.T, rdb *redis.Client, board string, user uuid.UUID) float64 {
	t.Helper()
	v, err := rdb.ZScore(context.Background(), "leaderboard:"+board, user.String()).Result()
	if err != nil {
		t.Fatalf("ZSCORE %s %s: %v", board, user, err)
	}
	return v
}

func TestRebuildRestoresBoardsAndUsernamesFromPostgres(t *testing.T) {
	pool, rdb := testStores(t)
	ctx := context.Background()

	alice := seedUser(t, pool, "alice")
	bob := seedUser(t, pool, "bob")
	seedScore(t, pool, alice, "speed-run", 100)
	seedScore(t, pool, alice, "speed-run", 50)
	seedScore(t, pool, alice, "trivia", 200)
	seedScore(t, pool, bob, "puzzle", 300)
	// an old row written straight to 'global' counts toward the global total but
	// must not make a per game board entry.
	seedScore(t, pool, alice, "global", 7)

	scoreRepo := repository.NewScoreRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	boards := repository.NewLeaderboardRepository(rdb)
	svc := NewRebuildService(scoreRepo, userRepo, boards)

	// Redis starts with wrong data: a stale member and a wrong score.
	rdb.ZAdd(ctx, "leaderboard:global", redis.Z{Score: 999999, Member: "stale-member"})
	rdb.ZAdd(ctx, "leaderboard:global", redis.Z{Score: 1, Member: alice.String()})

	check := func(label string) {
		t.Helper()
		if got := score(t, rdb, "speed-run", alice); got != 150 {
			t.Errorf("%s: alice speed-run = %v, want 150", label, got)
		}
		if got := score(t, rdb, "trivia", alice); got != 200 {
			t.Errorf("%s: alice trivia = %v, want 200", label, got)
		}
		if got := score(t, rdb, "puzzle", bob); got != 300 {
			t.Errorf("%s: bob puzzle = %v, want 300", label, got)
		}
		if got := score(t, rdb, "global", alice); got != 357 {
			t.Errorf("%s: alice global = %v, want 357 (150+200+7)", label, got)
		}
		if got := score(t, rdb, "global", bob); got != 300 {
			t.Errorf("%s: bob global = %v, want 300", label, got)
		}
		if _, err := rdb.ZScore(ctx, "leaderboard:global", "stale-member").Result(); err != redis.Nil {
			t.Errorf("%s: stale member should be gone, got err=%v", label, err)
		}
		name, err := boards.GetUsername(ctx, alice.String())
		if err != nil || name == "Unknown" || name == "" {
			t.Errorf("%s: alice username not restored: %q, %v", label, name, err)
		}
	}

	rebuilt, stats, err := svc.RebuildIfNeeded(ctx)
	if err != nil || !rebuilt {
		t.Fatalf("first RebuildIfNeeded: rebuilt=%v err=%v", rebuilt, err)
	}
	if stats.BoardEntries == 0 || stats.Usernames == 0 {
		t.Errorf("stats look empty: %+v", stats)
	}
	check("after first rebuild")

	// already in sync, nothing to do.
	if rebuilt, _, err := svc.RebuildIfNeeded(ctx); err != nil || rebuilt {
		t.Fatalf("second RebuildIfNeeded: rebuilt=%v err=%v, want false/nil", rebuilt, err)
	}

	// pretend Redis lost everything.
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := rdb.ZScore(ctx, "leaderboard:global", alice.String()).Result(); err != redis.Nil {
		t.Fatalf("flush did not empty Redis: %v", err)
	}
	rebuilt, _, err = svc.RebuildIfNeeded(ctx)
	if err != nil || !rebuilt {
		t.Fatalf("RebuildIfNeeded after wipe: rebuilt=%v err=%v", rebuilt, err)
	}
	check("after wipe")
}

func TestRebuildSkipsWhenAnotherInstanceHoldsTheLock(t *testing.T) {
	pool, rdb := testStores(t)
	ctx := context.Background()

	boards := repository.NewLeaderboardRepository(rdb)
	svc := NewRebuildService(repository.NewScoreRepository(pool), repository.NewUserRepository(pool), boards)

	if ok, err := boards.TryLockRebuild(ctx); err != nil || !ok {
		t.Fatalf("could not take the lock for the test: ok=%v err=%v", ok, err)
	}
	rebuilt, _, err := svc.RebuildIfNeeded(ctx)
	if err != nil || rebuilt {
		t.Fatalf("rebuilt=%v err=%v, want false/nil while the lock is held", rebuilt, err)
	}
	if synced, _ := boards.IsSynced(ctx); synced {
		t.Fatal("must not mark synced when it skipped the rebuild")
	}
}
