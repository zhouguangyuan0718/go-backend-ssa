package port

import (
	"strings"
	"testing"
)

// Run exactly the same graph-building probe against original upstream globals
// and the generated instance methods. No expected layout algorithm is duplicated.
func writeLayoutSnapshots(t *testing.T, dir string) {
	const probe = `package types
import("fmt";"SOURCE_POS";HOST_IMPORT)
func LayoutSnapshot(size int)[]string{
 SETUP
 UP.InitTypes(func(sym *Sym,typ *Type)Object{return &snapshotName{sym,typ}})
 var out []string
 var absent *Type;out=append(out,absent.String(),absent.LinkString(),absent.NameString(),fmt.Sprintf("%L",absent))
 for k,t:=range UP.Types {if t!=nil {out=append(out,fmt.Sprintf("builtin:%d:%s:%d:%d:%d:%d",k,t.String(),t.width,t.align,t.alg,t.ptrBytes))}}
 for _,t:=range []*Type{UP.ByteType,UP.RuneType,UP.AnyType,UP.ErrorType,UP.ComparableType}{out=append(out,fmt.Sprintf("alias:%s:%s:%d:%d",t.String(),t.LinkString(),t.width,t.alg))}
 out=append(out,fmt.Sprint("aliases:",UP.Identical(UP.ByteType,UP.Types[TUINT8]),UP.Identical(UP.RuneType,UP.Types[TINT32]),UP.Identical(UP.AnyType,UP.NewInterface(nil)),UP.Identical(nil,nil)))
 pkg:=UP.NewPkg("probe/layout","layout")
 field:=func(name string,t *Type)*Field{return UP.NewField(src.XPos{},pkg.Lookup(name),t)}
 i,b:=UP.Types[TINT],UP.Types[TUINT8]
 record:=UP.NewStruct([]*Field{field("A",b),field("P",UP.NewPtr(i)),field("Z",UP.NewStruct(nil))})
 blocked:=UP.NewStruct([]*Field{field("S",UP.NewSlice(i))})
 single:=UP.NewStruct([]*Field{field("_",UP.Types[TFLOAT64])})
 sig:=UP.NewSignature(nil,[]*Field{field("X",record)},[]*Field{field("Y",UP.Types[TFLOAT64])})
 method:=field("M",UP.NewSignature(nil,nil,nil))
 iface:=UP.NewInterface([]*Field{method})
 outer:=UP.NewInterface([]*Field{UP.NewField(src.XPos{},nil,iface),UP.NewField(src.XPos{},nil,iface)})
 for _,typ:=range []*Type{record,blocked,single,UP.NewArray(record,0),UP.NewArray(record,1),UP.NewArray(record,3),UP.NewSlice(i),UP.NewMap(i,record),UP.NewChan(i,Cboth),sig,iface,outer}{
  UP.CalcSize(typ);r,f:=typ.Registers()
  out=append(out,fmt.Sprintf("layout:%s:%d:%d:%d:%d:%d:%d:%t",typ.String(),typ.Size(),typ.Alignment(),UP.AlgType(typ),UP.PtrDataSize(typ),r,f,UP.IsComparable(typ)))
  if typ.IsStruct(){for _,f:=range typ.Fields(){out=append(out,fmt.Sprint("offset:",f.Offset))}}
 }
 out=append(out,fmt.Sprint("args:",sig.ArgWidth(),"methods:",len(outer.AllMethods()),outer.AllMethods()[0].Offset))
 UP.DeferCheckSize();UP.DeferCheckSize();pending:=UP.NewArray(i,3);UP.CheckSize(pending);UP.CheckSize(pending)
 out=append(out,fmt.Sprint("deferred:",pending.Deferwidth(),len(UP.deferredTypeStack)))
 UP.ResumeCheckSize();UP.ResumeCheckSize();out=append(out,fmt.Sprint("drained:",pending.Deferwidth(),pending.Size(),UP.defercalc,len(UP.deferredTypeStack)))
 obj:=&snapshotName{sym:pkg.Lookup("Node")};node:=UP.NewNamed(obj);obj.typ=node
 UP.DeferCheckSize();node.SetUnderlying(UP.NewStruct([]*Field{field("Next",UP.NewPtr(node)),field("V",i)}));UP.CheckSize(node);UP.ResumeCheckSize()
 out=append(out,fmt.Sprint("recursive:",node.Size(),UP.PtrDataSize(node),node.FieldType(0).Elem()==node,fmt.Sprintf("%L",node)))
 // Atomic align64 is special on 386; SIMD gates must belong to the instance.
 atom:=&snapshotName{sym:UP.NewPkg("sync/atomic","atomic").Lookup("align64")};at:=UP.NewNamed(atom);atom.typ=at;at.SetUnderlying(UP.NewStruct(nil));UP.CalcSize(at)
 out=append(out,fmt.Sprint("atomic:",at.Size(),at.Alignment()))
 for _,enabled:=range []bool{false,true}{EXPERIMENT.SIMD=enabled;obj:=&snapshotName{sym:UP.NewPkg("simd/archsimd","archsimd").Lookup("v128")};tag:=UP.NewNamed(obj);obj.typ=tag;tag.SetUnderlying(UP.NewStruct([]*Field{field("V",b)}));UP.CalcSize(tag);out=append(out,fmt.Sprint("simd:",tag.IsSIMD(),tag.Size(),tag.Alignment()));wrapped:=UP.NewStruct([]*Field{field("Tag",tag),field("Data",UP.NewArray(b,16))});UP.CalcSize(wrapped);r,f:=wrapped.Registers();out=append(out,fmt.Sprint("simd wrapper:",wrapped.IsSIMD(),wrapped.Size(),r,f))}
 UP.CalcSize(UP.NewChan(UP.NewArray(b,1<<16),Cboth))
 UP.CalcSize(UP.NewInterface([]*Field{field("M",sig),field("M",sig)}))
 // Low explicit MaxWidth checks overflow formatting with native %L.
 UP.MaxWidthFIELD=1024;UP.CalcSize(UP.NewArray(UP.Types[TUINT64],256));UP.MaxWidthFIELD=1<<50
 DIAGNOSTICS
 return out
}
`
	for _, variant := range []struct{ path, prefix, setup, host, experiment, diagnostics, maxWidth string }{
		{"internal/reference/types", "", `PtrSize=size;RegSize=size;MaxWidth=1<<50;defercalc=0;deferredTypeStack=nil;recvType=nil;Types=[NTYPE]*Type{};NewPtrCacheEnabled=true;BuiltinPkg=NewPkg("go.builtin","");UnsafePkg=NewPkg("unsafe","unsafe");LocalPkg=nopkg;base.Diagnostics=nil;buildcfg.Experiment.SIMD=false`, `"go-backend-ssa/internal/reference/base";"go-backend-ssa/internal/reference/buildcfg"`, "buildcfg.Experiment", `out=append(out,base.Diagnostics...)`, "MaxWidth"},
		{"internal/types", "u.", `u:=NewUniverse(size,size,1<<50)`, "", "u.Experiment", `for _,d:=range u.Diagnostics.Diagnostics(){out=append(out,d.Code+":"+d.Message)}`, "maxWidth"},
	} {
		pos := "go-backend-ssa/internal/cmd/src"
		if variant.prefix == "" {
			pos = "go-backend-ssa/internal/reference/src"
		}
		code := strings.NewReplacer("SETUP", variant.setup, "UP.", variant.prefix, "SOURCE_POS", pos, "HOST_IMPORT", variant.host, "EXPERIMENT", variant.experiment, "DIAGNOSTICS", variant.diagnostics, "MaxWidthFIELD", variant.maxWidth).Replace(probe)
		writeTestFile(t, dir, variant.path+"/adapter_layout_snapshot.go", code)
	}
}
