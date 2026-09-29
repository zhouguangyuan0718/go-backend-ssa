package port

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestPassiveABIPointersDoNotAllowUnsafeOperations(t *testing.T) {
	const decl = `package p;import "unsafe";type ABI struct{Equal func(unsafe.Pointer,uintptr)uintptr;Ptr *unsafe.Pointer;Nested struct{P unsafe.Pointer}};`
	cases := []struct {
		extra   string
		allowed bool
	}{
		{"", true},
		{`func Bad(p unsafe.Pointer)uintptr{return uintptr(p)}`, false},
		{`func Bad(p *byte)uintptr{return uintptr(unsafe.Pointer(p))}`, false},
		{`func Bad(a ABI)uintptr{return uintptr(a.Nested.P)}`, false},
		{`type Bad struct{N [unsafe.Sizeof(unsafe.Pointer(nil))]byte}`, false},
		{`func Bad(a ABI)byte{return *(*byte)(a.Nested.P)}`, false},
	}
	for _, test := range cases {
		t.Run(test.extra, func(t *testing.T) {
			fs := token.NewFileSet()
			f, err := parser.ParseFile(fs, "p.go", decl+test.extra, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
			conf := types.Config{Importer: importer.Default()}
			pkg, err := conf.Check("p", fs, []*ast.File{f}, info)
			if err != nil {
				t.Fatal(err)
			}
			err = auditOutput(pkg, []*ast.File{f}, info)
			if test.allowed && err != nil {
				t.Fatal(err)
			}
			if !test.allowed && (err == nil || !strings.Contains(err.Error(), "unclassified environment")) {
				t.Fatalf("unsafe operation accepted: %v", err)
			}
		})
	}
}
