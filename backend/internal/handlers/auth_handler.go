package handlers

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/auth"
	"github.com/bajajsakshi738/leaderboard-backend/internal/middleware"
	"github.com/bajajsakshi738/leaderboard-backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userRepo        *repository.UserRepository
	leaderboardRepo *repository.LeaderboardRepository
	tokenManager    *auth.TokenManager
	denylist        *auth.Denylist
}

func NewAuthHandler(userRepo *repository.UserRepository, leaderboardRepo *repository.LeaderboardRepository, tm *auth.TokenManager, denylist *auth.Denylist) *AuthHandler {
	return &AuthHandler{
		userRepo:        userRepo,
		leaderboardRepo: leaderboardRepo,
		tokenManager:    tm,
		denylist:        denylist,
	}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=30"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type authResponse struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Register creates the account, hashes the password, caches the username in
// Redis for the leaderboard and returns a JWT.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}

	if len(req.Password) > auth.MaxPasswordBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 72 bytes or fewer"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if !usernamePattern.MatchString(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username may only contain letters, numbers, and underscores"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process password"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	user, err := h.userRepo.Create(ctx, req.Username, req.Email, hash, false)
	if err != nil {
		if err == repository.ErrDuplicateUser {
			c.JSON(http.StatusConflict, gin.H{"error": "username or email already taken"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	// best effort. the name is cached again when they submit a score.
	_ = h.leaderboardRepo.SetUsername(ctx, user.ID.String(), user.Username)

	token, err := h.tokenManager.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	var resp authResponse
	resp.Token = token
	resp.User.ID = user.ID.String()
	resp.User.Username = user.Username
	resp.User.Email = user.Email

	c.JSON(http.StatusCreated, resp)
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login checks the credentials and returns a new JWT.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	user, err := h.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// same error for unknown email and wrong password, so it doesn't reveal
		// which emails are registered.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	token, err := h.tokenManager.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	var resp authResponse
	resp.Token = token
	resp.User.ID = user.ID.String()
	resp.User.Username = user.Username
	resp.User.Email = user.Email

	c.JSON(http.StatusOK, resp)
}

// Logout blocks the token used for this request, so it stops working right
// away instead of at its expiry time. other tokens of the same user (another
// browser, say) are not affected.
func (h *AuthHandler) Logout(c *gin.Context) {
	tokenID, _ := c.Get(middleware.ContextTokenIDKey)
	id, _ := tokenID.(string)
	expiry, hasExpiry := middleware.TokenExpiry(c)

	// a token made before this feature has no ID, so there is nothing to block.
	if id == "" || !hasExpiry {
		c.Status(http.StatusNoContent)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	if err := h.denylist.Revoke(ctx, id, expiry); err != nil {
		// tell the client, so it doesn't think the token is dead when it isn't.
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not log out on the server, try again"})
		return
	}
	c.Status(http.StatusNoContent)
}

// Me returns the logged in user, taken from the token claims.
func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get(middleware.ContextUserIDKey)
	username, _ := c.Get(middleware.ContextUsernameKey)
	c.JSON(http.StatusOK, gin.H{
		"id":       userID,
		"username": username,
	})
}
