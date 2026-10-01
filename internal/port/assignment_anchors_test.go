package port

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestAssignmentRegionAnchors(t *testing.T) {
	for _, test := range []struct {
		name, body string
		anchor     SourceCallAnchor
		matches    bool
		ambiguous  bool
	}{
		{"local-literal", "if true { State.Mode = 1 }", SourceCallAnchor{Assignment: "State.Mode", AssignmentLiteral: "1", Enclosing: "if"}, true, false},
		{"wrong-literal", "if true { State.Mode = 0 }", SourceCallAnchor{Assignment: "State.Mode", AssignmentLiteral: "1", Enclosing: "if"}, false, false},
		{"expression-not-literal", "if true { State.Mode = 1+0 }", SourceCallAnchor{Assignment: "State.Mode", AssignmentLiteral: "1", Enclosing: "if"}, false, false},
		{"import-alias", "if true { b.Flag.Mode = 1 }", SourceCallAnchor{Package: "example/base", Assignment: "Flag.Mode", Enclosing: "if"}, true, false},
		{"wrong-package", "if true { b.Flag.Mode = 1 }", SourceCallAnchor{Package: "example/other", Assignment: "Flag.Mode", Enclosing: "if"}, false, false},
		{"import-is-not-local", "if true { b.Flag.Mode = 1 }", SourceCallAnchor{Assignment: "b.Flag.Mode", Enclosing: "if"}, false, false},
		{"shadowed-import", "if b := State; true { b.Mode = 1 }", SourceCallAnchor{Package: "example/base", Assignment: "Mode", Enclosing: "if"}, false, false},
		{"shadowed-local", "if State := State; true { State.Mode = 1 }", SourceCallAnchor{Assignment: "State.Mode", Enclosing: "if"}, false, false},
		{"literal-disambiguates", "if true { State.Mode = 0; State.Mode = 1 }", SourceCallAnchor{Assignment: "State.Mode", AssignmentLiteral: "1", Enclosing: "if"}, true, false},
		{"duplicate", "if true { State.Mode = 1; State.Mode = 1 }", SourceCallAnchor{Assignment: "State.Mode", AssignmentLiteral: "1", Enclosing: "if"}, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, err := parser.ParseFile(token.NewFileSet(), "source.go", `package p; import b "example/base"; var State struct{Mode int}; func Run(){`+test.body+`}`, 0)
			if err != nil {
				t.Fatal(err)
			}
			fn := f.Decls[len(f.Decls)-1].(*ast.FuncDecl)
			got, err := regionAnchor(f, fn.Body.List[0], test.anchor)
			if got != test.matches || (err != nil) != test.ambiguous {
				t.Fatalf("match=%v, err=%v", got, err)
			}
		})
	}
}
