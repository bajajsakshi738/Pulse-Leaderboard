-- 0001_init.sql
-- Initial schema for the leaderboard system.
-- Run this manually, or via the migrate target in the Makefile,
-- against the Postgres instance pointed to by DATABASE_URL.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username      VARCHAR(50)  NOT NULL UNIQUE,
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,
    is_bot        BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS games (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code       VARCHAR(50)  NOT NULL UNIQUE,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- "global" is a reserved game code representing the aggregate
-- leaderboard across all activities.
INSERT INTO games (code, name)
VALUES
    ('global', 'Global Leaderboard'),
    ('speed-run', 'Speed Run Challenge'),
    ('trivia', 'Trivia Clash'),
    ('puzzle', 'Puzzle Master')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS scores (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_code  VARCHAR(50) NOT NULL REFERENCES games(code) ON DELETE RESTRICT,
    points     BIGINT NOT NULL CHECK (points > 0 AND points <= 10000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_scores_user_id ON scores (user_id);
CREATE INDEX IF NOT EXISTS idx_scores_game_code ON scores (game_code);
CREATE INDEX IF NOT EXISTS idx_scores_created_at ON scores (created_at);
CREATE INDEX IF NOT EXISTS idx_scores_game_created ON scores (game_code, created_at);
