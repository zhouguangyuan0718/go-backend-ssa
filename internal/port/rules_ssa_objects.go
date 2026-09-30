package port

// Preserve complete upstream constant groups, including their iota bases and
// aliases. SSA opGen.go refers directly to these assembler identities.
func ssaObjectMigration() []Unit {
	const root = "src/cmd/internal/obj/"
	return []Unit{
		{Source: root + "arm/a.out.go", Declarations: []string{"REG_R0", "AAND"}},
		{Source: root + "arm64/a.out.go", Declarations: []string{"REG_R0", "REGMIN", "AADC"}},
		{Source: root + "loong64/cpu.go", Declarations: []string{"REG_R0"}},
		{Source: root + "loong64/inst.go", Declarations: []string{"AABSD"}},
		{Source: root + "mips/a.out.go", Declarations: []string{"REG_R0", "AABSD"}},
		{Source: root + "ppc64/a.out.go", Declarations: []string{"REG_R0", "AADD"}},
		{Source: root + "ppc64/asm9_gtables.go", Declarations: []string{"AXXSETACCZ"}},
		{Source: root + "riscv/cpu.go", Declarations: []string{"REG_X0", "AADDI"}},
		{Source: root + "s390x/a.out.go", Declarations: []string{"REG_R0", "AADD"}},
		{Source: root + "wasm/a.out.go", Declarations: []string{"AGet", "REG_SP"}},
		{Source: root + "x86/a.out.go", Declarations: []string{"REG_AL"}},
		{Source: root + "x86/aenum.go", Declarations: []string{"AAAA"}},
	}
}
