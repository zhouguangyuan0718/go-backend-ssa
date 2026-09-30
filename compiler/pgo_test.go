package compiler_test

import (
	"bytes"
	"context"
	"fmt"
	"go-backend-ssa/compiler"
	"go-backend-ssa/pgo"
	"os"
	"strings"
	"testing"
)

func TestNativeProfileInput(t *testing.T) {
	source, err := os.ReadFile("../testdata/pgo/hot.go")
	if err != nil {
		t.Fatal(err)
	}
	req := request("p", string(source))
	req.Files[0].Name = "hot.go"
	cold, err := compiler.Compile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var expected []byte
	for _, name := range []string{"hot.pb", "hot.pprof", "hot.preprofile"} {
		data, err := os.ReadFile("../testdata/pgo/" + name)
		if err != nil {
			t.Fatal(err)
		}
		req.PGOProfile = &compiler.ProfileInput{Name: name, Data: data}
		result, err := compiler.Compile(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(cold.Archive, result.Archive) {
			t.Fatal("hot profile did not affect compilation", name)
		}
		if expected == nil {
			expected = bytes.Clone(result.Archive)
		} else if !bytes.Equal(expected, result.Archive) {
			t.Fatal("profile representations differ", name)
		}
		clear(data)
		if !bytes.Equal(expected, result.Archive) {
			t.Fatal("result retained profile input")
		}
	}
	req.PGOProfile = &compiler.ProfileInput{Name: "empty.pprof"}
	empty, err := compiler.Compile(context.Background(), req)
	if err != nil || !bytes.Equal(empty.Archive, cold.Archive) {
		t.Fatal("empty profile handling", err)
	}
	req.PGOProfile = &compiler.ProfileInput{Name: "bad.pprof", Data: []byte("not a profile")}
	if result, err := compiler.Compile(context.Background(), req); result != nil || err == nil || !strings.Contains(err.Error(), "bad.pprof: PGO error:") {
		t.Fatal("bad profile accepted", result, err)
	}
}

func TestConcurrentRawProfileProcessing(t *testing.T) {
	data, err := os.ReadFile("../testdata/pgo/hot.pprof")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("../testdata/pgo/hot.preprofile")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 16 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			p, err := pgo.FromPProf(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if _, err = p.WriteTo(&output); err != nil {
				t.Fatal(err)
			}
			if p.TotalWeight != 100 || !bytes.Equal(output.Bytes(), expected) {
				t.Fatal("raw profile edge map differs", p.TotalWeight, output.String())
			}
			prior := p.NamedEdgeMap.Weight[pgo.NamedCallEdge{CallerName: "p.Caller", CalleeName: "p.Hot", CallSiteOffset: 1}]
			empty, err := pgo.FromPProf(bytes.NewReader(nil))
			if err != nil || empty.TotalWeight != 0 || prior != 100 {
				t.Fatal("profile state leaked", err)
			}
		})
	}
}

// Walk's native package initializers must precede both local package allocation
// and PGO's lazy body imports; builtin error methods must use that local package.
func TestNativePackageInitializationOrder(t *testing.T) {
	for _, path := range []string{"example/p", "internal/runtime/math"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			req := request(path, `package math
 type E int
 func(E)Error()string{return "error"}
 func F()error{return E(1)}
 func Grow(n int)[]int{return make([]int,n)}
 `)
			if _, err := compiler.Compile(context.Background(), req); err != nil {
				t.Fatal(err)
			}
		})
	}
}
