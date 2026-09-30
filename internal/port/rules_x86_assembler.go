package port

import "strings"

// Preserve the native x86 assembler, including its ordinary/AVX tables and
// platform preprocessing. Every mutable table belongs to one Link instance.
func x86AssemblerMigration(units []Unit) []Unit {
	const root = "src/cmd/internal/obj/"
	ctx := func(fields map[string]string) *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Init: true, Selective: true, RedirectTrace: true, RedirectExit: true, Adapter: "adapter_context.go", Fields: fields}
	}
	fields := map[string]map[string]string{
		"asm6.go": {
			"plan9privates": "plan9privates",
			"ycover":        "ycover",
			"reg":           "reg",
			"regrex":        "regrex",
			"ynone":         "ynone",
			"ytext":         "ytext",
			"ynop":          "ynop",
			"yfuncdata":     "yfuncdata",
			"ypcdata":       "ypcdata",
			"yxorb":         "yxorb",
			"yaddl":         "yaddl",
			"yincl":         "yincl",
			"yincq":         "yincq",
			"ycmpb":         "ycmpb",
			"ycmpl":         "ycmpl",
			"yshb":          "yshb",
			"yshl":          "yshl",
			"ytestl":        "ytestl",
			"ymovb":         "ymovb",
			"ybtl":          "ybtl",
			"ymovw":         "ymovw",
			"ymovl":         "ymovl",
			"yret":          "yret",
			"ymovq":         "ymovq",
			"ymovbe":        "ymovbe",
			"ym_rl":         "ym_rl",
			"yrl_m":         "yrl_m",
			"ymb_rl":        "ymb_rl",
			"yml_rl":        "yml_rl",
			"yrl_ml":        "yrl_ml",
			"yml_mb":        "yml_mb",
			"yrb_mb":        "yrb_mb",
			"yxchg":         "yxchg",
			"ydivl":         "ydivl",
			"ydivb":         "ydivb",
			"yimul":         "yimul",
			"yimul3":        "yimul3",
			"ybyte":         "ybyte",
			"yin":           "yin",
			"yint":          "yint",
			"ypushl":        "ypushl",
			"ypopl":         "ypopl",
			"ywrfsbase":     "ywrfsbase",
			"yrdrand":       "yrdrand",
			"yclflush":      "yclflush",
			"ybswap":        "ybswap",
			"yscond":        "yscond",
			"yjcond":        "yjcond",
			"yloop":         "yloop",
			"ycall":         "ycall",
			"yduff":         "yduff",
			"yjmp":          "yjmp",
			"yfmvd":         "yfmvd",
			"yfmvdp":        "yfmvdp",
			"yfmvf":         "yfmvf",
			"yfmvx":         "yfmvx",
			"yfmvp":         "yfmvp",
			"yfcmv":         "yfcmv",
			"yfadd":         "yfadd",
			"yfxch":         "yfxch",
			"ycompp":        "ycompp",
			"ystsw":         "ystsw",
			"ysvrs_mo":      "ysvrs_mo",
			"ysvrs_om":      "ysvrs_om",
			"ymm":           "ymm",
			"yxm":           "yxm",
			"yxm_q4":        "yxm_q4",
			"yxcvm1":        "yxcvm1",
			"yxcvm2":        "yxcvm2",
			"yxr":           "yxr",
			"yxr_ml":        "yxr_ml",
			"ymr":           "ymr",
			"ymr_ml":        "ymr_ml",
			"yxcmpi":        "yxcmpi",
			"yxmov":         "yxmov",
			"yxcvfl":        "yxcvfl",
			"yxcvlf":        "yxcvlf",
			"yxcvfq":        "yxcvfq",
			"yxcvqf":        "yxcvqf",
			"yps":           "yps",
			"yxrrl":         "yxrrl",
			"ymrxr":         "ymrxr",
			"ymshuf":        "ymshuf",
			"ymshufb":       "ymshufb",
			"yxshuf":        "yxshuf",
			"yextrw":        "yextrw",
			"yextr":         "yextr",
			"yinsrw":        "yinsrw",
			"yinsr":         "yinsr",
			"ypsdq":         "ypsdq",
			"ymskb":         "ymskb",
			"ycrc32l":       "ycrc32l",
			"ycrc32b":       "ycrc32b",
			"yprefetch":     "yprefetch",
			"yaes":          "yaes",
			"yxbegin":       "yxbegin",
			"yxabort":       "yxabort",
			"ylddqu":        "ylddqu",
			"ypalignr":      "ypalignr",
			"ysha256rnds2":  "ysha256rnds2",
			"yblendvpd":     "yblendvpd",
			"ymmxmm0f38":    "ymmxmm0f38",
			"yextractps":    "yextractps",
			"ysha1rnds4":    "ysha1rnds4",
			"optab":         "optab",
			"opindex":       "opindex",
			"nop":           "nop",
			"isAndroid":     "isAndroid",
			"ymovtab":       "ymovtab",
		},
		"avx_optabs.go": {
			"_yandnl":             "_yandnl",
			"_ybextrl":            "_ybextrl",
			"_yblsil":             "_yblsil",
			"_ykaddb":             "_ykaddb",
			"_ykmovb":             "_ykmovb",
			"_yknotb":             "_yknotb",
			"_ykshiftlb":          "_ykshiftlb",
			"_yrorxl":             "_yrorxl",
			"_yv4fmaddps":         "_yv4fmaddps",
			"_yv4fmaddss":         "_yv4fmaddss",
			"_yvaddpd":            "_yvaddpd",
			"_yvaddsd":            "_yvaddsd",
			"_yvaddsubpd":         "_yvaddsubpd",
			"_yvaesdec":           "_yvaesdec",
			"_yvaesimc":           "_yvaesimc",
			"_yvaeskeygenassist":  "_yvaeskeygenassist",
			"_yvalignd":           "_yvalignd",
			"_yvandnpd":           "_yvandnpd",
			"_yvblendmpd":         "_yvblendmpd",
			"_yvblendpd":          "_yvblendpd",
			"_yvblendvpd":         "_yvblendvpd",
			"_yvbroadcastf128":    "_yvbroadcastf128",
			"_yvbroadcastf32x2":   "_yvbroadcastf32x2",
			"_yvbroadcastf32x4":   "_yvbroadcastf32x4",
			"_yvbroadcastf32x8":   "_yvbroadcastf32x8",
			"_yvbroadcasti32x2":   "_yvbroadcasti32x2",
			"_yvbroadcastsd":      "_yvbroadcastsd",
			"_yvbroadcastss":      "_yvbroadcastss",
			"_yvcmppd":            "_yvcmppd",
			"_yvcmpsd":            "_yvcmpsd",
			"_yvcomisd":           "_yvcomisd",
			"_yvcompresspd":       "_yvcompresspd",
			"_yvcvtdq2pd":         "_yvcvtdq2pd",
			"_yvcvtdq2ps":         "_yvcvtdq2ps",
			"_yvcvtpd2dq":         "_yvcvtpd2dq",
			"_yvcvtpd2dqx":        "_yvcvtpd2dqx",
			"_yvcvtpd2dqy":        "_yvcvtpd2dqy",
			"_yvcvtpd2qq":         "_yvcvtpd2qq",
			"_yvcvtpd2udqx":       "_yvcvtpd2udqx",
			"_yvcvtpd2udqy":       "_yvcvtpd2udqy",
			"_yvcvtph2ps":         "_yvcvtph2ps",
			"_yvcvtps2ph":         "_yvcvtps2ph",
			"_yvcvtps2qq":         "_yvcvtps2qq",
			"_yvcvtsd2si":         "_yvcvtsd2si",
			"_yvcvtsd2usil":       "_yvcvtsd2usil",
			"_yvcvtsi2sdl":        "_yvcvtsi2sdl",
			"_yvcvtudq2pd":        "_yvcvtudq2pd",
			"_yvcvtusi2sdl":       "_yvcvtusi2sdl",
			"_yvdppd":             "_yvdppd",
			"_yvexp2pd":           "_yvexp2pd",
			"_yvexpandpd":         "_yvexpandpd",
			"_yvextractf128":      "_yvextractf128",
			"_yvextractf32x4":     "_yvextractf32x4",
			"_yvextractf32x8":     "_yvextractf32x8",
			"_yvextractps":        "_yvextractps",
			"_yvfixupimmpd":       "_yvfixupimmpd",
			"_yvfixupimmsd":       "_yvfixupimmsd",
			"_yvfpclasspdx":       "_yvfpclasspdx",
			"_yvfpclasspdy":       "_yvfpclasspdy",
			"_yvfpclasspdz":       "_yvfpclasspdz",
			"_yvgatherdpd":        "_yvgatherdpd",
			"_yvgatherdps":        "_yvgatherdps",
			"_yvgatherpf0dpd":     "_yvgatherpf0dpd",
			"_yvgatherpf0dps":     "_yvgatherpf0dps",
			"_yvgatherqps":        "_yvgatherqps",
			"_yvgetexpsd":         "_yvgetexpsd",
			"_yvgetmantpd":        "_yvgetmantpd",
			"_yvgf2p8affineinvqb": "_yvgf2p8affineinvqb",
			"_yvinsertf128":       "_yvinsertf128",
			"_yvinsertf32x4":      "_yvinsertf32x4",
			"_yvinsertf32x8":      "_yvinsertf32x8",
			"_yvinsertps":         "_yvinsertps",
			"_yvlddqu":            "_yvlddqu",
			"_yvldmxcsr":          "_yvldmxcsr",
			"_yvmaskmovdqu":       "_yvmaskmovdqu",
			"_yvmaskmovpd":        "_yvmaskmovpd",
			"_yvmovapd":           "_yvmovapd",
			"_yvmovd":             "_yvmovd",
			"_yvmovddup":          "_yvmovddup",
			"_yvmovdqa":           "_yvmovdqa",
			"_yvmovdqa32":         "_yvmovdqa32",
			"_yvmovhlps":          "_yvmovhlps",
			"_yvmovhpd":           "_yvmovhpd",
			"_yvmovmskpd":         "_yvmovmskpd",
			"_yvmovntdq":          "_yvmovntdq",
			"_yvmovntdqa":         "_yvmovntdqa",
			"_yvmovq":             "_yvmovq",
			"_yvmovsd":            "_yvmovsd",
			"_yvpbroadcastb":      "_yvpbroadcastb",
			"_yvpbroadcastmb2q":   "_yvpbroadcastmb2q",
			"_yvpclmulqdq":        "_yvpclmulqdq",
			"_yvpcmpb":            "_yvpcmpb",
			"_yvpcmpeqb":          "_yvpcmpeqb",
			"_yvperm2f128":        "_yvperm2f128",
			"_yvpermd":            "_yvpermd",
			"_yvpermilpd":         "_yvpermilpd",
			"_yvpermpd":           "_yvpermpd",
			"_yvpermq":            "_yvpermq",
			"_yvpextrw":           "_yvpextrw",
			"_yvpinsrb":           "_yvpinsrb",
			"_yvpmovb2m":          "_yvpmovb2m",
			"_yvpmovdb":           "_yvpmovdb",
			"_yvpmovdw":           "_yvpmovdw",
			"_yvprold":            "_yvprold",
			"_yvpscatterdd":       "_yvpscatterdd",
			"_yvpscatterdq":       "_yvpscatterdq",
			"_yvpscatterqd":       "_yvpscatterqd",
			"_yvpshufbitqmb":      "_yvpshufbitqmb",
			"_yvpshufd":           "_yvpshufd",
			"_yvpslld":            "_yvpslld",
			"_yvpslldq":           "_yvpslldq",
			"_yvpsraq":            "_yvpsraq",
			"_yvptest":            "_yvptest",
			"_yvrcpss":            "_yvrcpss",
			"_yvroundpd":          "_yvroundpd",
			"_yvscalefpd":         "_yvscalefpd",
			"_yvshuff32x4":        "_yvshuff32x4",
			"_yvzeroall":          "_yvzeroall",
			"avxOptab":            "avxOptab",
		},
		"obj6.go": {
			"unaryDst":  "unaryDst",
			"Linkamd64": "Linkamd64",
			"Link386":   "Link386",
		},
	}
	for i := range units {
		u := &units[i]
		if strings.HasPrefix(u.Source, root+"x86/") && u.Context != nil {
			u.Context = ctx(u.Context.Fields)
		}
		switch u.Source {
		case root + "x86/asm6.go", root + "x86/obj6.go":
			name := u.Source[len(root+"x86/"):]
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
			u.Context = ctx(fields[name])
			if name == "asm6.go" {
				u.ContextParams = []string{"AsmBuf.*", "padJumpsCtx.*"}
			}
			if name == "asm6.go" {
				u.Calls = append(u.Calls, CallRule{Source: root + "util.go", Function: "Rconv", Method: "Link.Rconv"}, CallRule{Source: root + "util.go", Function: "Dconv", Method: "Link.Dconv"})
			}
			if name == "obj6.go" {
				u.ContextParamCalls = []ContextParamCallRule{{Source: root + "util.go", Method: "As.String", Field: "Link", ContextType: "Link"}}
			}
			u.State = append(u.State, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchAMD64", Field: "archAMD64"}, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "Arch386", Field: "arch386"})
			if name == "asm6.go" {
				u.Checks = map[string]string{"instinit": "checkLink", "span6": "checkFunctionAlloc"}
			} else {
				u.Checks = map[string]string{"CanUse1InsnTLS": "checkLink", "progedit": "checkProgAlloc", "preprocess": "checkFunctionAlloc", "errorCheck": "checkFunction"}
			}
		case root + "x86/evex.go":
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
		case root + "x86/a.out.go":
			u.Context.Fields["X86DWARFRegisters"] = "X86DWARFRegisters"
			u.Declarations = append(u.Declarations, "REG_NONE")
		case root + "link.go":
			u.Declarations = append(u.Declarations, "FuncInfo.SpillRegisterArgs", "FuncInfo.UnspillRegisterArgs", "Prog.GetFrom3", "Attribute.NoFrame", "Attribute.NoSplit", "Attribute.NeedCtxt", "Attribute.CFunc")
		case root + "sym.go":
			u.Declarations = append(u.Declarations, "Link.Float32Sym", "Link.Float64Sym", "Link.rodataKind")
		case root + "plist.go":
			u.Declarations = append(u.Declarations, "Plist", "Link.EmitEntryLiveness", "Link.EmitEntryStackMap", "Link.EmitEntryUnsafePoint", "Link.StartUnsafePoint", "Link.EndUnsafePoint", "MarkUnsafePoints")
		}
	}
	units = append(units, Unit{Source: root + "x86/avx_optabs.go", WholeFile: true, Context: ctx(fields["avx_optabs.go"])})
	units = append(units, Unit{Source: root + "x86/seh.go", WholeFile: true, Context: ctx(nil)})
	units = append(units, Unit{Source: root + "pass.go", WholeFile: true}, Unit{Source: root + "ld.go", WholeFile: true}, Unit{Source: root + "x86/ytab.go", WholeFile: true, Context: ctx(nil), ContextParams: []string{"ytab.*"}}, Unit{Source: "src/internal/abi/stack.go", WholeFile: true})
	return units
}
