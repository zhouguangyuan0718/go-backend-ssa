# go-backend-ssa

将 Go 1.27 官方编译器从源码解析到 SSA 的实现移植为可嵌入进程的 Go 库，供其他编译器后端直接消费。

库维护基础设施、公共数据模型、状态归属和生命周期；移植工具从固定上游提交抽取算法，通过可复现的结构改写接入这些设施。目标是每次编译拥有独立状态，并让统一 Build 的返回结果跨后续调用保活。当前低层 SSA 和指令接口使用原生 worker 缓存，须在同一 worker 复用前消费结果。

当前状态：**M1 已完成，M2 已完成原生类型布局与预声明类型初始化；M3 已接通源码解析、types2 类型检查、Unified IR writer/reader、前端分析和 walk、完整 IR 到 SSA，以及 AMD64 原生指令生成、x86 汇编、完整函数 Flush（PCLN/DWARF）、原生 Go 对象二进制序列化，以及最终 Unified 导出和包归档；原生函数编译入口、后台工作队列、声明固定点及完整包数据收尾已接通，原生编译器指令已贯通 parser 和 writer/reader，原生 types2 导入器已支持显式 Unified 依赖数据。统一源码 Build API、完整包编译驱动和上游工具链替换尚未完成。** 当前各阶段通过显式 Context 接入；源码测试夹具已串联这些阶段，不能据此视为完整编译器驱动。详细验收范围见下列阶段记录。

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
48. [M3 原生 slice 优化](docs/m3-slice.md)：切片独占性分析、原生堆转移节点及实例化依赖。
49. [M3 原生 CanSSA](docs/m3-canssa.md)：按输入类型所属目标进行 SSA 表示判定，供 walk 使用。
50. [M3 原生比较构造](docs/m3-compare.md)：完整 compare 算法、目标成本和有序比较 IR 构造。
51. [M3 walk 架构输入](docs/m3-walk-target.md)：原生索引/调用判定及按类型绑定架构字段。
52. [M3 局部零值调度器绑定](docs/m3-zero-schedule.md)：为原生 walk 的常量赋值保留零值行为。
53. [M3 原生 walk](docs/m3-walk.md)：完整排序/改写、实例缓存和公开 Walk 入口。
54. [M3 ABI 参数布局](docs/m3-abi-layout.md)：原生寄存器/栈分配、参数分解和实例合成类型缓存。
55. [M3 SSA 类型和位置](docs/m3-ssa-types.md)：原生 Types 的实例绑定及 Frontend、LocalSlot 等公开声明。
56. [M3 SSA 汇编编号](docs/m3-ssa-objects.md)：opcode/register 元数据依赖的各架构原生指令与寄存器常量。
57. [M3 SSA opcode 元数据](docs/m3-ssa-opcodes.md)：完整原生 opcode/register 表的实例隔离及无状态查询。
58. [M3 公开 SSA opcode](docs/m3-ssa-public-opcodes.md)：自动导出全部原生 opcode/BlockKind 常量和类型。
59. [M3 原生 SSA 图](docs/m3-ssa-graph.md)：完整图声明、原生基础建图方法和缓存生命周期。
60. [M3 SSA 稀疏容器和缓存池](docs/m3-ssa-containers.md)：原生泛型稀疏结构及按 Cache 隔离的基础分配器。
61. [M3 SSA 派生切片分配器](docs/m3-ssa-derived-allocators.md)：按类型审计原生切片重解释并保留完整分配算法。
62. [M3 SSA 支配树](docs/m3-ssa-dominators.md)：原生后序遍历、立即支配者、稀疏树和独立可达性验证。
63. [M3 SSA 调用辅助信息](docs/m3-ssa-aux.md)：原生调用 ABI/寄存器约束、Aux 转换和结果类型实例归属。
64. [M3 SSA 数值辅助操作](docs/m3-ssa-numeric.md)：原生 AuxInt 编码、浮点位模式、ARM 参数和算术标志位。
65. [M3 SSA 原生诊断](docs/m3-ssa-diagnostics.md)：存活性、文本/HTML、崩溃输出及实例绑定的 DOT/SVG 管线。
66. [M3 SSA 值改写](docs/m3-ssa-mutations.md)：完整 Value/Block、常量缓存、类型比较与原生 Copy/Phi 消除。
67. [M3 SSA 死代码清理](docs/m3-ssa-deadcode.md)：原生 numberLines/deadcode、语句位置迁移与死图回收。
68. [M3 SSA 完整校验](docs/m3-ssa-check.md)：原生 verifier、内存检查、循环嵌套与分支预测。
69. [M3 SSA 配置与 lowering](docs/m3-ssa-lowering.md)：完整原生架构配置、19 份重写文件、公共图构造与实例绑定。
70. [M3 SSA 通用重写与拆分](docs/m3-ssa-generic.md)：原生 generic/除法规则、复合值与软浮点拆分、SIMD 真值表及数值执行验证。
71. [M3 SSA 优化阶段](docs/m3-ssa-optimizer.md)：CSE、条件证明、常量传播、内存优化、CPU 特征与双版本图差分。
72. [M3 原生 SSA 调用和内存阶段](docs/m3-ssa-calls.md)：ABI 调用展开、写屏障、循环抢占检查及真实临时变量。
73. [M3 原生 SSA 调度和分配](docs/m3-ssa-allocation.md)：后期优化、调度、寄存器/栈槽分配及原生架构身份。
74. [M3 SSA 阶段配置与初始化](docs/m3-ssa-phases.md)：原生阶段表、选项解析、顺序校验及实例初始化分组。
75. [M3 完整原生 SSA Compile](docs/m3-ssa-compile.md)：完整阶段循环、统计/转储服务、公开入口及物理位置执行验证。
76. [M3 原生 SSA 前端与初始化](docs/m3-ssagen-frontend.md)：目标配置、运行时符号、defer 布局及原生 Frontend 服务。
77. [M3 原生栈帧分配](docs/m3-ssagen-frames.md)：局部变量活跃区间与合并、帧布局、栈偏移及实例隔离。
78. [M3 SSA 源码诊断](docs/m3-ssa-sources.md)：原生 HTML 构造、CFG 阶段选择、源码列及显式宿主输入。
79. [M3 完整 IR 到 SSA](docs/m3-ssagen-build.md)：原生构图、两套 Phi 算法、intrinsics/SIMD、完整 SSA 优化和帧元数据。
80. [M3 原生 AMD64 指令生成](docs/m3-ssagen-emission.md)：genssa、GC/参数活跃性、变量位置列表、普通和 SIMD 指令及实例格式化表。
81. [M3 原生 x86 汇编器](docs/m3-native-assembler.md)：普通/AVX/EVEX 编码、目标预处理、跳转布局及机器码/重定位差分。
82. [M3 原生函数 Flush](docs/m3-native-flush.md)：完整 Flush、PC 表、DWARF 变量/作用域/内联生成及实例编码器。
83. [M3 原生对象序列化](docs/m3-native-object.md)：符号编号、builtin ABI 表、原生二进制编码和显式输入/输出流。
84. [M3 原生导出与归档](docs/m3-native-archive.md)：最终 Unified bitstream、兼容性头、编译器/链接器归档成员及显式目标配置。
85. [M3 原生函数编译队列](docs/m3-native-compile-queue.md)：函数准备、闭包与多 worker 调度、完整 Compile、超大栈帧及最终重定位处理。
86. [M3 原生包数据收尾](docs/m3-native-package-data.md)：GC/插件表、文件嵌入、map 零值及函数辅助数据的原生收尾顺序。
87. [M3 原生声明固定点](docs/m3-native-declaration-fixed-point.md)：运行时类型、全局声明、新函数和 DWARF 收尾之间的完整原生迭代。
88. [M3 原生编译器指令](docs/m3-native-directives.md)：原生 pragma、linkname/cgo/embed/wasm 解析与文件记录、并发源码输入及 writer/reader 贯通。
89. [M3 原生类型导入器](docs/m3-native-type-importer.md)：Unified 导出数据的惰性类型加载、泛型/别名/方法字典和类型环境归属。
90. [M3 原生归档输入](docs/m3-native-archive-input.md)：显式可定位输入流、原生缓冲读取、归档格式校验与大小受限读取。
91. [M3 原生依赖导入](docs/m3-native-dependency-imports.md)：noder 路径查找、对象头匹配、types2/IR 同时导入和链接指纹。
92. [M3 原生 midway](docs/m3-native-midway.md)：SIMD 依赖分析、目标宽度专门化、语法深复制与分派生成。
93. [M3 原生源码前端](docs/m3-native-source-frontend.md)：checkFiles、midway 二次检查、AST 诊断及完整 unified 构建。
94. [M3 原生 B.Loop 与覆盖率](docs/m3-prepackage.md)：基准循环保活、覆盖率变量分类和初始化注册。
95. [M3 完整原生 IR 诊断](docs/m3-ir-reports.md)：文本追加、HTML 阶段报告、实例输出提供器与关闭流程。
96. [M3 原生包前端顺序](docs/m3-native-frontend-pipeline.md)：完整 Main 前端区间、独立阶段抽取及同实例编译入口。

