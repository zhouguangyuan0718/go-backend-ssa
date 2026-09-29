package port

import (
	"strings"
	"testing"
)

func writeLinkSnapshots(t *testing.T, dir string) {
	const template = `package types
 import("fmt";"go-backend-ssa/internal/cmd/obj";EXTRA)
 func LinkSnapshot()[]string{
  link:=&obj.Link{}
  SETUP
  var out []string
  for _,prefix:=range []string{"example/p","space name/p","go","type"}{for _,name:=range []string{"F","x","_"}{
   pkgName:="p";if prefix=="type"{pkgName="type"};sym:=UP.NewPkg(prefix,pkgName).Lookup(name)
   data:=sym.Linksym()
   sym.SetFunc(true)
   fn:=sym.Linksym()
   out=append(out,fmt.Sprintf("%s/%s:%s:%s:%s:%t:%t:%t",prefix,name,data.Name,data.Pkg,data.ABI(),data!=fn,fn.ABI()==obj.ABIInternal,data==link.LookupABI(data.Name,obj.ABI0)))
   sym.Linkname="external."+prefix+name
   renamed:=sym.LinksymABI(obj.ABI0)
   out=append(out,fmt.Sprintf("linkname:%s:%s:%s:%t",renamed.Name,renamed.Pkg,renamed.ABI(),renamed==link.Lookup(renamed.Name)))
  }}
  out=append(out,fmt.Sprint("type-package:",UP.TypeSymLookup("T")==UP.NewPkg("type","type").Lookup("T")))
  return out
 }
 `
	for _, v := range []struct{ path, prefix, setup, extra string }{
		{"internal/types", "u.", "u:=NewUniverse(8,8,1<<50);u.BindLink(link)", ""},
		{"internal/reference/types", "", `pkgMap=make(map[string]*Pkg);typepkg=NewPkg("type","type");base.Ctxt=link`, `"go-backend-ssa/internal/reference/base"`},
	} {
		code := strings.NewReplacer("UP.", v.prefix, "SETUP", v.setup, "EXTRA", v.extra).Replace(template)
		writeTestFile(t, dir, v.path+"/adapter_link_snapshot.go", code)
	}
	writeTestFile(t, dir, "links_test.go", `package comparison_test
 import("reflect";"testing";ported "go-backend-ssa/internal/types";native "go-backend-ssa/internal/reference/types")
 func TestNativeLinkSnapshot(t *testing.T){for range 2{want,got:=native.LinkSnapshot(),ported.LinkSnapshot();if !reflect.DeepEqual(want,got){t.Fatalf("native symbol binding differs\nwant %v\ngot %v",want,got)}}}
 `)
}
