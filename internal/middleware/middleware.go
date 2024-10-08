package api_middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Lutefd/challenge-bravo/internal/commons"
	"github.com/Lutefd/challenge-bravo/internal/logger"
	"github.com/Lutefd/challenge-bravo/internal/model"
	"github.com/Lutefd/challenge-bravo/internal/repository"
	"golang.org/x/time/rate"
)

type AuthMiddleware struct {
	userRepo repository.UserRepository
}

func NewAuthMiddleware(userRepo repository.UserRepository) *AuthMiddleware {
	return &AuthMiddleware{userRepo: userRepo}
}

var (
	Clients = make(map[string]*rate.Limiter)
	mu      sync.Mutex
)

func getRateLimiter(ip string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	limiter, exists := Clients[ip]
	if !exists {
		limiter = rate.NewLimiter(rate.Every(time.Second), commons.AllowedRPS)
		Clients[ip] = limiter
	}

	return limiter
}

func (am *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")

		if apiKey == "" {
			logger.Error("no API key provided")
			http.Error(w, "no API key provided", http.StatusUnauthorized)
			return
		}
		userDB, err := am.userRepo.GetByAPIKey(r.Context(), apiKey)
		if err != nil {
			logger.Errorf("invalid API key: %s", apiKey)
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		user := userDB.ToUser()
		ctx := context.WithValue(r.Context(), commons.UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireRole(role model.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			contextUser := r.Context().Value(commons.UserContextKey)

			user, ok := contextUser.(model.User)
			if !ok {
				logger.Error("user not found in context or has unexpected type")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if user.Role != role {
				logger.Errorf("user %s does not have required role %s", user.Username, role)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		limiter := getRateLimiter(ip)
		if !limiter.Allow() {
			logger.Errorf("rate limit exceeded for IP: %s", r.RemoteAddr)
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
