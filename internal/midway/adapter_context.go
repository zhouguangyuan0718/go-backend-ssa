package midway

import (
	"fmt"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/types2"
)

// Context binds midway to one source checker. Each operation's package, Info,
// and syntax trees must come from that checker and are mutated by native code.
type Context struct {
	Checker    *types2.Context
	GOARCH     string
	Experiment buildcfg.ExperimentFlags
	Debug      base.DebugFlags
	Warn       func(string, ...any)
}

func NewContext(checker *types2.Context, arch string, experiment buildcfg.ExperimentFlags, debug base.DebugFlags) *Context {
	if checker == nil {
		panic("midway requires a checker")
	}
	return &Context{Checker: checker, GOARCH: arch, Experiment: experiment, Debug: debug}
}
func (c *Context) warn(format string, args ...any) {
	if c.Warn != nil {
		c.Warn(format, args...)
	}
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) checkAnalysis(pkg *types2.Package, info *types2.Info) {
	if pkg == nil || pkg.Context() != c.Checker || info == nil {
		panic("midway requires a package and Info from its checker")
	}
}
func (c *Context) checkRewrite(pkg *types2.Package, info *types2.Info, a *Analyzer, sizes []int) {
	c.checkAnalysis(pkg, info)
	if a == nil || a.compilation != c || a.pkg != pkg || a.info != info {
		panic("midway analyzer belongs to another compilation or checked package")
	}
}
func (c *Context) checkCopy(pkg *types2.Package, info *types2.Info, vecLen int, a *Analyzer, suffix string) {
	c.checkRewrite(pkg, info, a, nil)
}
