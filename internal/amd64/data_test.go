package amd64

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func dataCompiler(t *testing.T, label string, fail bool) *gc.Context {
	t.Helper()
	b := testBackend(true)
	dwarfgen.NewContext(b, b.SSA.ReflectData)
	c := globalFinalizer(t, b)
	n := c.Package.Noder
	g := n.Typecheck.IR
	u := g.Types
	input := fstest.MapFS{"small": &fstest.MapFile{Data: []byte(label)}, "large": &fstest.MapFile{Data: []byte(strings.Repeat(label, 2048))}}
	n.ReflectData.StaticData.OpenFile = input.Open
	c.Writer.Link.OpenObjectInput = func(p string) (io.ReadCloser, error) { return input.Open(p) }
	if fail {
		n.ReflectData.StaticData.OpenFile = func(string) (fs.File, error) { return nil, fs.ErrNotExist }
	}
	g.Base.Flag.Cfg.Embed.Patterns = map[string][]string{"small": {"small"}, "large": {"large"}}
	g.Base.Flag.Cfg.Embed.Files = map[string]string{"small": "small", "large": "large"}
	for i, typ := range []*types.Type{u.Types[types.TSTRING], u.NewSlice(u.Types[types.TUINT8]), u.Types[types.TSTRING], u.NewSlice(u.Types[types.TUINT8])} {
		name := g.NewNameAt(src.XPos{}, u.LocalPkg.Lookup(fmt.Sprint("Embedded", i)), typ)
		name.Class = ir.PEXTERN
		pattern := "small"
		if i >= 2 {
			pattern = "large"
		}
		name.Embed = &[]ir.Embed{{Patterns: []string{pattern}}}
		n.Typecheck.Target.Embeds = append(n.Typecheck.Target.Embeds, name)
		n.Typecheck.Target.Externs = append(n.Typecheck.Target.Externs, name)
		c.DumpGlobal(name)
	}
	n.Typecheck.Target.PluginExports = []*ir.Name{n.Typecheck.Target.Embeds[0]}
	n.ReflectData.ZeroSize = 257
	c.Writer.Link.DwFixups.Finalize("example/p", false)
	return c
}
func TestNativePackageData(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			label := fmt.Sprint("input-", i)
			c := dataCompiler(t, label, false)
			c.DumpData()
			link := c.Writer.Link
			zero := link.Lookup("go:map.zero")
			if !zero.OnList() || zero.Size != 257 || !zero.Static() || !zero.DuplicateOK() || zero.Type != objabi.SRODATA {
				t.Fatal("map zero", zero)
			}
			if tab := link.Lookup("go:plugin.tabs"); !tab.OnList() || len(tab.R) != 2 {
				t.Fatal("plugin table", tab)
			}
			if exports := link.Lookup("go:plugin.exports"); !exports.OnList() || len(exports.R) != 1 || exports.R[0].Sym != c.Package.Noder.Typecheck.Target.Embeds[0].Linksym() {
				t.Fatal("plugin exports")
			}
			var output objectOutput
			if err := c.Writer.WriteObject(&output); err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(output.data, []byte(strings.Repeat(label, 2048))) {
				t.Fatal("streamed content missing")
			}
			retained := bytes.Clone(output.data)
			other := dataCompiler(t, "other", false)
			other.DumpData()
			var next objectOutput
			if err := other.Writer.WriteObject(&next); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(retained, output.data) || bytes.Equal(output.data, next.data) {
				t.Fatal("instance data isolation")
			}
			mustDataPanic(t, func() { c.DumpData() })
			mustDataPanic(t, func() { c.AddGCLocals() })
		})
	}
}
func mustDataPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("invalid data state accepted")
		}
	}()
	f()
}
func TestPackageDataFailure(t *testing.T) {
	c := dataCompiler(t, "input", true)
	c.DumpData()
	if !c.Package.Noder.Diagnostics.HasErrors() {
		t.Fatal("missing native embed diagnostic")
	}
	mustDataPanic(t, func() { c.DumpData() })
	mustDataPanic(t, func() { c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", Version: "go1.27.1"}) })
	c = dataCompiler(t, "next", false)
	c.DumpData()
	var out objectOutput
	if err := c.Writer.WriteObject(&out); err != nil {
		t.Fatal(err)
	}
}

func TestPackageDataPreflight(t *testing.T) {
	c, s := queueCompiler(t, 2, "package p;func F(x int)int{return x+1}")
	c.EnqueueFunc(c.Package.Noder.Typecheck.Target.Funcs[0], s)
	count := len(c.Writer.Link.Data)
	mustDataPanic(t, func() { c.DumpData() })
	if len(c.Writer.Link.Data) != count {
		t.Fatal("preflight changed data")
	}
	c.CompileQueued(nil)
	c.DumpData()
	c = dataCompiler(t, "input", false)
	other := dataCompiler(t, "other", false)
	original := c.Package.Noder.ReflectData.StaticData
	c.Package.Noder.ReflectData.StaticData = other.Package.Noder.ReflectData.StaticData
	mustDataPanic(t, func() { c.DumpData() })
	c.Package.Noder.ReflectData.StaticData = original
	c.DumpData()
	marker := new(int)
	c = dataCompiler(t, "input", false)
	c.Package.Noder.ReflectData.StaticData.OpenFile = func(string) (fs.File, error) { panic(marker) }
	func() {
		defer func() {
			if p := recover(); p != marker {
				t.Errorf("provider panic identity: %v", p)
			}
		}()
		c.DumpData()
	}()
	mustDataPanic(t, func() { c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", Version: "go1.27.1"}) })
	mustDataPanic(t, func() { c.DumpData() })
}
