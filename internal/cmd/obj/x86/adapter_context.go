package x86

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
	"io"
)

// Context installs the native x86 register/opcode formatters on one Link.
type Context struct {
	archAMD64, arch386  *sys.Arch
	X86DWARFRegisters   map[int16]int16
	plan9privates       *obj.LSym
	ycover              [Ymax * Ymax]uint8
	reg                 [MAXREG]int
	regrex              [MAXREG + 1]int
	ynone               []ytab
	ytext               []ytab
	ynop                []ytab
	yfuncdata           []ytab
	ypcdata             []ytab
	yxorb               []ytab
	yaddl               []ytab
	yincl               []ytab
	yincq               []ytab
	ycmpb               []ytab
	ycmpl               []ytab
	yshb                []ytab
	yshl                []ytab
	ytestl              []ytab
	ymovb               []ytab
	ybtl                []ytab
	ymovw               []ytab
	ymovl               []ytab
	yret                []ytab
	ymovq               []ytab
	ymovbe              []ytab
	ym_rl               []ytab
	yrl_m               []ytab
	ymb_rl              []ytab
	yml_rl              []ytab
	yrl_ml              []ytab
	yml_mb              []ytab
	yrb_mb              []ytab
	yxchg               []ytab
	ydivl               []ytab
	ydivb               []ytab
	yimul               []ytab
	yimul3              []ytab
	ybyte               []ytab
	yin                 []ytab
	yint                []ytab
	ypushl              []ytab
	ypopl               []ytab
	ywrfsbase           []ytab
	yrdrand             []ytab
	yclflush            []ytab
	ybswap              []ytab
	yscond              []ytab
	yjcond              []ytab
	yloop               []ytab
	ycall               []ytab
	yduff               []ytab
	yjmp                []ytab
	yfmvd               []ytab
	yfmvdp              []ytab
	yfmvf               []ytab
	yfmvx               []ytab
	yfmvp               []ytab
	yfcmv               []ytab
	yfadd               []ytab
	yfxch               []ytab
	ycompp              []ytab
	ystsw               []ytab
	ysvrs_mo            []ytab
	ysvrs_om            []ytab
	ymm                 []ytab
	yxm                 []ytab
	yxm_q4              []ytab
	yxcvm1              []ytab
	yxcvm2              []ytab
	yxr                 []ytab
	yxr_ml              []ytab
	ymr                 []ytab
	ymr_ml              []ytab
	yxcmpi              []ytab
	yxmov               []ytab
	yxcvfl              []ytab
	yxcvlf              []ytab
	yxcvfq              []ytab
	yxcvqf              []ytab
	yps                 []ytab
	yxrrl               []ytab
	ymrxr               []ytab
	ymshuf              []ytab
	ymshufb             []ytab
	yxshuf              []ytab
	yextrw              []ytab
	yextr               []ytab
	yinsrw              []ytab
	yinsr               []ytab
	ypsdq               []ytab
	ymskb               []ytab
	ycrc32l             []ytab
	ycrc32b             []ytab
	yprefetch           []ytab
	yaes                []ytab
	yxbegin             []ytab
	yxabort             []ytab
	ylddqu              []ytab
	ypalignr            []ytab
	ysha256rnds2        []ytab
	yblendvpd           []ytab
	ymmxmm0f38          []ytab
	yextractps          []ytab
	ysha1rnds4          []ytab
	optab               [870]Optab
	opindex             [(ALAST + 1) & obj.AMask]*Optab
	nop                 [][16]uint8
	ymovtab             []movtab
	_yandnl             []ytab
	_ybextrl            []ytab
	_yblsil             []ytab
	_ykaddb             []ytab
	_ykmovb             []ytab
	_yknotb             []ytab
	_ykshiftlb          []ytab
	_yrorxl             []ytab
	_yv4fmaddps         []ytab
	_yv4fmaddss         []ytab
	_yvaddpd            []ytab
	_yvaddsd            []ytab
	_yvaddsubpd         []ytab
	_yvaesdec           []ytab
	_yvaesimc           []ytab
	_yvaeskeygenassist  []ytab
	_yvalignd           []ytab
	_yvandnpd           []ytab
	_yvblendmpd         []ytab
	_yvblendpd          []ytab
	_yvblendvpd         []ytab
	_yvbroadcastf128    []ytab
	_yvbroadcastf32x2   []ytab
	_yvbroadcastf32x4   []ytab
	_yvbroadcastf32x8   []ytab
	_yvbroadcasti32x2   []ytab
	_yvbroadcastsd      []ytab
	_yvbroadcastss      []ytab
	_yvcmppd            []ytab
	_yvcmpsd            []ytab
	_yvcomisd           []ytab
	_yvcompresspd       []ytab
	_yvcvtdq2pd         []ytab
	_yvcvtdq2ps         []ytab
	_yvcvtpd2dq         []ytab
	_yvcvtpd2dqx        []ytab
	_yvcvtpd2dqy        []ytab
	_yvcvtpd2qq         []ytab
	_yvcvtpd2udqx       []ytab
	_yvcvtpd2udqy       []ytab
	_yvcvtph2ps         []ytab
	_yvcvtps2ph         []ytab
	_yvcvtps2qq         []ytab
	_yvcvtsd2si         []ytab
	_yvcvtsd2usil       []ytab
	_yvcvtsi2sdl        []ytab
	_yvcvtudq2pd        []ytab
	_yvcvtusi2sdl       []ytab
	_yvdppd             []ytab
	_yvexp2pd           []ytab
	_yvexpandpd         []ytab
	_yvextractf128      []ytab
	_yvextractf32x4     []ytab
	_yvextractf32x8     []ytab
	_yvextractps        []ytab
	_yvfixupimmpd       []ytab
	_yvfixupimmsd       []ytab
	_yvfpclasspdx       []ytab
	_yvfpclasspdy       []ytab
	_yvfpclasspdz       []ytab
	_yvgatherdpd        []ytab
	_yvgatherdps        []ytab
	_yvgatherpf0dpd     []ytab
	_yvgatherpf0dps     []ytab
	_yvgatherqps        []ytab
	_yvgetexpsd         []ytab
	_yvgetmantpd        []ytab
	_yvgf2p8affineinvqb []ytab
	_yvinsertf128       []ytab
	_yvinsertf32x4      []ytab
	_yvinsertf32x8      []ytab
	_yvinsertps         []ytab
	_yvlddqu            []ytab
	_yvldmxcsr          []ytab
	_yvmaskmovdqu       []ytab
	_yvmaskmovpd        []ytab
	_yvmovapd           []ytab
	_yvmovd             []ytab
	_yvmovddup          []ytab
	_yvmovdqa           []ytab
	_yvmovdqa32         []ytab
	_yvmovhlps          []ytab
	_yvmovhpd           []ytab
	_yvmovmskpd         []ytab
	_yvmovntdq          []ytab
	_yvmovntdqa         []ytab
	_yvmovq             []ytab
	_yvmovsd            []ytab
	_yvpbroadcastb      []ytab
	_yvpbroadcastmb2q   []ytab
	_yvpclmulqdq        []ytab
	_yvpcmpb            []ytab
	_yvpcmpeqb          []ytab
	_yvperm2f128        []ytab
	_yvpermd            []ytab
	_yvpermilpd         []ytab
	_yvpermpd           []ytab
	_yvpermq            []ytab
	_yvpextrw           []ytab
	_yvpinsrb           []ytab
	_yvpmovb2m          []ytab
	_yvpmovdb           []ytab
	_yvpmovdw           []ytab
	_yvprold            []ytab
	_yvpscatterdd       []ytab
	_yvpscatterdq       []ytab
	_yvpscatterqd       []ytab
	_yvpshufbitqmb      []ytab
	_yvpshufd           []ytab
	_yvpslld            []ytab
	_yvpslldq           []ytab
	_yvpsraq            []ytab
	_yvptest            []ytab
	_yvrcpss            []ytab
	_yvroundpd          []ytab
	_yvscalefpd         []ytab
	_yvshuff32x4        []ytab
	_yvzeroall          []ytab
	avxOptab            [735]Optab
	unaryDst            map[obj.As]bool
	Linkamd64           obj.LinkArch
	Link386             obj.LinkArch

	AMD64DWARFRegisters map[int16]int16
	isAndroid           bool
	Link                *obj.Link
	Anames              []string
	opSuffixTable       [15]string
	evexSuffixMap       [255]evexSuffix
}

