package gcbridge

import (
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/dwarf"
	"go-backend-ssa/internal/cmd/goobj"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/sys"
)

// Native object/driver data retain their upstream identity. Native assembler,
// linker emission and compiler entry points are not yet integrated.
type Link = obj.Link
type LinkArch = obj.LinkArch
type Prog = obj.Prog
type ProgAlloc = obj.ProgAlloc
type Addr = obj.Addr
type AddrType = obj.AddrType
type AddrName = obj.AddrName
type AddrPos = obj.AddrPos
type OperandPos = obj.OperandPos
type As = obj.As
type NativeFunc = obj.Func
type InlTree = obj.InlTree
type InlinedCall = obj.InlinedCall
type DwarfFixupTable = obj.DwarfFixupTable
type Arch = sys.Arch
type ArchFamily = sys.ArchFamily
type HeadType = objabi.HeadType
type DWARFScope = dwarf.Scope
type DWARFInlCalls = dwarf.InlCalls
type ImportedPkg = goobj.ImportedPkg
type Fingerprint = goobj.FingerprintType
type CmdFlags = base.CmdFlags
type CountFlag = base.CountFlag
type DebugFlags = base.DebugFlags
type HashDebug = base.HashDebug

// Native ABI tags remain usable by an external backend and compiler host.
type ABI = obj.ABI
type ABISet = obj.ABISet

const (
	ABI0           = obj.ABI0
	ABIInternal    = obj.ABIInternal
	ABISetCallable = obj.ABISetCallable
)
