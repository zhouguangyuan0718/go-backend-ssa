package amd64

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"io"
	"io/fs"
	"testing"
)

func TestNativeFrontendPipeline(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, s := loadQueueCompiler(t, 4, fmt.Sprintf(`package p
var Global = new(int)
func init(){*Global=%d}
func small(x int)int{return x+1}
func F(x int)int{return small(x)}
func Captures()[]*int { var out []*int;for i:=range 3{out=append(out,&i)};return out }
func Slice(n int)int{s:=make([]int,n);return len(s)}
type T int
func (x T) M()int{return int(x)}
var Method=(*T).M
func Assembly(p *int) int
func Call(p *int)int{return Assembly(p)}
`, i+1), true)
			c.Package.Noder.Flag.LowerL = 1
			c.PrepareFrontend(s, nil)
			if c.Package.Noder.Typecheck.IR.CurFunc != nil {
				t.Fatal("frontend left current function active")
			}
			inlined := false
			for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
				if fn.Sym().Name == "F" {
					ir.Visit(fn, func(n ir.Node) {
						if n.Op() == ir.OINLMARK {
							inlined = true
						}
					})
				}
			}
			if !inlined {
				t.Fatal("native frontend did not inline small")
			}
			c.CompileDeclarations(s, nil)
			if c.QueuedFunctions() != 0 {
				t.Fatal("queue not drained")
			}
			c.DumpData()
			c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
			var out objectOutput
			if e := c.WriteArchive(&out, gc.CombinedObject); e != nil {
				t.Fatal(e)
			}
			if !bytes.HasPrefix(out.data, []byte("!<arch>\n")) {
				t.Fatal("missing archive")
			}
			mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
		})
	}
}

func TestFrontendPipelinePreflight(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;func F()int{return 1}", true)
	other, foreign := loadQueueCompiler(t, 1, "package p;func Other()int{return 2}", true)
	mustDataPanic(t, func() { c.PrepareFrontend(foreign, nil) })
	target := c.Package.Noder.Typecheck.Target
	original := target.Funcs[0]
	target.Funcs = append(target.Funcs, other.Package.Noder.Typecheck.Target.Funcs[0])
	mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
	target.Funcs = target.Funcs[:1]
	originalImports := target.Imports
	target.Imports = append(target.Imports, other.Writer.Types.NewPkg("foreign/pkg", "pkg"))
	mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
	target.Imports = originalImports
	originalInits := target.Inits
	target.Inits = append(target.Inits, other.Package.Noder.Typecheck.Target.Funcs[0])
	mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
	target.Inits = originalInits
	originalExterns := target.Externs
	target.Externs = append(target.Externs, other.Package.Noder.Typecheck.Target.Funcs[0].Nname)
	mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
	target.Externs = originalExterns
	localImport := c.Writer.Types.NewPkg("local/pkg", "pkg")
	localImport.Lookup(".inittask").Def = other.Package.Noder.Typecheck.Target.Funcs[0].Nname
	target.Imports = []*types.Pkg{localImport}
	mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
	target.Imports = originalImports
	if original.Inl != nil {
		t.Fatal("invalid input modified local function")
	}
	c.PrepareFrontend(s, nil)
	mustDataPanic(t, func() { c.PrepareFrontend(s, nil) })
	c.CompileDeclarations(s, nil)
	c.DumpData()
}

func TestFrontendPipelineFailure(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;func F()int{return 1}", true)
	g := c.Package.Noder.Typecheck.IR
	g.Debug.AstDump = "F"
	marker := new(int)
	g.OpenAST = func(string, int, fs.FileMode) (io.WriteCloser, error) { panic(marker) }
	func() {
		defer func() {
			if p := recover(); p != marker {
				t.Errorf("lost provider panic: %v", p)
			}
		}()
		c.PrepareFrontend(s, nil)
		t.Error("provider failure accepted")
	}()
	for _, op := range []func(){func() { c.PrepareFrontend(s, nil) }, func() { c.CompileDeclarations(s, nil) }, func() { c.DumpData() }, func() { c.AddGCLocals() }, func() { c.CompileQueued(nil) }, func() { c.EnqueueFunc(c.Package.Noder.Typecheck.Target.Funcs[0], s) }} {
		mustDataPanic(t, op)
	}
}

func TestFrontendRuntimeWriteBarrierCollection(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;func F()int{return 1}", true)
	c.Package.Noder.Flag.CompilingRuntime = true
	c.PrepareFrontend(s, nil)
	c.CompileDeclarations(s, nil)
	c.Backend.NoWriteBarrierRecCheck()
	if c.Package.Noder.Diagnostics.HasErrors() {
		t.Fatal(c.Package.Noder.Diagnostics.Diagnostics())
	}
}
