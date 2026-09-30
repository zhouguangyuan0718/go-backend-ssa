package gcbridge

import (
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/dwarf"
	"go-backend-ssa/internal/cmd/goobj"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/sys"
)

// Native object and driver data retain their upstream identity.
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
type FuncInfo = obj.FuncInfo
type DWARFVar = dwarf.Var

// NewDwarfFixupTable binds native inline debug references to this Link.
func NewDwarfFixupTable(link *Link) *DwarfFixupTable { return obj.NewDwarfFixupTable(link) }

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

// Architectures owns the native architecture descriptors for a compiler setup.
type Architectures = sys.Context

func NewArchitectures() *Architectures { return sys.NewContext() }

// Symbols and relocations are available to standalone SSA consumers without
// importing a compiler-internal package. Zero alignment retains the native
// linker-default meaning; relocation order is not an execution order.
type SymbolKind = objabi.SymKind
type Relocation = obj.Reloc
type RelocType = objabi.RelocType
type SymbolAttributes = obj.Attribute

const (
	Sxxx             = objabi.Sxxx
	STEXT            = objabi.STEXT
	STEXTFIPS        = objabi.STEXTFIPS
	SRODATA          = objabi.SRODATA
	SRODATAFIPS      = objabi.SRODATAFIPS
	SNOPTRDATA       = objabi.SNOPTRDATA
	SNOPTRDATAFIPS   = objabi.SNOPTRDATAFIPS
	SDATA            = objabi.SDATA
	SDATAFIPS        = objabi.SDATAFIPS
	SBSS             = objabi.SBSS
	SNOPTRBSS        = objabi.SNOPTRBSS
	STLSBSS          = objabi.STLSBSS
	R_ADDR           = objabi.R_ADDR
	R_ADDROFF        = objabi.R_ADDROFF
	R_SIZE           = objabi.R_SIZE
	R_CALL           = objabi.R_CALL
	R_CALLIND        = objabi.R_CALLIND
	R_USEFIELD       = objabi.R_USEFIELD
	R_USETYPE        = objabi.R_USETYPE
	R_USEIFACE       = objabi.R_USEIFACE
	R_USEIFACEMETHOD = objabi.R_USEIFACEMETHOD
	R_USENAMEDMETHOD = objabi.R_USENAMEDMETHOD
	R_METHODOFF      = objabi.R_METHODOFF
	R_INITORDER      = objabi.R_INITORDER
	R_KEEP           = objabi.R_KEEP
	R_WEAK           = objabi.R_WEAK
	R_WEAKADDR       = objabi.R_WEAKADDR
)
