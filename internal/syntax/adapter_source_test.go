package syntax

import (
	"strings"
	"sync"
	"testing"
)

func TestSourcePositionsAndCallbackOwnership(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var a, b source
			var errorsA, errorsB []string
			a.init(strings.NewReader("a\xffb"), func(line, col uint, msg string) {
				errorsA = append(errorsA, msg)
				if line != 1 || col != 2 {
					t.Errorf("error location %d:%d", line, col)
				}
			})
			b.init(strings.NewReader("α\nx"), func(_, _ uint, msg string) { errorsB = append(errorsB, msg) })
			type at struct {
				ch        rune
				line, col uint
			}
			var got []at
			for b.nextch(); b.ch >= 0; b.nextch() {
				line, col := b.pos()
				got = append(got, at{b.ch, line, col})
				a.nextch()
			}
			for a.ch >= 0 {
				a.nextch()
			}
			want := []at{{'α', 1, 1}, {'\n', 1, 3}, {'x', 2, 1}}
			if len(got) != len(want) {
				t.Errorf("characters: %v", got)
				return
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("position %d: %v != %v", i, got[i], want[i])
				}
			}
			if len(errorsA) != 1 || !strings.Contains(errorsA[0], "UTF-8") || len(errorsB) != 0 {
				t.Errorf("callbacks mixed: %v / %v", errorsA, errorsB)
			}
		}()
	}
	wg.Wait()
}

func TestSyntaxLineDirectiveAndSaturation(t *testing.T) {
	file := NewFileBase("actual.go")
	line := NewLineBase(MakePos(file, 10, 3), "original.go", false, 100, 5)
	p := MakePos(line, 12, 7)
	if p.FileBase() != file || p.RelLine() != 102 || p.RelCol() != 7 || p.String() != "original.go:102:7[actual.go:12:7]" {
		t.Fatal(p)
	}
	if MakePos(file, PosMax+1, PosMax+1).Line() != PosMax {
		t.Fatal("line saturation changed")
	}
}
