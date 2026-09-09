package middleware

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
)

// Draft-8 rate-limit header names (draft-ietf-httpapi-ratelimit-headers).
const (
	headerRateLimit       = "RateLimit"
	headerRateLimitPolicy = "RateLimit-Policy"
)

// Fiber's limiter still emits the pre-standard X-RateLimit-* trio.
const (
	legacyLimit     = "X-RateLimit-Limit"
	legacyRemaining = "X-RateLimit-Remaining"
	legacyReset     = "X-RateLimit-Reset"
)

// policyName is the single quota policy of this service.
const policyName = "default"

// SetRateLimitHeaders writes one draft-8 quota policy on the response.
func SetRateLimitHeaders(c fiber.Ctx, quota, windowSeconds, remaining, resetSeconds int) {
	c.Set(headerRateLimitPolicy, `"`+policyName+`";q=`+strconv.Itoa(quota)+";w="+strconv.Itoa(windowSeconds))
	c.Set(headerRateLimit, `"`+policyName+`";r=`+strconv.Itoa(remaining)+";t="+strconv.Itoa(resetSeconds))
}

// RateLimitHeaders rewrites the legacy X-RateLimit-* trio into the draft-8 pair stats-api
// emits. Register it *before* the limiter: it works on the way out, RateLimitReached on 429.
func RateLimitHeaders(windowSeconds int) fiber.Handler {
	policyWindow := ";w=" + strconv.Itoa(windowSeconds)

	return func(c fiber.Ctx) error {
		err := c.Next()

		limit := c.GetRespHeader(legacyLimit)
		if limit == "" {
			return err
		}
		c.Set(headerRateLimitPolicy, `"`+policyName+`";q=`+limit+policyWindow)
		c.Set(headerRateLimit, `"`+policyName+`";r=`+c.GetRespHeader(legacyRemaining)+";t="+c.GetRespHeader(legacyReset))

		// The legacy trio carries the same numbers in a format nothing should target now.
		c.Response().Header.Del(legacyLimit)
		c.Response().Header.Del(legacyRemaining)
		c.Response().Header.Del(legacyReset)
		return err
	}
}
