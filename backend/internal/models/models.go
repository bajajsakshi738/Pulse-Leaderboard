package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered account.
type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	IsBot        bool      `json:"is_bot"`
}

// PublicUser is the safe-to-expose representation of a User.
type PublicUser struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func (u *User) ToPublic() PublicUser {
	return PublicUser{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

// Game is something scores belong to. 'global' is reserved for the combined board.
type Game struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ScoreEntry is one score submission. rows are never changed, each
// submission is a new row, which is what the reports use.
type ScoreEntry struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	GameCode  string    `json:"game_code"`
	Points    int64     `json:"points"`
	CreatedAt time.Time `json:"created_at"`
}

// LeaderboardEntry is one row of a ranked board.
type LeaderboardEntry struct {
	Rank     int64  `json:"rank"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Score    int64  `json:"score"`
}

// RankResponse is returned when a specific user asks for their own rank.
type RankResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Score    int64  `json:"score"`
	Rank     int64  `json:"rank"` // 1-based; -1 if not ranked yet
}

// TopPlayerReportRow is one row of the top players report (from Postgres).
type TopPlayerReportRow struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	GameCode    string `json:"game_code"`
	TotalPoints int64  `json:"total_points"`
	Submissions int64  `json:"submissions"`
}
