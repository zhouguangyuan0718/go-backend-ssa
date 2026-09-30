# go-backend-ssa

将 Go 1.27 官方编译器从源码解析到 SSA 的实现移植为可嵌入进程的 Go 库，供其他编译器后端直接消费。

库维护基础设施、公共数据模型、状态归属和生命周期；移植工具从固定上游提交抽取算法，通过可复现的结构改写接入这些设施。每次调用拥有独立编译状态，返回结果在后续调用期间仍有效。

当前状态：**M1 已完成，M2 已完成原生类型布局与预声明类型初始化，M3 已提供源码解析和原生 types2 类型检查入口，并提供已检查 AST 到原生 Unified IR stub 的 writer，以及实例独立的原生 IR 图构造接口与 typecheck universe 初始化；尚无 Go 源码到 SSA 的 Build API。** 当前可用编译器 syntax.Parse/AST、实例独立的 types2.Config.Check/Info、位置、诊断、目标布局输入、包符号查找、实例化 SSA 特殊类型、tuple/results 及指针/切片/数组/map/channel 构造、未定类型与常量映射、字段/结构体/签名构造，以及本地 stub 到读取阶段原生包 IR、原生整包去虚化/内联的入口和上游 CalcSize、InitTypes、类型等价与比较算法；完整前端和编译流程仍待移植。

- 语言范围：Go 1.27；其他语言版本尚不支持。
- 首个源码基线：官方 `go1.27.1`，提交 `862c888e612ac346c7c4d99c9392bdfd265f33b0`。
- 交付形式：普通 Go module，进程内调用，返回本库的 SSA 对象。
- 对外保证目标：调用隔离、无共享可变编译状态、可重入、跨调用并发安全。
- 并列验收目标：可替换官方 Go 1.27 编译器对应实现，接回原生后端并完成工具链构建。
- 初始目标配置：`linux/amd64` 和 `linux/arm64`；`linux/386` 已加入布局隔离验证，不表示支持完整的 386 编译器。

## 阅读顺序

