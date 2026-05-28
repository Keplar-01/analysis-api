package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		userID, _ := claims["user_id"].(string)
		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string)
		isActive := true
		if isActiveRaw, ok := claims["is_active"]; ok {
			if activeBool, ok := isActiveRaw.(bool); ok {
				isActive = activeBool
			}
		}
		analysisQuota := 0
		if quotaRaw, ok := claims["analysis_quota"]; ok {
			if quotaFloat, ok := quotaRaw.(float64); ok {
				analysisQuota = int(quotaFloat)
			}
		}
		if !isActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "account is disabled"})
			return
		}

		c.Set("user_id", userID)
		c.Set("email", email)
		c.Set("role", role)
		c.Set("analysis_quota", analysisQuota)
		c.Next()
	}
}
