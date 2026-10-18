# Pulse — Real-Time Leaderboard System

A full-stack, real-time leaderboard built to demonstrate a production-shaped
architecture: Go + Gin API, Redis sorted sets for live ranking, Postgres for
durable history, a WebSocket push channel, and an Angular frontend whose
Global board updates live as scores come in.

This is a side project / portfolio piece — there's a built-in fake user
generator so the board has live traffic without needing real users.

```
leaderboard-system/
├── backend/     Go API, WebSocket hub, fake score generator
└── frontend/    Angular 18 (standalone components, signals), Space Grotesk font
```

---

## Architecture

```
                +-------------+        +--------------------+
   Angular  --->|   Go API    |------->|   Postgres          |
   (REST)       |  (Gin)      | write  |  (score history,    |
                |             |        |   users)            |
                |             |        +--------------------+
                |             |
                |             |------->+--------------------+
                |             | ZSET   |   Redis             |
                |             | ops    |  (live leaderboard)  |
                |             |        +--------------------+
                |             |
   Angular  <---|  WebSocket  | (broadcasts top-N every 1s,
   (live push)  |  Hub        |  read from Redis)
                +-------------+
                       ^
                       | HTTP (same API as real users)
                +------+------+
                |  Generator   |  500 simulated bot users,
                |  worker      |  submitting scores every tick
                +-------------+
```

**Why two databases?** Redis ZSETs give O(log N) rank queries and top-N
reads — fast enough for thousands of concurrent score updates — but Redis
data can be flushed or evicted. Postgres holds the immutable, durable
history of every score ever submitted, which is what powers the "top
players over the last week" report. Every score write goes to Postgres
first; Redis is only ever a fast copy on top of it.

