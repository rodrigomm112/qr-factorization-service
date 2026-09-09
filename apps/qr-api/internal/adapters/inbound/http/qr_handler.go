package http

import (
	"github.com/gofiber/fiber/v3"

	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/dto"
	"github.com/rodrigomm/proyectot/qr-api/internal/adapters/inbound/http/problem"
	"github.com/rodrigomm/proyectot/qr-api/internal/application"
)

// matrixShape is the `input` member: {rows, cols}.
type matrixShape struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

// qrMeta describes how the answer was produced; the *Ms members are volatile.
type qrMeta struct {
	Algorithm       string  `json:"algorithm"`
	Mode            string  `json:"mode"`
	QShape          [2]int  `json:"qShape"`
	RShape          [2]int  `json:"rShape"`
	DecompositionMs float64 `json:"decompositionMs"`
	StatisticsMs    float64 `json:"statisticsMs"`
}

// qrResponse is the body of a successful POST /api/v1/qr.
type qrResponse struct {
	RequestID  string                       `json:"requestId"`
	Input      matrixShape                  `json:"input"`
	Q          [][]float64                  `json:"q"`
	R          [][]float64                  `json:"r"`
	Statistics application.StatisticsReport `json:"statistics"`
	Meta       qrMeta                       `json:"meta"`
}

func newQrHandler(uc *application.DecomposeAndAnalyze) fiber.Handler {
	return func(c fiber.Ctx) error {
		rows, mode, violations, err := dto.DecodeQrRequest(c.Body())
		if err != nil {
			return problem.MalformedJSON(err)
		}

		result, err := uc.Execute(c.Context(), application.DecomposeCommand{
			Rows:         rows,
			Mode:         mode,
			DecodeIssues: violations,
			// The caller's token travels verbatim to stats-api: one identity end to end (ADR-0007).
			Authorization: c.Get(fiber.HeaderAuthorization),
			RequestID:     c.RequestID(),
		})
		if err != nil {
			return err
		}

		return c.Status(fiber.StatusOK).JSON(qrResponse{
			RequestID:  c.RequestID(),
			Input:      matrixShape{Rows: result.InputRows, Cols: result.InputCols},
			Q:          result.Q,
			R:          result.R,
			Statistics: result.Statistics,
			Meta: qrMeta{
				Algorithm:       "householder",
				Mode:            string(result.Mode),
				QShape:          [2]int{len(result.Q), len(result.Q[0])},
				RShape:          [2]int{len(result.R), len(result.R[0])},
				DecompositionMs: result.DecompositionMs,
				StatisticsMs:    result.StatisticsMs,
			},
		})
	}
}
