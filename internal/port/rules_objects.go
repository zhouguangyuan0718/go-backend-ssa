package port

// Complete upstream data declarations needed by noder and the native backend.
// These are exact structs/interfaces, not replacements for object algorithms.
// Dependencies precede their users in the ordinary migration order.
func objectDeclarations() []Unit {
	return []Unit{
		{Source: "src/cmd/internal/hash/hash.go", WholeFile: true},
		{Source: "src/internal/goarch/goarch.go", Declarations: []string{"ArchFamilyType"}},
		{Source: "src/cmd/internal/sys/arch.go", Declarations: []string{"Arch", "ArchFamily"}},
		{Source: "src/cmd/internal/dwarf/dwarf.go", Declarations: []string{"InlCall", "InlCalls", "Range", "Scope", "Sym", "Var"}},
		{Source: "src/cmd/internal/goobj/objfile.go", Declarations: []string{"FingerprintType", "ImportedPkg"}},
		{Source: "src/cmd/internal/cov/covcmd/cmddefs.go", Declarations: []string{"CoverFixupConfig"}},
		{Source: "src/cmd/compile/internal/base/debug.go", Declarations: []string{"DebugFlags"}},
		{Source: "src/cmd/compile/internal/base/flag.go", Declarations: []string{"CmdFlags", "CountFlag"}, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{"Flag": "Flag"}}},
		{Source: "src/cmd/internal/objabi/head.go", Declarations: []string{"HeadType"}},
		{Source: "src/cmd/internal/obj/dwarf.go", Declarations: []string{"DwarfFixupTable", "declOffset", "fnState", "relFixup", "symFixups"}},
		{Source: "src/cmd/internal/obj/inl.go", Declarations: []string{"InlTree", "InlinedCall", "InlTree.Add", "InlTree.AllParents", "InlTree.Parent", "InlTree.InlinedFunction", "InlTree.InlinedFuncName", "InlTree.InlinedFuncPkg", "InlTree.CallPos", "Link.OutermostPos", "Link.InnermostPos", "Link.AllPos"}},
		{Source: "src/cmd/internal/obj/plist.go", Declarations: []string{"ProgAlloc", "Link.Globl", "Link.GloblPos"}},
	}
}
