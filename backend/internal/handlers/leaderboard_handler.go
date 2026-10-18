package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/middleware"
	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/bajajsakshi738/leaderboard-backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type LeaderboardHandler struct {
	leaderboardRepo *repository.LeaderboardRepository
	scoreService    *services.ScoreService
	scoreRepo       *repository.ScoreRepository
}

func NewLeaderboardHandler(leaderboardRepo *repository.LeaderboardRepository, scoreService *services.ScoreService, scoreRepo *repository.ScoreRepository) *LeaderboardHandler {
	return &LeaderboardHandler{
		leaderboardRepo: leaderboardRepo,
		scoreService:    scoreService,
		scoreRepo:       scoreRepo,
	}
}

// GetTop handles GET /api/leaderboard/:gameCode/top?n=10
func (h *LeaderboardHandler) GetTop(c *gin.Context) {
	gameCode := c.Param("gameCode")
	if !requireKnownGame(c, gameCode) {
		return
	}
	n := int64(10)
	if nStr := c.Query("n"); nStr != "" {
		parsed, err := strconv.ParseInt(nStr, 10, 64)
		if err != nil || parsed <= 0 || parsed > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "n must be a positive integer up to 500"})
			return
		}
		n = parsed
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	entries, err := h.leaderboardRepo.GetTopN(ctx, gameCode, n)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch leaderboard"})
		return
	}

	total, err := h.leaderboardRepo.CountRanked(ctx, gameCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count ranked users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"game_code":    gameCode,
		"entries":      entries,
		"total_ranked": total,
	})
}

// GetMyRank handles GET /api/leaderboard/:gameCode/me (requires auth)
func (h *LeaderboardHandler) GetMyRank(c *gin.Context) {
	gameCode := c.Param("gameCode")
	if !requireKnownGame(c, gameCode) {
		return
	}
	userID, _ := c.Get(middleware.ContextUserIDKey)
	username, _ := c.Get(middleware.ContextUsernameKey)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rank, err := h.leaderboardRepo.GetUserRank(ctx, gameCode, userID.(string), username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch rank"})
		return
	}

	c.JSON(http.StatusOK, rank)
}

// GetRankForUser handles GET /api/leaderboard/:gameCode/rank/:userID (public)
func (h *LeaderboardHandler) GetRankForUser(c *gin.Context) {
	gameCode := c.Param("gameCode")
	if !requireKnownGame(c, gameCode) {
		return
	}
	userIDParam := c.Param("userID")

	if _, err := uuid.Parse(userIDParam); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	username, err := h.leaderboardRepo.GetUsername(ctx, userIDParam)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch user"})
		return
	}

	rank, err := h.leaderboardRepo.GetUserRank(ctx, gameCode, userIDParam, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch rank"})
		return
	}

	c.JSON(http.StatusOK, rank)
}

// maxPointsPerSubmission caps one submission so a single request can't
// dominate a board. struct tags need literal numbers, so SubmitScore checks
// it again in case the tag and this constant drift apart.
const maxPointsPerSubmission = 10000

type submitScoreRequest struct {
	GameCode string `json:"game_code" binding:"required"`
	Points   int64  `json:"points" binding:"required,gt=0,lte=10000"`
}

// SubmitScore handles POST /api/scores (requires auth)
func (h *LeaderboardHandler) SubmitScore(c *gin.Context) {
	var req submitScoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	// scores go to a real game only. the global board is updated by the server,
	// so writing to it directly is rejected.
	if !models.IsPlayableGame(req.GameCode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "game_code must be one of: " + playableGamesText()})
		return
	}

	// second check for the cap, in case the struct tag is edited.
	if req.Points > maxPointsPerSubmission {
		c.JSON(http.StatusBadRequest, gin.H{"error": "points exceeds the maximum allowed per submission"})
		return
	}

	userIDStr, _ := c.Get(middleware.ContextUserIDKey)
	username, _ := c.Get(middleware.ContextUsernameKey)

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user context"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	entry, err := h.scoreService.SubmitScore(ctx, userID, username.(string), req.GameCode, req.Points)
	if err != nil {
		if entry != nil {
			// saved in Postgres but the Redis update failed. the score is safe, just
			// not live yet.
			c.JSON(http.StatusOK, gin.H{
				"warning": "score saved, but live leaderboard update may be delayed",
				"score":   entry,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit score"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"score": entry})
}

// GetMyHistory handles GET /api/scores/history?game_code=&limit= (requires auth)
func (h *LeaderboardHandler) GetMyHistory(c *gin.Context) {
	userIDStr, _ := c.Get(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user context"})
		return
	}

	gameCode := c.Query("game_code")
	if !optionalKnownGame(c, gameCode) {
		return
	}
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	history, err := h.scoreRepo.GetUserHistory(ctx, userID, gameCode, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"history": history})
}

// GetTopPlayersReport handles GET /api/reports/top-players?game_code=&period=week&limit=10
func (h *LeaderboardHandler) GetTopPlayersReport(c *gin.Context) {
	gameCode := c.Query("game_code")
	if !optionalKnownGame(c, gameCode) {
		return
	}
	period := c.DefaultQuery("period", "week")

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var since time.Time
	switch period {
	case "day":
		since = time.Now().AddDate(0, 0, -1)
	case "week":
		since = time.Now().AddDate(0, 0, -7)
	case "month":
		since = time.Now().AddDate(0, -1, 0)
	case "all":
		since = time.Time{} // zero value: beginning of time
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "period must be one of: day, week, month, all"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	report, err := h.scoreRepo.GetTopPlayersReport(ctx, gameCode, since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate report"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"period":    period,
		"game_code": gameCode,
		"report":    report,
	})
}
