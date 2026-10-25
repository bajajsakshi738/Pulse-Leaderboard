# Pulse

A real-time leaderboard I built to learn how live rankings work. Players submit scores and the top 20 on the Global board updates in the browser every second.

Built with Go (Gin), PostgreSQL, Redis, WebSockets and Angular 18.

There is also a bot generator that plays as 500 fake users, so the board has something moving without real traffic.

```
backend/    Go API, WebSocket hub, bot generator
frontend/   Angular app
```

## How it works

Every score goes to Postgres first, which keeps the full history. Then the API adds the points to a Redis sorted set, which keeps the players ranked, so reading the top N or one player's rank is cheap (O(log N) according to the Redis docs, I haven't benchmarked it). Once a second a broadcaster reads the Global top 20 from Redis and the WebSocket hub sends it to every open browser.

If Redis loses its data, the API rebuilds the boards and usernames from Postgres when it starts. It builds them under temporary keys and swaps them in, so nobody sees a half-built board.

If the Redis write fails after Postgres worked, the score is safe but that player's live rank stays behind until the next rebuild. Nothing retries it.

## Auth and limits

- Passwords are hashed with bcrypt. Login returns a JWT (HS256) that lasts 72 hours by default.
- Each token has an ID. Logout stores that ID in Redis until the token would have expired, and the auth middleware refuses it. Only the token that sent the logout is blocked.
- Rate limits use a Lua script in Redis so the counter and its expiry are set together. Register and login share 10 requests per minute per IP. Score submits are 1 per 3 seconds per user.
- If Redis is down, login and register return 503, and score submits are let through.
- A score has to be between 1 and 10,000 points. The API checks it and so does a database constraint.
- The WebSocket only accepts connections from the origins in `ALLOWED_ORIGINS`, and request bodies are capped at 16 KiB.

## Tests

About 30 Go tests, run with the race detector against a real Postgres and Redis. They cover input validation, register/login/submit, the Redis rebuild, logout, the rate limiter and the WebSocket origin check. The frontend and the broadcaster have no tests. A GitHub Actions workflow in `.github/workflows/ci.yml` runs them on every push.

## Limitations

- **Scores aren't verified.** They come from the client, so someone with an account can cheat. The rate limit and the 10,000 cap only limit the damage. A real fix is to have the server work out the score itself.
- **The rebuild only runs at startup.** If Redis restarts while the API is running, it won't notice.
- **A retried score request counts twice.** There is no idempotency key.
- **Public reads and WebSocket connections aren't rate limited,** and there is no cap on connections.
- **Tokens last 72 hours,** are kept in `localStorage`, and there are no refresh tokens. If Redis is down the logout check is skipped, and tokens made before logout existed can't be blocked.
- **No password reset.** The app sends no email.
- **Only the Global board updates live.** The game boards load over REST when you open them.
- **Ties** are ordered by user ID, not by who got there first.
- **The frontend doesn't clear an expired token** when it gets a 401.
- **The bots aren't marked** as bots in the database.
