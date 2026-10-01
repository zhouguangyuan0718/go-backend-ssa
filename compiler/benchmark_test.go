package compiler_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"go-backend-ssa/compiler"
)

// BenchmarkSourcePipeline measures a fresh compilation and owned result on
// every iteration. The fixture has generic calls, loops, maps, closures, defer,
// interfaces and 64 ordinary functions; it excludes dependency I/O and linking.
func BenchmarkSourcePipeline(b *testing.B) {
	var source strings.Builder
	source.WriteString(`package p
type Box[T any] struct { Value T }
func Id[T any](x T) T { return x }
type Node struct { Next *Node; N int }
func(n *Node) Value() int { return n.N }
func Call(x interface{Value() int}) int { return x.Value() }
func Closure(x int) func() int { return func() int { x++; return x } }
func Defer(x *int) { defer func(){ *x++ }(); *x+=2 }
func Map(m map[int]int, n int) int { for i:=0;i<n;i++ {m[i]=i*i};return m[n-1] }
`)
	for i := range 64 {
		fmt.Fprintf(&source, "func F%d(xs []int) int { n:=Id(Box[int]{Value:%d}).Value; for i,x:=range xs { n+=i*x }; return n }\n", i, i)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, stage := range []string{"built", "early-optimized", "compile"} {
			b.Run(arch+"/"+stage, func(b *testing.B) {
				req := compiler.Request{ImportPath: "bench/p", Files: []compiler.SourceFile{{Name: "bench.go", Data: []byte(source.String())}}, Target: compiler.Target{GOOS: "linux", GOARCH: arch, GOAMD64: 1}}
				if arch == "arm64" {
					req.Target.GOAMD64 = 0
					req.Target.GOARM64 = "v8.0"
				}
				if stage != "compile" {
					req.Options.Stage = compiler.Stage(stage)
				}
				b.ReportAllocs()
				b.SetBytes(int64(source.Len()))
				b.ResetTimer()
				for b.Loop() {
					if stage == "compile" {
						r, err := compiler.Compile(context.Background(), req)
						if err != nil {
							b.Fatal(err)
						}
						runtime.KeepAlive(r)
					} else {
						r, err := compiler.Build(context.Background(), req)
						if err != nil {
							b.Fatal(err)
						}
						runtime.KeepAlive(r)
					}
				}
			})
		}
	}
}