## 当前可运行的工具

本地 module 路径为 `go-backend-ssa`，工具链为 Go 1.27.1。维护命令读取固定 Git 对象，不修改来源 Go 工作树；日常测试不需要来源仓库或网络。

```sh
export GOTOOLCHAIN=go1.27.1
export GO_SSA_SOURCE_REPO=/path/to/local/golang-go

go run ./cmd/syncgo inspect -repo "$GO_SSA_SOURCE_REPO" -out _work/inspection.json -summary _work/inspection-summary.json
go run ./cmd/syncgo generate -repo "$GO_SSA_SOURCE_REPO"
go run ./cmd/syncgo check -repo "$GO_SSA_SOURCE_REPO"
go test -race -timeout 30m ./...
```

上游版本和移植规则统一维护在 [internal/port/rules.go](internal/port/rules.go)，不需要 JSON 配置。审计报告按需生成到 `_work/`；`sync/generated.json` 仅由工具自动维护。

设定 `GO_SSA_SOURCE_REPO` 时，测试额外验证官方 go1.27.1 再生成、两版上游位置/符号回归与 SSA 类型切片差分测试、源码树未变以及 go1.27.0 → go1.27.1 升级；仓库需包含两个 tag。完整双版本差分包含多组独立原生编译，使用 `-timeout 30m`，避免默认 10 分钟时限。无该变量时仅跳过这项集成测试，其他测试照常执行。

