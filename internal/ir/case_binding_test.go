package ir

import "testing"

func TestRawCaseBinding(t *testing.T) {
	a, b := graphContext(8), graphContext(4)
	child := a.NewInt(a.Types.Pos, 7)
	n := &CaseClause{List: Nodes{child}}
	if a.BindCaseClause(n) != n || !a.OwnsNode(n) || n.Op() != OXXX || n.Pos().IsKnown() || n.List[0] != child {
		t.Fatal("changed native raw case")
	}
	for _, candidate := range []*CaseClause{n, {List: Nodes{b.NewInt(b.Types.Pos, 1)}}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid binding accepted")
				}
			}()
			a.BindCaseClause(candidate)
		}()
	}
	if a.OwnsNode(&CaseClause{}) {
		t.Fatal("unbound case gained ownership")
	}
}
