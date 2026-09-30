# 逐步细化与实施

状态：M0 已建立；M1 的审计工具、最小生成闭环和小切片升级演练已实现，见 [M1 记录](m1.md)；M2 第一批（多文件移植、位置/诊断/目标输入、位置桥接）已实现，详见 [M2 记录](m2.md)；M2 第二至七批已实现 [真实结构、Universe、SSA 身份、复合类型/签名、原生布局/预声明类型及未定类型状态解耦](m2-universe.md)，M3 已完成 [源码到原生 AST 的首批接口](m3-syntax.md) 、[原生 types2 类型检查](m3-types2.md) 、[Unified IR 编码基础](m3-unified.md) 、[noder/原生后端共享数据声明](m3-objects.md) 、[noder 位置映射](m3-noder.md) 、[原生 Unified IR writer](m3-writer.md) 、[原生 rangefunc 重写](m3-rangefunc.md) 、[原生符号/内联位置](m3-link.md) 、[原生 IR 图](m3-ir.md) 、[原生 typecheck universe](m3-typecheck.md) 、[原生 runtime 声明](m3-runtime.md) 、[原生 IR 检查](m3-typecheck-ir.md) 、[reader 记录解码](m3-reader-metadata.md) 、[对象数据写入](m3-object-data.md)、[原生类型描述符与 ITab](m3-descriptors.md)、[reader 字典与共享状态](m3-reader-dictionaries.md) 、[原生类型、对象与函数体读取](m3-reader-bodies.md) 、[原生静态初始化调度](m3-staticinit.md) 、[原生调用图基础](m3-callgraph.md) 、[原生内联启发式](m3-inline-heuristics.md) 、[原生可内联判定/函数体驱动](m3-native-inlining.md) 、[原生包注册/读取](m3-package-reader.md) 、[原生函数符号/DWARF 内联修正](m3-text-symbols.md) 、[原生内联调用展开](m3-inline-expansion.md) 、[原生去虚化/整包内联](m3-interleaved.md) 、[原生方法包装函数](m3-wrappers.md) 、[原生循环变量捕获](m3-loopvar.md) 、[原生包初始化任务](m3-pkginit.md) 、[原生 ABI wrappers](m3-abi-wrappers.md) 、[原生 deadlocals](m3-deadlocals.md) 、[原生 escape](m3-escape.md) 、[原生 slice 优化](m3-slice.md) 、[walk 所需的原生 CanSSA](m3-canssa.md) 、[原生 compare](m3-compare.md) 和 [walk 架构输入](m3-walk-target.md)、[局部零值调度器绑定](m3-zero-schedule.md)、[完整 walk](m3-walk.md)、[ABI 参数布局](m3-abi-layout.md)、[SSA 类型和位置](m3-ssa-types.md)、[SSA 汇编编号](m3-ssa-objects.md)、[SSA opcode 元数据](m3-ssa-opcodes.md)、[公开 SSA opcode](m3-ssa-public-opcodes.md)、[原生 SSA 图](m3-ssa-graph.md)、[SSA 稀疏容器和缓存池](m3-ssa-containers.md)、[SSA 派生切片分配器](m3-ssa-derived-allocators.md)、[SSA 支配树](m3-ssa-dominators.md)、[SSA 调用辅助信息](m3-ssa-aux.md)、[SSA 数值辅助操作](m3-ssa-numeric.md)、[SSA 原生诊断](m3-ssa-diagnostics.md)、[SSA 值改写和 Copy/Phi 清理](m3-ssa-mutations.md)、[SSA 死代码和语句位置](m3-ssa-deadcode.md)、[SSA 完整校验和循环分析](m3-ssa-check.md)、[SSA 配置和架构 lowering](m3-ssa-lowering.md)、[SSA 通用重写和复合值拆分](m3-ssa-generic.md)；完整 noder reader/IR 统一驱动及原生编译器回接尚未实现。函数级原生 IR→SSA 及 AMD64 指令链生成已接通，当前验收范围见 [完整 IR 到 SSA](m3-ssagen-build.md) 、[AMD64 指令生成](m3-ssagen-emission.md) 、[完整函数 Flush](m3-native-flush.md) 、[原生对象序列化](m3-native-object.md) 和 [原生导出与归档](m3-native-archive.md)。下表列出阶段目标，不能将某一切片通过扩大为完整编译器能力。

