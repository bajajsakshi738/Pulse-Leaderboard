package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const denylistPrefix = "leaderboard:revoked:"

// RevocationChecker tells RequireAuth if a token was logged out.
type RevocationChecker interface {
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

// Denylist keeps the IDs of tokens that were logged out. each entry is deleted
// by Redis when the token would have expired anyway, so the list stays small.
type Denylist struct {
	rdb *redis.Client
}

func NewDenylist(rdb *redis.Client) *Denylist {
	return &Denylist{rdb: rdb}
}

// Revoke blocks the token until its own expiry time.
func (d *Denylist) Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // already expired, nothing to block
	}
	return d.rdb.Set(ctx, denylistPrefix+tokenID, "1", ttl).Err()
}

// IsRevoked says if the token was logged out.
func (d *Denylist) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	n, err := d.rdb.Exists(ctx, denylistPrefix+tokenID).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
