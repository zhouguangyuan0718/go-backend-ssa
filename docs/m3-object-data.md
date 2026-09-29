# M3 原生对象数据和重定位写入

为 reader 的泛型字典与后续类型元数据生成补齐真实数据写入路径。本批迁入完整 `cmd/internal/obj/data.go`、`fips140.go`、`textflag.go` 和 `compile/internal/objw/objw.go`，并选择原生 Link.Globl/GloblPos/Diag、LSym 类型元数据访问器以及所需符号/重定位常量和字符串方法。FIPS 调试使用迁来的 internal/bisect 匹配算法；BitVec 使用原生结构及 New 构造，不是替代位图模型。

## 公共入口

公共 `objw.Context` 是原生 objw 函数迁为实例方法后的别名，直接消费现有 `types.Universe` 与 `gcbridge.Link/LinkSymbol`。整数、浮点、字符串/字节、地址及偏移写入最终修改原生 `LSym.P/R/Size/Type`；Global 登记同一个符号到 `Link.Data`。

```go
layout, _ := target.NewLayout("linux", "amd64")
link := &gcbridge.Link{Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{
    Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian,
}}}
u, _ := types.NewUniverseWithPositions(layout, &link.PosTable)
w := objw.NewContext(u, link, "linux")

symbol, referred := link.Lookup("p.dictionary"), link.Lookup("p.type")
w.Uintptr(symbol, 0, 42)
end := w.SymPtr(symbol, 8, referred, 0)
w.Global(symbol, int32(end), objw.RODATA|objw.DUPOK)
```

公开 `BitVec` 是同一原生结构的别名。Global 的 DUPOK/RODATA/NOPTR/LOCAL/TLSBSS 标志沿用原名。地址保留为原生重定位项，不假设已解析链接地址。调用者按原生协议设置 `Link.DiagFunc` 收集可继续执行的对象诊断；Link.Diag 仍递增 Link.Errors 再调用该回调。

## 状态和目标

原生 Link 的所有既有字段及回调类型保持不变，仅追加两个受审计的字段：`dataState`（TargetOS、TraceWriter）和 `bisectFIPS`。生成后的完整原生声明对照会单独验证这两个追加字段，再比较全部原始字段。

- GOOS 改用 Link.TargetOS，GOARCH 直接使用既有 Link.Arch.Name；字节序和字宽也来自原生 Link.Arch。
- objw.NewContext 校验目标字宽、字节序和共享位置表，绑定已有 Universe/Link，并禁止通过该入口变更已绑定的 OS。调用者不应直接改写活跃实例的目标字段，也不能跨实例混用符号。
- 原生 bisectFIPS 改为 Link 实例字段，SetFIPSDebugHash 变为 Link 方法。原生 FIPS 平台排除、符号分类、元数据排除和重定位检查均保留，没有用空检查替代。
- FIPS 调试输出进入显式 TraceWriter，nil 时丢弃；原生 log.Fatal/Fatalf 的退出点变为 panic，避免终止库宿主。log 的进程级前缀/时间戳和 exit 行为不属于库输出契约。
- LSym.Grow 的唯一外部依赖是 stateless Fatalf 边界，保持原生签名，不为 LSym 引入额外所有权字段。

同实例的符号写入和 FIPS 配置变更由调用者串行；不同实例不共享目标或调试匹配器。Link/LSym 的可变字段仍是原生低层接口，本批未宣称可防止任意直接字段修改或跨实例符号误用。

## 迁移工具调整

工具升至 `syncgo-m3-v9`：支持给零值可用的原生 Context 添加 receiver initializer，而不要求上游已有构造函数。已有字段和对象身份保留，依赖输入可先由宿主提供。若规则把真正的原生构造函数当作 receiver initializer，则拒绝迁移。obj.Link 的 FIPS 状态初始为零值，不需要反复初始化或 Reset。

host redirection 在显式 RedirectExit 下新增 log.Fatal/Fatalf 的 typed 绑定，并检查适配签名；其他日志/宿主调用仍由原有审计约束。并发 fixture 验证了原生字段保活、实例初始化独立以及退出点改为 panic。版本升级导致已有产物首行工具版本更新，并不表示所有编译器算法都有修改。

## 验证范围

- 大小端与 32/64 位整数、地址对齐，强/弱地址和偏移重定位、Uvarint、BitVec 的固定小端字节序。
- 直接原生 LSym 的浮点、字符串/字节、CU-relative 和 DWARF text address 写入。
- Global 符号类型、大小、标志、Data 登记和重复声明诊断。
- 12 个并发实例交错使用 FIPS 启用/禁用匹配器，检查诊断/trace 隔离及旧符号字节保活；Windows/386、Wasm、AIX 的原生排除规则。
- 无效调试模式和未对齐整数失败后，已有匹配器/数据保持不变。
- 独立 consumer 仅通过公共包构造字节、原生引用和重定位。
- 固定官方源码对照涵盖六组 OS/架构/字宽/字节序输入，比较完整字节、符号类型、大小、标志、重定位和诊断；原生对象声明与无需状态改写的方法另有完整源码对照。

本批不代表类型描述符或泛型字典已生成，也不输出可链接对象文件。reader 的类型/声明实例化和函数体、reflectdata、walk/escape/SSA 及原生工具链替换仍在后续链路中。

本批验证完成：195 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...`、独立 consumer、两版官方 objw 差分及既有回归全部通过。同一套规则跨 go1.27.0/go1.27.1 仍仅 1 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-object-data-tests.log`。