| 阶段 | 交付 | 验收门槛 |
| --- | --- | --- |
| M0 整体设计 | 架构、状态归属、移植流程、API、pin | 设计相互一致，明确未实现能力 |
| M1 源码审计与移植工具 | Go module、cmd/syncgo inspect；候选依赖图；全部可达状态清单；最小生成闭环 | 精确 pin；错误版本拒绝；同输入生成相同；不修改来源工作树；残留 internal import 可检测 |
| M2 基础设施与接口细化 | TargetLayout、Universe、符号、位置、诊断、包/函数 Context；Aux 公共可用性设计 | 32/64 位类型布局隔离；同包路径不同源码隔离；实例回调无全局装配；规则的函数值/闭包场景验证 |
| M3 源码到 Built SSA | syntax、types2、noder、必要 IR/typecheck/walk/escape 路径、ssagen 构造 | 整数/布尔、分支、循环、Phi、局部 load/store、普通函数；语义不支持明确报错；旧图保活；并发 Build |
| M4 包导入与实例化 | Unified IR artifact 生产/消费、包初始化、跨包调用、泛型最小闭环 | 两包往返；命名类型身份、泛型函数体和字典信息完整；不兼容 artifact 拒绝；失败恢复 |
| M5 EarlyOptimized | 前端内联/去虚拟化等集成；SSA 指定 pass 配置；完整阶段边界 | 与固定上游同阶段比较；不能跳过 necessary cleanup；内存链/Phi 验证；小程序可执行差分 |
| M6 语言与后端元数据完善 | 闭包、接口、map/channel、go、defer/panic/recover、静态数据、runtime 类型等 | 每项能力单独端到端回归；工作队列直到稳定；后端消费 Aux 不导入 internal |
| M7 发布准备 | 文档示例、完整能力矩阵、性能/内存基线、Go 1.27 patch 升级流程 | 支持目标矩阵、全状态审计、race、跨版本再生成与差分通过 |

新增并列验收目标：[替换官方编译器对应实现并回接原生后端](go-compiler-integration.md)。M2 加入核心类型/IR/符号的宿主消费者审计及最小桥接试验；M6 完成驱动/原生后端回接；M7 纳入替换后的工具链自举、标准库重建和回归。该目标不能只用自定义后端测试代替。

M3 的前端依赖代码可能已经包含泛型或接口实现，但这不代表这些能力已经验证。能力声明只随相应验收用例开放。逃逸分析等影响语义正确性的阶段不能为缩小范围而无条件跳过。

## 首个实现批次：M1

本批已完成工具实现与首批切片；完整类型 Universe、Build 与原生回接仍在后续阶段。

1. 确定本地 module 路径和 Go 1.27.1 构建工具链，不复用当前 GoALLC rc2 编译器作为正式验证工具。
2. 从固定 Git 对象读取 syntax/types2/noder/ir/types/typecheck/ssagen/ssa 的候选源码及递归依赖，覆盖后续前端算法包。
3. 实现声明和状态 inventory：身份、读写、初始化依赖、环境调用、callback、unsafe/linkname、构建条件。
4. 明确人工维护与移植声明的所有权，按单个包形成可编译的小闭环。
5. 实现 import 重定位和一条 Context 结构转换，覆盖调用、函数值和闭包；未知形式报错。
6. 在 Go 1.27.0 / 1.27.1 上对这个小切片演练升级，评估规则维护量，再扩展。

“状态清单完整”指所选生产依赖闭包，并非只 rg 一次 `var`。某些构建条件导致的路径在扩大目标范围时必须重新审计。

## 验证矩阵

| 维度 | 必须验证的场景 |
| --- | --- |
| 调用隔离 | A→B→A；相同 import path 不同源码；先失败后成功；先取消后成功 |
| 并发 | 多个 Build 交错；amd64/arm64；加入 386 暴露指针宽度缓存；race detector |
| 生命周期 | 同包多个函数；多次后续 Build 后旧函数/类型/符号仍有效；无需 Reset |
| 源语言 | 用特性矩阵逐项覆盖语法、类型规则、泛型、闭包、初始化及运行时构造 |
| SSA | verifier、Phi 前驱、memory chain、Aux 类型、调用签名、不可达块、阶段不变量 |
| 后端消费 | 独立测试 module 仅使用公共包；一个测试后端或受支持子集解释器验证执行结果 |
| 可复现移植 | clean generate/check；规则失配报错；生成器不覆盖手写文件 |
| 语义差分 | 固定输入对比锁定版本官方前端/编译器；规范化差异与语义差异区分 |
| 输出确定性 | 多次调用的规范化图、artifact 和诊断排序；固定依赖内容 |

amd64/arm64 都是 64 位，不能单独证明目标布局隔离。386 首先作为布局和状态压力测试目标，不自动承诺其全部 runtime 特性支持。

Built 阶段未必可用官方 GOSSAFUNC dump 精确截取；测试可以在临时、固定提交的参考编译器中增加只读观测 hook。观测补丁必须独立于库转换规则。文本图相同仅是辅助，执行结果及结构不变量才是语义依据。

