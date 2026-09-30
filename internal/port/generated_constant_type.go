package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// Bootstrap configuration constants are produced by upstream generators rather
// than tracked as Go declarations. Derive their scalar type from the pinned
// generator's literal declaration format, and retain that source's digest.
// No generator is executed and no host configuration value enters the output.
func generatedConstantType(s *Source, spec *ast.ValueSpec, source string) (ast.Expr, string, error) {
	fail := func() (ast.Expr, string, error) {
		return nil, "", fmt.Errorf("generated constant type is not proven by %s", source)
	}
	if len(spec.Values) != 1 {
		return fail()
	}
	id, ok := spec.Values[0].(*ast.Ident)
	if !ok {
		return fail()
	}
	data, err := s.Read(source)
	if err != nil {
		return nil, "", err
	}
	file, err := parser.ParseFile(token.NewFileSet(), source, data, 0)
	if err != nil {
		return nil, "", err
	}
	found := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Fprintf" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Obj != nil {
			return true
		}
		isFmt := false
		for _, im := range file.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			name := "fmt"
			if im.Name != nil {
				name = im.Name.Name
			}
			isFmt = isFmt || (p == "fmt" && pkg.Name == name)
		}
		if !isFmt {
			return true
		}
		format, ok := call.Args[1].(*ast.BasicLit)
		if !ok || format.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(format.Value)
		if err != nil {
			return true
		}
		if value == "const "+id.Name+" = `%s`\n" || value == "const "+id.Name+" = %q\n" {
			found++
		}
		return true
	})
	if found != 1 {
		return fail()
	}
	// Both accepted upstream declaration templates necessarily produce strings.
	return ast.NewIdent("string"), hash(data), nil
}
