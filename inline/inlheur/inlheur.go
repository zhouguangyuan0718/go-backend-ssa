// Package inlheur exposes native inline function properties and call-site scoring.
// Inlinability metadata and phase order must be supplied by the compiler driver.
package inlheur

import (
	compiler "go-backend-ssa/internal/inline/inlheur"
	"go-backend-ssa/pgoir"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

func NewContext(tc *typecheck.Context, pg *pgoir.Context) *Context {
	return compiler.NewContext(tc, pg)
}
func DeserializeFromString(s string) *FuncProps { return compiler.DeserializeFromString(s) }

type FuncProps = compiler.FuncProps
type FuncPropBits = compiler.FuncPropBits
type ParamPropBits = compiler.ParamPropBits
type ResultPropBits = compiler.ResultPropBits
type CallSite = compiler.CallSite
type CallSiteTab = compiler.CallSiteTab
type ActualExprPropBits = compiler.ActualExprPropBits
type CSPropBits = compiler.CSPropBits

const FuncPropNeverReturns = compiler.FuncPropNeverReturns
const ParamNoInfo = compiler.ParamNoInfo
const ParamFeedsInterfaceMethodCall = compiler.ParamFeedsInterfaceMethodCall
const ParamMayFeedInterfaceMethodCall = compiler.ParamMayFeedInterfaceMethodCall
const ParamFeedsIndirectCall = compiler.ParamFeedsIndirectCall
const ParamMayFeedIndirectCall = compiler.ParamMayFeedIndirectCall
const ParamFeedsIfOrSwitch = compiler.ParamFeedsIfOrSwitch
const ParamMayFeedIfOrSwitch = compiler.ParamMayFeedIfOrSwitch
const ResultNoInfo = compiler.ResultNoInfo
const ResultIsAllocatedMem = compiler.ResultIsAllocatedMem
const ResultIsConcreteTypeConvertedToInterface = compiler.ResultIsConcreteTypeConvertedToInterface
const ResultAlwaysSameConstant = compiler.ResultAlwaysSameConstant
const ResultAlwaysSameFunc = compiler.ResultAlwaysSameFunc
const ResultAlwaysSameInlinableFunc = compiler.ResultAlwaysSameInlinableFunc
const ActualExprConstant = compiler.ActualExprConstant
const ActualExprIsConcreteConvIface = compiler.ActualExprIsConcreteConvIface
const ActualExprIsFunc = compiler.ActualExprIsFunc
const ActualExprIsInlinableFunc = compiler.ActualExprIsInlinableFunc
const CallSiteInLoop = compiler.CallSiteInLoop
const CallSiteOnPanicPath = compiler.CallSiteOnPanicPath
const CallSiteInInitFunc = compiler.CallSiteInInitFunc
