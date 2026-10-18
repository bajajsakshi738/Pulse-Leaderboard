package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")
var ErrDuplicateUser = errors.New("username or email already exists")

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

// Create inserts a user and returns the saved row.
func (r *UserRepository) Create(ctx context.Context, username, email, passwordHash string, isBot bool) (*models.User, error) {
	const query = `
		INSERT INTO users (username, email, password_hash, is_bot)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, email, password_hash, is_bot, created_at
	`

	var u models.User
	err := r.db.QueryRow(ctx, query, username, email, passwordHash, isBot).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsBot, &u.CreatedAt,
	)
	if err != nil {
		// 23505 is the Postgres code for a unique violation.
		if isUniqueViolation(err) {
			return nil, ErrDuplicateUser
		}
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	const query = `
		SELECT id, username, email, password_hash, is_bot, created_at
		FROM users WHERE email = $1
	`
	var u models.User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsBot, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("fetching user by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	const query = `
		SELECT id, username, email, password_hash, is_bot, created_at
		FROM users WHERE id = $1
	`
	var u models.User
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsBot, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("fetching user by id: %w", err)
	}
	return &u, nil
}

// isUniqueViolation checks for Postgres error 23505 using pgx's error type.
// matching on the message text would be fragile.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ForEachUsername streams every user's ID and name, to refill the username
// cache after Redis lost its data.
func (r *UserRepository) ForEachUsername(ctx context.Context, fn func(id uuid.UUID, username string) error) error {
	rows, err := r.db.Query(ctx, `SELECT id, username FROM users`)
	if err != nil {
		return fmt.Errorf("querying usernames: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var username string
		if err := rows.Scan(&id, &username); err != nil {
			return fmt.Errorf("scanning username: %w", err)
		}
		if err := fn(id, username); err != nil {
			return err
		}
	}
	return rows.Err()
}
