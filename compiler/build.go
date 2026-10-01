package compiler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/internal/cmd/objabi"
	internalgc "go-backend-ssa/internal/gc"
	"go-backend-ssa/ir"
	"go-backend-ssa/position"
	"go-backend-ssa/ssa"
	"go-backend-ssa/ssagen"
	"go-backend-ssa/types"
	"io"
	"slices"
)

type Stage string

const (
	Built          Stage = "built"
	EarlyOptimized Stage = "early-optimized"
)

type Metadata struct {
	Race                                      bool `json:",omitempty"`
	Shared                                    bool `json:",omitempty"`
	UpstreamVersion, UpstreamCommit, Language string
	Target                                    Target
	Experiments                               string
	Stage                                     Stage
}

type RuntimeType struct {
	Type   *types.Type
	Symbol *gcbridge.LinkSymbol
}
type Import struct {
	Path, Name string
	// ArchiveSHA256 identifies the explicit native archive adapter input.
	ArchiveSHA256  [sha256.Size]byte
	ArtifactSHA256 [sha256.Size]byte
	Fingerprint    gcbridge.Fingerprint
}
type InitDependency struct {
	ImportPath string
	Task       *gcbridge.LinkSymbol
}
type MapInitializer struct {
	Function *ir.Func
	Variable *ir.Name
}
type Initialization struct {
	// Task is the native initializer record. Its pointer relocations retain the
	// actual function order, and Dependencies follow native R_INITORDER entries.
	Task            *gcbridge.LinkSymbol
	Functions       []*ir.Func
	Dependencies    []InitDependency
	MapInitializers []MapInitializer
}

// Package owns a complete source compilation at the selected SSA boundary. Graphs,
// types, symbols and positions retain their shared identities; later builds do
// not reset their storage. Operations on the same package require serialization.
// Machine code, frame offsets, liveness and runtime stack metadata are backend
// work. This result is not an archive and does not provide a native finish call.
type Package struct {
	Name, ImportPath string
	Artifact         *Artifact
	Metadata         Metadata
	Functions        []*ssa.Func
	Declarations     *ir.Package
	Types            *types.Universe
	Symbols, Globals []*gcbridge.LinkSymbol
	RuntimeTypes     []RuntimeType
	Init             Initialization
	Imports          []Import
	Positions        *position.Table
	Inlining         *gcbridge.InlTree
	OpenDefers       map[*ssa.Func]ssagen.OpenDeferLayout
	Diagnostics      []diagnostic.Diagnostic
}

