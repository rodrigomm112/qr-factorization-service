package application

import (
	"time"

	"github.com/rodrigomm/proyectot/qr-api/internal/domain/matrix"
)

// TokenCommand is the client-credentials request.
type TokenCommand struct {
	ClientID     *string
	ClientSecret *string
}

// TokenResult is the issued access token.
type TokenResult struct {
	AccessToken string
	TokenType   string
	ExpiresIn   int
	IssuedAt    time.Time
	Scope       string
}

// IssueToken exchanges client credentials for an HS256 access token both services take.
type IssueToken struct {
	credentials CredentialVerifier
	tokens      TokenIssuer
	scope       string
}

// NewIssueToken wires the use case.
func NewIssueToken(credentials CredentialVerifier, tokens TokenIssuer, scope string) *IssueToken {
	return &IssueToken{credentials: credentials, tokens: tokens, scope: scope}
}

// Execute returns *ErrValidation for a missing field, ErrUnauthorized for anything else.
func (uc *IssueToken) Execute(cmd TokenCommand) (*TokenResult, error) {
	var violations []matrix.Violation
	if cmd.ClientID == nil || *cmd.ClientID == "" {
		violations = append(violations, matrix.Violation{
			Pointer: "/clientId",
			Code:    matrix.CodeMissingField,
			Message: "clientId is required",
		})
	}
	if cmd.ClientSecret == nil || *cmd.ClientSecret == "" {
		violations = append(violations, matrix.Violation{
			Pointer: "/clientSecret",
			Code:    matrix.CodeMissingField,
			Message: "clientSecret is required",
		})
	}
	if len(violations) > 0 {
		return nil, NewValidationError(violations)
	}

	if !uc.credentials.Verify(*cmd.ClientID, *cmd.ClientSecret) {
		return nil, ErrUnauthorized
	}

	token, issuedAt, expiresAt, err := uc.tokens.Issue(*cmd.ClientID)
	if err != nil {
		return nil, err
	}
	return &TokenResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(expiresAt.Sub(issuedAt).Seconds()),
		IssuedAt:    issuedAt,
		Scope:       uc.scope,
	}, nil
}

// DemoSubject is the `sub` of tokens minted without credentials; it only tells the logs apart.
const DemoSubject = "demo"

// IssueDemoToken mints the same HS256 token as IssueToken with no credentials, for the public demo.
// A browser cannot keep a client secret, so the SPA gets a token from this instead; the route is
// enabled by DEMO_TOKEN_ENABLED and rate limited like /auth/token.
type IssueDemoToken struct {
	tokens TokenIssuer
	scope  string
}

// NewIssueDemoToken wires the use case.
func NewIssueDemoToken(tokens TokenIssuer, scope string) *IssueDemoToken {
	return &IssueDemoToken{tokens: tokens, scope: scope}
}

// Execute issues a token for DemoSubject.
func (uc *IssueDemoToken) Execute() (*TokenResult, error) {
	token, issuedAt, expiresAt, err := uc.tokens.Issue(DemoSubject)
	if err != nil {
		return nil, err
	}
	return &TokenResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int(expiresAt.Sub(issuedAt).Seconds()),
		IssuedAt:    issuedAt,
		Scope:       uc.scope,
	}, nil
}
