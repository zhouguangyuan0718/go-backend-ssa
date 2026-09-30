package ssa

import (
	"sync"
	"testing"

	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
)

func auxFixture(size int) (*types.Universe, *abi.ABIConfig) {
	link := new(obj.Link)
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	graph := ir.NewContext(u, link)
	typecheck.NewContext(graph).InitUniverse()
	return u, abi.NewContext(graph).NewABIConfig(4, 2, 0, 1)
}

func TestNativeAuxCallOwnership(t *testing.T) {
	u, cfg := auxFixture(4)
	info := cfg.ABIAnalyzeTypes([]*types.Type{u.Types[types.TINT]}, []*types.Type{u.Types[types.TSTRING]})
	call := StaticAuxCall(&obj.LSym{Name: "f"}, info)
	result := call.LateExpansionResultType()
	if cfg.Universe() != u || result.Universe() != u || result.NumFields() != 2 || result.FieldType(0) != u.Types[types.TSTRING] || result.FieldType(1) != u.TypeMem {
		t.Fatal("call lost ABI universe")
	}
	if auxCallContext(call).opcodeTables != nil {
		t.Fatal("call type query allocated backend metadata")
	}
	empty := ClosureAuxCall(cfg.ABIAnalyzeTypes(nil, nil))
	if empty.LateExpansionResultType() != u.TypeResultMem {
		t.Fatal("empty call lost canonical memory result")
	}
	var wg sync.WaitGroup
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other, config := auxFixture(8)
			fresh := OwnAuxCall(nil, config.ABIAnalyzeTypes(nil, []*types.Type{other.Types[types.TINT]}))
			got := fresh.LateExpansionResultType()
			if got.Universe() != other || got.FieldType(0).Size() != 8 || got.FieldType(1) != other.TypeMem || got.FieldType(1) == result.FieldType(1) {
				t.Error("independent call shared types")
			}
			zero := InterfaceAuxCall(config.ABIAnalyzeTypes(nil, nil))
			if zero.LateExpansionResultType() != other.TypeResultMem {
				t.Error("independent zero-result call")
			}
		}()
	}
	wg.Wait()
	if call.TypeOfArg(0).Size() != 4 || result.FieldType(0) != u.Types[types.TSTRING] || call.LateExpansionResultType().FieldType(1) != u.TypeMem || empty.LateExpansionResultType() != u.TypeResultMem {
		t.Fatal("retained call changed")
	}
	if auxToCall(callToAux(call)) != call {
		t.Fatal("call Aux conversion changed identity")
	}
}

func TestNativeAuxCallRejectsUnownedResults(t *testing.T) {
	u, cfg := auxFixture(8)
	foreign, _ := auxFixture(4)
	info := cfg.ABIAnalyzeTypes(nil, []*types.Type{u.Types[types.TINT]})
	call := ClosureAuxCall(info)
	retained := call.LateExpansionResultType()
	info.OutParam(0).Type = foreign.Types[types.TINT]
	mustPanic := func(name string, fn func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("accepted unowned call")
				}
			}()
			fn()
		})
	}
	mustPanic("foreign result", func() { call.LateExpansionResultType() })
	mustPanic("missing ABI", func() { new(AuxCall).LateExpansionResultType() })
	mustPanic("nil call", func() { var a *AuxCall; a.LateExpansionResultType() })
	mustPanic("zero ABI config", func() { ClosureAuxCall(new(abi.ABIParamResultInfo)).LateExpansionResultType() })
	if retained.FieldType(0) != u.Types[types.TINT] || retained.FieldType(1) != u.TypeMem {
		t.Fatal("rejected query changed retained result")
	}
}
