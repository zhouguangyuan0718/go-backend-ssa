package types2_test

import (
	"fmt"
	"go-backend-ssa/syntax"
	"go-backend-ssa/types2"
	"strings"
)

func ExampleConfig_Check() {
	file, err := syntax.Parse(syntax.NewFileBase("add.go"), strings.NewReader("package p;func Add(x,y int)int{return x+y}"), nil, nil, syntax.CheckBranches)
	if err != nil {
		panic(err)
	}
	state := types2.NewContext()
	config := types2.Config{Context: state, GoVersion: "go1.27", Sizes: state.SizesFor("gc", "amd64")}
	pkg, err := config.Check("example/p", []*syntax.File{file}, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(pkg.Scope().Lookup("Add").Type())
	// Output: func(x int, y int) int
}
