package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

type rangeContinuation struct {
	fn                        *ast.FuncDecl
	guard                     *ast.DeferStmt
	loop                      *ast.RangeStmt
	constants                 map[ast.Stmt]bool
	element, method, argument string
	length                    int64
}

// Resolve all anchors against typed native source before context adaptation.
// The complete loop body and epilogue remain native statements. Reject lexical
// changes that would make moving the guard or constants change their meaning.
func prepareRangeContinuations(files []*ast.File, info *types.Info, units []Unit) ([]rangeContinuation, error) {
	var out []rangeContinuation
	for i, file := range files {
		for _, name := range sortedKeys(units[i].RangeContinuations) {
			r := units[i].RangeContinuations[name]
			fail := func() error { return fmt.Errorf("unsafe or missing range continuation: %s", name) }
			var fn *ast.FuncDecl
			for _, d := range file.Decls {
				if f, ok := d.(*ast.FuncDecl); ok && f.Recv == nil && f.Name.Name == name {
					if fn != nil {
						return nil, fail()
					}
					fn = f
				}
			}
			if fn == nil || fn.Body == nil || fn.Type.TypeParams != nil || fn.Type.Results != nil {
				return nil, fail()
			}
			var arg types.Object
			for _, f := range fn.Type.Params.List {
				for _, id := range f.Names {
					if id.Name == r.Argument {
						arg = info.Defs[id]
					}
				}
			}
			if arg == nil {
				return nil, fail()
			}
			if _, ok := arg.Type().(*types.Pointer); !ok {
				return nil, fail()
			}
			b := rangeContinuation{fn: fn, constants: make(map[ast.Stmt]bool), method: r.Method, argument: r.Argument}
			guardIndex, loopIndex := -1, -1
			for j, stmt := range fn.Body.List {
				if guard, ok := stmt.(*ast.DeferStmt); ok {
					if b.guard != nil {
						return nil, fail()
					}
					lit, ok := guard.Call.Fun.(*ast.FuncLit)
					if !ok || len(guard.Call.Args) != 0 || len(lit.Type.Params.List) != 0 || lit.Type.Results != nil {
						return nil, fail()
					}
					b.guard, guardIndex = guard, j
				}
				if loop, ok := stmt.(*ast.RangeStmt); ok {
					id, ok := loop.X.(*ast.Ident)
					if !ok || id.Name != r.Range {
						continue
					}
					if b.loop != nil {
						return nil, fail()
					}
					obj := info.Uses[id]
					if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
						return nil, fail()
					}
					if _, ok := obj.(*types.Var); !ok {
						return nil, fail()
					}
					arr, ok := obj.Type().(*types.Array)
					if !ok {
						return nil, fail()
					}
					elem, ok := arr.Elem().(*types.Named)
					if !ok || elem.Obj().Pkg() != obj.Pkg() {
						return nil, fail()
					}
					key, kok := loop.Key.(*ast.Ident)
					val, vok := loop.Value.(*ast.Ident)
					if !kok || !vok || key.Name != "_" || val.Name == "_" || loop.Tok != token.DEFINE {
						return nil, fail()
					}
					b.loop, loopIndex, b.element = loop, j, elem.Obj().Name()
					b.length = arr.Len()
				}
			}
			if guardIndex <= 0 || loopIndex <= guardIndex || loopIndex >= len(fn.Body.List)-1 {
				return nil, fail()
			}
			invalid := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncLit:
					return false
				case *ast.ReturnStmt, *ast.LabeledStmt, *ast.GoStmt:
					invalid = true
				case *ast.BranchStmt:
					if n.Tok == token.GOTO {
						invalid = true
					}
				case *ast.DeferStmt:
					if n != b.guard {
						invalid = true
					}
				case *ast.Ident:
					if n.Name == "syncgoStagePasses" || n.Name == "syncgoStageStart" {
						invalid = true
					}
					if n.Name == r.Argument && info.Defs[n] != nil && info.Defs[n] != arg {
						invalid = true
					}
				}
				return true
			})
			if invalid {
				return nil, fail()
			}
			// A const declaration is safe to hoist only if it shadows no earlier
			// reference, including inside the deferred guard's function literal.
			for _, stmt := range fn.Body.List[guardIndex+1 : loopIndex] {
				switch stmt := stmt.(type) {
				case *ast.AssignStmt:
					if stmt.Tok == token.DEFINE {
						return nil, fail()
					}
				case *ast.DeclStmt:
					decl, ok := stmt.Decl.(*ast.GenDecl)
					if !ok || decl.Tok != token.CONST {
						return nil, fail()
					}
					for _, spec := range decl.Specs {
						vs := spec.(*ast.ValueSpec)
						for _, id := range vs.Names {
							obj, ok := info.Defs[id].(*types.Const)
							if !ok {
								return nil, fail()
							}
							ast.Inspect(fn.Body, func(n ast.Node) bool {
								if use, ok := n.(*ast.Ident); ok && use.Pos() < decl.Pos() && use.Name == id.Name && info.Uses[use] != nil && info.Uses[use] != obj {
									invalid = true
								}
								return true
							})
						}
					}
					b.constants[stmt] = true
				}
			}
			if invalid {
				return nil, fail()
			}
			out = append(out, b)
		}
	}
	return out, nil
}

