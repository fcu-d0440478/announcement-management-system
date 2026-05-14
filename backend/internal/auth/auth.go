package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"announcement-management-system/backend/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const userKey contextKey = "user"

type Claims struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	jwt.RegisteredClaims
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func IssueToken(secret string, user models.User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Name:     user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func Middleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		tokenString := strings.TrimPrefix(header, "Bearer ")
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(*Claims)
		if !ok {
			http.Error(w, "invalid claims", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CurrentUser(r *http.Request) *Claims {
	user, _ := r.Context().Value(userKey).(*Claims)
	return user
}

func CanManage(role string) bool {
	return role == "admin" || role == "editor"
}

func IsAdmin(role string) bool {
	return role == "admin"
}

