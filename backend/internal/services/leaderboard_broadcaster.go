package services

import (
	"context"
	"log"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/bajajsakshi738/leaderboard-backend/internal/websocket"
)

// LeaderboardBroadcastMessage is what we send to every client each tick.
type LeaderboardBroadcastMessage struct {
	Type      string      `json:"type"` // "leaderboard_update"
	GameCode  string      `json:"game_code"`
	Entries   interface{} `json:"entries"`
	Timestamp int64       `json:"timestamp"`
}

// LeaderboardBroadcaster reads the top N from Redis on a timer and sends it
// to every client through the hub. a timer keeps the message volume the same
// however many scores come in, and one second still feels real-time.
type LeaderboardBroadcaster struct {
	repo     *repository.LeaderboardRepository
	hub      *websocket.Hub
	interval time.Duration
	topN     int64
	gameCode string
}

func NewLeaderboardBroadcaster(repo *repository.LeaderboardRepository, hub *websocket.Hub, interval time.Duration, topN int64, gameCode string) *LeaderboardBroadcaster {
	return &LeaderboardBroadcaster{
		repo:     repo,
		hub:      hub,
		interval: interval,
		topN:     topN,
		gameCode: gameCode,
	}
}

// Run ticks until ctx is cancelled. start it with `go broadcaster.Run(ctx)`.
func (b *LeaderboardBroadcaster) Run(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if b.hub.ClientCount() == 0 {
				// nobody connected, skip the Redis read.
				continue
			}

			entries, err := b.repo.GetTopN(ctx, b.gameCode, b.topN)
			if err != nil {
				log.Printf("leaderboard broadcaster: failed to fetch top N: %v", err)
				continue
			}

			b.hub.Broadcast(LeaderboardBroadcastMessage{
				Type:      "leaderboard_update",
				GameCode:  b.gameCode,
				Entries:   entries,
				Timestamp: time.Now().Unix(),
			})
		}
	}
}
