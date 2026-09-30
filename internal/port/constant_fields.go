package port

import (
	"encoding/binary"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"reflect"
	"strconv"
	"strings"
)

// Preserve scalar queries on native integer opcodes without allocating a local
// copy of a large table or introducing mutable package state. All data comes
// from the checked native literal. The full literal still initializes each
// context; this projection is used only by explicitly selected query functions.
func projectConstantFields(fs *token.FileSet, files []*ast.File, info *types.Info, units []Unit) ([][]ast.Decl, error) {
	helpers := make([][]ast.Decl, len(files))
	reserved := map[string]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				reserved[id.Name] = true
			}
			return true
		})
	}
	replacements := map[ast.Node]ast.Expr{}
	for fileIndex, u := range units {
		for _, rule := range u.ConstantFields {
			f := files[fileIndex]
			var obj types.Object
			var literal *ast.CompositeLit
			for _, d := range f.Decls {
				g, ok := d.(*ast.GenDecl)
				if !ok || g.Tok != token.VAR {
					continue
				}
				for _, s := range g.Specs {
					v := s.(*ast.ValueSpec)
					if len(v.Names) == 1 && v.Names[0].Name == rule.Variable && len(v.Values) == 1 {
						obj = info.Defs[v.Names[0]]
						literal, _ = v.Values[0].(*ast.CompositeLit)
					}
				}
			}
			if obj == nil || literal == nil {
				return nil, fmt.Errorf("constant field requires a literal array: %s", rule.Variable)
			}
			array, ok := obj.Type().Underlying().(*types.Array)
			if !ok {
				return nil, fmt.Errorf("constant field requires an array: %s", rule.Variable)
			}
			record, ok := array.Elem().Underlying().(*types.Struct)
			if !ok {
				return nil, fmt.Errorf("constant field requires struct elements: %s", rule.Variable)
			}
			fieldIndex := -1
			for j := 0; j < record.NumFields(); j++ {
				if record.Field(j).Name() == rule.Field && !record.Field(j).Embedded() {
					fieldIndex = j
					break
				}
			}
			if fieldIndex < 0 {
				return nil, fmt.Errorf("constant field not found: %s.%s", rule.Variable, rule.Field)
			}
			field := record.Field(fieldIndex)
			basic, ok := field.Type().Underlying().(*types.Basic)
			if !ok || basic.Info()&(types.IsBoolean|types.IsInteger|types.IsString) == 0 {
				return nil, fmt.Errorf("constant field must be a boolean, integer, or string: %s", rule.Field)
			}
			for _, name := range []string{"int", "uint64"} {
				if obj.Pkg().Scope().Lookup(name) != nil {
					return nil, fmt.Errorf("constant field helper requires unshadowed builtin %s", name)
				}
			}
			if !constantAggregate(literal, info) {
				return nil, fmt.Errorf("constant field table has a nonconstant initializer: %s", rule.Variable)
			}
			values, err := constantColumn(literal, array, record, fieldIndex, basic, info)
			if err != nil {
				return nil, err
			}
			// Audit all selected native consumers, including ones that retain their
			// context. A pointer to another field (e.g. reg) cannot mutate this column.
			for _, file := range files {
				if err := checkConstantFieldUses(file, info, obj, field); err != nil {
					return nil, err
				}
			}
			selected := map[string]bool{}
			for _, name := range rule.Functions {
				if selected[name] {
					return nil, fmt.Errorf("duplicate constant field query: %s", name)
				}
				selected[name] = true
			}
			counts := map[string]int{}
			var indexType types.Type
			helperName := "constantRead_" + rule.Variable + "_" + rule.Field
			for reserved[helperName] {
				helperName += "_"
			}
			reserved[helperName] = true
			for _, queryFile := range files {
				for _, d := range queryFile.Decls {
					fn, ok := d.(*ast.FuncDecl)
					if !ok {
						continue
					}
					name := fn.Name.Name
					if fn.Recv != nil {
						name = receiverType(fn) + "." + name
					}
					if !selected[name] {
						continue
					}
					ast.Inspect(fn.Body, func(n ast.Node) bool {
						sel, ok := n.(*ast.SelectorExpr)
						if !ok || info.Selections[sel] == nil || info.Selections[sel].Obj() != field {
							return true
						}
						index, ok := ast.Unparen(sel.X).(*ast.IndexExpr)
						if !ok {
							return true
						}
						id, ok := ast.Unparen(index.X).(*ast.Ident)
						if !ok || info.Uses[id] != obj {
							return true
						}
						if replacements[sel] != nil {
							err = fmt.Errorf("duplicate constant field replacement: %s", name)
							return false
						}
						typ := types.Default(info.TypeOf(index.Index))
						if indexType == nil {
							indexType = typ
						}
						if !types.Identical(indexType, typ) {
							err = fmt.Errorf("constant field queries require one index type: %s", rule.Field)
							return false
						}
						replacements[sel] = &ast.CallExpr{Fun: &ast.Ident{Name: helperName, NamePos: sel.Pos()}, Args: []ast.Expr{index.Index}, Lparen: sel.Pos(), Rparen: sel.End()}
						counts[name]++
						return true
					})
				}
			}
			if err != nil {
				return nil, err
			}
			for _, name := range rule.Functions {
				if counts[name] == 0 {
					return nil, fmt.Errorf("constant field query has no matching read: %s", name)
				}
			}
			qualifier := func(pkg *types.Package) string {
				if pkg == obj.Pkg() {
					return ""
				}
				for _, im := range f.Imports {
					p, _ := strconv.Unquote(im.Path.Value)
					if p == pkg.Path() {
						if im.Name != nil {
							return im.Name.Name
						}
						return path.Base(p)
					}
				}
				return pkg.Name()
			}
			source := constantLookup(helperName, types.TypeString(indexType, qualifier), types.TypeString(field.Type(), qualifier), basic, values)
			parsed, err := parser.ParseFile(token.NewFileSet(), "constant_field.go", "package "+f.Name.Name+"\n"+source, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			setTokenPositions(reflect.ValueOf(parsed), f.End())
			helpers[fileIndex] = append(helpers[fileIndex], parsed.Decls...)
		}
	}
	// An index may itself contain a projected query. Compose replacements from
	// the inside out, so replacing the outer selector cannot hide the inner one
	// from context dependency analysis. Map iteration order must not matter.
	prepared := map[ast.Node]bool{}
	var prepare func(ast.Node)
	prepare = func(n ast.Node) {
		if prepared[n] {
			return
		}
		ast.Inspect(replacements[n], func(child ast.Node) bool {
			if replacements[child] != nil {
				prepare(child)
				return false
			}
			return true
		})
		rewriteExprs(replacements[n], replacements)
		prepared[n] = true
	}
	for n := range replacements {
		prepare(n)
	}
	for _, f := range files {
		rewriteExprs(f, replacements)
	}
	return helpers, nil
}

