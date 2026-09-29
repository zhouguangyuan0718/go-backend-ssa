package ir

import (
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func TestOracleOwnershipAndConcurrentGraphs(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			c := graphContext(8)
			u := c.Types
			pos := src.XPos{}
			fn := c.NewFunc(pos, pos, u.LocalPkg.Lookup("F"), u.NewSignature(nil, nil, nil))
			n := c.NewNameAt(pos, u.LocalPkg.Lookup("x"), u.Types[types.TINT])
			n.Class = PAUTO
			n.Curfn = fn
			lit := c.NewInt(pos, int64(i))
			assign := c.NewAssignStmt(pos, n, lit)
			assign.Def = true
			n.Defn = assign
			fn.Body = Nodes{assign}
			var ro ReassignOracle
			ro.Init(fn)
			if ro.Reassigned(n) || ro.StaticValue(n) != lit {
				t.Error("single definition")
			}
			foreign := graphContext(8).NewInt(pos, 33)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("foreign node accepted")
					}
				}()
				ro.StaticValue(foreign)
			}()
			fn.Body = append(fn.Body, c.NewAssignStmt(pos, n, c.NewInt(pos, 44)))
			ro.Init(fn)
			if !ro.Reassigned(n) || ro.StaticValue(n) != n {
				t.Error("reassignment")
			}
		})
	}
	wg.Wait()
}
