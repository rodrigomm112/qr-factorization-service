package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// signingMethod is the only accepted algorithm; the verifier's allow-list reads it too.
const signingMethod = "HS256"

// leeway tolerated on exp/nbf/iat to absorb clock skew between the two services.
const leeway = 30 * time.Second

// Claims are the claims minted by qr-api and understood by both services.
type Claims struct {
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// TokenServiceConfig configures [TokenService].
type TokenServiceConfig struct {
	Secret []byte
	Issuer string
	// Audience is the full `aud` claim written into new tokens.
	Audience []string
	// RequiredAudience must be present in `aud` for a token to be accepted.
	RequiredAudience string
	TTL              time.Duration
	Scope            string
	// Now defaults to time.Now.
	Now func() time.Time
}

// TokenService signs and verifies HS256 access tokens.
type TokenService struct {
	cfg    TokenServiceConfig
	parser *jwt.Parser
}

// NewTokenService fixes the parser: algorithm allow-list, issuer, audience, clock leeway.
func NewTokenService(cfg TokenServiceConfig) *TokenService { //nolint:gocritic // the config is built once at boot; a copy is clearer than a shared pointer
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{signingMethod}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.RequiredAudience),
		jwt.WithLeeway(leeway),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return cfg.Now() }),
	)
	return &TokenService{cfg: cfg, parser: parser}
}

// Issue signs a token for the subject. It satisfies application.TokenIssuer.
func (s *TokenService) Issue(subject string) (token string, issuedAt, expiresAt time.Time, err error) {
	issuedAt = s.cfg.Now().UTC().Truncate(time.Second)
	expiresAt = issuedAt.Add(s.cfg.TTL)

	claims := Claims{
		Scope: s.cfg.Scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.cfg.Issuer,
			Subject:   subject,
			Audience:  s.cfg.Audience,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.NewString(),
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.cfg.Secret)
	if err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("security: signing token: %w", err)
	}
	return token, issuedAt, expiresAt, nil
}

// ErrInvalidToken covers every verification failure; callers must not distinguish reasons.
var ErrInvalidToken = errors.New("security: invalid token")

// Verify parses and validates a token, returning its claims.
func (s *TokenService) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := s.parser.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return s.cfg.Secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
