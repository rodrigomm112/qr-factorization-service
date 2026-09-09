package application_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rodrigomm/proyectot/qr-api/internal/application"
	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

type fakeCredentials struct {
	clientID string
	secret   string
	calls    atomic.Int64 // subtests run in parallel against one fake
}

func (f *fakeCredentials) Verify(clientID, clientSecret string) bool {
	f.calls.Add(1)
	return clientID == f.clientID && clientSecret == f.secret
}

type fakeIssuer struct {
	token string
	err   error
	subs  []string
}

func (f *fakeIssuer) Issue(subject string) (string, time.Time, time.Time, error) {
	f.subs = append(f.subs, subject)
	if f.err != nil {
		return "", time.Time{}, time.Time{}, f.err
	}
	issuedAt := time.Unix(1_700_000_000, 0).UTC()
	return f.token, issuedAt, issuedAt.Add(time.Hour), nil
}

func ptr(s string) *string { return &s }

func TestIssueToken_HappyPath(t *testing.T) {
	t.Parallel()

	issuer := &fakeIssuer{token: "signed.jwt.value"}
	uc := application.NewIssueToken(&fakeCredentials{clientID: "demo-client", secret: "s3cret"}, issuer, "qr:compute stats:compute")

	result, err := uc.Execute(application.TokenCommand{ClientID: ptr("demo-client"), ClientSecret: ptr("s3cret")})
	require.NoError(t, err)
	require.Equal(t, "signed.jwt.value", result.AccessToken)
	require.Equal(t, "Bearer", result.TokenType)
	require.Equal(t, 3600, result.ExpiresIn)
	require.Equal(t, "qr:compute stats:compute", result.Scope)
	require.Equal(t, []string{"demo-client"}, issuer.subs)
}

func TestIssueToken_SameErrorForEveryCredentialFailure(t *testing.T) {
	t.Parallel()

	uc := application.NewIssueToken(&fakeCredentials{clientID: "demo-client", secret: "s3cret"}, &fakeIssuer{token: "t"}, "scope")

	for _, tc := range []struct {
		name   string
		id     string
		secret string
	}{
		{"unknown client", "other-client", "s3cret"},
		{"wrong secret", "demo-client", "wrong"},
		{"both wrong", "other-client", "wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := uc.Execute(application.TokenCommand{ClientID: ptr(tc.id), ClientSecret: ptr(tc.secret)})
			require.ErrorIs(t, err, application.ErrUnauthorized)
		})
	}
}

func TestIssueToken_MissingFields(t *testing.T) {
	t.Parallel()

	credentials := &fakeCredentials{clientID: "demo-client", secret: "s3cret"}
	uc := application.NewIssueToken(credentials, &fakeIssuer{token: "t"}, "scope")

	for _, tc := range []struct {
		name string
		cmd  application.TokenCommand
		want []matrix.Violation
	}{
		{
			name: "both absent",
			cmd:  application.TokenCommand{},
			want: []matrix.Violation{
				{Pointer: "/clientId", Code: matrix.CodeMissingField, Message: "clientId is required"},
				{Pointer: "/clientSecret", Code: matrix.CodeMissingField, Message: "clientSecret is required"},
			},
		},
		{
			name: "secret absent",
			cmd:  application.TokenCommand{ClientID: ptr("demo-client")},
			want: []matrix.Violation{
				{Pointer: "/clientSecret", Code: matrix.CodeMissingField, Message: "clientSecret is required"},
			},
		},
		{
			name: "empty strings count as missing",
			cmd:  application.TokenCommand{ClientID: ptr(""), ClientSecret: ptr("")},
			want: []matrix.Violation{
				{Pointer: "/clientId", Code: matrix.CodeMissingField, Message: "clientId is required"},
				{Pointer: "/clientSecret", Code: matrix.CodeMissingField, Message: "clientSecret is required"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := uc.Execute(tc.cmd)
			var validation *application.ErrValidation
			require.ErrorAs(t, err, &validation)
			require.Equal(t, tc.want, validation.Violations)
		})
	}
	require.Zero(t, credentials.calls.Load(), "credentials are never checked when a field is missing")
}

func TestIssueToken_SigningFailurePropagates(t *testing.T) {
	t.Parallel()

	boom := errors.New("hsm unavailable")
	uc := application.NewIssueToken(&fakeCredentials{clientID: "c", secret: "s"}, &fakeIssuer{err: boom}, "scope")
	_, err := uc.Execute(application.TokenCommand{ClientID: ptr("c"), ClientSecret: ptr("s")})
	require.ErrorIs(t, err, boom)
}

func TestIssueDemoToken_UsesTheDemoSubject(t *testing.T) {
	t.Parallel()

	issuer := &fakeIssuer{token: "demo.jwt"}
	result, err := application.NewIssueDemoToken(issuer, "scope").Execute()
	require.NoError(t, err)
	require.Equal(t, "demo.jwt", result.AccessToken)
	require.Equal(t, 3600, result.ExpiresIn)
	require.Equal(t, []string{application.DemoSubject}, issuer.subs)

	_, err = application.NewIssueDemoToken(&fakeIssuer{err: errors.New("hsm down")}, "scope").Execute()
	require.Error(t, err)
}
