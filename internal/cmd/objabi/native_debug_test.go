package objabi_test

import (
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/objabi"
	"os"
	"os/exec"
	"testing"
)

func TestNativeDebugHelp(t *testing.T) {
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for official compiler comparison")
	}
	flags := base.DefaultDebugFlags()
	help, err := objabi.ParseDebugFlags(&flags, func(string, string, int, string) string { return "" }, []string{"help"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "tool", "compile", "-d=help")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOCOMPILEDEBUG=", "GOSSAFUNC=")
	reference, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("official help: %v\n%s", err, reference)
	}
	if help != string(reference) {
		t.Fatalf("native help differs: official=%d library=%d\n%s", len(reference), len(help), help)
	}
}
