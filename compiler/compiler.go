// Package compiler compiles Go source with the migrated native frontend and
// backend. All source, dependencies and target settings are explicit inputs.
// Independent calls own independent compiler graphs and may run concurrently.
package compiler

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go-backend-ssa/internal/base"
	"go/version"
	"io"
	"io/fs"
	"maps"
	"strconv"
	"strings"
	"time"

	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/amd64"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/coverage"
	"go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/liveness"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/pkginit"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/staticinit"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
	"go-backend-ssa/internal/walk"
)

const Version = "go1.27.1"
const UpstreamCommit = "862c888e612ac346c7c4d99c9392bdfd265f33b0"

type Target struct {
	GOOS, GOARCH string
	GOAMD64      int
	// Experiments is parsed by the pinned native parser, independent of the host.
	Experiments string
}
type SourceFile struct {
	Name string
	Data []byte
}
type Options struct {
	Workers                                            int
	DisableInlining, DisableOptimization, DisableDwarf bool
	Complete, Standard, CompilingRuntime               bool
	AssemblyHeader, NoLocalImports                     bool
	// Language defaults to go1.27 and may select an earlier language version.
	Language, BuildID, TrimPath, LocalImportPrefix string
}
type EmbedConfig struct {
	Patterns map[string][]string
	Files    map[string]string
	Open     func(string) (fs.File, error)
}

// SourcePaths controls native position and import path handling without reading
// process state. Empty WorkingDirectory and HostOS use / and linux.
type SourcePaths struct {
	WorkingDirectory, GOROOT, HostOS string
}
type Request struct {
	ImportPath string
	Files      []SourceFile
	Target     Target
	Options    Options
	Paths      SourcePaths
	// Imports maps canonical import paths to complete matching compiler archives.
	// Inputs must remain unchanged during Compile. Returned data is independent.
	Imports   map[string][]byte
	ImportMap map[string]string
	// SymABIs supplies native assembler ABI declarations, when present.
	SymABIs []byte
	Embed   EmbedConfig
}
type Result struct {
	Archive        []byte
	AssemblyHeader []byte
	PackageName    string
	ImportPath     string
	Target         Target
	Diagnostics    []diagnostic.Diagnostic
}
type Error struct{ Diagnostics []diagnostic.Diagnostic }

func (e *Error) Error() string {
	var messages []string
	for _, d := range e.Diagnostics {
		if d.Severity == diagnostic.Error {
			messages = append(messages, d.String())
		}
	}
	return strings.Join(messages, "\n")
}

