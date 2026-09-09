package security_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/security"
)

var (
	testSecret = []byte("a-test-secret-of-at-least-32-bytes!!")
	otherKey   = []byte("a-different-secret-of-32-bytes-min!!")
)

func newService(t *testing.T, now func() time.Time) *security.TokenService {
	t.Helper()
	return security.NewTokenService(security.TokenServiceConfig{
		Secret:           testSecret,
		Issuer:           "qr-api",
		Audience:         []string{"qr-api", "stats-api"},
		RequiredAudience: "qr-api",
		TTL:              time.Hour,
		Scope:            "qr:compute stats:compute",
		Now:              now,
	})
}

func TestTokenService_RoundTrip(t *testing.T) {
	t.Parallel()

	svc := newService(t, nil)
	token, issuedAt, expiresAt, err := svc.Issue("demo-client")
	require.NoError(t, err)
	require.Equal(t, time.Hour, expiresAt.Sub(issuedAt))

	claims, err := svc.Verify(token)
	require.NoError(t, err)
	require.Equal(t, "qr-api", claims.Issuer)
	require.Equal(t, "demo-client", claims.Subject)
	require.Equal(t, jwt.ClaimStrings{"qr-api", "stats-api"}, claims.Audience)
	require.Equal(t, "qr:compute stats:compute", claims.Scope)
	require.NotEmpty(t, claims.ID, "every token carries a jti")
	require.Equal(t, issuedAt, claims.IssuedAt.UTC())
	require.Equal(t, expiresAt, claims.ExpiresAt.UTC())
}

func TestTokenService_JTIIsUniquePerToken(t *testing.T) {
	t.Parallel()

	svc := newService(t, nil)
	first, _, _, err := svc.Issue("demo-client")
	require.NoError(t, err)
	second, _, _, err := svc.Issue("demo-client")
	require.NoError(t, err)

	a, err := svc.Verify(first)
	require.NoError(t, err)
	b, err := svc.Verify(second)
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)
}

func TestTokenService_RejectsAlgNone(t *testing.T) {
	t.Parallel()

	svc := newService(t, nil)
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, security.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "qr-api",
			Subject:   "demo-client",
			Audience:  jwt.ClaimStrings{"qr-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = svc.Verify(unsigned)
	require.ErrorIs(t, err, security.ErrInvalidToken)
}

func TestTokenService_RejectsForeignSignature(t *testing.T) {
	t.Parallel()

	other := security.NewTokenService(security.TokenServiceConfig{
		Secret: otherKey, Issuer: "qr-api", Audience: []string{"qr-api"},
		RequiredAudience: "qr-api", TTL: time.Hour,
	})
	token, _, _, err := other.Issue("demo-client")
	require.NoError(t, err)

	_, err = newService(t, nil).Verify(token)
	require.ErrorIs(t, err, security.ErrInvalidToken)
}

func TestTokenService_RejectsWrongAudienceAndIssuer(t *testing.T) {
	t.Parallel()

	t.Run("audience", func(t *testing.T) {
		t.Parallel()
		foreign := security.NewTokenService(security.TokenServiceConfig{
			Secret: testSecret, Issuer: "qr-api", Audience: []string{"stats-api"},
			RequiredAudience: "stats-api", TTL: time.Hour,
		})
		token, _, _, err := foreign.Issue("demo-client")
		require.NoError(t, err)
		_, err = newService(t, nil).Verify(token)
		require.ErrorIs(t, err, security.ErrInvalidToken)
	})

	t.Run("issuer", func(t *testing.T) {
		t.Parallel()
		foreign := security.NewTokenService(security.TokenServiceConfig{
			Secret: testSecret, Issuer: "someone-else", Audience: []string{"qr-api"},
			RequiredAudience: "qr-api", TTL: time.Hour,
		})
		token, _, _, err := foreign.Issue("demo-client")
		require.NoError(t, err)
		_, err = newService(t, nil).Verify(token)
		require.ErrorIs(t, err, security.ErrInvalidToken)
	})
}

func TestTokenService_ExpiryAndLeeway(t *testing.T) {
	t.Parallel()

	issuedAt := time.Unix(1_700_000_000, 0).UTC()
	issuer := security.NewTokenService(security.TokenServiceConfig{
		Secret: testSecret, Issuer: "qr-api", Audience: []string{"qr-api"},
		RequiredAudience: "qr-api", TTL: time.Hour,
		Now: func() time.Time { return issuedAt },
	})
	token, _, expiresAt, err := issuer.Issue("demo-client")
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		at    time.Time
		valid bool
	}{
		{"before expiry", expiresAt.Add(-time.Minute), true},
		{"inside the 30s leeway", expiresAt.Add(20 * time.Second), true},
		{"past the leeway", expiresAt.Add(31 * time.Second), false},
		{"long expired", expiresAt.Add(24 * time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			at := tc.at
			verifier := security.NewTokenService(security.TokenServiceConfig{
				Secret: testSecret, Issuer: "qr-api", Audience: []string{"qr-api"},
				RequiredAudience: "qr-api", TTL: time.Hour,
				Now: func() time.Time { return at },
			})
			_, err := verifier.Verify(token)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, security.ErrInvalidToken)
		})
	}
}

func TestTokenService_RejectsGarbage(t *testing.T) {
	t.Parallel()

	svc := newService(t, nil)
	for _, token := range []string{"", "not-a-jwt", "a.b.c", "eyJhbGciOiJIUzI1NiJ9..sig"} {
		_, err := svc.Verify(token)
		require.ErrorIs(t, err, security.ErrInvalidToken, "token %q", token)
	}
}
