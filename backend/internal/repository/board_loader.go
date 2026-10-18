package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/redis/go-redis/v9"
)

const (
	// set once the boards are rebuilt from Postgres. if Redis is wiped this key
	// goes too, which is how we know a rebuild is needed.
	syncedKey = "leaderboard:meta:synced"
	// stops two server instances rebuilding at the same time.
	rebuildLockKey = "leaderboard:meta:rebuild-lock"
	rebuildLockTTL = 2 * time.Minute

	loaderBatchSize = 1000
)

// IsSynced is true if the boards were already rebuilt (or never lost).
func (r *LeaderboardRepository) IsSynced(ctx context.Context) (bool, error) {
	n, err := r.rdb.Exists(ctx, syncedKey).Result()
	if err != nil {
		return false, fmt.Errorf("checking synced marker: %w", err)
	}
	return n > 0, nil
}

// MarkSynced records that the boards match Postgres.
func (r *LeaderboardRepository) MarkSynced(ctx context.Context) error {
	return r.rdb.Set(ctx, syncedKey, time.Now().UTC().Format(time.RFC3339), 0).Err()
}

// TryLockRebuild takes the lock, or returns false if another instance has
// it. the lock expires by itself, so a crash can't block it forever.
func (r *LeaderboardRepository) TryLockRebuild(ctx context.Context) (bool, error) {
	ok, err := r.rdb.SetNX(ctx, rebuildLockKey, "1", rebuildLockTTL).Result()
	if err != nil {
		return false, fmt.Errorf("taking rebuild lock: %w", err)
	}
	return ok, nil
}

// UnlockRebuild releases the lock.
func (r *LeaderboardRepository) UnlockRebuild(ctx context.Context) error {
	return r.rdb.Del(ctx, rebuildLockKey).Err()
}

func tempBoardKey(gameCode string) string {
	return leaderboardKey(gameCode) + ":rebuild-tmp"
}

// BoardLoader rebuilds the boards from scratch. it writes to temporary keys
// and swaps each one in with RENAME, so nobody sees a half built board and
// old members that are not in Postgres don't stay.
type BoardLoader struct {
	rdb     *redis.Client
	pipe    redis.Pipeliner
	queued  int
	touched map[string]bool
}

// NewBoardLoader starts a rebuild and clears temp keys left by a crashed one.
func (r *LeaderboardRepository) NewBoardLoader(ctx context.Context) (*BoardLoader, error) {
	for _, code := range models.AllBoardCodes() {
		if err := r.rdb.Del(ctx, tempBoardKey(code)).Err(); err != nil {
			return nil, fmt.Errorf("clearing temporary board: %w", err)
		}
	}
	return &BoardLoader{rdb: r.rdb, pipe: r.rdb.Pipeline(), touched: map[string]bool{}}, nil
}

// AddScore sets a user's score on a board.
func (l *BoardLoader) AddScore(ctx context.Context, gameCode, userID string, score int64) error {
	l.pipe.ZAdd(ctx, tempBoardKey(gameCode), redis.Z{Score: float64(score), Member: userID})
	l.touched[gameCode] = true
	return l.queue(ctx)
}

// SetUsername refills one name in the username cache.
func (l *BoardLoader) SetUsername(ctx context.Context, userID, username string) error {
	l.pipe.Set(ctx, usernameKey(userID), username, 0)
	return l.queue(ctx)
}

func (l *BoardLoader) queue(ctx context.Context) error {
	l.queued++
	if l.queued < loaderBatchSize {
		return nil
	}
	return l.flush(ctx)
}

func (l *BoardLoader) flush(ctx context.Context) error {
	if l.queued == 0 {
		return nil
	}
	_, err := l.pipe.Exec(ctx)
	l.queued = 0
	if err != nil {
		return fmt.Errorf("writing rebuild batch: %w", err)
	}
	return nil
}

// Commit swaps the built boards in. boards with no scores are emptied.
func (l *BoardLoader) Commit(ctx context.Context) error {
	if err := l.flush(ctx); err != nil {
		return err
	}
	for _, code := range models.AllBoardCodes() {
		if l.touched[code] {
			if err := l.rdb.Rename(ctx, tempBoardKey(code), leaderboardKey(code)).Err(); err != nil {
				return fmt.Errorf("swapping board %q into place: %w", code, err)
			}
			continue
		}
		if err := l.rdb.Del(ctx, leaderboardKey(code)).Err(); err != nil {
			return fmt.Errorf("emptying board %q: %w", code, err)
		}
	}
	return nil
}
