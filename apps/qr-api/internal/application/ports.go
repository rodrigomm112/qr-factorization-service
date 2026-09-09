package application

import (
	"context"
	"time"
)

// LabeledMatrix is one matrix of the downstream request body.
type LabeledMatrix struct {
	Label  string      `json:"label"`
	Values [][]float64 `json:"values"`
}

// StatisticsRequest is the stats-api payload; the caller's token goes verbatim (ADR-0007).
type StatisticsRequest struct {
	Matrices      []LabeledMatrix
	Authorization string
	RequestID     string
}

// StatisticsSummary is the aggregate over every submitted matrix.
type StatisticsSummary struct {
	Max         float64 `json:"max"`
	Min         float64 `json:"min"`
	Sum         float64 `json:"sum"`
	Average     float64 `json:"average"`
	Count       int     `json:"count"`
	AnyDiagonal bool    `json:"anyDiagonal"`
}

// MatrixStatistics is the per-matrix breakdown returned by stats-api.
type MatrixStatistics struct {
	Index      int     `json:"index"`
	Label      string  `json:"label"`
	Rows       int     `json:"rows"`
	Cols       int     `json:"cols"`
	Max        float64 `json:"max"`
	Min        float64 `json:"min"`
	Sum        float64 `json:"sum"`
	Average    float64 `json:"average"`
	IsDiagonal bool    `json:"isDiagonal"`
}

// DiagonalTolerance is the hybrid absolute/relative tolerance stats-api applied.
type DiagonalTolerance struct {
	Absolute float64 `json:"absolute"`
	Relative float64 `json:"relative"`
}

// StatisticsMeta describes how the figures were produced.
type StatisticsMeta struct {
	SummationAlgorithm string            `json:"summationAlgorithm"`
	DiagonalTolerance  DiagonalTolerance `json:"diagonalTolerance"`
	ElapsedMs          float64           `json:"elapsedMs"`
}

// StatisticsReport mirrors openapi.yaml's schema: the stats-api body minus its requestId.
type StatisticsReport struct {
	Summary  StatisticsSummary  `json:"summary"`
	Matrices []MatrixStatistics `json:"matrices"`
	Meta     StatisticsMeta     `json:"meta"`
}

// StatisticsClient is the outbound port; it returns application errors, never transport ones.
type StatisticsClient interface {
	Compute(ctx context.Context, req StatisticsRequest) (StatisticsReport, error)
}

// TokenIssuer signs an access token for a subject.
type TokenIssuer interface {
	Issue(subject string) (token string, issuedAt, expiresAt time.Time, err error)
}

// CredentialVerifier checks a client-credentials pair in constant time.
type CredentialVerifier interface {
	Verify(clientID, clientSecret string) bool
}