func constantAggregate(expr ast.Expr, info *types.Info) bool {
	if info.Types[expr].Value != nil {
		return true
	}
	if id, ok := expr.(*ast.Ident); ok {
		_, nilValue := info.Uses[id].(*types.Nil)
		return nilValue
	}
	literal, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	_, record := info.TypeOf(literal).Underlying().(*types.Struct)
	for _, elt := range literal.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if !record && info.Types[kv.Key].Value == nil {
				return false
			}
			elt = kv.Value
		}
		if !constantAggregate(elt, info) {
			return false
		}
	}
	return true
}

func constantColumn(literal *ast.CompositeLit, array *types.Array, record *types.Struct, fieldIndex int, basic *types.Basic, info *types.Info) ([]constant.Value, error) {
	var zero constant.Value = constant.MakeInt64(0)
	if basic.Info()&types.IsBoolean != 0 {
		zero = constant.MakeBool(false)
	} else if basic.Info()&types.IsString != 0 {
		zero = constant.MakeString("")
	}
	values := make([]constant.Value, array.Len())
	for j := range values {
		values[j] = zero
	}
	index := int64(0)
	for _, elt := range literal.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			var valid bool
			index, valid = constant.Int64Val(info.Types[kv.Key].Value)
			if !valid {
				return nil, fmt.Errorf("constant field has an invalid array index")
			}
			elt = kv.Value
		}
		row, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("constant field requires literal rows")
		}
		column := 0
		for _, value := range row.Elts {
			if kv, ok := value.(*ast.KeyValueExpr); ok {
				id, ok := kv.Key.(*ast.Ident)
				if !ok {
					return nil, fmt.Errorf("constant field has an invalid struct key")
				}
				column = -1
				for j := 0; j < record.NumFields(); j++ {
					if record.Field(j).Name() == id.Name {
						column = j
						break
					}
				}
				value = kv.Value
			}
			if column == fieldIndex {
				v := info.Types[value].Value
				if v == nil {
					return nil, fmt.Errorf("nonconstant field value")
				}
				values[index] = v
			}
			column++
		}
		index++
	}
	return values, nil
}

