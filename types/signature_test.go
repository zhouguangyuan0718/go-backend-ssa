package types_test

import (
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/position"
	"go-backend-ssa/types"
	"sync"
	"testing"
)

func TestFieldsStructsAndSignatures(t *testing.T) {
	u := universe(t, "amd64")
	value, _ := u.Scalar(types.Int)
	value.SetHasShape(true)
	value.SetNotInHeap(true)
	pkg := u.NewPkg("p", "p")
	var table position.Table
	pos := table.XPos(position.Make(position.NewFile("p.go", "/p.go"), 9, 4))
	field := u.NewField(pos, pkg.Lookup("Value"), value)
	field.Note = "json:\"value\""
	field.SetNointerface(true)
	copied := field.Copy()
	if copied == field || copied.Type != value || copied.Sym != field.Sym || copied.Pos != pos || copied.Note != field.Note || !copied.Nointerface() {
		t.Fatal("field copy changed upstream identity or metadata")
	}
	copied.SetNointerface(false)
	if !field.Nointerface() {
		t.Fatal("field copy shares flags")
	}
	fields := []*types.Field{field}
	record := u.NewStruct(fields)
	var nativeField *gcbridge.Field = record.Field(0)
	var nativeStruct *gcbridge.Struct = record.StructType()
	if nativeField != field || nativeStruct != record.StructType() || record.Kind() != types.StructKind || !record.IsStruct() || record.Universe() != u || record.NumFields() != 1 || record.FieldType(0) != value || !record.HasShape() || !record.NotInHeap() {
		t.Fatal("struct construction changed")
	}
	// Structs retain the upstream input slice ownership, including its entries.
	if &record.Fields()[0] != &fields[0] {
		t.Fatal("struct field slice copied")
	}
	replacement := u.NewField(pos, pkg.Lookup("Replacement"), value)
	fields[0] = replacement
	if record.Field(0) != replacement {
		t.Fatal("field slice storage changed")
	}
	receiver := u.NewField(pos, nil, u.NewPtr(record))
	parameter := u.NewField(pos, pkg.Lookup("args"), u.NewSlice(value))
	parameter.SetIsDDD(true)
	result := u.NewField(pos, nil, value)
	params, results := []*types.Field{parameter}, []*types.Field{result}
	signature := u.NewSignature(receiver, params, results)
	if signature.Kind() != types.Func || signature.Universe() != u || signature.NumRecvs() != 1 || signature.NumParams() != 1 || signature.NumResults() != 1 || signature.Recv() != receiver || signature.Param(0) != parameter || signature.Result(0) != result || !signature.IsVariadic() || !signature.HasShape() {
		t.Fatal("signature sections or flags changed")
	}
	if len(signature.RecvParamsResults()) != 3 || len(signature.RecvParams()) != 2 || len(signature.ParamsResults()) != 2 {
		t.Fatal("parameter projections changed")
	}
	tuple := signature.ResultsTuple()
	if !tuple.IsFuncArgStruct() || tuple.Universe() != u || tuple.Field(0) != result || &tuple.Fields()[0] != &signature.Results()[0] {
		t.Fatal("result tuple lost shared fields/owner")
	}
	// Signature copies slice entries into allParams, but preserves Field pointers.
	params[0] = replacement
	results[0] = replacement
	if signature.Param(0) != parameter || signature.Result(0) != result {
		t.Fatal("signature retained caller slice instead of copying entries")
	}
	method := u.NewField(pos, pkg.Lookup("Method"), signature)
	if !method.IsMethod() || field.IsMethod() {
		t.Fatal("method classification changed")
	}
	empty := u.NewSignature(nil, nil, nil)
	if empty.Recv() != nil || empty.NumRecvs() != 0 || empty.NumParams() != 0 || empty.NumResults() != 0 || empty.IsVariadic() || empty.ResultsTuple().NumFields() != 0 || !empty.ResultsTuple().IsFuncArgStruct() {
		t.Fatal("empty signature semantics changed")
	}
	pair := u.NewTuple(value, u.TypeMem)
	multi := u.NewResults([]*types.Type{value, u.TypeMem})
	if pair.FieldType(0) != value || pair.FieldType(1) != u.TypeMem || multi.NumFields() != 2 || multi.FieldType(1) != u.TypeMem {
		t.Fatal("backend tuple/results accessors changed")
	}
	factory := u.NewSignature
	for i := 0; i < 20; i++ {
		other := universe(t, "386")
		other.NewSignature(nil, nil, nil)
	}
	if factory(nil, []*types.Field{parameter}, nil).Param(0) != parameter || signature.Result(0) != result {
		t.Fatal("later universe invalidated field graph")
	}
}

func TestFieldGraphOwnershipChecks(t *testing.T) {
	a, b := universe(t, "amd64"), universe(t, "386")
	x, _ := a.Scalar(types.Int)
	y, _ := b.Scalar(types.Int)
	foreignSymbol := b.NewPkg("p", "p").Lookup("Foreign")
	good := a.NewField(position.XPos{}, nil, x)
	badType := &types.Field{Type: y}
	badSym := &types.Field{Type: x, Sym: foreignSymbol}
	for name, call := range map[string]func(){
		"new field foreign type":   func() { a.NewField(position.XPos{}, nil, y) },
		"new field foreign symbol": func() { a.NewField(position.XPos{}, foreignSymbol, x) },
		"new field unowned symbol": func() { a.NewField(position.XPos{}, new(types.Sym), x) },
		"nil type":                 func() { a.NewField(position.XPos{}, nil, nil) },
		"struct nil field":         func() { a.NewStruct([]*types.Field{nil}) },
		"struct nil type":          func() { a.NewStruct([]*types.Field{{}}) },
		"struct foreign type":      func() { a.NewStruct([]*types.Field{badType}) },
		"struct foreign symbol":    func() { a.NewStruct([]*types.Field{badSym}) },
		"foreign receiver":         func() { a.NewSignature(badType, nil, nil) },
		"foreign parameter":        func() { a.NewSignature(nil, []*types.Field{badType}, nil) },
		"foreign result symbol":    func() { a.NewSignature(nil, nil, []*types.Field{badSym}) },
		"nil parameter entry":      func() { a.NewSignature(nil, []*types.Field{nil}, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("foreign/incomplete field accepted")
				}
			}()
			call()
		})
	}
	if a.NewStruct([]*types.Field{good}).FieldType(0) != x || b.NewSignature(nil, nil, nil).Universe() != b {
		t.Fatal("rejected input affected later construction")
	}
}

func TestConcurrentSignatureUniverses(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := universe(t, []string{"amd64", "arm64", "386"}[i%3])
			x, _ := u.Scalar(types.Int)
			pkg := u.NewPkg("same", "same")
			for j := 0; j < 50; j++ {
				f := u.NewField(position.XPos{}, pkg.Lookup("Value"), x)
				s := u.NewSignature(nil, []*types.Field{f}, []*types.Field{f})
				s.Param(0).SetIsDDD(j%2 == 0)
				if s.ResultsTuple().Field(0).Type.Universe() != u || s.Result(0).Sym.Universe() != u || s.IsVariadic() != (j%2 == 0) {
					t.Error("signature graph crossed instances")
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