1. [整体设计](docs/design.md)：目标、分层、代码归属与确定的取舍。
2. [完整编译流程](docs/pipeline.md)：AST、类型检查、IR、优化、walk 和 SSA 的移植范围。
3. [状态与生命周期](docs/state.md)：Context、类型缓存、回调、返回结果和并发。
4. [源码移植机制](docs/porting.md)：抽取、符号改写、补丁、审计和升级。
5. [公共 API 与后端契约](docs/api.md)：输入、依赖导入、SSA 形态、runtime 和错误。
6. [实施路线](docs/roadmap.md)：逐步细化、实现和验收。
7. [源码基线与证据](docs/baseline.md)：本次实际检查的版本和入口。
8. [原生编译器回接](docs/go-compiler-integration.md)：共享对象模型、gcbridge、native tail 与自举验收。
9. [M1 实现与验证](docs/m1.md)：首批工具与切片记录。
10. [M2 基础设施](docs/m2.md)：第一批位置设施与归属设计。
11. [M2 类型与 Universe](docs/m2-universe.md)：第二至七批实现、对外入口及类型布局边界。
12. [M3 原生语法树](docs/m3-syntax.md)：Parse、AST、无全局解析状态与上游差分。
13. [M3 类型检查迁移](docs/m3-types2.md)：源码类型检查入口、Context 隔离、显式导入/尺寸及完整上游差分。
14. [M3 Unified IR 编码基础](docs/m3-unified.md)：原生 codec、解码失败隔离及字节级差分。
15. [M3 原生对象声明](docs/m3-objects.md)：Link/Prog/架构/驱动输入、公共别名及完整声明对照。
16. [M3 noder 位置映射](docs/m3-noder.md)：syntax.Pos 到原生 Link.PosTable、路径输入及上游差分。
17. [M3 原生 writer](docs/m3-writer.md)：已检查 AST 到 Unified IR stub、类型缓存解耦及双版本完整字节对照。
18. [M3 rangefunc 重写](docs/m3-rangefunc.md)：原生函数迭代 lowering、实例 helper 包和完整编码对照。
19. [M3 原生符号与内联位置](docs/m3-link.md)：ABI 符号身份、Link 显式绑定和内联位置还原。
20. [M3 原生 IR 图](docs/m3-ir.md)：节点构造/复制/遍历、真实 ir.Name 类型绑定、函数/闭包及位置归属。
21. [M3 原生 typecheck universe](docs/m3-typecheck.md)：内建名称、常量、操作表初始化及共享函数状态。
22. [M3 原生 runtime 声明](docs/m3-runtime.md)：隐藏声明、类型参数替换、ABI 符号与实例计时。
23. [M3 原生 IR 检查](docs/m3-typecheck-ir.md)：表达式/语句改写、函数声明、嵌入成员和闭包捕获布局。
24. [M3 reader 记录解码](docs/m3-reader-metadata.md)：原生读取结构、包/位置/引用记录、内联位置和 ScopeMarker。
25. [M3 原生对象数据写入](docs/m3-object-data.md)：LSym 数据/重定位、Global 登记及实例 FIPS 状态。
26. [M3 原生运行时布局](docs/m3-rttype.md)：目标 ABI 布局、实例 Cursor 和类型元数据写入基础。
27. [M3 原生类型元数据引用](docs/m3-reflectdata.md)：类型符号、生成队列、IR 引用、接口标记及 map 内部布局。
28. [M3 原生 GC 位图](docs/m3-gc-masks.md)：完整 bitvec/typebits、GC 符号登记与写入、并发后端用法。
29. [M3 原生哈希与相等比较](docs/m3-algorithms.md)：完整算法生成、闭包数据、实例缓存和公开 EqFor。
30. [M3 原生静态数据](docs/m3-staticdata.md)：字符串、常量与 slice 初始化、实例计数器及并发函数值登记。
31. [M3 原生类型描述符与 ITab](docs/m3-descriptors.md)：完整类型数据、方法表、接口转换、插件导出和基础类型写入。
32. [M3 reader 字典与共享状态](docs/m3-reader-dictionaries.md)：字典解码、shape 处理、原生名称和共享 IR 配置/诊断。
33. [M3 原生类型、对象与函数体读取](docs/m3-reader-bodies.md)：真实泛型实例、闭包函数体、字典写入及作用域。
34. [M3 原生静态初始化调度](docs/m3-staticinit.md)：包初始化读取、静态/动态调度、map 分离和 R_KEEP。
35. [M3 原生调用图](docs/m3-callgraph.md)：函数强连通分量、实例 profile 归属和原生流格式。
36. [M3 原生内联启发式](docs/m3-inline-heuristics.md)：局部重赋值分析、函数属性、调用点评分及实例日志。
37. [M3 原生可内联判定与函数体驱动](docs/m3-native-inlining.md)：真实成本/PGO 决策、hash 状态解耦和生产 readBodies。
38. [M3 原生包注册与读取](docs/m3-package-reader.md)：导入函数/方法查找、真实 PGO 绑定及本地包 IR 读取入口。
39. [M3 原生函数符号](docs/m3-text-symbols.md)：InitLSym、文本属性、FuncID 及 DWARF 内联引用修正。
40. [M3 原生内联调用展开](docs/m3-inline-expansion.md)：真实 InlinedCallExpr、导入函数体、闭包、副作用及实例回调绑定。
41. [M3 原生去虚化与整包内联](docs/m3-interleaved.md)：静态/PGO 去虚化、原生固定点驱动、状态绑定及源码图差分。
42. [M3 原生方法包装函数](docs/m3-wrappers.md)：接收者/方法值 wrapper、原生收尾优化及导入去重。
43. [M3 原生循环变量捕获](docs/m3-loopvar.md)：原生 loopvar 改写、声明搬移、continue 处理和实例诊断。
44. [M3 原生包初始化任务](docs/m3-pkginit.md)：原生 MakeTask、初始化记录和 ASan 全局变量登记。
45. [M3 原生 ABI wrappers](docs/m3-abi-wrappers.md)：汇编 ABI 输入、原生 wrapper IR 生成及实例绑定。
46. [M3 原生 deadlocals](docs/m3-deadlocals.md)：实例独立的无用局部赋值消除、闭包符号初始化及原生差分。
47. [M3 原生 escape](docs/m3-escape.md)：原生逃逸分析、参数摘要、闭包捕获分类和实例缓存。

## 当前可运行的工具

本地 module 路径为 `go-backend-ssa`，工具链为 Go 1.27.1。维护命令读取固定 Git 对象，不修改来源 Go 工作树；日常测试不需要来源仓库或网络。

```sh
export GOTOOLCHAIN=go1.27.1
export GO_SSA_SOURCE_REPO=/path/to/local/golang-go

go run ./cmd/syncgo inspect -repo "$GO_SSA_SOURCE_REPO" -out _work/inspection.json -summary _work/inspection-summary.json
go run ./cmd/syncgo generate -repo "$GO_SSA_SOURCE_REPO"
go run ./cmd/syncgo check -repo "$GO_SSA_SOURCE_REPO"
go test -race -timeout 20m ./...
```

