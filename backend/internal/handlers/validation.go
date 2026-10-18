package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/bajajsakshi738/leaderboard-backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// bindError turns a ShouldBindJSON error into a status and a short message
// that is safe to show. the raw error is never sent back, it names internal
// fields and validator rules.
func bindError(err error) (int, string) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return http.StatusRequestEntityTooLarge, "request body is too large"
	}

	if errors.Is(err, io.EOF) {
		return http.StatusBadRequest, "request body is empty"
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) || errors.Is(err, io.ErrUnexpectedEOF) {
		return http.StatusBadRequest, "request body is not valid JSON, or a field has the wrong type"
	}

	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) && len(verrs) > 0 {
		return http.StatusBadRequest, fieldMessage(verrs[0])
	}

	return http.StatusBadRequest, "invalid request"
}

// fieldMessage says in plain words which rule failed first.
func fieldMessage(fe validator.FieldError) string {
	switch fe.Field() {
	case "Username":
		return "username must be 3 to 30 characters"
	case "Email":
		return "a valid email address is required"
	case "Password":
		if fe.Tag() == "min" {
			return "password must be at least 6 characters"
		}
		return "password is required"
	case "GameCode":
		return "game_code is required"
	case "Points":
		return "points must be a whole number from 1 to 10000"
	}
	return "invalid request"
}

// respondBindError writes the error response for a failed bind.
func respondBindError(c *gin.Context, err error) {
	status, msg := bindError(err)
	c.JSON(status, gin.H{"error": msg})
}

// requireKnownGame answers 404 and returns false for an unknown board.
func requireKnownGame(c *gin.Context, code string) bool {
	if models.IsKnownGame(code) {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "unknown game"})
	return false
}

// optionalKnownGame is the same for an optional query parameter. empty
// means all games and is allowed.
func optionalKnownGame(c *gin.Context, code string) bool {
	if code == "" || models.IsKnownGame(code) {
		return true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "unknown game_code"})
	return false
}

// playableGamesText is used in the error for a bad submit.
func playableGamesText() string {
	return strings.Join(models.PlayableGameCodes, ", ")
}