// Compile returns an owned native compiler/linker archive after all compiler
// checks succeed. It never invokes an external compiler, reads process settings,
// or opens dependencies implicitly. Cancellation is checked between synchronous
// stages; backend workers finish before this call returns. This native output
// entry point does not implement the independent Built/EarlyOptimized SSA API.
func Compile(ctx context.Context, req Request) (result *Result, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("compiler requires a context")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if req.Target.GOOS != "linux" || req.Target.GOARCH != "amd64" || req.Target.GOAMD64 < 1 || req.Target.GOAMD64 > 4 {
		return nil, fmt.Errorf("unsupported native target: %+v", req.Target)
	}
	if req.ImportPath == "" || len(req.Files) == 0 || req.Options.Workers < 0 {
		return nil, fmt.Errorf("compiler requires an import path, source files and nonnegative worker count")
	}
	language := req.Options.Language
	if language == "" {
		language = "go1.27"
	}
	if !version.IsValid(language) || version.Lang(language) != language || version.Compare(language, "go1.27") > 0 {
		return nil, fmt.Errorf("unsupported Go language version %q", language)
	}
	workingDirectory := req.Paths.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = "/"
	}
	hostOS := req.Paths.HostOS
	if hostOS == "" {
		hostOS = "linux"
	}
	exp, e := buildcfg.ParseGOEXPERIMENT(req.Target.GOOS, req.Target.GOARCH, req.Target.Experiments)
	if e != nil {
		return nil, e
	}
	arch := sys.NewContext().ArchAMD64
	link := &obj.Link{Arch: &obj.LinkArch{Arch: arch}, TargetOS: req.Target.GOOS, Pkgpath: req.ImportPath, Pathname: workingDirectory, UseBASEntries: true, Flag_locationlists: !req.Options.DisableDwarf}
	if e = link.Headtype.Set(req.Target.GOOS); e != nil {
		return nil, e
	}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg(req.ImportPath, "")
	u.BuiltinPkg.Prefix = "go:builtin"
	u.UnsafePkg.Name = "unsafe"
	u.Experiment = *exp
	graph := ir.NewContext(u, link)
	graph.Debug = base.DefaultDebugFlags()
	diagnostics := func() []diagnostic.Diagnostic {
		return append(graph.Diagnostics.Diagnostics(), u.Diagnostics.Diagnostics()...)
	}
	errors := func() bool { return graph.Diagnostics.HasErrors() || u.Diagnostics.HasErrors() || link.Errors != 0 }
	link.DiagFunc = func(f string, a ...any) { graph.Diagnostics.Add(diagnostic.Error, "", src.Pos{}, fmt.Sprintf(f, a...)) }
	defer func() {
		failure := recover()
		graph.CloseHTMLWriters()
		if failure != nil {
			panic(failure)
		}
		if errors() {
			result = nil
			err = &Error{Diagnostics: diagnostics()}
		}
	}()
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	tc.InitRuntime()
	flags := &graph.Base.Flag
	flags.Lang = language
	flags.BuildID = req.Options.BuildID
	flags.TrimPath = req.Options.TrimPath
	flags.NoLocalImports = req.Options.NoLocalImports
	flags.D = req.Options.LocalImportPrefix
	if req.Options.AssemblyHeader {
		flags.AsmHdr = "explicit-output"
	}
	flags.Complete = req.Options.Complete
	flags.Std = req.Options.Standard
	flags.CompilingRuntime = req.Options.CompilingRuntime
	flags.WB = true
	flags.Dwarf = !req.Options.DisableDwarf
	flags.LowerC = req.Options.Workers
	if flags.LowerC == 0 {
		flags.LowerC = 1
	}
	flags.LowerL = 1
	if req.Options.DisableInlining {
		flags.LowerL = 0
	}
	if req.Options.DisableOptimization {
		flags.N = 1
	}
	flags.GenDwarfInl = 2
	link.CompressInstructions = graph.Debug.CompressInstructions != 0
	link.Flag_optimize = !req.Options.DisableOptimization
	if exp.LoopVar {
		graph.Debug.LoopVar = 1
	}
	graph.Base.PrepareNativeFlags(&graph.Debug)
	graph.Base.ConvertHash = graph.Base.NewHashDebug("converthash", "qn", nil)
	flags.Cfg.ImportMap = maps.Clone(req.ImportMap)
	flags.Cfg.PackageFile = make(map[string]string, len(req.Imports))
	archives := make(map[string][]byte, len(req.Imports))
	for name, data := range req.Imports {
		flags.Cfg.PackageFile[name] = name
		archives[name] = bytes.Clone(data)
	}
	flags.Cfg.Embed.Files = maps.Clone(req.Embed.Files)
	flags.Cfg.Embed.Patterns = make(map[string][]string, len(req.Embed.Patterns))
	for pattern, files := range req.Embed.Patterns {
		flags.Cfg.Embed.Patterns[pattern] = append([]string(nil), files...)
	}
	nd, e := noder.NewContext(types2.NewContext(), tc)
	if e != nil {
		return nil, e
	}
	nd.GOROOT = req.Paths.GOROOT
	writer := objw.NewContext(u, link, req.Target.GOOS)
	rt := rttype.NewContext(writer)
	rt.Init()
	sd := staticdata.NewContext(graph, writer)
	sd.OpenFile = req.Embed.Open
	if req.Embed.Open != nil {
		link.OpenObjectInput = func(name string) (io.ReadCloser, error) { return req.Embed.Open(name) }
	}
	nd.ReflectData = reflectdata.NewContext(tc, rt, sd)
	nd.StaticInit = staticinit.NewContext(tc, nd.ReflectData)
	backend := ssagen.NewContext(tc)
	backend.Inline = nd.Inline
	backend.SSA = ssa.NewContext(abi.NewContext(graph), nd.ReflectData, nd.Inline.Log)
	backend.SSA.GOAMD64 = req.Target.GOAMD64
	backend.SSA.FramePointerEnabled = true
	backend.SSA.Now = func() time.Time { return time.Time{} }
	backend.Liveness = liveness.NewContext(backend.SSA)
	amd64.NewContext(backend)
	dwarf := dwarfgen.NewContext(backend, nd.ReflectData)
	producer := flag.NewFlagSet("compile", flag.ContinueOnError)
	producer.SetOutput(io.Discard)
	producer.Var(new(countFlag), "N", "")
	producer.Var(new(countFlag), "l", "")
	producer.Bool("dwarf", true, "")
	if flags.N != 0 {
		_ = producer.Set("N", "1")
	}
	if req.Options.DisableInlining {
		_ = producer.Set("l", "1")
	}
	if req.Options.DisableDwarf {
		_ = producer.Set("dwarf", "false")
		flags.GenDwarfInl = 0
		link.DwFixups = nil
		link.DebugInfo = nil
		link.GenAbstractFunc = nil
	}
	dwarf.Flags = producer
	dwarf.RecordFlags("N", "l", "dwarf")
	backend.InitTables()
	config := gc.ObjectConfig{GOOS: req.Target.GOOS, GOARCH: req.Target.GOARCH, GOAMD64: req.Target.GOAMD64, Version: Version}
	nd.BindImports(config, hostOS, func(name string) (noder.DependencyFile, error) {
		data, ok := archives[name]
		if !ok {
			return nil, fmt.Errorf("dependency archive not supplied: %q", name)
		}
		return &archiveInput{Reader: bytes.NewReader(data), name: name}, nil
	})
	parsed := make([]*noder.ParsedFile, len(req.Files))
	seen := make(map[string]bool, len(req.Files))
	for i, file := range req.Files {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if file.Name == "" || seen[file.Name] {
			return nil, fmt.Errorf("source filename is empty or duplicated: %q", file.Name)
		}
		seen[file.Name] = true
		parsed[i], e = nd.ParseFile(file.Name, strings.NewReader(string(file.Data)))
		if e != nil {
			return nil, e
		}
	}
	if _, e = nd.CompileParsedPackage(parsed); e != nil {
		return nil, e
	}
	if len(tc.Target.Embeds) != 0 && req.Embed.Open == nil {
		return nil, fmt.Errorf("embedded files require an explicit input provider")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	dwarf.RecordPackageName()
	backend.InitConfig()
	coverage.NewContext(tc).Fixup()
	syms := backend.NewSymABIs()
	if len(req.SymABIs) != 0 {
		data := bytes.Clone(req.SymABIs)
		backend.ReadFile = func(string) ([]byte, error) { return data, nil }
		syms.ReadSymABIs("input.symabis")
	}
	finalizer := gc.NewContext(writer)
	finalizer.BindPackage(pkginit.NewContext(nd))
	finalizer.BindBackend(backend, walk.NewContext(nd, escape.NewContext(tc, nd.Inline.Log)))
	finalizer.PrepareFrontend(syms, nil)
	if errors() {
		return nil, &Error{Diagnostics: diagnostics()}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	finalizer.CompileDeclarations(syms, nil)
	if errors() {
		return nil, &Error{Diagnostics: diagnostics()}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	finalizer.FinishPackage()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if errors() {
		return nil, &Error{Diagnostics: diagnostics()}
	}
	finalizer.SetObjectHeader(config)
	var output archiveOutput
	if e = finalizer.WriteArchive(&output, gc.CombinedObject); e != nil {
		return nil, e
	}
	var assembly archiveOutput
	if req.Options.AssemblyHeader {
		if e = finalizer.WriteAssemblyHeader(&assembly); e != nil {
			return nil, e
		}
	}
	finalizer.CheckPackage()
	if errors() {
		return nil, &Error{Diagnostics: diagnostics()}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &Result{Archive: output.data, AssemblyHeader: assembly.data, PackageName: u.LocalPkg.Name, ImportPath: req.ImportPath, Target: req.Target, Diagnostics: diagnostics()}, nil
}

type archiveInput struct {
	*bytes.Reader
	name string
}

func (f *archiveInput) Close() error { return nil }
func (f *archiveInput) Name() string { return f.name }

type archiveOutput struct {
	data   []byte
	offset int64
}

func (f *archiveOutput) Write(p []byte) (int, error) {
	end := f.offset + int64(len(p))
	if end < f.offset || f.offset < 0 {
		return 0, fmt.Errorf("archive offset overflow")
	}
	if end > int64(len(f.data)) {
		f.data = append(f.data, make([]byte, end-int64(len(f.data)))...)
	}
	copy(f.data[f.offset:end], p)
	f.offset = end
	return len(p), nil
}
func (f *archiveOutput) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		off += f.offset
	case io.SeekEnd:
		off += int64(len(f.data))
	default:
		return 0, fmt.Errorf("invalid archive seek")
	}
	if off < 0 {
		return 0, fmt.Errorf("negative archive seek")
	}
	f.offset = off
	return off, nil
}

type countFlag int

func (c *countFlag) String() string { return strconv.Itoa(int(*c)) }
func (c *countFlag) Get() any       { return int(*c) }
func (c *countFlag) Set(s string) error {
	v, e := strconv.Atoi(s)
	if e == nil {
		*c = countFlag(v)
	}
	return e
}
func (c *countFlag) IsCountFlag() bool { return true }
