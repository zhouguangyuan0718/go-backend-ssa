package compiler_test

import (
	"context"
	"fmt"

	"go-backend-ssa/compiler"
)

func ExampleBuild() {
	pkg, err := compiler.Build(context.Background(), compiler.Request{
		ImportPath: "example/math",
		Target:     compiler.Target{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1},
		Files:      []compiler.SourceFile{{Name: "math.go", Data: []byte("package math; func Add(a,b int) int { return a+b }")}},
		Options:    compiler.Options{Stage: compiler.Built},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(pkg.ImportPath, pkg.Metadata.Stage)
	fmt.Println(len(pkg.Functions) > 0, pkg.Artifact != nil)
	// Output:
	// example/math built
	// true true
}