// Build runs the source frontend and native declaration fixed point, stopping
// every function after Phi insertion (Built) or early fuse (EarlyOptimized). Scheduling is
// serial regardless of Options.Workers. Imports use Request's explicit native
// archive adapter or immutable versioned artifacts. The result includes its own
// import artifact with complete native type and function-body export data.
func Build(ctx context.Context, req Request) (*Package, error) {
	var result *Package
	_, err := compile(ctx, req, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func builtPackage(req Request, language string, c *internalgc.Context, held []*ssagen.Construction, diagnostics []diagnostic.Diagnostic) *Package {
	n := c.Package.Noder
	u, link := c.Writer.Types, c.Writer.Link
	p := &Package{Name: u.LocalPkg.Name, ImportPath: req.ImportPath, Metadata: Metadata{Race: req.Options.Race, Shared: req.Options.Shared, UpstreamVersion: Version, UpstreamCommit: UpstreamCommit, Language: language, Target: req.Target, Experiments: u.Experiment.String(), Stage: selectedStage(req)}, Declarations: n.Typecheck.Target, Types: u, Positions: &link.PosTable, Inlining: &link.InlTree, Diagnostics: diagnostics, OpenDefers: make(map[*ssa.Func]ssagen.OpenDeferLayout)}
	for _, h := range held {
		f := h.Func()
		if f.RegAlloc != nil || h.NativeCompleted() {
			panic("package build crossed native SSA boundary")
		}
		p.Functions = append(p.Functions, f)
		if d := h.OpenDefers(); d.Bits != nil {
			p.OpenDefers[f] = d
		}
	}
	// Native scheduling is an implementation detail, not package result order.
	slices.SortFunc(p.Functions, func(a, b *ssa.Func) int { return compareNameABI(a, b) })
	for _, t := range n.ReflectData.RuntimeTypes() {
		p.RuntimeTypes = append(p.RuntimeTypes, RuntimeType{Type: t, Symbol: n.ReflectData.TypeLinksym(t)})
	}
	p.Symbols = link.Symbols()
	p.Globals = append([]*gcbridge.LinkSymbol(nil), link.Data...)
	if sym := u.LocalPkg.Syms[".inittask"]; sym != nil {
		if v, ok := sym.Def.(*ir.Name); ok {
			p.Init.Task = v.Linksym()
		}
	}
	tasks := map[*gcbridge.LinkSymbol]string{}
	for _, pkg := range n.Typecheck.Target.Imports {
		in := Import{Path: pkg.Path, Name: pkg.Name}
		if a := req.Artifacts[pkg.Path]; a != nil {
			in.ArtifactSHA256 = a.SHA256()
		}
		if data, ok := req.Imports[pkg.Path]; ok {
			in.ArchiveSHA256 = sha256.Sum256(data)
		}
		for _, imported := range link.Imports {
			if imported.Pkg == pkg.Path {
				in.Fingerprint = imported.Fingerprint
				break
			}
		}
		p.Imports = append(p.Imports, in)
		if sym := pkg.Syms[".inittask"]; sym != nil {
			if v, ok := sym.Def.(*ir.Name); ok {
				tasks[v.Linksym()] = pkg.Path
			}
		}
	}
	functionsBySymbol := map[*gcbridge.LinkSymbol]*ir.Func{}
	for _, f := range p.Functions {
		fn := f.Frontend().Func()
		functionsBySymbol[fn.LSym] = fn
	}
	if p.Init.Task != nil {
		for _, r := range p.Init.Task.R {
			if r.Type == objabi.R_INITORDER {
				p.Init.Dependencies = append(p.Init.Dependencies, InitDependency{ImportPath: tasks[r.Sym], Task: r.Sym})
			} else if r.Type == objabi.R_ADDR {
				fn := functionsBySymbol[r.Sym]
				if fn == nil {
					panic("initializer record references an unbuilt function")
				}
				p.Init.Functions = append(p.Init.Functions, fn)
			}
		}
	}
	for fn, v := range n.StaticInit.MapInitToVar {
		p.Init.MapInitializers = append(p.Init.MapInitializers, MapInitializer{Function: fn, Variable: v})
	}
	slices.SortFunc(p.Init.MapInitializers, func(a, b MapInitializer) int { return compareString(a.Function.LSym.Name, b.Function.LSym.Name) })
	return p
}
func compareString(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
func compareNameABI(a, b *ssa.Func) int { return compareString(a.NameABI(), b.NameABI()) }

func selectedStage(req Request) Stage {
	if req.Options.Stage == "" {
		return Built
	}
	return req.Options.Stage
}

func validateStage(req Request, building bool) error {
	if req.Options.AssemblyListing < 0 {
		return fmt.Errorf("assembly listing requires a nonnegative detail level")
	}
	if building {
		if req.Options.AssemblyListing != 0 {
			return fmt.Errorf("assembly listings require native Compile")
		}
		if req.Options.Stage != "" && req.Options.Stage != Built && req.Options.Stage != EarlyOptimized {
			return fmt.Errorf("unsupported SSA build stage %q", req.Options.Stage)
		}
		if req.Options.AssemblyHeader {
			return fmt.Errorf("assembly headers require native Compile")
		}
	} else if req.Options.Stage != "" {
		return fmt.Errorf("SSA stage requires Build, not native Compile")
	}
	return nil
}

// Native archives defer large embedded-file reads until serialization. A Built
// result must instead own those bytes before returning to its caller.
func ownEmbeddedData(ctx context.Context, req Request, symbols []*gcbridge.LinkSymbol) error {
	for _, sym := range symbols {
		file := sym.File()
		if file == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		input, err := req.Embed.Open(file.Name)
		if err != nil {
			return fmt.Errorf("embedded input %q: %w", file.Name, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(input, file.Size+1))
		closeErr := input.Close()
		if readErr != nil {
			return fmt.Errorf("embedded input %q: %w", file.Name, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("embedded input %q: %w", file.Name, closeErr)
		}
		if int64(len(data)) != file.Size {
			return fmt.Errorf("embedded input %q changed size", file.Name)
		}
		sym.P, sym.Extra = data, nil
	}
	return ctx.Err()
}
