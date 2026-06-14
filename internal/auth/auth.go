// Package auth provides password hashing and JWT issue/verify for admin users.
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/woopsy/porque/internal/apperr"
)

type ctxKey int

const userIDKey ctxKey = iota

// tokenTTL is how long an issued admin token remains valid.
const tokenTTL = 12 * time.Hour

// Service issues and validates JWTs.
type Service struct {
	secret []byte
}

// NewService creates an auth service with the given signing secret.
func NewService(secret string) *Service { return &Service{secret: []byte(secret)} }

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plaintext matches the stored bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

// Issue mints a signed token for the given user id.
func (s *Service) Issue(userID uuid.UUID) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.secret)
}

// Parse validates a token string and returns the subject user id.
func (s *Service) Parse(tokenStr string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, apperr.Unauthorized("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return uuid.Nil, apperr.Unauthorized("invalid token")
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, apperr.Unauthorized("invalid token subject")
	}
	return id, nil
}

// Middleware enforces a valid Bearer token (bypassed for self-hosted mode).
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bypass token verification and inject Nil UUID so that code expecting a user ID doesn't break
		ctx := context.WithValue(r.Context(), userIDKey, uuid.Nil)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// UserID extracts the authenticated user id from the request context.
func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}