func NewContext(link *obj.Link) *Context {
	if link == nil || link.Arch == nil || link.Arch.Arch == nil || link.TargetOS == "" {
		panic("x86 formatting requires a target")
	}
	arches := sys.NewContext()
	c := &Context{Link: link, archAMD64: arches.ArchAMD64, arch386: arches.Arch386}
	switch link.Arch.Family {
	case sys.AMD64:
		c.archAMD64 = link.Arch.Arch
	case sys.I386:
		c.arch386 = link.Arch.Arch
	default:
		panic("x86 assembler requires an x86 target")
	}
	c.initialize()
	return c
}

func (c *Context) fatalLog(args ...any)            { panic(fmt.Sprint(args...)) }
func (c *Context) fatalLogf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (c *Context) traceWriter() io.Writer {
	if c.Link.TraceWriter != nil {
		return c.Link.TraceWriter
	}
	return io.Discard
}
func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }
func (c *Context) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}

func (c *Context) tracePrintf(f string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), f, args...)
}

func (c *Context) checkLink(link *obj.Link) {
	if c == nil || link != c.Link || link == nil || link.Arch == nil || (link.Arch.Arch != c.archAMD64 && link.Arch.Arch != c.arch386) {
		panic("x86 assembler link belongs to another target or instance")
	}
}
func (c *Context) checkProgAlloc(link *obj.Link, p *obj.Prog, newprog obj.ProgAlloc) {
	c.checkLink(link)
	if p == nil || p.Ctxt != link || newprog == nil {
		panic("x86 assembler requires an owned instruction and allocator")
	}
}
func (c *Context) checkFunction(link *obj.Link, s *obj.LSym) {
	c.checkLink(link)
	if s == nil || s.Func() == nil || s.Func().Text == nil || s.Func().Text.Ctxt != link || s.Func().Text.From.Sym != s {
		panic("x86 assembler function belongs to another instance")
	}
}
func (c *Context) checkFunctionAlloc(link *obj.Link, s *obj.LSym, newprog obj.ProgAlloc) {
	c.checkFunction(link, s)
	if newprog == nil {
		panic("x86 assembler requires an instruction allocator")
	}
}
