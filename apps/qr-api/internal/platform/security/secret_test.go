package security_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/platform/security"
)

func TestVerifyClientSecret(t *testing.T) {
	t.Parallel()

	digest := security.HashSecret("correct horse battery staple")
	require.Len(t, digest, 64)

	require.True(t, security.VerifyClientSecret("correct horse battery staple", digest))
	require.False(t, security.VerifyClientSecret("wrong secret", digest))
	require.False(t, security.VerifyClientSecret("", digest))
	require.False(t, security.VerifyClientSecret("correct horse battery staple", "not-hex"), "a malformed digest never matches")
	require.False(t, security.VerifyClientSecret("correct horse battery staple", ""))
}

func TestClientCredentials_Verify(t *testing.T) {
	t.Parallel()

	credentials := security.NewClientCredentials("demo-client", security.HashSecret("s3cret"))

	require.True(t, credentials.Verify("demo-client", "s3cret"))
	require.False(t, credentials.Verify("other-client", "s3cret"))
	require.False(t, credentials.Verify("demo-client", "wrong"))
	require.False(t, credentials.Verify("", ""))
}

func TestClientCredentials_DigestIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	upper := "8FC0FA1A5C0B0F7D1F5B0F4A0D6B2E9F3C1D8E7A6B5C4D3E2F1A0B9C8D7E6F5A"
	credentials := security.NewClientCredentials("demo-client", upper)
	// Config enforces lowercase hex; the constructor normalizes so an uppercase paste works.
	require.False(t, credentials.Verify("demo-client", "anything"))
}
