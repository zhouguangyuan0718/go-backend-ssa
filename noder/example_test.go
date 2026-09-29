package noder_test

import (
	"fmt"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/noder"
	"go-backend-ssa/syntax"
	"go-backend-ssa/target"
	"go-backend-ssa/types"
	"go-backend-ssa/types2"
	"strings"
)

func ExampleNewContext() {
	file, err := syntax.Parse(syntax.NewFileBase("id.go"), strings.NewReader("package p;func Id[T any](x T)T{return x};var X=Id(1)"), nil, nil, syntax.CheckBranches)
	if err != nil {
		panic(err)
	}
	checker := types2.NewContext()
	info := &types2.Info{
		StoreTypesInSyntax: true,
		Defs:               map[*syntax.Name]types2.Object{},
		Uses:               map[*syntax.Name]types2.Object{},
		Instances:          map[*syntax.Name]types2.Instance{},
		Selections:         map[*syntax.SelectorExpr]*types2.Selection{},
		Implicits:          map[syntax.Node]types2.Object{},
		Scopes:             map[syntax.Node]*types2.Scope{},
		FileVersions:       map[*syntax.PosBase]string{},
	}
	files := []*syntax.File{file}
	config := types2.Config{Context: checker, GoVersion: "go1.27", Sizes: checker.SizesFor("gc", "amd64")}
	pkg, err := config.Check("example/p", files, info)
	if err != nil {
		panic(err)
	}
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		panic(err)
	}
	universe, err := types.NewUniverse(layout)
	if err != nil {
		panic(err)
	}
	link := &gcbridge.Link{Pathname: "/src", Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8}}}
	writer, err := noder.NewContext(checker, universe, link)
	if err != nil {
		panic(err)
	}
	stub, err := writer.WritePackage(pkg, info, files, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(stub) > 0, pkg.Scope().Lookup("X").Type())
	// Output: true int
}