**What if Redis loses its data?** When the server starts, it checks a
"synced" marker in Redis. If the marker is missing (first start, or Redis
was wiped), it adds up every user's points from Postgres, writes the boards
and the display names back to Redis, and sets the marker. The new boards are
built under temporary keys and swapped in, so nobody sees a half-built board.
Limits: this runs only at startup, and it does not repair a single score whose
Redis write failed while the server was running (that score is safe in
Postgres, but that player's live rank stays behind until the next rebuild).

**Why WebSockets instead of polling?** The frontend opens one socket and
the server pushes a fresh top-20 snapshot every second (only when at least
one client is connected, to avoid pointless Redis reads). This is what
makes the "auto-shuffle in real time" effect work without the browser
hammering the API.

---

## Running locally

### Prerequisites
- Go 1.22+
- Node.js 18+ and npm
- Docker (for Postgres + Redis, or run them natively if you prefer)

### 1. Start Postgres + Redis

```bash
cd backend
docker compose up -d postgres redis
```

### 2. Configure and run the backend

```bash
cd backend
cp .env.example .env       # then set JWT_SECRET, GENERATOR_BOT_PASSWORD and
                            # RATE_LIMIT_BYPASS_TOKEN to your own random values
go mod tidy                 # resolves dependencies and creates go.sum (commit it)
go run ./cmd/server         # starts the API on :8080 (same as `make run`)
```

The tables are created for you: the Postgres container runs everything in
`migrations/` the first time it starts. You only need to apply them by hand
(`make migrate`, which needs the `psql` tool) if you point the API at a
database that already existed. To start over with an empty database, run
`docker compose down -v` and then `docker compose up -d postgres redis` again;
this also deletes the bot accounts, which matters if you change
`GENERATOR_BOT_PASSWORD` (see below).

Verify it's up: `curl http://localhost:8080/health` should return `{"status":"ok"}`

### 3. Run the fake score generator (optional, for live demo traffic)

In a second terminal:

```bash
cd backend
API_BASE_URL=http://localhost:8080 make run-generator
```

If the generator logs `login returned status 401`, the bots already exist in
the database with a different password than the one in your `.env`. Either put
the old password back or reset the database as described above.

This registers 500 bot users (first run only — subsequent runs log them
in). Every second, 3 random bots each submit a score, so 3 scores per
second, about 260,000 rows a day. That keeps the board moving, but it still
fills the `scores` table steadily, so watch the size of a free database.
Stop the generator (Ctrl+C locally, Suspend on Render) when you are not
demoing, or lower `GENERATOR_ACTIVE_PER_TICK` / raise `GENERATOR_TICK_INTERVAL`.

The generator reads `backend/.env`. It needs the same
`RATE_LIMIT_BYPASS_TOKEN` as the server: all 500 bots share one IP
address, so without the token the server's login limit (10 per minute)
would block most of them with `429`.

### 4. Run the frontend

```bash
cd frontend
npm install
npm start
```

Visit `http://localhost:4200`.

---

## Deploying live

> **Not yet verified.** These steps and `backend/render.yaml` have not been
> run end to end. Treat them as a starting point: the Render blueprint paths
> may need changing for this repo layout (`backend/` and `frontend/` folders),
> and you may prefer a hosted Postgres elsewhere (for example Supabase) over
> Render's.

### Backend to Render

1. Push this repo to GitHub.
2. In the Render dashboard: **New +** then **Blueprint**, point it at the
   repo. Render reads `backend/render.yaml` and provisions:
   - `leaderboard-api` (the Go API, Docker-built)
   - `leaderboard-generator` (the fake score generator, as a background
     worker)
   - `leaderboard-postgres` (managed Postgres, free tier)
   - `leaderboard-redis` (managed Redis, free tier)
3. After the first deploy, run the migration against the live database.
   Render shows the external connection string on the Postgres service
   page:
   ```bash
   psql "<external-connection-string-from-render>" -f backend/migrations/0001_init.sql
   ```
4. Open the `leaderboard-generator` service and set `API_BASE_URL` to
   your deployed API's public URL (e.g. `https://leaderboard-api.onrender.com`).
   Render can't auto-resolve this for a Docker worker, so it's set
   manually in `render.yaml` — there's a comment there explaining why.
5. Update `ALLOWED_ORIGINS` on `leaderboard-api` to your deployed frontend
   URL once you have it (next section).

> **Free tier note:** Render's free web services spin down after periods
> of inactivity and take 30 to 60 seconds to wake on the next request.
> The first load after idling will be slow. That's expected, not a bug.

### Frontend to Vercel

1. Update `frontend/src/environments/environment.prod.ts` with your real
   Render API URL (both `apiBaseUrl` and `wsUrl`, note `wss://` not
   `ws://` since Render serves HTTPS).
2. In Vercel: **New Project**, import the repo, set **root directory**
   to `frontend`. Vercel reads `frontend/vercel.json` for the build
   command and output directory automatically.
3. Deploy. Then go back to Render and set `leaderboard-api`'s
   `ALLOWED_ORIGINS` env var to your Vercel URL (e.g.
   `https://pulse-leaderboard.vercel.app`), so CORS allows the live
   frontend to call the API.

---

## Frontend pages

| Route | Page | Needs login |
|---|---|---|
| `/leaderboard/:game` | Live board (`global`, `speed-run`, `trivia`, `puzzle`) | no |
| `/submit` | Submit a score and see your rank change | yes |
| `/profile` | My scores: ranks per board and submission history | yes |
| `/reports` | Top players by period, from the Postgres history | no |
| `/login`, `/register` | Auth | no |

## API reference (quick)

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/auth/register` | none (rate limited) | Create account, returns JWT |
| POST | `/api/auth/login` | none (rate limited) | Log in, returns JWT |
| GET | `/api/auth/me` | required | Current user from JWT |
| POST | `/api/auth/logout` | required | Blocks the token that sent the request (returns 204) |
| GET | `/api/leaderboard/:gameCode/top?n=10` | none | Top N entries |
| GET | `/api/leaderboard/:gameCode/me` | required | Your rank on a board |
| GET | `/api/leaderboard/:gameCode/rank/:userID` | none | Anyone's rank |
| POST | `/api/scores` | required (rate limited) | Submit a score `{game_code, points}` |
| GET | `/api/scores/history?game_code=&limit=` | required | Your score history |
| GET | `/api/reports/top-players?period=week&game_code=&limit=` | none | Postgres-aggregated report |
| GET | `/ws/leaderboard` | none | WebSocket, streams `leaderboard_update` every second |

Game codes seeded by the migration: `global`, `speed-run`, `trivia`, `puzzle`.

---

## Security and abuse protection

| Protection | How it works |
|---|---|
| Passwords | Hashed with bcrypt; never stored or logged in plain text |
| SQL injection | Every query uses parameter placeholders (`$1`, `$2`, ...) |
| Authentication | Signed JWT (HS256); the signing secret exists only on the server |
| Score limits | 1 to 10,000 points per submission, checked in the API and by a database constraint |
| Input checks | Scores go only to the real games (`speed-run`, `trivia`, `puzzle`); the global board cannot be written to directly; unknown games on read endpoints get `404`; passwords are limited to 72 bytes (what bcrypt can use); request bodies are capped at 16 KiB |
| Error messages | Validation errors return short plain messages, never the raw validator text |
| Rate limiting | Counters in Redis, so the limit holds across several server instances (see below) |
| CORS | Only origins in `ALLOWED_ORIGINS` may call the API from a browser |
| WebSocket origin | The hub only accepts handshakes from `ALLOWED_ORIGINS`; other websites get `403` |
| Secrets | Read from environment variables; `.env` is git-ignored |

**Rate limits**

| Endpoint | Limit | Counted per |
|---|---|---|
| `POST /api/auth/register`, `POST /api/auth/login` | 10 per minute (one shared budget) | client IP |
| `POST /api/scores` | 1 per 3 seconds | user |

When a limit is hit the API answers `429 Too Many Requests` with a
`Retry-After` header (seconds). Reads such as the leaderboard are not limited.

- The bot generator skips the limits by sending the secret
  `RATE_LIMIT_BYPASS_TOKEN` in the `X-RateLimit-Bypass` header. If the token
  is empty on the server, the bypass is switched off entirely.
- `TRUST_PROXY_HEADER` must be `true` only behind a reverse proxy such as
  Render. It makes the limiter use the last `X-Forwarded-For` entry (the one
  the proxy added). Locally it stays `false`, so a client cannot dodge the
  limit by faking that header. If it is `false` behind a proxy, all users
  share the proxy's IP and one budget.
- If Redis is unreachable, score submits are let through (an error is
  logged). Login and register are refused with `503` instead: with the
  counters unreadable there is no brute-force protection, so it is safer to
  turn those requests away until Redis is back. The cost is that nobody can
  log in during a Redis outage.
- Fixed-window counters allow a short burst at a window boundary. A sliding
  window or token bucket would be smoother and is a natural upgrade.

**Known limitations (see also "Known limitations" at the end):** the JWT is
kept in `localStorage`, there is no refresh-token rotation, a logged-out token
keeps working if Redis is down (see below), and the WebSocket origin check cannot stop a non-browser client that
sends no (or a forged) `Origin` header, which is fine here because the socket
only broadcasts public leaderboard data.

---

## Testing

```bash
cd backend
go test ./...                 # runs without any services; some tests skip themselves
```

Some tests need a real Postgres (with the migrations applied) and Redis. Start
them with `docker compose up -d postgres redis`, then:

```bash
export TEST_DATABASE_URL="postgres://leaderboard:leaderboard@localhost:5432/leaderboard?sslmode=disable"
export TEST_REDIS_ADDR="localhost:6379"
go test -race -count=1 ./...
```

These tests use Redis database 15 and **flush it**, and they create and delete
their own users (names start with `flowtest_` or `rebuildtest_`). Do not point
`TEST_REDIS_ADDR` at a Redis you care about.

| Area | What is covered |
|---|---|
| Input validation | Bad register, login and score requests give the right status and a clean message; unknown games are rejected; the global board cannot be written to |
| Register, login, submit | Full flow against real storage: duplicate account, same error for wrong password and unknown email, scores land in Postgres and in both Redis boards |
| Redis rebuild | Boards and usernames come back after a wipe; stale members disappear; a held lock makes a second instance skip |
| Logout | A logged-out token is refused on every logged-in route, another token of the same user still works, the block expires with the token, old tokens without an ID still work, logout says 503 when Redis is down, the check is skipped when Redis is down |
| Rate limiter | Limits, recovery, bypass token, per-user keys, fail-open and fail-closed when Redis is down |
| WebSocket origin | Matching rules and a real handshake |

Not covered: the frontend (no unit tests; it was checked by hand in a browser)
and the WebSocket broadcaster. GitHub Actions (`.github/workflows/ci.yml`)
runs the Go tests with the race detector and builds the frontend on every push.
It needs `backend/go.sum` to be committed.

---

## Design decisions worth knowing

- **Scores are cumulative.** A board shows each player's running total
  (`ZINCRBY`), not their best single score. For a best-score board you would
  use `ZADD ... GT` instead. The history table still keeps every submission.
- **The live board updates on a one-second timer, not on every score.** One
  snapshot per second costs the same however many scores arrive, while pushing
  on every change would flood clients when traffic is high. For a person
  watching a leaderboard, one second reads as real-time.
- **Postgres first, then Redis.** If Postgres fails, Redis is never touched, so
  Redis can never be ahead of the history. The reverse failure (Postgres
  succeeds, Redis fails) leaves a stale live rank, never a lost score.
- **The hub never waits for a slow WebSocket client.** Each client has its own
  queue of 16 messages. If the queue is full, that client misses that update
  and everyone else still gets theirs. A client that stops answering pings
  (60 seconds) or whose send takes longer than 10 seconds is disconnected.

---

## What's intentionally simple, and why

- **Auth is plain JWT, no refresh tokens.** Fine for a demo; for
  production you'd add refresh-token rotation and shorter access-token
  TTLs.
- **The generator talks to the public API, not the database directly.**
  It exercises the exact same code path a real client would, which is
  more representative of real load than a script that writes straight to
  Postgres or Redis.
- **The broadcaster only streams the `global` board.** The per-game boards
  are loaded over REST when you open them and do not update by themselves,
  to keep the WebSocket payload simple. Swapping this for one socket per game, or a
  `game_code` subscribe message, is a natural next step if you want fully
  live per-game boards too.

---

## Known limitations

- **No load test.** There are no numbers for concurrent connections or
  latency, so none are claimed.
- **Redis rebuild runs only at startup**, and only when the "synced" marker is
  missing. A single failed Redis write while running is not retried.
- **A score submitted at the exact moment of a rebuild can be missed by it**
  (it stays safe in Postgres). The rebuild runs before the server starts
  serving, so the window is small.
- **Ties** between equal scores are ordered by user ID (Redis default), not by
  who reached the score first.
- **Only the Global board updates live.** The three game boards load when
  opened.
- **No live daily or weekly boards.** The Top players page covers 24 hours,
  7 days and 30 days, but it is calculated from Postgres each time.
- **Scores come from the client and the server does not verify them.** Anyone
  with an account can send up to 10,000 points every 3 seconds, so the
  leaderboard can be cheated. The rate limit and the per-submit cap only limit
  the damage. A proper fix is server-authoritative scoring, where the server
  works out the score from what the player did and never accepts a number from
  the client. That needs a real game running on the server, which this project
  does not have.
- **Public read endpoints are not rate limited.** `GET /api/leaderboard/:game/top`,
  `GET /api/leaderboard/:game/rank/:userID` and `GET /api/reports/top-players`
  can be called without a login and as often as a client likes. The reports
  endpoint queries Postgres on every call, so it is the easiest one to overload.
  Only login, register and score submission are limited.
- **No limit on WebSocket connections.** One client can open many.
- **No rank-around-me view**, no metrics, and logging is plain text.
- **Migrations are plain SQL files** that Docker applies on first start, not
  managed by a migration tool.
- **The JWT is stored in `localStorage`** and has no refresh tokens. It lasts
  72 hours by default (`JWT_EXPIRY_HOURS`), which is long.
- **Logout only blocks the token that sent it.** Each token has an ID, and
  `POST /api/auth/logout` saves that ID in Redis until the token would have
  expired. Other tokens of the same user (another browser) keep working. If
  Redis is down, the check is skipped so score submits keep working, so a
  logged-out token works again until it expires. Tokens made before this
  feature have no ID and cannot be blocked. Every logged-in request now makes
  one extra Redis lookup.
- **Fixed-window rate limiting** allows a short burst at a window boundary.
- **The deploy files are untested.** The steps in "Deploying live" have not
  been run end to end, and `render.yaml` paths may need adjusting for this
  folder layout. Expect a round of fixes on the first deploy.
