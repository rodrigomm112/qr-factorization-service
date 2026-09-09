package qr

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// defaultFixtureDir is contracts/examples, five levels up from this package (go test's cwd).
const defaultFixtureDir = "../../../../../contracts/examples"

// TestGenerateFixtures writes the two golden fixtures of the tall 3x2 case, which cannot be
// hand-computed without rounding ambiguity. encoding/json emits the numbers, so the files
// round-trip exactly. Set FIXTURE_OUT=1, or to a path, to write them.
func TestGenerateFixtures(t *testing.T) {
	dir := os.Getenv("FIXTURE_OUT")
	switch dir {
	case "":
		t.Skip("set FIXTURE_OUT=1 (or a directory) to regenerate the golden fixtures")
	case "1", "true":
		dir = defaultFixtureDir
	}

	_, d := decompose(t, [][]float64{{1, 2}, {3, 4}, {5, 6}}, ModeFull)
	q, r := d.Q.Rows2D(), d.R.Rows2D()

	write := func(name, content string) {
		require.NoError(t, os.WriteFile(dir+"/"+name, []byte(content), 0o644))
	}

	write("qr.output.tall-3x2.json", "{\n"+
		`  "q": `+rowsJSON(t, q, "  ")+",\n"+
		`  "r": `+rowsJSON(t, r, "  ")+"\n"+
		"}\n")

	write("statistics.request.qr-tall-3x2.json", "{\n"+
		`  "matrices": [`+"\n"+
		labeled(t, "Q", q)+",\n"+
		labeled(t, "R", r)+"\n"+
		"  ]\n}\n")
}

func labeled(t *testing.T, label string, rows [][]float64) string {
	t.Helper()
	return "    {\n" +
		`      "label": "` + label + `",` + "\n" +
		`      "values": ` + rowsJSON(t, rows, "      ") + "\n" +
		"    }"
}

// rowsJSON prints one row per line, the house style of contracts/examples.
func rowsJSON(t *testing.T, rows [][]float64, indent string) string {
	t.Helper()
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for _, v := range row {
			encoded, err := json.Marshal(v)
			require.NoError(t, err)
			cells = append(cells, string(encoded))
		}
		lines = append(lines, indent+"  ["+strings.Join(cells, ", ")+"]")
	}
	return "[\n" + strings.Join(lines, ",\n") + "\n" + indent + "]"
}
