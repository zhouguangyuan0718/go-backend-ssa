package amd64

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"io/fs"
	"strings"
	"testing"
)

func TestNativePackageFinish(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, s := loadQueueCompilerWithConfig(t, 4, fmt.Sprintf("package p;var M=map[int]int{1:%d};func F()int{return M[1]}", i+1), true, func(c *gc.Context) { c.Package.Noder.Debug.WrapGlobalMapCtl = 2 })
			c.PrepareFrontend(s, nil)
			c.CompileDeclarations(s, nil)
			mapSym := c.Writer.Link.Lookup("example/p.M")
			for _, r := range mapSym.R {
				if r.Type == objabi.R_KEEP {
					t.Fatal("keep relocation emitted before finish")
				}
			}
			if len(c.Package.Noder.StaticInit.MapInitToVar) == 0 {
				t.Fatal("map initializer was not outlined")
			}
			c.FinishPackage()
			keep := 0
			for _, r := range mapSym.R {
				if r.Type == objabi.R_KEEP {
					keep++
					if r.Sym == nil || len(r.Sym.P) == 0 {
						t.Fatal("keep target lacks compiled code")
					}
				}
			}
			if keep != 1 || !c.Writer.Link.ObjectNumbered() {
				t.Fatal("native finish", keep, c.Writer.Link.ObjectNumbered())
			}
			c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
			var out objectOutput
			if e := c.WriteArchive(&out, gc.CombinedObject); e != nil {
				t.Fatal(e)
			}
			saved := bytes.Clone(out.data)
			other, syms := loadQueueCompiler(t, 2, "package p;func Other()int{return 7}", true)
			other.CompilePackage(syms, nil)
			if !bytes.Equal(out.data, saved) {
				t.Fatal("later package altered retained archive")
			}
			mustDataPanic(t, func() { c.FinishPackage() })
			mustDataPanic(t, func() { c.DumpData() })
		})
	}
}

func TestPackageFinishPreflight(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;func F()int{return 1}", true)
	mustDataPanic(t, func() { c.FinishPackage() })
	c.PrepareFrontend(s, nil)
	mustDataPanic(t, func() { c.FinishPackage() })
	if c.Writer.Link.ObjectNumbered() {
		t.Fatal("preflight numbered incomplete graph")
	}
	c.CompileDeclarations(s, nil)
	c.FinishPackage()
	mustDataPanic(t, func() { c.CompilePackage(s, nil) })
}

func TestPackageFinishRuntimeDiagnostic(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;var G *int;func F(p *int){G=p}", true)
	c.Package.Noder.Flag.CompilingRuntime = true
	c.Package.Noder.Typecheck.Target.Funcs[0].Pragma |= ir.Nowritebarrierrec
	c.PrepareFrontend(s, nil)
	c.CompileDeclarations(s, nil)
	if c.Package.Noder.Diagnostics.HasErrors() {
		t.Fatal("write barrier diagnosed before finish")
	}
	c.FinishPackage()
	found := false
	for _, d := range c.Package.Noder.Diagnostics.Diagnostics() {
		found = found || strings.Contains(d.Message, "write barrier prohibited")
	}
	if !found {
		t.Fatal("finish omitted runtime write barrier check")
	}
	mustDataPanic(t, func() { c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", Version: "go1.27.1"}) })
	mustDataPanic(t, func() { c.FinishPackage() })
}

func TestPackageFinishDataFailure(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;var Embedded string;func F()int{return 1}", true)
	n := c.Package.Noder
	for _, v := range n.Typecheck.Target.Externs {
		if v.Sym().Name == "Embedded" {
			v.Embed = &[]ir.Embed{{Patterns: []string{"input"}}}
			n.Typecheck.Target.Embeds = append(n.Typecheck.Target.Embeds, v)
		}
	}
	n.Flag.Cfg.Embed.Patterns = map[string][]string{"input": {"input"}}
	n.Flag.Cfg.Embed.Files = map[string]string{"input": "input"}
	marker := new(int)
	n.ReflectData.StaticData.OpenFile = func(string) (fs.File, error) {
		mustDataPanic(t, func() { c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", Version: "go1.27.1"}) })
		panic(marker)
	}
	c.PrepareFrontend(s, nil)
	c.CompileDeclarations(s, nil)
	func() {
		defer func() {
			if p := recover(); p != marker {
				t.Errorf("lost provider panic: %v", p)
			}
		}()
		c.FinishPackage()
		t.Error("failed data accepted")
	}()
	if c.Writer.Link.ObjectNumbered() {
		t.Fatal("numbered failed data")
	}
	for _, op := range []func(){func() { c.FinishPackage() }, func() { c.DumpData() }, func() { c.AddGCLocals() }, func() { c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", Version: "go1.27.1"}) }} {
		mustDataPanic(t, op)
	}
}

func TestPackageFinishNumberingFailure(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;func F()int{return 1}", true)
	c.PrepareFrontend(s, nil)
	c.CompileDeclarations(s, nil)
	c.Writer.Link.Pkgpath = ""
	mustDataPanic(t, func() { c.FinishPackage() })
	if c.Writer.Link.ObjectNumbered() {
		t.Fatal("failed numbering marked successful")
	}
	c.Writer.Link.Pkgpath = "example/p"
	mustDataPanic(t, func() { c.FinishPackage() })
	mustDataPanic(t, func() { c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", Version: "go1.27.1"}) })
}

func TestPackageFinishMapOutliningDisabled(t *testing.T) {
	c, s := loadQueueCompilerWithConfig(t, 1, "package p;var M=map[int]int{1:2}", true, func(c *gc.Context) { c.Package.Noder.Debug.WrapGlobalMapCtl = 1 })
	c.CompilePackage(s, nil)
	for _, r := range c.Writer.Link.Lookup("example/p.M").R {
		if r.Type == objabi.R_KEEP {
			t.Fatal("disabled map outlining emitted keep relocation")
		}
	}
	if !c.Writer.Link.ObjectNumbered() {
		t.Fatal("numbering missing")
	}
}
