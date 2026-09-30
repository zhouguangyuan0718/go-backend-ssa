package compiler_test

import (
	"bytes"
	"context"
	"go-backend-ssa/compiler"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeProfileCompilerOracle(t *testing.T) {
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for pinned compiler/objdump oracle")
	}
	dir := t.TempDir()
	source, e := os.ReadFile("../testdata/pgo/hot.go")
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, "hot.go")
	if e = os.WriteFile(file, source, 0600); e != nil {
		t.Fatal(e)
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off", "GO111MODULE=off")
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("go %v: %v\n%s", args, e, b)
		}
		return b
	}
	var hotArchive []byte
	for _, name := range []string{"cold", "hot.pb", "hot.pprof", "hot.preprofile"} {
		req := request("p", string(source))
		req.Files[0].Name = file
		req.Options.Complete = true
		ref := filepath.Join(dir, name+".a")
		args := []string{"tool", "compile", "-p", "p", "-complete", "-o", ref}
		if name != "cold" {
			data, e := os.ReadFile("../testdata/pgo/" + name)
			if e != nil {
				t.Fatal(e)
			}
			profile := filepath.Join(dir, name)
			if e = os.WriteFile(profile, data, 0600); e != nil {
				t.Fatal(e)
			}
			req.PGOProfile = &compiler.ProfileInput{Name: profile, Data: data}
			args = append(args, "-pgoprofile", profile)
			if name == "hot.pprof" {
				processed := filepath.Join(dir, "official.preprofile")
				run("tool", "preprofile", "-i", profile, "-o", processed)
				actual, e := os.ReadFile(processed)
				if e != nil {
					t.Fatal(e)
				}
				expected, e := os.ReadFile("../testdata/pgo/hot.preprofile")
				if e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(actual, expected) {
					t.Fatalf("raw profile differs from official preprocessing: %s", actual)
				}
			}
		}
		run(append(args, file)...)
		result, e := compiler.Compile(context.Background(), req)
		if e != nil {
			t.Fatal(e)
		}
		expected, e := os.ReadFile(ref)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(result.Archive, expected) {
			t.Fatalf("%s archive differs: library=%d native=%d", name, len(result.Archive), len(expected))
		}
		ours := filepath.Join(dir, "library-"+name+".a")
		if e = os.WriteFile(ours, result.Archive, 0600); e != nil {
			t.Fatal(e)
		}
		assembly := run("tool", "objdump", "-s", "^p.Caller$", ours)
		if len(assembly) == 0 {
			t.Fatal("no Caller machine code")
		}
		if name == "cold" {
			if !bytes.Contains(assembly, []byte("p.Hot")) {
				t.Fatalf("cold baseline did not call Hot: %s", assembly)
			}
		} else {
			if bytes.Contains(assembly, []byte("p.Hot")) {
				t.Fatalf("hot call was not inlined: %s", assembly)
			}
			if hotArchive == nil {
				hotArchive = bytes.Clone(result.Archive)
			} else if !bytes.Equal(hotArchive, result.Archive) {
				t.Fatal("profile formats changed native code")
			}
		}
		t.Logf("%s: full archive matches official compiler; hot-call machine code verified", name)
	}
}
