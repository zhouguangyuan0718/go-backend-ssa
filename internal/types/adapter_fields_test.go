package types

import (
	"go-backend-ssa/internal/cmd/src"
	"testing"
)

func TestFieldAndStructLayoutRemainUncomputed(t *testing.T) {
	u := NewUniverse(8, 8, 1<<50)
	value, _ := u.Scalar(TINT)
	f := u.NewField(src.XPos{}, nil, value)
	record := u.NewStruct([]*Field{f})
	signature := u.NewSignature(nil, []*Field{f}, []*Field{f})
	if f.Offset != BADWIDTH || record.width != BADWIDTH || record.align != 0 || signature.ResultsTuple().width != BADWIDTH {
		t.Fatal("constructed fake field/struct layout")
	}
	// Retain upstream's guard against replacing fields once layout is fixed.
	record.align = 8
	defer func() {
		if recover() == nil {
			t.Fatal("calculated struct allowed field replacement")
		}
		if record.Field(0) != f {
			t.Fatal("fields changed before rejecting update")
		}
	}()
	record.setFields(nil)
}
