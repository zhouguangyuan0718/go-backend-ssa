package amd64

import (
	"bytes"
	"errors"
	"fmt"
	"go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"strings"
	"testing"
)

func archiveCompiler(t *testing.T, answer int) *gc.Context {
	t.Helper()
	backend := testBackend(true)
	dwarfgen.NewContext(backend, backend.SSA.ReflectData)
	c := globalFinalizer(t, backend)
	n := c.Package.Noder
	n.Debug.SyncFrames = -1
	n.Flag.Lang = "go1.27"
	n.Flag.Complete = true
	file, err := syntax.Parse(syntax.NewFileBase("archive.go"), strings.NewReader(fmt.Sprintf("package p;const Answer int=%d", answer)), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, Instances: map[*syntax.Name]types2.Instance{}, FileVersions: map[*syntax.PosBase]string{}}
	config := types2.Config{Context: n.Checker, GoVersion: "go1.27", Sizes: n.Checker.SizesFor("gc", "amd64")}
	pkg, err := config.Check("example/p", []*syntax.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	closures, err := n.RewriteRangeFuncs(pkg, info, []*syntax.File{file})
	if err != nil {
		t.Fatal(err)
	}
	data, err := n.WritePackage(pkg, info, []*syntax.File{file}, closures)
	if err != nil {
		t.Fatal(err)
	}
	n.ReadLocalPackage(data)
	c.Package.MakeTask()
	for _, decl := range n.Typecheck.Target.Externs {
		switch decl.Op() {
		case ir.ONAME:
			c.DumpGlobal(decl)
		case ir.OLITERAL:
			c.DumpGlobalConst(decl)
		}
	}
	n.ReflectData.WriteGCSymbols()
	n.ReflectData.StaticData.WriteFuncSyms()
	c.Writer.Link.DwFixups.Finalize("example/p", false)
	c.AddGCLocals()
	c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
	return c
}

type archiveTrackedOutput struct {
	objectOutput
	closed bool
}

func (s *archiveTrackedOutput) Close() error { s.closed = true; return nil }
func TestNativeArchiveContexts(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := archiveCompiler(t, i+1)
			var combined archiveTrackedOutput
			if err := c.WriteArchive(&combined, gc.CombinedObject); err != nil {
				t.Fatal(err)
			}
			saved := bytes.Clone(combined.data)
			if !bytes.HasPrefix(saved, []byte("!<arch>\n")) || !bytes.Contains(saved, []byte("__.PKGDEF")) || !bytes.Contains(saved, []byte("_go_.o")) || !bytes.Contains(saved, []byte("\n$$B\nu")) || combined.closed {
				t.Fatal("native container/ownership lost")
			}
			other := archiveCompiler(t, i+101)
			var different objectOutput
			if err := other.WriteArchive(&different, gc.CombinedObject); err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(saved, different.data) || !bytes.Equal(saved, combined.data) {
				t.Fatal("independent archive changed prior output")
			}
			split := archiveCompiler(t, i+1)
			var compiler, linker objectOutput
			if err := split.WriteArchive(&compiler, gc.CompilerObject); err != nil {
				t.Fatal(err)
			}
			if err := split.WriteArchive(&linker, gc.LinkerObject); err != nil {
				t.Fatal(err)
			}
			joined := append(bytes.Clone(compiler.data), linker.data[8:]...)
			if !bytes.Equal(saved, joined) {
				t.Fatal("split and combined members differ")
			}
			unused := new(objectOutput)
			if err := c.WriteArchive(unused, gc.CombinedObject); err == nil || len(unused.data) != 0 {
				t.Fatal("repeat archive wrote output", err)
			}
		})
	}
}
func TestArchiveFailureBoundary(t *testing.T) {
	for _, kind := range []string{"write", "seek"} {
		t.Run(kind, func(t *testing.T) {
			c := archiveCompiler(t, 42)
			failure := errors.New("archive " + kind + " failed")
			stream := new(objectOutput)
			if kind == "write" {
				stream.writeError = failure
			} else {
				stream.seekError = failure
			}
			if err := c.WriteArchive(stream, gc.CompilerObject); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			unused := new(objectOutput)
			if err := c.WriteArchive(unused, gc.LinkerObject); err == nil || len(unused.data) != 0 {
				t.Fatal("failed compilation reused", err)
			}
			next := archiveCompiler(t, 43)
			if err := next.WriteArchive(new(objectOutput), gc.CombinedObject); err != nil {
				t.Fatal(err)
			}
		})
	}
	c := archiveCompiler(t, 44)
	out := new(objectOutput)
	if err := c.WriteArchive(out, gc.LinkerObject); err == nil || len(out.data) != 0 {
		t.Fatal("linker member before export accepted")
	}
	if err := c.WriteArchive(out, gc.CombinedObject); err != nil {
		t.Fatal(err)
	}
}
