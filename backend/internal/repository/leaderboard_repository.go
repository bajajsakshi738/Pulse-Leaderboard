package repository

import (
	"context"
	"fmt"

	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/redis/go-redis/v9"
)

// LeaderboardRepository holds the Redis sorted set operations.
//
// keys:
//
//	leaderboard:<gameCode>        -> ZSET, member = userID, score = points
//	leaderboard:username:<userID> -> STRING, display name
//
// a sorted set makes adding points, getting the top N and finding a rank all
// O(log N), with no sorting in our code.
type LeaderboardRepository struct {
	rdb *redis.Client
}

func NewLeaderboardRepository(rdb *redis.Client) *LeaderboardRepository {
	return &LeaderboardRepository{rdb: rdb}
}

func leaderboardKey(gameCode string) string {
	return fmt.Sprintf("leaderboard:%s", gameCode)
}

func usernameKey(userID string) string {
	return fmt.Sprintf("leaderboard:username:%s", userID)
}

// IncrementScore adds points to the user's score on the game board and on
// the global board, and caches the username so reads don't need Postgres.
func (r *LeaderboardRepository) IncrementScore(ctx context.Context, gameCode, userID, username string, delta int64) error {
	pipe := r.rdb.Pipeline()

	pipe.ZIncrBy(ctx, leaderboardKey(gameCode), float64(delta), userID)
	if gameCode != "global" {
		pipe.ZIncrBy(ctx, leaderboardKey("global"), float64(delta), userID)
	}
	pipe.Set(ctx, usernameKey(userID), username, 0)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("incrementing score: %w", err)
	}
	return nil
}

// SetUsername caches the display name without touching the score.
func (r *LeaderboardRepository) SetUsername(ctx context.Context, userID, username string) error {
	return r.rdb.Set(ctx, usernameKey(userID), username, 0).Err()
}

// GetUsername returns the cached name, or "Unknown" if it isn't cached.
func (r *LeaderboardRepository) GetUsername(ctx context.Context, userID string) (string, error) {
	val, err := r.rdb.Get(ctx, usernameKey(userID)).Result()
	if err == redis.Nil {
		return "Unknown", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting username: %w", err)
	}
	return val, nil
}

// GetTopN returns the top N entries, highest score first (ZREVRANGE).
func (r *LeaderboardRepository) GetTopN(ctx context.Context, gameCode string, n int64) ([]models.LeaderboardEntry, error) {
	results, err := r.rdb.ZRevRangeWithScores(ctx, leaderboardKey(gameCode), 0, n-1).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching top %d: %w", n, err)
	}

	entries := make([]models.LeaderboardEntry, 0, len(results))
	for i, z := range results {
		userID := fmt.Sprintf("%v", z.Member)
		username, _ := r.GetUsername(ctx, userID)
		entries = append(entries, models.LeaderboardEntry{
			Rank:     int64(i) + 1,
			UserID:   userID,
			Username: username,
			Score:    int64(z.Score),
		})
	}
	return entries, nil
}

// GetUserRank returns a user's rank and score. Redis ranks start at 0, we
// return 1-based. Rank is -1 if the user has no score yet.
func (r *LeaderboardRepository) GetUserRank(ctx context.Context, gameCode, userID, username string) (models.RankResponse, error) {
	pipe := r.rdb.Pipeline()
	rankCmd := pipe.ZRevRank(ctx, leaderboardKey(gameCode), userID)
	scoreCmd := pipe.ZScore(ctx, leaderboardKey(gameCode), userID)
	_, err := pipe.Exec(ctx)

	resp := models.RankResponse{
		UserID:   userID,
		Username: username,
		Rank:     -1,
		Score:    0,
	}

	if err != nil && err != redis.Nil {
		return resp, fmt.Errorf("fetching user rank: %w", err)
	}

	rank, rankErr := rankCmd.Result()
	score, scoreErr := scoreCmd.Result()

	if rankErr == redis.Nil || scoreErr == redis.Nil {
		// no score yet on this board.
		return resp, nil
	}
	if rankErr != nil {
		return resp, fmt.Errorf("reading rank: %w", rankErr)
	}
	if scoreErr != nil {
		return resp, fmt.Errorf("reading score: %w", scoreErr)
	}

	resp.Rank = rank + 1
	resp.Score = int64(score)
	return resp, nil
}

// CountRanked is how many users have a score on the board.
func (r *LeaderboardRepository) CountRanked(ctx context.Context, gameCode string) (int64, error) {
	count, err := r.rdb.ZCard(ctx, leaderboardKey(gameCode)).Result()
	if err != nil {
		return 0, fmt.Errorf("counting ranked members: %w", err)
	}
	return count, nil
}
