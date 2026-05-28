package handler

import (
	"net/http"
	"time"

	"github.com/diploma/analysis-api-service/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	jwtSecret    string
	devUserID    string
	devUserEmail string
	devUserRole  string
}

func NewAuthHandler(jwtSecret, devUserID, devUserEmail, devUserRole string) *AuthHandler {
	return &AuthHandler{
		jwtSecret:    jwtSecret,
		devUserID:    devUserID,
		devUserEmail: devUserEmail,
		devUserRole:  devUserRole,
	}
}

// IssueDevToken возвращает JWT для локальной работы с analysis-api без core-api.
func (h *AuthHandler) IssueDevToken(c *gin.Context) {
	const expiresIn = int64(24 * 60 * 60)

	claims := jwt.MapClaims{
		"user_id":        h.devUserID,
		"email":          h.devUserEmail,
		"role":           h.devUserRole,
		"analysis_quota": 1000,
		"is_active":      true,
		"exp":            time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
		"iat":            time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	c.JSON(http.StatusOK, model.DevTokenResponse{
		Token:     signed,
		ExpiresIn: expiresIn,
		UserID:    h.devUserID,
		Email:     h.devUserEmail,
		Role:      h.devUserRole,
	})
}
