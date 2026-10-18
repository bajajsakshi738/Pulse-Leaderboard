// Command generator fakes lots of users submitting scores to the live API,
// so the leaderboard has traffic to demo without real users.
//
// 1. make sure N bot users exist (register them, or log in if they do)
// 2. every tick, pick some bots and submit a random score for each
//
// it only uses the public HTTP API, so it can run anywhere.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

type botUser struct {
	Username string
	Email    string
	Password string
	Token    string
	UserID   string
}

var gameCodes = []string{"speed-run", "trivia", "puzzle"}

// bypassToken is sent in X-RateLimit-Bypass on every request when set.
var bypassToken string

// newRequest builds a JSON POST with the bypass header if configured.
func newRequest(url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if bypassToken != "" {
		req.Header.Set("X-RateLimit-Bypass", bypassToken)
	}
	return req, nil
}

func main() {
	// load backend/.env if it exists. real environment variables win.
	_ = godotenv.Load()

	apiBaseURL := getEnv("API_BASE_URL", "http://localhost:8080")
	numBots := getEnvInt("GENERATOR_NUM_BOTS", 500)
	tickInterval := getEnvDuration("GENERATOR_TICK_INTERVAL", 1*time.Second)
	activePerTick := getEnvInt("GENERATOR_ACTIVE_PER_TICK", 3)

	// no default password on purpose. an earlier version hardcoded one in the
	// source, so anyone reading it could log in as a bot. fail with a clear
	// message instead.
	botPassword := os.Getenv("GENERATOR_BOT_PASSWORD")
	if botPassword == "" {
		log.Fatal("GENERATOR_BOT_PASSWORD is required; set it in your .env file (see .env.example)")
	}

	// optional secret so the server doesn't rate limit the generator. it must
	// match the server's RATE_LIMIT_BYPASS_TOKEN. all bots share one IP, so
	// without it the login limit would block most of them.
	bypassToken = os.Getenv("RATE_LIMIT_BYPASS_TOKEN")
	if bypassToken == "" {
		log.Println("warning: RATE_LIMIT_BYPASS_TOKEN is not set; the server's rate limits will apply to the bots")
	}

	log.Printf("fake score generator starting: apiBaseURL=%s numBots=%d tickInterval=%s activePerTick=%d",
		apiBaseURL, numBots, tickInterval, activePerTick)

	client := &http.Client{Timeout: 5 * time.Second}

	bots := provisionBots(client, apiBaseURL, numBots, botPassword)
	if len(bots) == 0 {
		log.Fatal("no bot users were provisioned; aborting")
	}
	log.Printf("provisioned %d bot users, starting score submission loop", len(bots))

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for range ticker.C {
		n := activePerTick
		if n > len(bots) {
			n = len(bots)
		}
		selected := rng.Perm(len(bots))[:n]

		var wg sync.WaitGroup
		for _, idx := range selected {
			wg.Add(1)
			bot := bots[idx]
			seed := rng.Int63()
			go func(b *botUser, s int64) {
				defer wg.Done()
				localRng := rand.New(rand.NewSource(s))
				submitRandomScore(client, apiBaseURL, b, localRng)
			}(bot, seed)
		}
		wg.Wait()
	}
}

// provisionBots makes sure `count` bots exist and are logged in, and returns
// their tokens. if registering fails because the bot exists, we log in.
func provisionBots(client *http.Client, apiBaseURL string, count int, botPassword string) []*botUser {
	bots := make([]*botUser, 0, count)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// limit concurrency so we don't send hundreds of registrations at once.
	sem := make(chan struct{}, 20)

	for i := 0; i < count; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()

			username := fmt.Sprintf("bot_user_%04d", i)
			email := fmt.Sprintf("bot_user_%04d@generator.local", i)

			bot := &botUser{Username: username, Email: email, Password: botPassword}

			token, userID, err := registerBot(client, apiBaseURL, bot)
			if err != nil {
				// probably registered on an earlier run, so log in.
				token, userID, err = loginBot(client, apiBaseURL, bot)
				if err != nil {
					log.Printf("failed to provision bot %s: %v", username, err)
					return
				}
			}

			bot.Token = token
			bot.UserID = userID

			mu.Lock()
			bots = append(bots, bot)
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	return bots
}

func registerBot(client *http.Client, apiBaseURL string, bot *botUser) (token, userID string, err error) {
	payload := map[string]string{
		"username": bot.Username,
		"email":    bot.Email,
		"password": bot.Password,
	}
	body, _ := json.Marshal(payload)

	req, err := newRequest(apiBaseURL+"/api/auth/register", body)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("register returned status %d", resp.StatusCode)
	}

	return parseAuthResponse(resp.Body)
}

func loginBot(client *http.Client, apiBaseURL string, bot *botUser) (token, userID string, err error) {
	payload := map[string]string{
		"email":    bot.Email,
		"password": bot.Password,
	}
	body, _ := json.Marshal(payload)

	req, err := newRequest(apiBaseURL+"/api/auth/login", body)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("login returned status %d", resp.StatusCode)
	}

	return parseAuthResponse(resp.Body)
}

func parseAuthResponse(body io.Reader) (token, userID string, err error) {
	var parsed struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return "", "", err
	}
	return parsed.Token, parsed.User.ID, nil
}

func submitRandomScore(client *http.Client, apiBaseURL string, bot *botUser, rng *rand.Rand) {
	gameCode := gameCodes[rng.Intn(len(gameCodes))]
	points := int64(rng.Intn(50) + 1) // 1-50 points per tick

	payload := map[string]interface{}{
		"game_code": gameCode,
		"points":    points,
	}
	body, _ := json.Marshal(payload)

	req, err := newRequest(apiBaseURL+"/api/scores", body)
	if err != nil {
		log.Printf("bot %s: failed to build request: %v", bot.Username, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+bot.Token)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("bot %s: score submission failed: %v", bot.Username, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		log.Printf("bot %s: score submission returned status %d", bot.Username, resp.StatusCode)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
