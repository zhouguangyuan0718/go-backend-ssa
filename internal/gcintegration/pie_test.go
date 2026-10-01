package gcintegration

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedPIEDriver(t *testing.T) {
	if os.Getenv("GO_SSA_PIE_INTEGRATION") == "" {
		t.Skip("set GO_SSA_PIE_INTEGRATION and Linux driver fixture inputs")
	}
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	f.write(t, "go.mod", "module example/pieprobe\n\ngo 1.27\n")
	f.write(t, "main.go", `package main
/*
static int add(int a,int b){return a+b;}
*/
import "C"
import("fmt";"runtime")
var X=40
//go:noinline
func F()int{return int(C.add(C.int(X),2))}
func main(){fn:=F;runtime.GC();fmt.Println(fn())}
`)
	for _, mode := range []string{"internal", "external"} {
		t.Run(mode, func(t *testing.T) {
			for _, library := range []bool{false, true} {
				name := fmt.Sprintf("pie-%s-%t", mode, library)
				f.goRun(t, library, name, "build", "-a", "-p=4", "-buildmode=pie", "-ldflags=-linkmode="+mode, "-o", "/work/"+name, ".")
				image, err := elf.Open(filepath.Join(f.work, name))
				if err != nil {
					t.Fatal(err)
				}
				typ := image.Type
				image.Close()
				if typ != elf.ET_DYN {
					t.Fatal("output is not PIE", typ)
				}
				if out := f.execute(t, "/work/"+name); string(out) != "42\n" {
					t.Fatalf("PIE execution: %q", out)
				}
				if library {
					f.verifyTrace(t, name, "runtime", "runtime/cgo", "main")
				}
			}
		})
	}
}
