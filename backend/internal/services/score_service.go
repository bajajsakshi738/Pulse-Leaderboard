package services

import (
	"context"
	"fmt"

	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/google/uuid"
)

// ScoreService writes a score to both stores:
//  1. Postgres: a history row, the source of truth
//  2. Redis: adds to the sorted sets for the live board
//
// Postgres goes first. if it fails we never touch Redis, so Redis can't get
// ahead of the history. if Redis fails after Postgres worked, the score is
// safe but that player's live rank is behind. nothing retries that write;
// RebuildService only runs at startup, and only if Redis lost its data.
type ScoreService struct {
	scoreRepo       *repository.ScoreRepository
	leaderboardRepo *repository.LeaderboardRepository
}

func NewScoreService(scoreRepo *repository.ScoreRepository, leaderboardRepo *repository.LeaderboardRepository) *ScoreService {
	return &ScoreService{
		scoreRepo:       scoreRepo,
		leaderboardRepo: leaderboardRepo,
	}
}

// SubmitScore saves a score and updates the live board.
func (s *ScoreService) SubmitScore(ctx context.Context, userID uuid.UUID, username, gameCode string, points int64) (*models.ScoreEntry, error) {
	entry, err := s.scoreRepo.Insert(ctx, userID, gameCode, points)
	if err != nil {
		return nil, fmt.Errorf("persisting score: %w", err)
	}

	if err := s.leaderboardRepo.IncrementScore(ctx, gameCode, userID.String(), username, points); err != nil {
		// saved in Postgres, but tell the caller Redis failed so the live board may
		// be behind for this user.
		return entry, fmt.Errorf("score saved but live leaderboard update failed: %w", err)
	}

	return entry, nil
}