编译器源码从 `src/cmd/compile/` 以下保留目录、包名和文件名；辅助依赖分别归入 `internal/cmd/` 与 `internal/go/`。当前移植代码包括完整位置模块、完整前端语法解析器与 AST、完整 types2 checker、pkgbits codec、rangefunc 重写器和 noder writer，原生 IR 图构造与完整 typecheck 生产包，以及 types/objabi 的结构和算法切片；reader 的类型/对象实例化、函数体和字典读取主体已接入并完成源码差分；整包内联、方法包装、loopvar、pkginit、ABI wrappers、deadlocals、escape、slice 和完整 walk 已接通；源码经 ssagen 构造 SSA、机器码和原生编译队列已接通；原生手动图构造、架构配置及 `ssa.Compile` 完整阶段入口已公开。

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
| [internal/walk/walk.go](internal/walk/walk.go)、[order.go](internal/walk/order.go)、[switch.go](internal/walk/switch.go) | 完整原生 walk、实例状态及公开 walk.Context.Walk |

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

源码解析入口及可运行示例见 [syntax](syntax/syntax.go) 和 [M3 记录](docs/m3-syntax.md)。`syntax.Parse` 后可调用 `types2.Config.Check` 获得类型检查结果，示例见 [types2](types2/example_test.go)。低层逐阶段入口可在检查成功后调用 RewriteRangeFuncs，再写出原生 Unified IR stub，并通过 ReadLocalPackage 读取为原生 ir.Package；之后可调用 Interleaved 完成整包优化，再调用 MakeWrappers 生成原生方法包装函数，随后逐函数调用 loopvar.ForCapture 处理循环变量捕获，再调用 pkginit.MakeTask 生成原生初始化任务，随后调用 ssagen 的 SymABIs.GenABIWrappers，并依次执行 deadlocals、escape、slice 和 walk；含编译器指令的源码通过 ParseFile/WriteParsedPackage 保留原生文件记录，见 [编译器指令](docs/m3-native-directives.md)；`CompileParsedPackage` 已接通原生 checkFiles、midway 二次检查、rangefunc 与完整 unified，见 [源码前端](docs/m3-native-source-frontend.md)。完成目标、SSA、liveness 配置及 InitConfig/InitTables 后，可对完成 walk 的函数调用 ssagen.BuildSSA，得到原生优化、寄存器分配和帧布局后的 SSA，见 [完整 IR 到 SSA](docs/m3-ssagen-build.md)。AMD64 可在 InitConfig 前通过 amd64.NewContext 安装目标处理函数，然后用 objw.NewProgs 和 ssagen.GenSSA 输出原生指令链及 GC/调试元数据，见 [原生 AMD64 指令生成](docs/m3-ssagen-emission.md)。原生 x86 汇编器已迁入并通过机器码及重定位差分，见 [原生 x86 汇编器](docs/m3-native-assembler.md)。通过 dwarfgen.NewContext 绑定调试信息回调后，objw.Progs.Flush 已执行完整原生汇编、PC 表和 DWARF 生成，见 [原生函数 Flush](docs/m3-native-flush.md)。随后通过 gc.Context.AddGCLocals 完成函数辅助数据注册，并在其他包级数据就绪后调用 objw.Context.WriteObject 输出原生 Go 对象二进制载荷，见 [原生对象序列化](docs/m3-native-object.md)。符号编号后可通过 noder.Context.ExportUnified 输出最终导出 bitstream，或通过 gc.Context.WriteArchive 写出原生包归档，见 [原生导出与归档](docs/m3-native-archive.md)。通过 gc.Context.BindBackend、EnqueueFunc 和 CompileQueued 可运行完整原生函数准备与后台队列，见 [函数编译队列](docs/m3-native-compile-queue.md)。通过 gc.Context.DumpData 可执行完整原生包数据收尾，嵌入文件通过显式 provider 输入，见 [包数据收尾](docs/m3-native-package-data.md)。通过 gc.Context.CompileDeclarations 执行完整原生声明固定点，见 [声明固定点](docs/m3-native-declaration-fixed-point.md)。统一源码 Build 驱动、整包前端组装和上游工具链替换仍待完成。

显式依赖的最终 Unified 导出数据可通过 `importer.NewContext(checker).ReadExport` 加载，同一对象可用作 `types2.Config.Importer`。见 [原生类型导入器](docs/m3-native-type-importer.md)。

原生归档输入已迁入，支持显式内存流以及原生格式和截断错误处理，见 [归档输入](docs/m3-native-archive-input.md)。目标对象头匹配及 noder 依赖组装已接通，见 [依赖导入](docs/m3-native-dependency-imports.md)。通过 `BindImports` 配置后，noder Context 本身可作为 `types2.Config.Importer`；原生 `checkFiles` 与完整 unified 已通过 `CompileParsedPackage` 接通。
