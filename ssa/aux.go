package ssa

import (
	"go-backend-ssa/abi"
	"go-backend-ssa/gcbridge"
	compiler "go-backend-ssa/internal/ssa"
)

type Aux = compiler.Aux
type Sym = compiler.Sym
type AuxCall = compiler.AuxCall
type AuxNameOffset = compiler.AuxNameOffset
type ValAndOff = compiler.ValAndOff

func StringToAux(s string) Aux { return compiler.StringToAux(s) }
func StaticAuxCall(symbol *gcbridge.LinkSymbol, info *abi.ABIParamResultInfo) *AuxCall {
	return compiler.StaticAuxCall(symbol, info)
}
func InterfaceAuxCall(info *abi.ABIParamResultInfo) *AuxCall { return compiler.InterfaceAuxCall(info) }
func ClosureAuxCall(info *abi.ABIParamResultInfo) *AuxCall   { return compiler.ClosureAuxCall(info) }
func OwnAuxCall(symbol *gcbridge.LinkSymbol, info *abi.ABIParamResultInfo) *AuxCall {
	return compiler.OwnAuxCall(symbol, info)
}
