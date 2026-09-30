package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestStateMovesIntoExistingReceivers(t *testing.T) {
	const scanner = `package p
 type scanner struct{}
 var table [4]int
 func init(){for i:=range table{table[i]=i+1}}
 func hash(i int)int{return i&(len(table)-1)}
 func(s *scanner)init(){}
 func(s *scanner)Read(i int)int{return table[hash(i)]}
 func(s *scanner)Write(i,n int){table[hash(i)]=n}
 func(s *scanner)Callback()func()int{return func()int{return table[1]}}
 func(s *scanner)Shadow()int{table:=[2]int{9};return table[0]}
 `
	const scope = `package p
 type node struct{N int}
 type scope struct{}
 var invalid=new(node)
 func newScope()*scope{return &scope{}}
 func another()*scope{return new(scope)}
 func(s *scope)Invalid()*node{return invalid}
 `
	const printer = `package p
 type printer struct{}
 var space=[]byte(" ")
 func newPrinter()*printer{return &printer{}}
 func(p *printer)Bytes()[]byte{return space}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/scanner.go": scanner, "src/cmd/compile/internal/p/scope.go": scope, "src/cmd/compile/internal/p/printer.go": printer})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/scanner.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "scanner", InitMethod: "init", RunInit: true, Fields: map[string]string{"table": "table"}}},
		{Source: "src/cmd/compile/internal/p/scope.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "scope", Fields: map[string]string{"invalid": "invalid"}}},
		{Source: "src/cmd/compile/internal/p/printer.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "printer", Fields: map[string]string{"space": "space"}}},
	}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestReceivers(t *testing.T){var a,b scanner;a.init();b.init();old:=a.Callback();a.Write(1,9);if old()!=9||b.Read(1)!=2||a.Shadow()!=9{t.Fatal("receiver identity")};a.init();if old()!=2{t.Fatal("reinitialization failed")};x,y:=newScope(),another();x.Invalid().N=1;if y.Invalid().N!=0||x.Invalid()!=x.Invalid(){t.Fatal("sentinel state")};p,q:=newPrinter(),newPrinter();p.Bytes()[0]='!';if q.Bytes()[0]!=' '{t.Fatal("shared printer bytes")};var wg sync.WaitGroup;for i:=range 16{wg.Go(func(){var s scanner;s.init();s.Write(1,i);if s.Callback()()!=i{t.Error("concurrent receiver")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ name, source, original, changed, want string }{
		{"free runtime state", plan.Units[0].Source, scanner, scanner + "\nfunc Global()int{return table[0]}", "outside scanner methods"},
		{"field collision", plan.Units[0].Source, scanner, strings.Replace(scanner, "struct{}", "struct{table [4]int}", 1), "field collision"},
		{"missing initialization", plan.Units[0].Source, scanner, strings.Replace(scanner, "func init()", "func renamed()", 1), "outside scanner methods"},
		{"unkeyed allocation", plan.Units[1].Source, scope, strings.Replace(strings.Replace(scope, "type scope struct{}", "type scope struct{N int}", 1), "&scope{}", "&scope{0}", 1), "unkeyed receiver"},
		{"zero scope", plan.Units[1].Source, scope, scope + "\nfunc zero()*scope{var s scope;return &s}", "zero receiver"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.cache[tc.source] = []byte(tc.changed)
			defer func() { s.cache[tc.source] = []byte(tc.original) }()
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}

func TestZeroReceiverStateAcrossFiles(t *testing.T) {
	const cache = `package p
 import receiverStateImport0 "fmt"
 type Cache struct{N int}
 func New()*Cache { return new(Cache) }
 func Zero()*Cache { var c Cache; return &c }
 func Literal()*Cache{return &Cache{N:4}}
 func(c *Cache)String()string{return receiverStateImport0.Sprint(c.N)}
 `
	const state = `package p
 import original "sync"
 var pools [2]original.Pool
 func(c *Cache)Put(i int,v any){pools[i].Put(v)}
 func(c *Cache)Get(i int)any{return pools[i].Get()}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/cache.go": cache, "src/cmd/compile/internal/p/state.go": state})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/cache.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/p/state.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "Cache", Fields: map[string]string{"pools": "pools"}}},
	}}
	dir := t.TempDir()
	r, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestCache(t *testing.T){a,b:=New(),Zero();a.Put(0,1);if b.Get(0)!=nil||a.String()!="0"||Literal().N!=4{t.Fatal("state shared or constructor changed")};if v:=a.Get(0);v!=nil&&v!=1{t.Fatal(v)};var wg sync.WaitGroup;for i:=0;i<8;i++{wg.Go(func(){c:=New();for j:=0;j<100;j++{c.Put(1,j);if v:=c.Get(1);v!=nil&&v!=j{t.Error("foreign pool value")}}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Run("selected methods", func(t *testing.T) {
		subset := plan
		subset.Units = append([]Unit(nil), plan.Units...)
		subset.Units[1].WholeFile = false
		subset.Units[1].Declarations = []string{"Cache.Get"}
		result, err := Render(source, subset, dir)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(result.Files["internal/p/state.go"]), "func (c *Cache) Put") {
			t.Fatal("unselected receiver method leaked")
		}
	})
	for _, tc := range []struct{ name, src, want string }{
		{"outside receiver", state + "\nfunc bad()any{return pools[0].Get()}", "outside Cache methods"},
		{"initializer", strings.Replace(state, "[2]original.Pool", "[2]original.Pool = [2]original.Pool{}", 1), "cross-file receiver state requires zero initialization"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source.cache[plan.Units[1].Source] = []byte(tc.src)
			defer func() { source.cache[plan.Units[1].Source] = []byte(state) }()
			if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}