上游版本和移植规则统一维护在 [internal/port/rules.go](internal/port/rules.go)，不需要 JSON 配置。审计报告按需生成到 `_work/`；`sync/generated.json` 仅由工具自动维护。

设定 `GO_SSA_SOURCE_REPO` 时，测试额外验证官方 go1.27.1 再生成、两版上游位置/符号回归与 SSA 类型切片差分测试、源码树未变以及 go1.27.0 → go1.27.1 升级；仓库需包含两个 tag。完整双版本差分包含多组独立原生编译，使用 `-timeout 20m`，避免默认 10 分钟时限。无该变量时仅跳过这项集成测试，其他测试照常执行。

编译器源码从 `src/cmd/compile/` 以下保留目录、包名和文件名；辅助依赖分别归入 `internal/cmd/` 与 `internal/go/`。当前移植代码包括完整位置模块、完整前端语法解析器与 AST、完整 types2 checker、pkgbits codec、rangefunc 重写器和 noder writer，原生 IR 图构造与完整 typecheck 生产包，以及 types/objabi 的结构和算法切片；reader 的类型/对象实例化、函数体和字典读取主体已接入并完成源码差分；整包内联、方法包装、loopvar、pkginit、ABI wrappers、deadlocals 和 escape 已接通；slice、walk 和 SSA 流程尚未接通。

移植后的源码直接保存在本项目，和手写适配、转换规则一起纳入版本管理。使用本库时不需要先运行移植工具；维护者更新上游版本时再重新生成。

| 当前移植代码 | 内容 |
| --- | --- |
| [internal/cmd/src/pos.go](internal/cmd/src/pos.go)、[xpos.go](internal/cmd/src/xpos.go) | 位置、压缩位置、位置表与调试标记 |
| [internal/syntax/parser.go](internal/syntax/parser.go)、[scanner.go](internal/syntax/scanner.go)、[nodes.go](internal/syntax/nodes.go) | 编译器语法解析、独立扫描状态及原生 AST |
| [internal/types2/api.go](internal/types2/api.go) | 原生类型检查、实例 Context、类型/对象及 Info |
| [internal/go/pkgbits/encoder.go](internal/go/pkgbits/encoder.go)、[decoder.go](internal/go/pkgbits/decoder.go) | Unified IR 底层编码/解码、实例诊断与可恢复失败 |
| [internal/types/type.go](internal/types/type.go)、[sym.go](internal/types/sym.go) | 真实 Type/Sym 结构、标志位与选定构造算法 |
| [internal/cmd/obj/link.go](internal/cmd/obj/link.go) | 完整原生 Link/LinkArch/Prog/Addr/LSym 及依赖数据声明 |
| [internal/types/size.go](internal/types/size.go)、[universe.go](internal/types/universe.go)、[identity.go](internal/types/identity.go) | 原生布局/延迟队列、预声明类型初始化及等价判断 |
| [internal/types/pkg.go](internal/types/pkg.go) | Universe 包表、字符串驻留与 Pkg 查找方法 |
| [internal/cmd/objabi/path.go](internal/cmd/objabi/path.go) | 符号路径编码与解码 |
| [internal/rangefunc/rewrite.go](internal/rangefunc/rewrite.go) | 原生 range-over-func 重写、显式 checker 与 runtime helper 归属 |
| [internal/noder/writer.go](internal/noder/writer.go) | 原生 Unified IR 声明/类型/函数体写入，独立缓存与既有 checker 绑定 |
| [internal/noder/posmap.go](internal/noder/posmap.go) | 原生 syntax.Pos 到 Link.PosTable 映射、line pragma 与实例位置缓存 |
| [internal/ir/node.go](internal/ir/node.go)、[expr.go](internal/ir/expr.go)、[func.go](internal/ir/func.go) | 原生 IR 节点、构造/复制/遍历、名称、函数/闭包、常量及格式化 |
| [internal/typecheck/typecheck.go](internal/typecheck/typecheck.go)、[expr.go](internal/typecheck/expr.go)、[stmt.go](internal/typecheck/stmt.go) | 原生 IR 表达式/语句检查与改写、声明和内建调用 |

手写适配示例为 [internal/types/adapter_context.go](internal/types/adapter_context.go)，生成来源和哈希记录在 [sync/generated.json](sync/generated.json)。

## 当前公开基础设施

