package gcintegration

import (
	"os"
	"testing"
)

func TestPinnedCompilerRegression(t *testing.T) {
	if os.Getenv("GO_SSA_COMPILER_REGRESSION") == "" {
		t.Skip("set GO_SSA_COMPILER_REGRESSION and Linux driver fixture inputs")
	}
	testPinnedNativeRegression(t, "compiler", "cmd/compile/internal/ssa", "cmd/compile/internal/test")
}

func TestPinnedRangeFuncRegression(t *testing.T) {
	if os.Getenv("GO_SSA_COMPILER_REGRESSION") == "" {
		t.Skip("set GO_SSA_COMPILER_REGRESSION and Linux driver fixture inputs")
	}
	testPinnedNativeRegressionRun(t, "rangefunc", "^TestIssue77597$", "cmd/compile/internal/test")
}

func TestPinnedAssemblyListingRegression(t *testing.T) {
	if os.Getenv("GO_SSA_COMPILER_REGRESSION") == "" {
		t.Skip("set GO_SSA_COMPILER_REGRESSION and Linux driver fixture inputs")
	}
	testPinnedNativeRegressionRun(t, "assembly-listing", "^(TestDashS|TestIssue16214)$", "cmd/compile/internal/test")
}
