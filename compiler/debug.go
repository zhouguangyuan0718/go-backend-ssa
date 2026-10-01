package compiler

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/objabi"
	"io"
	"io/fs"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// DebugOptions supplies native debug settings and optional host services.
// Flags are applied in order with the native -d syntax. No process environment,
// output path, source file, clock, or command is discovered implicitly.
type DebugOptions struct {
	Flags               []string
	Function, Directory string
	OpenFile            func(string, int, fs.FileMode) (io.WriteCloser, error)
	MakeDirectory       func(string, fs.FileMode) error
	OpenSource          func(string) (io.ReadCloser, error)
	ExpandSourcePath    func(string) string
	Now                 func() time.Time
	ReadMemStats        func(*runtime.MemStats)
	LookPath            func(string) (string, error)
	Command             func(string, ...string) *exec.Cmd
}

// DebugHelp carries native help text instead of terminating the host process.
type DebugHelp struct{ Text string }

func (h *DebugHelp) Error() string { return h.Text }

type ssaDebugOption struct {
	phase, flag string
	value       int
	text        string
}

func parseDebugOptions(debug *base.DebugFlags, settings DebugOptions) ([]ssaDebugOption, error) {
	var phases []ssaDebugOption
	help, err := objabi.ParseDebugFlags(debug, func(phase, flag string, value int, text string) string {
		phases = append(phases, ssaDebugOption{phase, flag, value, text})
		return ""
	}, settings.Flags)
	if err != nil {
		return nil, err
	}
	if help != "" {
		return nil, &DebugHelp{help}
	}
	if settings.Function != "" && (settings.OpenFile == nil || settings.MakeDirectory == nil || settings.OpenSource == nil) {
		return nil, fmt.Errorf("SSA function reports require explicit output, directory, and source providers")
	}
	if debug.AstDump != "" && settings.OpenFile == nil {
		return nil, fmt.Errorf("AST reports require an explicit output provider")
	}
	for _, p := range phases {
		if p.flag == "dump" && settings.OpenFile == nil {
			return nil, fmt.Errorf("SSA dumps require an explicit output provider")
		}
		if p.flag == "time" && p.value != 0 && settings.Now == nil {
			return nil, fmt.Errorf("SSA timing requires an explicit clock")
		}
		if p.flag == "mem" && p.value != 0 && settings.ReadMemStats == nil {
			return nil, fmt.Errorf("SSA memory reports require an explicit memory statistics provider")
		}
	}
	return phases, nil
}

type debugBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *debugBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *debugBuffer) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.data.Bytes())
}

// debugFiles closes optional report streams even when construction stops at an
// SSA boundary or a compiler/provider fails. Native closes remain single-use.
type debugFiles struct {
	mu    sync.Mutex
	files []*debugFile
}
type debugFile struct {
	mu      sync.Mutex
	output  io.WriteCloser
	closed  bool
	failure error
}

func (f *debugFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.output.Write(p)
	if f.failure == nil {
		f.failure = err
	}
	return n, err
}
func (f *debugFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		err := f.output.Close()
		if f.failure == nil {
			f.failure = err
		}
	}
	return f.failure
}
func (f *debugFiles) wrap(open func(string, int, fs.FileMode) (io.WriteCloser, error)) func(string, int, fs.FileMode) (io.WriteCloser, error) {
	return func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
		output, err := open(path, flags, mode)
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, fmt.Errorf("debug output provider returned a nil stream")
		}
		file := &debugFile{output: output}
		f.mu.Lock()
		f.files = append(f.files, file)
		f.mu.Unlock()
		return file, nil
	}
}
func (f *debugFiles) close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var failure error
	for _, file := range f.files {
		if err := file.Close(); err != nil && failure == nil {
			failure = err
		}
	}
	return failure
}
