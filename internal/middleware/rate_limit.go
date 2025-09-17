package middleware

import (
	"net/http"

	"github.com/hadarco13/mini-seller/internal/errors"
	"golang.org/x/time/rate"
)

func RateLimiterMiddleware(r float64, b int) func(next http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(r), b)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !limiter.Allow() {
				rateErr := errors.NewNetworkError("RATE_LIMIT_EXCEEDED", "Too many requests").
					WithContext("client_ip", req.RemoteAddr).
					WithContext("path", req.URL.Path).
					WithUserMessage("You are making too many requests. Please slow down and try again later.").
					WithHTTPStatus(http.StatusTooManyRequests).
					WithRetryable(true)
				WriteErrorResponse(w, req, rateErr)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
