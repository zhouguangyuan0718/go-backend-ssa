// Package compiler compiles Go source with the migrated native frontend and
// backend. All source, dependencies and target settings are explicit inputs.
// Independent calls own independent compiler graphs and may run concurrently.
package compiler

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"flag"
	"fmt"
	"go-backend-ssa/internal/base"
	"go/version"
	"io"
	"io/fs"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"

	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/amd64"
	"go-backend-ssa/internal/arm64"
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
	"go-backend-ssa/internal/pgoir"
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
	// GOARM64 uses the native feature syntax; empty selects v8.0 on arm64.
	GOARM64 string `json:",omitempty"`
	// Experiments is parsed by the pinned native parser, independent of the host.
	Experiments string
}
type SourceFile struct {
	Name string
	Data []byte
}
type Options struct {
	// AssemblyListing selects native -S detail: zero disables it, 1 prints symbols
	// and instructions, 2 adds DWARF auxiliary symbols, and 3 adds symbol indexes.
	// Only Compile emits machine assembly; Build rejects nonzero levels.
	AssemblyListing int
	// OptimizationReport selects native -m detail; zero disables it.
	OptimizationReport int
	// Diagnostic flags correspond to native -e, -C, and -L.
	UnlimitedErrors, OmitErrorColumns, ActualErrorFiles bool
	// ClobberDead and ClobberDeadReg enable native dead stack/register debugging.
	ClobberDead, ClobberDeadReg bool
	// Spectre uses native comma-separated all, index, and ret mitigation settings.
	Spectre string
	// Stage applies to Build only; the empty value selects Built.
	Stage                                              Stage
	Workers                                            int
	DisableInlining, DisableOptimization, DisableDwarf bool
	Complete, Standard, CompilingRuntime               bool
	AssemblyHeader, NoLocalImports                     bool
	Race                                               bool
	Shared                                             bool
	InstallSuffix                                      string
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

// ProfileInput supplies a raw (optionally gzip-compressed) pprof profile or a
// native preprocessed profile. Name is used only to identify diagnostics.
type ProfileInput struct {
	Name string
	Data []byte
}
type Request struct {
	ImportPath string
	Files      []SourceFile
	Target     Target
	Options    Options
	Debug      DebugOptions
	Paths      SourcePaths
	// Imports maps canonical import paths to complete matching compiler archives.
	// Inputs must remain unchanged during Compile. Returned data is independent.
	Imports map[string][]byte
	// Artifacts supplies immutable library import artifacts. Include their exact
	// dependency closure; an import path cannot also appear in Imports.
	Artifacts map[string]*Artifact
	ImportMap map[string]string
	// SymABIs supplies native assembler ABI declarations, when present.
	SymABIs    []byte
	PGOProfile *ProfileInput
	Embed      EmbedConfig
}
type Result struct {
	Archive         []byte
	AssemblyHeader  []byte
	AssemblyListing []byte
	DebugOutput     []byte
	PackageName     string
	ImportPath      string
	Target          Target
	Diagnostics     []diagnostic.Diagnostic
}
type Error struct {
	Diagnostics []diagnostic.Diagnostic
	// Report preserves native formatting, ordering, and diagnostic limits.
	Report []byte
}

func (e *Error) Error() string {
	if len(e.Report) != 0 {
		return strings.TrimSuffix(string(e.Report), "\n")
	}
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
func Compile(ctx context.Context, req Request) (*Result, error) { return compile(ctx, req, nil) }

func compile(ctx context.Context, req Request, built **Package) (result *Result, err error) {
	if err = validateStage(req, built != nil); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, fmt.Errorf("compiler requires a context")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var armFeatures buildcfg.Goarm64Features
	if req.Target, armFeatures, err = normalizeTarget(req.Target); err != nil {
		return nil, err
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
	if e = validateArtifactInputs(req, language, exp.String()); e != nil {
		return nil, e
	}
	architectures := sys.NewContext()
	arch := architectures.ArchAMD64
	if req.Target.GOARCH == "arm64" {
		arch = architectures.ArchARM64
	}
	link := &obj.Link{Arch: &obj.LinkArch{Arch: arch}, Pkgpath: req.ImportPath, Pathname: workingDirectory, UseBASEntries: true, Flag_locationlists: !req.Options.DisableDwarf && built == nil}
	var assemblyListing bytes.Buffer
	link.Bso = bufio.NewWriter(&assemblyListing)
	link.Debugasm = req.Options.AssemblyListing
	link.TargetOS = req.Target.GOOS
	link.Flag_shared = req.Options.Shared
	if e = link.Headtype.Set(req.Target.GOOS); e != nil {
		return nil, e
	}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.BuiltinPkg.Prefix = "go:builtin"
	u.UnsafePkg.Name = "unsafe"
	u.Experiment = *exp
	graph := ir.NewContext(u, link)
	graph.Debug = base.DefaultDebugFlags()
	phaseOptions, e := parseDebugOptions(&graph.Debug, req.Debug)
	if e != nil {
		return nil, e
	}
	var debugOutput debugBuffer
	var reportFiles debugFiles
	if req.Debug.OpenFile != nil {
		req.Debug.OpenFile = reportFiles.wrap(req.Debug.OpenFile)
	}
	graph.TraceWriter = &debugOutput
	link.TraceWriter = &debugOutput
	graph.OpenAST = req.Debug.OpenFile
	graph.ASTWorkingDirectory = func() (string, error) { return workingDirectory, nil }
	diagnostics := func() []diagnostic.Diagnostic {
		return append(graph.Diagnostics.Diagnostics(), u.Diagnostics.Diagnostics()...)
	}
	errors := func() bool { return graph.Diagnostics.HasErrors() || u.Diagnostics.HasErrors() || link.Errors != 0 }
	link.DiagFunc = func(f string, a ...any) {
		graph.Diagnostics.Add(diagnostic.Error, "", src.Pos{}, fmt.Sprintf(f, a...))
		graph.Base.ReportError(src.XPos{}, 0, f, a...)
	}
	defer func() {
		failure := recover()
		graph.CloseHTMLWriters()
		closeErr := reportFiles.close()
		if closeErr != nil {
			result = nil
			err = stderrors.Join(err, closeErr)
			if built != nil {
				*built = nil
			}
		}
		if failure != nil {
			if _, sourceExit := failure.(base.SourceErrorExit); !sourceExit {
				panic(failure)
			}
		}
		if errors() {
			graph.Base.FlushReports()
			result = nil
			err = &Error{Diagnostics: diagnostics(), Report: debugOutput.snapshot()}
		}
	}()
	tc := typecheck.NewContext(graph)
	lowering := walk.NewPackageState(tc)
	u.LocalPkg = u.NewPkg(req.ImportPath, "")
	tc.InitUniverse()
	tc.InitRuntime()
	flags := &graph.Base.Flag
	if req.Options.UnlimitedErrors {
		flags.LowerE = 1
	}
	if req.Options.OmitErrorColumns {
		flags.C = 1
	}
	if req.Options.ActualErrorFiles {
		flags.L = 1
	}
	flags.S = base.CountFlag(req.Options.AssemblyListing)
	flags.LowerM = base.CountFlag(req.Options.OptimizationReport)
	flags.Spectre = req.Options.Spectre
	flags.ClobberDead = req.Options.ClobberDead
	flags.ClobberDeadReg = req.Options.ClobberDeadReg
	flags.Lang = language
	flags.BuildID = req.Options.BuildID
	flags.Race = req.Options.Race
	flags.Shared = &link.Flag_shared
	flags.InstallSuffix = req.Options.InstallSuffix
	graph.Base.Experiment = *exp
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
	flags.Dwarf = !req.Options.DisableDwarf && built == nil
	flags.LowerC = req.Options.Workers
	if flags.LowerC == 0 || built != nil {
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
	if e = graph.Base.PrepareNativeFlags(&graph.Debug); e != nil {
		return nil, e
	}
	flags.Cfg.ImportMap = maps.Clone(req.ImportMap)
	flags.Cfg.PackageFile = make(map[string]string, len(req.Imports))
	archives := make(map[string][]byte, len(req.Imports))
	for name, data := range req.Imports {
		flags.Cfg.PackageFile[name] = name
		archives[name] = bytes.Clone(data)
	}
	artifacts := maps.Clone(req.Artifacts)
	for name := range artifacts {
		flags.Cfg.PackageFile[name] = name
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
	nd.OpenSyntaxHTML = req.Debug.OpenFile
	nd.SyntaxWorkingDirectory = func() (string, error) { return workingDirectory, nil }
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
	lowering.BindFrontend(nd, escape.NewContext(tc, nd.Inline.Log))
	finalizer := gc.NewContext(writer)
	finalizer.BindPackage(pkginit.NewContext(nd))
	finalizer.PrepareInstrumentation()
	backend := ssagen.NewContext(tc)
	backend.Inline = nd.Inline
	backend.SSA = ssa.NewContext(abi.NewContext(graph), nd.ReflectData, nd.Inline.Log)
	backend.SSA.GOAMD64 = req.Target.GOAMD64
	backend.GOARM64 = armFeatures
	backend.SSA.FramePointerEnabled = true
	backend.SSA.Now = func() time.Time { return time.Time{} }
	if req.Debug.Now != nil {
		backend.SSA.Now = req.Debug.Now
	}
	backend.SSA.ReadMemStats = req.Debug.ReadMemStats
	backend.SSA.TraceWriter = &debugOutput
	backend.SSA.DumpDirectory = req.Debug.Directory
	backend.SSA.OpenHTML = req.Debug.OpenFile
	backend.SSA.WorkingDirectory = func() (string, error) { return workingDirectory, nil }
	backend.SSA.LookPath = req.Debug.LookPath
	backend.SSA.Command = req.Debug.Command
	if req.Debug.OpenFile != nil {
		backend.SSA.CreateDump = func(name string) (io.WriteCloser, error) {
			return req.Debug.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		}
	}
	backend.DumpFunction, backend.DumpDirectory = req.Debug.Function, req.Debug.Directory
	backend.MakeDumpDirectory = req.Debug.MakeDirectory
	backend.OpenSource = req.Debug.OpenSource
	backend.ExpandSourcePath = req.Debug.ExpandSourcePath
	if backend.ExpandSourcePath == nil {
		backend.ExpandSourcePath = func(path string) string { return path }
	}
	for _, option := range phaseOptions {
		if message := backend.SSA.PhaseOption(option.phase, option.flag, option.value, option.text); message != "" {
			return nil, fmt.Errorf("%s", message)
		}
	}
	backend.Liveness = liveness.NewContext(backend.SSA)
	backend.Liveness.ClobberDeadHash = req.Debug.ClobberDeadHash
	if req.Target.GOARCH == "amd64" {
		amd64.NewContext(backend)
	} else {
		arm64.NewContext(backend)
	}
	dwarf := dwarfgen.NewContext(backend, nd.ReflectData)
	producer := flag.NewFlagSet("compile", flag.ContinueOnError)
	producer.SetOutput(io.Discard)
	producer.Var(new(countFlag), "N", "")
	producer.Var(new(countFlag), "l", "")
	producer.Bool("dwarf", true, "")
	producer.Bool("race", false, "")
	producer.Bool("shared", false, "")
	producer.String("spectre", "", "")
	if req.Options.Spectre != "" {
		_ = producer.Set("spectre", req.Options.Spectre)
	}
	if req.Options.Shared {
		_ = producer.Set("shared", "true")
	}
	if req.Options.Race {
		_ = producer.Set("race", "true")
	}
	if flags.N != 0 {
		_ = producer.Set("N", "1")
	}
	if req.Options.DisableInlining {
		_ = producer.Set("l", "1")
	}
	if req.Options.DisableDwarf || built != nil {
		_ = producer.Set("dwarf", "false")
		flags.GenDwarfInl = 0
		link.DwFixups = nil
		link.DebugInfo = nil
		link.GenAbstractFunc = nil
	}
	dwarf.Flags = producer
	dwarf.RecordFlags("N", "l", "race", "shared", "dwarf", "spectre")
	backend.InitEnv()
	backend.InitTables()
	config := gc.ObjectConfig{GOOS: req.Target.GOOS, GOARCH: req.Target.GOARCH, GOAMD64: req.Target.GOAMD64, GOARM64: armFeatures, Version: Version}
	nd.BindImports(config, hostOS, func(name string) (noder.DependencyFile, error) {
		if a := artifacts[name]; a != nil {
			return noder.NewUnifiedDependency(name, a.payload), nil
		}
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
		parsed[i], e = nd.ParseSourceFile(file.Name, strings.NewReader(string(file.Data)))
		if e != nil {
			var parsing noder.ParseErrors
			if !stderrors.As(e, &parsing) {
				return nil, e
			}
			nd.ReportParseErrors(parsing)
			if parsed[i] == nil {
				return nil, &Error{Diagnostics: diagnostics()}
			}
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
	var profile *pgoir.Profile
	if req.PGOProfile != nil {
		name := req.PGOProfile.Name
		if name == "" {
			name = "input.pprof"
		}
		data := bytes.Clone(req.PGOProfile.Data)
		flags.PgoProfile = name
		nd.Inline.PGO.OpenProfile = func(string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
		graph.Base.Timer.Start("fe", "pgo-load-profile")
		profile, e = nd.Inline.PGO.New(name)
		if e != nil {
			return nil, fmt.Errorf("%s: PGO error: %w", name, e)
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
	}
	finalizer.BindBackend(backend, lowering)
	finalizer.PrepareFrontend(syms, profile)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if built != nil {
		buildDeclarations := finalizer.BuildDeclarations
		if req.Options.Stage == EarlyOptimized {
			buildDeclarations = finalizer.BuildEarlyDeclarations
		}
		held, e := buildDeclarations(syms, profile, ctx.Err)
		if e != nil {
			return nil, e
		}
		if errors() {
			return nil, &Error{Diagnostics: diagnostics()}
		}
		if graph.Debug.WrapGlobalMapCtl != 1 {
			nd.StaticInit.AddKeepRelocations()
		}
		finalizer.DumpData()
		if errors() {
			return nil, &Error{Diagnostics: diagnostics()}
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = ownEmbeddedData(ctx, req, link.Data); err != nil {
			return nil, err
		}
		var unified bytes.Buffer
		if err = nd.ExportIndependentUnified(&unified); err != nil {
			return nil, err
		}
		pkg := builtPackage(req, language, finalizer, held, diagnostics())
		if pkg.Artifact, err = packageArtifact(req, pkg, unified.String()); err != nil {
			return nil, err
		}
		if errors() {
			return nil, &Error{Diagnostics: diagnostics()}
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		for _, h := range held {
			if f := h.Func(); f.HTMLWriter != nil {
				f.HTMLWriter.Close()
				f.HTMLWriter = nil
			}
		}
		graph.Base.FlushReports()
		pkg.DebugOutput = debugOutput.snapshot()
		*built = pkg
		return nil, nil
	}
	finalizer.CompileDeclarations(syms, profile)
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
	graph.Base.FlushReports()
	if e = link.Bso.Flush(); e != nil {
		return nil, e
	}
	return &Result{Archive: output.data, AssemblyHeader: assembly.data, AssemblyListing: assemblyListing.Bytes(), DebugOutput: debugOutput.snapshot(), PackageName: u.LocalPkg.Name, ImportPath: req.ImportPath, Target: req.Target, Diagnostics: diagnostics()}, nil
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
