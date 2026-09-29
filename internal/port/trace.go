package port

import (
	"fmt"
	"go/types"
)

// Only reviewed diagnostic output and explicit input operations are redirected. Other os/runtime
// operations still fail the environment audit; this is not an ambient host API.
func hostReferences(info *types.Info, trace, exit, readFile bool) map[types.Object]string {
	refs := map[types.Object]string{}
	for _, obj := range info.Uses {
		if obj == nil || obj.Pkg() == nil {
			continue
		}
		switch obj.Pkg().Path() {
		case "fmt":
			if !trace {
				continue
			}
			if _, ok := obj.(*types.Func); !ok {
				continue
			}
			switch obj.Name() {
			case "Printf":
				refs[obj] = "tracePrintf"
			case "Println":
				refs[obj] = "tracePrintln"
			case "Print":
				refs[obj] = "tracePrint"
			}
		case "log":
			if _, ok := obj.(*types.Func); ok && exit {
				switch obj.Name() {
				case "Fatal":
					refs[obj] = "fatalLog"
				case "Fatalf":
					refs[obj] = "fatalLogf"
				}
			}
		case "os":
			if _, ok := obj.(*types.Func); ok && readFile && obj.Name() == "ReadFile" {
				refs[obj] = "readFile"
			}
			if _, ok := obj.(*types.Func); ok && exit && obj.Name() == "Exit" {
				refs[obj] = "abort"
			}
			if !trace {
				continue
			}
			if _, ok := obj.(*types.Var); ok && (obj.Name() == "Stdout" || obj.Name() == "Stderr") {
				refs[obj] = "traceWriter"
			}
		}
	}
	return refs
}

func checkTraceAdapters(units []Unit, info *types.Info, out *types.Package) error {
	for _, u := range units {
		if u.Context == nil || (!u.Context.RedirectTrace && !u.Context.RedirectExit && !u.Context.RedirectReadFile) {
			continue
		}
		ctx := out.Scope().Lookup(u.Context.Type)
		if ctx == nil {
			return fmt.Errorf("trace context missing")
		}
		for obj, name := range hostReferences(info, u.Context.RedirectTrace, u.Context.RedirectExit, u.Context.RedirectReadFile) {
			method, _, _ := types.LookupFieldOrMethod(types.NewPointer(ctx.Type()), true, out, name)
			if _, ok := method.(*types.Func); !ok {
				return fmt.Errorf("trace adapter method missing: %s", name)
			}
			want := "func() io.Writer"
			if name != "traceWriter" {
				want = types.TypeString(unnamedSignature(obj.Type()), nil)
			}
			if got := types.TypeString(unnamedSignature(method.Type()), nil); got != want {
				return fmt.Errorf("trace adapter signature mismatch: %s: got %s, want %s", name, got, want)
			}
		}
		return nil
	}
	return nil
}
