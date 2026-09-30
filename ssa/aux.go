package ssa

import (
	"go-backend-ssa/abi"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/internal/cmd/obj/s390x"
	compiler "go-backend-ssa/internal/ssa"
)

type Aux = compiler.Aux
type Sym = compiler.Sym
type AuxCall = compiler.AuxCall
type AuxNameOffset = compiler.AuxNameOffset
type ValAndOff = compiler.ValAndOff

// Native S390X Aux values remain available to external backends without internal imports.
type S390XCCMask = s390x.CCMask
type S390XRotateParams = s390x.RotateParams

func NewS390XRotateParams(start, end, amount uint8) S390XRotateParams {
	return s390x.NewRotateParams(start, end, amount)
}

func StringToAux(s string) Aux { return compiler.StringToAux(s) }
func StaticAuxCall(symbol *gcbridge.LinkSymbol, info *abi.ABIParamResultInfo) *AuxCall {
	return compiler.StaticAuxCall(symbol, info)
}
func InterfaceAuxCall(info *abi.ABIParamResultInfo) *AuxCall { return compiler.InterfaceAuxCall(info) }
func ClosureAuxCall(info *abi.ABIParamResultInfo) *AuxCall   { return compiler.ClosureAuxCall(info) }
func OwnAuxCall(symbol *gcbridge.LinkSymbol, info *abi.ABIParamResultInfo) *AuxCall {
	return compiler.OwnAuxCall(symbol, info)
}