func checkConstantFieldUses(file *ast.File, info *types.Info, table types.Object, field *types.Var) error {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	up := func(n ast.Node) ast.Node {
		for {
			p, ok := parents[n].(*ast.ParenExpr)
			if !ok {
				return n
			}
			n = p
		}
	}
	read := func(n ast.Node) bool {
		n = up(n)
		switch p := parents[n].(type) {
		case *ast.AssignStmt:
			for _, lhs := range p.Lhs {
				if lhs == n {
					return false
				}
			}
		case *ast.RangeStmt:
			if p.Key == n || p.Value == n {
				return false
			}
		case *ast.IncDecStmt:
			return false
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				return false
			}
		case *ast.SelectorExpr:
			if sel := info.Selections[p]; sel != nil && sel.Kind() != types.FieldVal {
				return false
			}
		}
		for p := parents[n]; p != nil; p = parents[p] {
			if call, ok := p.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if o := info.Uses[sel.Sel]; o != nil && o.Pkg() != nil && o.Pkg().Path() == "unsafe" {
						return false
					}
				}
			}
		}
		return true
	}
	for id, obj := range info.Uses {
		if obj != table || parents[id] == nil {
			continue
		}
		root := up(id)
		switch p := parents[root].(type) {
		case *ast.CallExpr:
			name, ok := p.Fun.(*ast.Ident)
			if ok && len(p.Args) == 1 && p.Args[0] == root {
				if b, ok := info.Uses[name].(*types.Builtin); ok && (b.Name() == "len" || b.Name() == "cap") {
					continue
				}
			}
		case *ast.RangeStmt:
			if p.X == root {
				continue
			}
		case *ast.IndexExpr:
			if p.X != root {
				break
			}
			top := up(p)
			if sel, ok := parents[top].(*ast.SelectorExpr); ok {
				selection := info.Selections[sel]
				if selection == nil || selection.Kind() != types.FieldVal {
					break
				}
				if selection.Obj() != field {
					continue
				}
				if read(sel) {
					continue
				}
			} else if read(top) {
				continue
			}
		}
		return fmt.Errorf("constant field %s.%s has a write or unsupported escape", table.Name(), field.Name())
	}
	return nil
}

func constantLookup(name, indexType, resultType string, basic *types.Basic, values []constant.Value) string {
	var b strings.Builder
	fmt.Fprintf(&b, "func %s(syncgoIndex %s) %s {\n", name, indexType, resultType)
	if basic.Info()&types.IsBoolean != 0 {
		data := make([]byte, len(values))
		for j, v := range values {
			if constant.BoolVal(v) {
				data[j] = 1
			}
		}
		fmt.Fprintf(&b, "const syncgoData=%q\nreturn %s(syncgoData[syncgoIndex]!=0)\n}\n", string(data), resultType)
		return b.String()
	}
	// Check the original index before multiplying or converting it. This also
	// preserves the original bound and index value in Go's bounds panic.
	fmt.Fprintf(&b, "const syncgoBounds=%q\n_ = syncgoBounds[syncgoIndex]\nsyncgoI:=int(syncgoIndex)\n", strings.Repeat("\x00", len(values)))
	if basic.Info()&types.IsString != 0 {
		offsets := make([]byte, 8*(len(values)+1))
		var data strings.Builder
		for j, v := range values {
			binary.LittleEndian.PutUint64(offsets[j*8:], uint64(data.Len()))
			data.WriteString(constant.StringVal(v))
		}
		binary.LittleEndian.PutUint64(offsets[len(values)*8:], uint64(data.Len()))
		fmt.Fprintf(&b, "const syncgoData=%q\nconst syncgoOffsets=%q\n", data.String(), string(offsets))
		fmt.Fprintf(&b, "syncgoOffset:=func(syncgoJ int) uint64{return %s}\nreturn %s(syncgoData[syncgoOffset(syncgoI):syncgoOffset(syncgoI+1)])\n}\n", packedWord("syncgoOffsets", "syncgoJ"), resultType)
	} else {
		data := make([]byte, 8*len(values))
		for j, v := range values {
			word, ok := constant.Uint64Val(v)
			if !ok {
				signed, _ := constant.Int64Val(v)
				word = uint64(signed)
			}
			binary.LittleEndian.PutUint64(data[j*8:], word)
		}
		fmt.Fprintf(&b, "const syncgoData=%q\nreturn %s(%s)\n}\n", string(data), resultType, packedWord("syncgoData", "syncgoI"))
	}
	return b.String()
}

func packedWord(data, index string) string {
	var terms []string
	for j := 0; j < 8; j++ {
		terms = append(terms, fmt.Sprintf("uint64(%s[%s*8+%d])<<%d", data, index, j, j*8))
	}
	return strings.Join(terms, " | ")
}
