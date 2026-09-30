package pkginit

import (
	"bytes"
	"go-backend-ssa/internal/ir"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
)

type phaseReport struct {
	bytes.Buffer
	opens, closes int
}

func (r *phaseReport) Close() error { r.closes++; return nil }
func capturePhaseReports(t *testing.T, g *ir.Context) func() string {
	t.Helper()
	reports := map[string]*phaseReport{}
	g.OpenAST = func(p string, f int, m fs.FileMode) (io.WriteCloser, error) {
		r := reports[p]
		if r == nil {
			r = new(phaseReport)
			reports[p] = r
		}
		if f&os.O_TRUNC != 0 {
			r.Reset()
		}
		r.opens++
		return r, nil
	}
	g.ASTWorkingDirectory = func() (string, error) { return "/reports", nil }
	t.Cleanup(func() {
		g.CloseHTMLWriters()
		for p, r := range reports {
			if r.opens != r.closes {
				t.Error("unclosed report", p, r.opens, r.closes)
			}
		}
	})
	return func() string {
		var text strings.Builder
		for p, r := range reports {
			if strings.HasSuffix(p, ".ast") {
				text.WriteString(r.String())
			}
		}
		return text.String()
	}
}
