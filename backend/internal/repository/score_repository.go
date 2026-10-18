package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ScoreRepository struct {
	db *pgxpool.Pool
}

func NewScoreRepository(db *pgxpool.Pool) *ScoreRepository {
	return &ScoreRepository{db: db}
}

// Insert saves one score as a new row. this is the permanent copy, it
// survives Redis restarts.
func (r *ScoreRepository) Insert(ctx context.Context, userID uuid.UUID, gameCode string, points int64) (*models.ScoreEntry, error) {
	const query = `
		INSERT INTO scores (user_id, game_code, points)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, game_code, points, created_at
	`
	var s models.ScoreEntry
	err := r.db.QueryRow(ctx, query, userID, gameCode, points).Scan(
		&s.ID, &s.UserID, &s.GameCode, &s.Points, &s.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting score: %w", err)
	}
	return &s, nil
}

// GetUserHistory returns a user's latest scores, newest first, optionally for
// one game.
func (r *ScoreRepository) GetUserHistory(ctx context.Context, userID uuid.UUID, gameCode string, limit int) ([]models.ScoreEntry, error) {
	var rows pgx.Rows
	var err error

	if gameCode == "" {
		const query = `
			SELECT id, user_id, game_code, points, created_at
			FROM scores
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		`
		rows, err = r.db.Query(ctx, query, userID, limit)
	} else {
		const query = `
			SELECT id, user_id, game_code, points, created_at
			FROM scores
			WHERE user_id = $1 AND game_code = $2
			ORDER BY created_at DESC
			LIMIT $3
		`
		rows, err = r.db.Query(ctx, query, userID, gameCode, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("querying score history: %w", err)
	}
	defer rows.Close()

	var entries []models.ScoreEntry
	for rows.Next() {
		var s models.ScoreEntry
		if err := rows.Scan(&s.ID, &s.UserID, &s.GameCode, &s.Points, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning score row: %w", err)
		}
		entries = append(entries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating score rows: %w", err)
	}
	return entries, nil
}

// GetTopPlayersReport adds up points and submissions per user in a time
// window. it uses Postgres because it looks at history, not current standing.
func (r *ScoreRepository) GetTopPlayersReport(ctx context.Context, gameCode string, since time.Time, limit int) ([]models.TopPlayerReportRow, error) {
	var rows pgx.Rows
	var err error

	if gameCode == "" || gameCode == "global" {
		const query = `
			SELECT u.id, u.username, 'global' AS game_code,
			       SUM(s.points) AS total_points, COUNT(*) AS submissions
			FROM scores s
			JOIN users u ON u.id = s.user_id
			WHERE s.created_at >= $1
			GROUP BY u.id, u.username
			ORDER BY total_points DESC
			LIMIT $2
		`
		rows, err = r.db.Query(ctx, query, since, limit)
	} else {
		const query = `
			SELECT u.id, u.username, s.game_code,
			       SUM(s.points) AS total_points, COUNT(*) AS submissions
			FROM scores s
			JOIN users u ON u.id = s.user_id
			WHERE s.created_at >= $1 AND s.game_code = $2
			GROUP BY u.id, u.username, s.game_code
			ORDER BY total_points DESC
			LIMIT $3
		`
		rows, err = r.db.Query(ctx, query, since, gameCode, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("querying top players report: %w", err)
	}
	defer rows.Close()

	var report []models.TopPlayerReportRow
	for rows.Next() {
		var row models.TopPlayerReportRow
		var userID uuid.UUID
		if err := rows.Scan(&userID, &row.Username, &row.GameCode, &row.TotalPoints, &row.Submissions); err != nil {
			return nil, fmt.Errorf("scanning report row: %w", err)
		}
		row.UserID = userID.String()
		report = append(report, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating report rows: %w", err)
	}
	return report, nil
}

// ForEachUserGameTotal streams each user's total points per game from the
// score history, for rebuilding the boards. rows with game 'global' are
// skipped, the global board comes from ForEachUserTotal.
func (r *ScoreRepository) ForEachUserGameTotal(ctx context.Context, fn func(userID uuid.UUID, gameCode string, total int64) error) error {
	const query = `
		SELECT user_id, game_code, SUM(points)::bigint
		FROM scores
		WHERE game_code <> 'global'
		GROUP BY user_id, game_code
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("querying per-game totals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID uuid.UUID
		var gameCode string
		var total int64
		if err := rows.Scan(&userID, &gameCode, &total); err != nil {
			return fmt.Errorf("scanning per-game total: %w", err)
		}
		if err := fn(userID, gameCode, total); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ForEachUserTotal streams each user's total across all games (their global
// score).
func (r *ScoreRepository) ForEachUserTotal(ctx context.Context, fn func(userID uuid.UUID, total int64) error) error {
	const query = `
		SELECT user_id, SUM(points)::bigint
		FROM scores
		GROUP BY user_id
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("querying user totals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID uuid.UUID
		var total int64
		if err := rows.Scan(&userID, &total); err != nil {
			return fmt.Errorf("scanning user total: %w", err)
		}
		if err := fn(userID, total); err != nil {
			return err
		}
	}
	return rows.Err()
}
