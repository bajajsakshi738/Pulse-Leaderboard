package services

import (
	"context"
	"fmt"
	"log"

	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/google/uuid"
)

// RebuildService restores the Redis boards from Postgres.
//
// Redis can lose its data (restart, flush, new container). then the boards
// are empty and names show as "Unknown". this adds up the score history in
// Postgres and writes the totals back.
type RebuildService struct {
	scores *repository.ScoreRepository
	users  *repository.UserRepository
	boards *repository.LeaderboardRepository
}

func NewRebuildService(scores *repository.ScoreRepository, users *repository.UserRepository, boards *repository.LeaderboardRepository) *RebuildService {
	return &RebuildService{scores: scores, users: users, boards: boards}
}

// RebuildStats counts what a rebuild wrote.
type RebuildStats struct {
	BoardEntries int
	Usernames    int
}

// RebuildIfNeeded rebuilds only if the "synced" marker is missing: first
// start, or Redis lost its data. if several servers start together, one takes
// the lock and the rest skip.
//
// limit: a score submitted at the exact moment of a rebuild can be missed,
// because we write absolute totals. it runs before this instance serves
// traffic so the window is small, and the score is still safe in Postgres.
func (s *RebuildService) RebuildIfNeeded(ctx context.Context) (bool, RebuildStats, error) {
	synced, err := s.boards.IsSynced(ctx)
	if err != nil {
		return false, RebuildStats{}, err
	}
	if synced {
		return false, RebuildStats{}, nil
	}

	locked, err := s.boards.TryLockRebuild(ctx)
	if err != nil {
		return false, RebuildStats{}, err
	}
	if !locked {
		log.Println("rebuild: another instance is rebuilding; skipping")
		return false, RebuildStats{}, nil
	}
	defer func() { _ = s.boards.UnlockRebuild(context.Background()) }()

	// check again with the lock held, another instance may have just finished.
	if synced, err := s.boards.IsSynced(ctx); err != nil {
		return false, RebuildStats{}, err
	} else if synced {
		return false, RebuildStats{}, nil
	}

	stats, err := s.Rebuild(ctx)
	if err != nil {
		return false, stats, err
	}
	if err := s.boards.MarkSynced(ctx); err != nil {
		return true, stats, err
	}
	return true, stats, nil
}

// Rebuild always rebuilds, ignoring the marker.
func (s *RebuildService) Rebuild(ctx context.Context) (RebuildStats, error) {
	var stats RebuildStats

	loader, err := s.boards.NewBoardLoader(ctx)
	if err != nil {
		return stats, err
	}

	// per game boards
	err = s.scores.ForEachUserGameTotal(ctx, func(userID uuid.UUID, gameCode string, total int64) error {
		stats.BoardEntries++
		return loader.AddScore(ctx, gameCode, userID.String(), total)
	})
	if err != nil {
		return stats, fmt.Errorf("rebuilding game boards: %w", err)
	}

	// global board: each user's total across all games
	err = s.scores.ForEachUserTotal(ctx, func(userID uuid.UUID, total int64) error {
		stats.BoardEntries++
		return loader.AddScore(ctx, "global", userID.String(), total)
	})
	if err != nil {
		return stats, fmt.Errorf("rebuilding global board: %w", err)
	}

	// display names
	err = s.users.ForEachUsername(ctx, func(id uuid.UUID, username string) error {
		stats.Usernames++
		return loader.SetUsername(ctx, id.String(), username)
	})
	if err != nil {
		return stats, fmt.Errorf("rebuilding usernames: %w", err)
	}

	if err := loader.Commit(ctx); err != nil {
		return stats, err
	}
	return stats, nil
}
