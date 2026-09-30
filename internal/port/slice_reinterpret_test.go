package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeSliceReinterpretation(t *testing.T) {
	const source = `package p
 import alias "unsafe"
 type Header struct{Data alias.Pointer;Len,Cap int}
 type Wide struct{a,b int64}
 type Narrow int32
 type Cache struct{saved []Wide}
 func(c *Cache)allocBase(n int)[]Wide{if cap(c.saved)>=n{s:=c.saved[:n];c.saved=nil;return s};return make([]Wide,n,n+8)}
 func(c *Cache)freeBase(s []Wide){clear(s);c.saved=s}
 func(c *Cache)Allocate(n int)[]Narrow{
 var base Wide
 var derived Narrow
 if alias.Sizeof(base)%alias.Sizeof(derived)!=0{panic("bad")}
 scale:=alias.Sizeof(base)/alias.Sizeof(derived)
 b:=c.allocBase(int((uintptr(n)+scale-1)/scale))
 s:=Header{Data:alias.Pointer(&b[0]),Len:n,Cap:cap(b)*int(scale)}
 return *(*[]Narrow)(alias.Pointer(&s))
 }
 func(c *Cache)Free(s []Narrow){
 var base Wide
 var derived Narrow
 scale:=alias.Sizeof(base)/alias.Sizeof(derived)
 b:=Header{Data:alias.Pointer(&s[0]),Len:int((uintptr(len(s))+scale-1)/scale),Cap:int((uintptr(cap(s))+scale-1)/scale)}
 c.freeBase(*(*[]Wide)(alias.Pointer(&b)))
 }
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	rule := SliceReinterpretationRule{Allocate: "Cache.Allocate", Free: "Cache.Free", BaseAllocate: "Cache.allocBase", BaseFree: "Cache.freeBase"}
	unit := Unit{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, SliceReinterpretations: []SliceReinterpretationRule{rule}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
	dir := t.TempDir()
	unreviewed := plan
	unreviewed.Units = []Unit{unit}
	unreviewed.Units[0].SliceReinterpretations = nil
	if _, err := Render(s, unreviewed, dir); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unclassified unsafe accepted: %v", err)
	}
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "native/p.go", strings.Replace(source, "package p", "package native", 1))
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"runtime";"go-backend-ssa/native")
 func TestSlices(t *testing.T){a:=new(Cache);b:=new(native.Cache);for _,n:=range []int{1,2,3,4,5,31,32,33,257}{x,y:=a.Allocate(n),b.Allocate(n);if len(x)!=len(y)||cap(x)!=cap(y){t.Fatal("layout")};for i:=range x{if x[i]!=Narrow(y[i])||x[i]!=0{t.Fatal("not cleared")};x[i]=Narrow(i+1);y[i]=native.Narrow(i+1)};runtime.GC();for i:=range x{if x[i]!=Narrow(y[i]){t.Fatal("lost backing allocation")}};a.Free(x);b.Free(y)};if len(a.saved)==0{t.Fatal("free did not return the native base view")}}
 `)
	cmd := exec.Command("go", "test", "-race", "-gcflags=all=-d=checkptr=2", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Run("renamed locals", func(t *testing.T) {
		renamed := strings.NewReplacer("var base Wide", "var backing Wide", "Sizeof(base)", "Sizeof(backing)", "var derived Narrow", "var output Narrow", "Sizeof(derived)", "Sizeof(output)").Replace(source)
		s.cache[unit.Source] = []byte(renamed)
		defer func() { s.cache[unit.Source] = []byte(source) }()
		if _, err := Render(s, plan, dir); err != nil {
			t.Fatal(err)
		}
	})
	for _, tc := range []struct{ name, old, new, want string }{
		{"offset backing", "&b[0]", "&b[1]", "pattern changed"},
		{"capacity", "Cap:cap(b)*int(scale)", "Cap:cap(b)*int(scale)+1", "pattern changed"},
		{"length", "Len:n,", "Len:n+1,", "pattern changed"},
		{"round up", "uintptr(n)+scale-1", "uintptr(n)+scale", "pattern changed"},
		{"free length", "uintptr(len(s))+scale-1", "uintptr(cap(s))+scale-1", "pattern changed"},
		{"free capacity", "uintptr(cap(s))+scale-1", "uintptr(len(s))+scale-1", "pattern changed"},
		{"missing divisibility check", "if alias.Sizeof(base)%alias.Sizeof(derived)!=0{panic(\"bad\")}", "", "pattern changed"},
		{"different panic", "panic(\"bad\")", "panic(\"other\")", "pattern changed"},
		{"extra unsafe", "var base Wide", "_ = alias.Pointer(new(int));var base Wide", "pattern changed"},
		{"shadow builtin", "var base Wide", "cap:=func(s []Wide)int{return 100};_ = cap;var base Wide", "pattern changed"},
		{"GC class", "type Narrow int32", "type Narrow *int", "GC layouts differ"},
		{"size ratio", "type Narrow int32", "type Narrow [3]int32", "incompatible scratch element layout"},
		{"zero size", "type Narrow int32", "type Narrow struct{}", "division by zero"},
		{"header extra field", "Len,Cap int}", "Len,Cap int;extra int}", "GC-visible slice header"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.cache[unit.Source] = []byte(strings.Replace(source, tc.old, tc.new, 1))
			defer func() { s.cache[unit.Source] = []byte(source) }()
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
	for _, extra := range []string{`func Bad(p *int)alias.Pointer{return alias.Pointer(p)}`, `func Bad(p *int)uintptr{return alias.Sizeof(p)}`} {
		s.cache[unit.Source] = []byte(source + extra)
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Fatalf("unlisted unsafe accepted: %v", err)
		}
	}
}
