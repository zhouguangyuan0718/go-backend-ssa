package gcintegration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestPinnedRaceDriver(t *testing.T) {
	if os.Getenv("GO_SSA_RACE_INTEGRATION") == "" {
		t.Skip("set GO_SSA_RACE_INTEGRATION and Linux driver fixture inputs")
	}
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	f.write(t, "go.mod", "module example/raceprobe\n\ngo 1.27\n")
	f.write(t, "main.go", `package main
import("fmt";"os";"sync";"unsafe")
var value int
func main(){
 mode:="safe";if len(os.Args)>1{mode=os.Args[1]}
 if mode=="checkptr" {a:=new([2]int);p:=uintptr(unsafe.Pointer(&a[0]));fmt.Println(*(*int)(unsafe.Pointer(p+3*unsafe.Sizeof(a[0]))));return}
 var wg sync.WaitGroup;var mu sync.Mutex;start:=make(chan struct{});wg.Add(2)
 for range 2{go func(){defer wg.Done();<-start;for range 100{if mode=="safe"{mu.Lock()};value++;if mode=="safe"{mu.Unlock()}}}()}
 close(start);wg.Wait();fmt.Println(mode,value)
}
`)
	for _, library := range []bool{false, true} {
		t.Run(fmt.Sprint(library), func(t *testing.T) {
			name := fmt.Sprintf("race-%t", library)
			f.goRun(t, library, name, "build", "-a", "-race", "-p=4", "-o", "/work/"+name, ".")
			if actual := f.execute(t, "/work/"+name); string(actual) != "safe 200\n" {
				t.Fatalf("safe execution: %q", actual)
			}
			out, err := f.commandResult([]string{"GORACE=halt_on_error=1 exitcode=66"}, "/work/"+name, "racy")
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 66 || !bytes.Contains(out, []byte("WARNING: DATA RACE")) {
				t.Fatalf("race not detected: %v\n%s", err, out)
			}
			out, err = f.commandResult(nil, "/work/"+name, "checkptr")
			if err == nil || !bytes.Contains(out, []byte("checkptr:")) {
				t.Fatalf("implicit race checkptr was not enforced: %v\n%s", err, out)
			}
			if library {
				f.verifyTrace(t, name, "runtime", "runtime/cgo", "runtime/race", "sync", "main")
			}
			t.Log("safe execution, deliberate race and implicit checkptr match native behavior")
		})
	}
}
