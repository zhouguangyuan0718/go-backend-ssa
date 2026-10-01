package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go-backend-ssa/compiler"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const debugSource = `package p
func Add(x,y float64)float64{return x*y+1.5}
func Escape(x int)*int{return &x}
func Sum(values []int)int{n:=0;for i:=0;i<len(values);i++{n+=values[i]};return n}
func Use()int{return *Escape(Sum([]int{1,2,3}))}
`

func TestDebugCompilerOracle(t *testing.T) {
	file := filepath.Join(t.TempDir(), "debug.go")
	if err := os.WriteFile(file, []byte(debugSource), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, mode := range []struct {
			name  string
			flags []string
			m     int
		}{
			{"ssa-check", []string{"ssa/check/on"}, 0},
			{"cse-disabled", []string{"ssa/generic_cse/off,ssa/lowered_cse/off"}, 0},
			{"hash-quiet", []string{"fmahash=qn,checkptr=0"}, 0},
			{"hash-report", []string{"fmahash=vy"}, 0},
			{"escape", nil, 1}, {"escape-detail", nil, 2},
			{"prove", []string{"ssa/prove/debug=1"}, 0},
		} {
			t.Run(arch+"/"+mode.name, func(t *testing.T) {
				t.Parallel()
				req := request("p", debugSource)
				req.Files[0].Name = file
				if arch == "arm64" {
					req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
				}
				req.Options.OptimizationReport = mode.m
				req.Options.Workers = 4
				req.Debug.Flags = mode.flags
				result, err := compiler.Compile(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				output := bytes.Clone(result.DebugOutput)
				if mode.m > 0 && !bytes.Contains(output, []byte("moved to heap")) {
					t.Fatal("missing escape report", string(output))
				}
				if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
					return
				}
				archive := filepath.Join(t.TempDir(), "native.a")
				args := []string{"tool", "compile", "-p=p", "-o", archive}
				for _, d := range mode.flags {
					args = append(args, "-d="+d)
				}
				if mode.m > 0 {
					args = append(args, fmt.Sprintf("-m=%d", mode.m))
				}
				args = append(args, file)
				cmd := exec.Command("go", args...)
				cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=")
				reference, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("official: %v\n%s", err, reference)
				}
				native, err := os.ReadFile(archive)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(native, result.Archive) {
					t.Fatalf("archive differs: official=%d library=%d", len(native), len(result.Archive))
				}
				if !bytes.Equal(reference, output) {
					t.Fatalf("debug output differs: official=%d library=%d\nofficial:\n%s\nlibrary:\n%s", len(reference), len(output), reference, output)
				}
			})
		}
	}
}

func TestDebugSettingsErrorsAndIsolation(t *testing.T) {
	for _, value := range []string{"no-such-key=1", "checkptr=bad", "ssa/no-such-phase/on", "fmahash=???", "fipshash=???", "ssa/genssa/dump=F", "ssa/all/time", "ssa/all/mem"} {
		t.Run(value, func(t *testing.T) {
			req := request("p", debugSource)
			req.Debug.Flags = []string{value}
			result, err := compiler.Compile(context.Background(), req)
			if err == nil || result != nil {
				t.Fatal("invalid settings accepted")
			}
		})
	}
	req := request("p", debugSource)
	req.Debug.Flags = []string{"help"}
	result, err := compiler.Compile(context.Background(), req)
	var help *compiler.DebugHelp
	if result != nil || !errors.As(err, &help) || !strings.Contains(help.Text, "ssa/help") {
		t.Fatal("help", result, err)
	}
	req.Debug = compiler.DebugOptions{}
	result, err = compiler.Compile(context.Background(), req)
	if err != nil || len(result.DebugOutput) != 0 || len(result.Diagnostics) != 0 {
		t.Fatal("debug state leaked", err)
	}
	for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
		req.Options.Stage = stage
		req.Options.OptimizationReport = 1
		req.Debug.Flags = []string{"ssa/check/on"}
		p, err := compiler.Build(context.Background(), req)
		if err != nil || len(p.DebugOutput) == 0 {
			t.Fatal("SSA stage report", stage, err)
		}
	}
}

func TestDebugDumpProviders(t *testing.T) {
	file := filepath.Join(t.TempDir(), "dump.go")
	if err := os.WriteFile(file, []byte(debugSource), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			directory := t.TempDir()
			req := request("p", debugSource)
			req.Files[0].Name = file
			if arch == "arm64" {
				req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
			}
			req.Debug = compiler.DebugOptions{Flags: []string{"ssa/genssa/dump=Sum"}, Directory: directory, OpenFile: func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				return os.OpenFile(path, flags, mode)
			}}
			result, err := compiler.Compile(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			files, err := filepath.Glob(filepath.Join(directory, "*.dump"))
			if err != nil || len(files) != 1 {
				t.Fatal("dump files", files, err)
			}
			dump, err := os.ReadFile(files[0])
			if err != nil || !bytes.Contains(dump, []byte("TEXT")) {
				t.Fatal("assembly dump", string(dump), err)
			}
			if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
				return
			}
			referenceDirectory := t.TempDir()
			archive := filepath.Join(referenceDirectory, "native.a")
			cmd := exec.Command("go", "tool", "compile", "-p=p", "-d=ssa/genssa/dump=Sum", "-o", archive, file)
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR="+referenceDirectory)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("official: %v\n%s", err, out)
			}
			reference, err := os.ReadFile(filepath.Join(referenceDirectory, filepath.Base(files[0])))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(reference, dump) {
				t.Fatalf("dump differs:\nofficial:\n%s\nlibrary:\n%s", reference, dump)
			}
			native, err := os.ReadFile(archive)
			if err != nil || !bytes.Equal(native, result.Archive) {
				t.Fatal("dump mode archive differs", err)
			}
		})
	}
}

type countedDebugFile struct {
	*os.File
	closes *int
}

func (f *countedDebugFile) Close() error { *f.closes++; return f.File.Close() }

func TestDebugHTMLProviders(t *testing.T) {
	file := filepath.Join(t.TempDir(), "html.go")
	if err := os.WriteFile(file, []byte(debugSource), 0600); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []compiler.Stage{"", compiler.Built, compiler.EarlyOptimized} {
		t.Run(string(stage), func(t *testing.T) {
			req := request("p", debugSource)
			req.Files[0].Name = file
			req.Options.Stage = stage
			req.Options.Workers = 1
			opened, closed := 0, 0
			directory := t.TempDir()
			req.Debug = compiler.DebugOptions{Function: "Sum", Directory: directory, OpenFile: func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				f, err := os.OpenFile(path, flags, mode)
				if err != nil {
					return nil, err
				}
				opened++
				return &countedDebugFile{f, &closed}, nil
			}, MakeDirectory: os.MkdirAll, OpenSource: func(path string) (io.ReadCloser, error) { return os.Open(path) }}
			if stage == "" {
				if _, err := compiler.Compile(context.Background(), req); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := compiler.Build(context.Background(), req); err != nil {
					t.Fatal(err)
				}
			}
			if opened == 0 || opened != closed {
				t.Errorf("debug file lifecycle: opened=%d closed=%d", opened, closed)
			}
			files, err := filepath.Glob(filepath.Join(directory, "*.html"))
			if err != nil || len(files) != 1 {
				t.Fatal("HTML files", files, err)
			}
			html, err := os.ReadFile(files[0])
			if err != nil || !bytes.Contains(html, []byte("</html>")) || !bytes.Contains(html, []byte("Sum")) {
				t.Fatal("unfinished function report", err)
			}
		})
	}
}

type failedDebugStream struct {
	failure error
	closes  *int
}

func (s *failedDebugStream) Write([]byte) (int, error) { return 0, s.failure }
func (s *failedDebugStream) Close() error              { *s.closes++; return nil }

func TestDebugOutputFailureAndCancellation(t *testing.T) {
	sentinel := errors.New("report output failed")
	req := request("p", debugSource)
	closed := 0
	req.Debug = compiler.DebugOptions{Flags: []string{"ssa/genssa/dump=Sum"}, OpenFile: func(string, int, fs.FileMode) (io.WriteCloser, error) {
		return &failedDebugStream{sentinel, &closed}, nil
	}}
	result, err := compiler.Compile(context.Background(), req)
	if result != nil || !errors.Is(err, sentinel) || closed != 1 {
		t.Fatal("report write failure", result, err, closed)
	}
	for _, stage := range []compiler.Stage{"", compiler.Built, compiler.EarlyOptimized} {
		t.Run("cancel-"+string(stage), func(t *testing.T) {
			directory := t.TempDir()
			file := filepath.Join(directory, "cancel.go")
			if err := os.WriteFile(file, []byte(debugSource), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := request("p", debugSource)
			req.Files[0].Name = file
			req.Options.Stage = stage
			req.Options.Workers = 1
			opened, closed := 0, 0
			req.Debug = compiler.DebugOptions{Function: "Sum", Directory: directory, OpenFile: func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				out, err := os.OpenFile(path, flags, mode)
				if err != nil {
					return nil, err
				}
				opened++
				cancel()
				return &countedDebugFile{out, &closed}, nil
			}, MakeDirectory: os.MkdirAll, OpenSource: func(path string) (io.ReadCloser, error) { return os.Open(path) }}
			if stage == "" {
				r, e := compiler.Compile(ctx, req)
				if r != nil || !errors.Is(e, context.Canceled) {
					t.Fatal("cancelled native output", r, e)
				}
			} else {
				p, e := compiler.Build(ctx, req)
				if p != nil || !errors.Is(e, context.Canceled) {
					t.Fatal("cancelled SSA output", p, e)
				}
			}
			if opened != 1 || closed != 1 {
				t.Fatal("cancelled report not closed", opened, closed)
			}
		})
	}
}