`inline` 提供原生可内联判定与成本/PGO 政策，`noder.Context.ReadBodies` 运行实例函数体队列驱动，`inline/inlheur` 提供原生函数属性与调用点评分，`logopt` 提供实例优化日志记录与读取；`pgo` 暴露原生预处理 profile 流格式，`pgoir` 提供已有 IR 图的 profile 归属（noder 已绑定原生函数查找，依赖数据需事先注册）；`position` 暴露上游位置对象，`diagnostic` 提供实例诊断收集，`target` 提供显式布局输入；`ir` 提供原生节点和实例构造入口，`typecheck` 提供原生 IR 检查与改写，`objw` 提供原生符号数据和重定位写入，`staticdata` 提供原生静态数据和函数值符号，`staticinit` 提供原生静态初始化调度及 map 初始化分离，`rttype` 提供目标运行时布局和原生写入游标，`reflectdata` 提供原生类型描述符、ITab、运行时类型引用、map 布局、GC 位图写入和相等函数选择，`bitvec/typebits` 提供原生位图及类型指针槽扫描，`types` 提供 Universe 和包符号查找，`gcbridge` 共用位置及 Type/Pkg/Sym 对象。见 [独立 consumer 示例](testdata/consumer/consumer_test.go)。每次编译需独立创建位置表和 base；XPos 必须用原表解码。同一实例的可变操作由调用方串行。

后端当前可直接使用实例中的上游 SSA 类型和构造函数：

```go
layout, _ := target.NewLayout("linux", "amd64")
u, _ := types.NewUniverse(layout)
value, _ := u.Scalar(types.Int)
pair := u.NewTuple(value, u.TypeMem)
results := u.NewResults([]*types.Type{u.TypeMem})
// pair.IsMemory() == true；results == u.TypeResultMem。
```

`NewTuple` 和 `NewResults` 保留上游返回 `*Type` 的签名；跨 Universe 或 nil 元素会 panic。`NewResults` 按上游语义保留输入 slice，调用后不能将其修改为外来类型。SSA 单例字段也应当作只读绑定使用。以上接口尚不生成 SSA 函数图。

复合类型也直接使用上游入口：`u.NewPtr(value)`、`u.NewSlice(value)`、`u.NewArray(value, n)`、`u.NewMap(key, value)`、`u.NewChan(value, types.SendRecv)`；通过 `Elem`、`Key`、`NumElem`、`ChanDir` 等方法读取。`u.NewPtrCacheEnabled` 只控制当前实例。指针构造和下述 CalcSize 布局均使用实例目标宽度。

未定类型使用上游名字：`u.UntypedInt`、`u.UntypedRune`、`u.UntypedString` 等；常量映射为 `u.UntypedTypes[constant.Int]` 和 `u.DefaultKinds[constant.Int]`。`Type.IsUntyped` 等分类方法直接复用上游实现，预声明类型由下述 InitTypes 初始化；源码类型检查使用独立的 types2 入口。

字段和签名使用 `u.NewField(pos, sym, typ)`、`u.NewStruct(fields)`、`u.NewSignature(recv, params, results)`。通过 `FieldType`、`Params`、`Results`、`ResultsTuple` 等原生访问器消费，公开 `types.Field` 与 `gcbridge.Field` 是同一对象类型。字段偏移和结构体布局由下述 CalcSize 计算。

类型布局入口示例见 [独立消费者测试](testdata/consumer/consumer_test.go)。先创建 Universe，再调用原生 `u.InitTypes(defTypeName)`；回调为传入的 Sym/Type 提供宿主的 `types.Object`。随后可直接调用 `Type.Size/Alignment/Registers/HasPointers` 与 `u.CalcSize/PtrDataSize/AlgType/Identical`。当前不提供假 `ir.Name`；宿主绑定契约及失败处理见 M2 文档。

源码解析入口及可运行示例见 [syntax](syntax/syntax.go) 和 [M3 记录](docs/m3-syntax.md)。`syntax.Parse` 后可调用 `types2.Config.Check` 获得类型检查结果，示例见 [types2](types2/example_test.go)。检查成功后，可按 [noder 示例](noder/example_test.go)调用 RewriteRangeFuncs，再写出原生 Unified IR stub，并通过 ReadLocalPackage 读取为原生 ir.Package；之后可调用 Interleaved 完成整包优化，再调用 MakeWrappers 生成原生方法包装函数，随后逐函数调用 loopvar.ForCapture 处理循环变量捕获，再调用 pkginit.MakeTask 生成原生初始化任务，随后调用 ssagen 的 SymABIs.GenABIWrappers；完整 unified 尾部、实验性 midway 和 compiler directives 尚未接入。尚无 SSA 构造入口。