测试后端只作为验证依赖，不进入核心库生产路径；没有完成执行验证时，不把 verifier 通过表述为语义正确。

## 需要在后续细化中解决的事项

| 事项 | 决策阶段 | 当前默认 |
| --- | --- | --- |
| 正式 module 发布路径 | M1 / 发布前 | 可先本地 go-backend-ssa，不假设 GitHub 组织 |
| Type 的 Universe 引用形式 | M2 | 独立所有权，保留上游算法，试验后定字段/参数 |
| Aux 和 local slot 的公开表示 | M2 | 每种可达信息完整、无需导入 internal |
| goobj/LSym 依赖的最小兼容面 | M1–M4 | 按声明闭包适配，不能空 stub |
| artifact 与官方 .a 的互操作 | M4 以后 | 本库严格版本匹配的 artifact 优先 |
| 首个可执行验证后端 | M3–M5 | 小范围验证，不绑定核心库到 LLVM |
| open defer、GC、ABI 完整元数据 | M5–M6 | 明确记录 Go runtime 约定，逐项实现 |
| PGO/cgo/汇编/实验开关 | M7 之后另立范围 | 完整流程当前拒绝，不静默降级；原生 profile/调用图已作为内联依赖迁入 |

技术细节通过源码审计和原型推进，不把每个实现选择都变成用户批准门槛。发布、远端仓库等操作按后续实际指令处理。

本批已迁入原生 SSA 的 CSE、prove、SCCP、已知位、分支/短路、内存优化和 CPU 特征推导，并验证目标 lowering 前后的图及独立语义。见 [SSA 优化阶段](m3-ssa-optimizer.md)。调用展开、写屏障、循环抢占检查和原生临时变量也已迁入，见 [SSA 调用和内存阶段](m3-ssa-calls.md)。后期优化、调度及寄存器/栈槽分配已迁入，见 [SSA 调度和分配](m3-ssa-allocation.md)。阶段表、原生选项和顺序校验已实例化，见 [SSA 阶段配置](m3-ssa-phases.md)。完整原生 Compile 及计时、统计、转储服务已接通，见 [SSA Compile](m3-ssa-compile.md)。原生 ssafn 前端和目标/运行时初始化已接通，见 [SSA 前端与初始化](m3-ssagen-frontend.md)。原生 AllocFrame、局部变量合并及其活跃区间分析已接通，见 [栈帧分配](m3-ssagen-frames.md)。原生 HTML 构造、阶段选择和源码列已接通，见 [源码诊断](m3-ssa-sources.md)。完整原生 IR 到 SSA 构造、Phi、intrinsics 和 SIMD builder 已接通，见 [完整 IR 到 SSA](m3-ssagen-build.md)。AMD64 原生 genssa、GC/参数活跃性、变量位置表及普通/SIMD 指令生成已接通，见 [AMD64 指令生成](m3-ssagen-emission.md)。原生 x86 汇编器及机器码/重定位差分已接通，见 [原生 x86 汇编器](m3-native-assembler.md)。完整 PCLN/DWARF 与生产 Flush 已接通，见 [原生函数 Flush](m3-native-flush.md)。统一驱动及替换/自举仍需后续验收。

原生对象输出已接通符号编号、二进制编码及函数辅助数据收尾，见 [本阶段记录](m3-native-object.md)。完整包驱动、对象链接、程序执行和工具链替换仍待验证；当前的逐字节差分不能代替这些验收。

原生最终 Unified 导出和 `.a` 容器已迁入，见 [导出与归档记录](m3-native-archive.md)。后续重点转向完整包编译驱动及固定点收尾，完成真实链接/执行后继续第二目标后端和替换工具链验收。

原生函数准备、完整 `ssagen.Compile` 与多 worker 队列已接通，见 [函数编译队列](m3-native-compile-queue.md)。后续包级驱动仍须按上游顺序迭代类型、全局声明和新函数，直至固定点，再执行完整数据收尾。

完整原生包数据收尾及文件嵌入已接通，见 [包数据收尾](m3-native-package-data.md)。后续继续迁入包级类型/全局/函数固定点和源码驱动，再进行真实链接执行及替换验收。

运行时类型、全局声明、新增函数和 DWARF 的原生固定点已接通，见 [声明固定点](m3-native-declaration-fixed-point.md)。后续重心转向 noder 源码加载、完整前端驱动与真实链接执行。

原生编译器指令已贯通源码 parser、Unified writer 与 reader，见 [编译器指令](m3-native-directives.md)。下一步接通原生 `checkFiles`、显式依赖导入及完整源码加载驱动。
