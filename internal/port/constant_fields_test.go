package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestConstantFieldQueries(t *testing.T) {
	source := `package p
 type Kind int32
 type Number int16
 type row struct{name string;code Number;flag bool;bits uint64;regs []int}
 var table=[5]row{0:{"a\x00β",-32768,true,1<<63+3,[]int{7}},2:{name:"中",code:32767,bits:1<<64-1},4:{name:"last",flag:true}}
 func(k Kind)String()string{return table[k].name}
 func Code(i int)Number{return table[i].code}
 func Flag(i int)bool{return table[i].flag}
 func Bits(i uint64)uint64{return table[i].bits}
 func Eval(f func()int)string{return table[Kind(f())].name}
 func Closure(k Kind)func()string{return func()string{return table[k].name}}
 func Nested(i Kind)string{return table[Kind(table[int(i)].code)].name}
 func Mutate(v int){table[0].regs[0]=v}
 func Mutable()int{return table[0].regs[0]}
 func Ref()*[]int{return &table[0].regs}
 func Copy()string{r:=table[0];r.name="copy";return r.name}
 func Shadow()string{table:=[]row{{name:"shadow"}};return table[0].name}
 func Collision()int{constantRead_table_name:=9;return constantRead_table_name}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source, "src/cmd/compile/internal/p/query.go": `package p
 type Query struct{Index Kind}
 func(q Query)Name()string{return table[q.Index].name}
 func CrossFile(f func()Kind) string {return table[f()].name}
 func CrossClosure(k Kind)func()string{return func()string{return table[k].name}}
 `})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{"table": "table"}}, ConstantFields: []ConstantFieldRule{
		{Variable: "table", Field: "name", Functions: []string{"Kind.String", "Eval", "Closure", "Nested", "Query.Name", "CrossFile", "CrossClosure"}},
		{Variable: "table", Field: "code", Functions: []string{"Code", "Nested"}},
		{Variable: "table", Field: "flag", Functions: []string{"Flag"}},
		{Variable: "table", Field: "bits", Functions: []string{"Bits"}},
	}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit, {Source: "src/cmd/compile/internal/p/query.go", WholeFile: true}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p; type contextState struct {table [5]row};type Context struct {*contextState};func newContext()*Context{c:=&Context{contextState:new(contextState)};c.initialize();return c}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	// The same literal is re-created for each instance; only scalar queries use
	// encoded constant data. Native integer receivers keep fmt.Stringer.
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "native/p.go", strings.Replace(source, "package p", "package native", 1))
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
 import("fmt";"sync";"testing";native "go-backend-ssa/native")
 func panicText(f func())(text string){defer func(){text=fmt.Sprint(recover())}();f();return}
 func TestProjection(t *testing.T){
  var _ fmt.Stringer=Kind(0)
  for i:=0;i<5;i++{if Kind(i).String()!=native.Kind(i).String()||Code(i)!=Number(native.Code(i))||Flag(i)!=native.Flag(i)||Bits(uint64(i))!=native.Bits(uint64(i)){t.Fatal("different scalar value",i)}}
  calls:=0;if (Query{Index:2}).Name()!=native.Kind(2).String(){t.Fatal("cross-file receiver")};if CrossFile(func()Kind{calls++;return 4})!="last"||calls!=1||CrossClosure(0)()!=native.Kind(0).String(){t.Fatal("cross-file evaluation")};calls=0;if Eval(func()int{calls++;return 2})!=native.Kind(2).String()||calls!=1{t.Fatal("index evaluation changed")}
  if Nested(1)!=native.Nested(1){t.Fatal("nested projection")};if Closure(4)()!="last"||Shadow()!="shadow"||Collision()!=9{t.Fatal("closure or shadowing changed")}
  for _,i:=range []int{-1,5,1<<30}{if a,b:=panicText(func(){_ = Kind(i).String()}),panicText(func(){_=native.Kind(i).String()});a!=b{t.Fatal("bounds",a,b)}}
  for _,i:=range []uint64{5,1<<63,1<<64-1}{if a,b:=panicText(func(){_ = Bits(i)}),panicText(func(){_=native.Bits(i)});a!=b{t.Fatal("unsigned bounds",a,b)}}
  old:=newContext();old.Mutate(17)
  if old.Copy()!="copy"||Kind(0).String()!=native.Kind(0).String(){t.Fatal("copy alias")}
  var wg sync.WaitGroup
  for range 8{wg.Go(func(){c:=newContext();if c.Mutable()!=7{t.Error("shared table")};c.Mutate(29);(*c.Ref())[0]=31;if c.Mutable()!=31||old.Mutable()!=17||Code(0)!=-32768||!Flag(0){t.Error("instance state or scalar query")}})};wg.Wait()
  if n:=testing.AllocsPerRun(100,func(){_=Kind(2).String();_=Code(1);_=Bits(4);_=Flag(3)});n!=0{t.Fatal("query allocates",n)}
 }
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Run("cross-file column write", func(t *testing.T) {
		name := "src/cmd/compile/internal/p/query.go"
		original, err := s.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		s.cache[name] = append(append([]byte(nil), original...), []byte(`func WriteOther(){table[0].name="changed"}`)...)
		defer func() { s.cache[name] = original }()
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "write or unsupported escape") {
			t.Fatalf("cross-file mutation accepted: %v", err)
		}
	})
	for name, extra := range map[string]string{
		"column write":            `func Bad(){table[0].name="different"}`,
		"column increment":        `func Bad(){table[0].code++}`,
		"column address":          `func Bad()*Number{return &(table[0].code)}`,
		"row replacement":         `func Bad(){table[0]=row{}}`,
		"row address":             `func Bad()*row{return &table[0]}`,
		"table replacement":       `func Bad(){table=[5]row{}}`,
		"table address":           `func Bad()*[5]row{return &table}`,
		"table slice":             `func Bad()[]row{return table[:]}`,
		"table alias":             `var alias=table`,
		"column range assignment": `func Bad(){for table[0].code=range Number(3){}}`,
		"row range assignment":    `func Bad(){for _,table[0]=range table{}}`,
		"implicit row address":    `func(r *row)Change(){r.code=1};func Bad(){table[0].Change()}`,
		"implicit column address": `func(n *Number)Change(){*n=1};func Bad(){table[0].code.Change()}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[unit.Source] = []byte(source + extra)
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "write or unsupported escape") {
				t.Fatalf("unsafe query accepted: %v", err)
			}
		})
	}
	t.Run("nonconstant initializer", func(t *testing.T) {
		s.cache[unit.Source] = []byte(strings.Replace(source, "-32768,true", "variable,true", 1) + "var variable Number=1")
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "nonconstant initializer") {
			t.Fatal(err)
		}
	})
	t.Run("unsafe layout query", func(t *testing.T) {
		s.cache[unit.Source] = []byte(strings.Replace(source, "package p", "package p;import \"unsafe\"", 1) + "func Bad()uintptr{return unsafe.Offsetof(table[0].code)}")
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "write or unsupported escape") {
			t.Fatal(err)
		}
	})
	t.Run("shadowed helper builtin", func(t *testing.T) {
		s.cache[unit.Source] = []byte(source + "type uint64 = uint")
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unshadowed builtin uint64") {
			t.Fatal(err)
		}
	})
	for name, field := range map[string]string{"missing field": "absent", "mutable field": "regs"} {
		t.Run(name, func(t *testing.T) {
			s.cache[unit.Source] = []byte(source)
			bad := unit
			bad.ConstantFields = []ConstantFieldRule{{Variable: "table", Field: field, Functions: []string{"Kind.String"}}}
			if _, err := Render(s, Plan{InspectRoots: plan.InspectRoots, Units: []Unit{bad}}, dir); err == nil {
				t.Fatal("invalid constant field accepted")
			}
		})
	}
	t.Run("promoted state type mismatch", func(t *testing.T) {
		s.cache[unit.Source] = []byte(source)
		writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type other [5]row;type contextState struct{table other};type Context struct{*contextState}`)
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "does not preserve state type") {
			t.Fatal(err)
		}
	})
}