func applyRangeContinuations(bindings []rangeContinuation) error {
	for _, b := range bindings {
		fn := b.fn
		receiver := "syncgoCtx"
		if fn.Recv != nil {
			if len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
				return fmt.Errorf("range continuation requires a context: %s", fn.Name.Name)
			}
			receiver = fn.Recv.List[0].Names[0].Name
		} else {
			found := false
			for _, stmt := range fn.Body.List {
				if a, ok := stmt.(*ast.AssignStmt); ok {
					for _, lhs := range a.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && id.Name == receiver {
							found = true
						}
					}
				}
			}
			if !found {
				return fmt.Errorf("range continuation requires a context: %s", fn.Name.Name)
			}
		}
		gi, li := -1, -1
		for i, stmt := range fn.Body.List {
			if stmt == b.guard {
				gi = i
			}
			if stmt == b.loop {
				li = i
			}
		}
		if gi < 0 || li <= gi {
			return fmt.Errorf("range continuation boundary changed: %s", fn.Name.Name)
		}
		outer := append([]ast.Stmt(nil), fn.Body.List[:gi]...)
		var start []ast.Stmt
		for _, stmt := range fn.Body.List[gi+1 : li] {
			if b.constants[stmt] {
				outer = append(outer, stmt)
			} else {
				start = append(start, stmt)
			}
		}
		pos := b.guard.Pos()
		array := b.loop.X
		b.loop.X = &ast.CallExpr{Fun: ast.NewIdent("syncgoStagePasses"), Args: []ast.Expr{array}}
		run := &ast.FuncLit{Type: &ast.FuncType{Func: pos, Params: &ast.FieldList{Opening: pos, Closing: pos, List: []*ast.Field{
			{Names: []*ast.Ident{ast.NewIdent("syncgoStagePasses")}, Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Len: &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(b.length, 10)}, Elt: ast.NewIdent(b.element)}}}}, Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Elt: ast.NewIdent(b.element)}}}}}},
			{Names: []*ast.Ident{ast.NewIdent("syncgoStageStart")}, Type: ast.NewIdent("bool")},
		}}}, Body: &ast.BlockStmt{Lbrace: pos, Rbrace: fn.Body.Rbrace}}
		run.Body.List = []ast.Stmt{b.guard, &ast.IfStmt{If: pos, Cond: ast.NewIdent("syncgoStageStart"), Body: &ast.BlockStmt{Lbrace: pos, List: start, Rbrace: pos}}, b.loop}
		run.Body.List = append(run.Body.List, fn.Body.List[li+1:]...)
		call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(receiver), Sel: ast.NewIdent(b.method)}, Args: []ast.Expr{ast.NewIdent(b.argument), run}}
		fn.Body.List = append(outer, &ast.ExprStmt{X: call})
	}
	return nil
}
