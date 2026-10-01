package objabi

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/go/buildcfg"
)

// Context supplies the native path setting and owns the lazy package-property
// cache. Package classifications still come from the upstream tables.
type Context struct {
	debugOutput                                                         bytes.Buffer
	ObjectConfig                                                        *buildcfg.Context
	Experiment                                                          buildcfg.ExperimentFlags
	GOROOT                                                              string
	runtimePkgs, extraNoInstrumentPkgs, noRaceFuncPkgs, allowAsmABIPkgs []string
	pkgSpecialsOnce                                                     func() map[string]PkgSpecial
}

func NewContext() *Context { return newContext() }

type debugExit struct {
	code    int
	message string
}

func (c *Context) abort(code int)       { panic(debugExit{code: code}) }
func (c *Context) fatalLog(args ...any) { panic(debugExit{code: 2, message: fmt.Sprint(args...)}) }
func (c *Context) fatalLogf(format string, args ...any) {
	panic(debugExit{code: 2, message: fmt.Sprintf(format, args...)})
}
func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(&c.debugOutput, args...) }
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(&c.debugOutput, format, args...)
}

// ParseDebugFlags runs the native parser in order. Help returns its complete
// text and stops parsing; invalid settings return errors. Unknown panics retain
// their normal behavior. The caller discards the debug graph after an error.
func ParseDebugFlags(debug any, ssa DebugSSA, settings []string) (help string, err error) {
	c := newContext()
	defer func() {
		if failure := recover(); failure != nil {
			if exit, ok := failure.(debugExit); ok {
				if exit.code == 0 {
					help = c.debugOutput.String()
				} else {
					err = fmt.Errorf("%s", exit.message)
				}
			} else {
				panic(failure)
			}
		}
	}()
	parser := c.NewDebugFlag(debug, ssa)
	for _, setting := range settings {
		if err = parser.Set(setting); err != nil {
			return "", err
		}
	}
	return "", nil
}
