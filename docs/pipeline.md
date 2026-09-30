# Go 1.27 编译流程与移植范围

下表按官方 go1.27.1 源码划分。它是语义阶段图，不是可以任意重排的函数列表。初始化和阶段顺序需要用锁定源码的调用链验证。

| 阶段 | 上游核心包 | 库化工作 |
| --- | --- | --- |
| 目标与 Universe 初始化 | base、types、typecheck、rttype、ssagen | 实例化目标布局、builtin/runtime 类型和 intrinsic 表，保留初始化依赖 |
| 扫描、解析、AST | syntax | 接受显式文件内容与位置，移植编译器自身 AST |
| 源码类型检查 | types2、noder | 独立 checker Context、导入器、目标 Sizes、语言版本 |
| 特定语法展开 | rangefunc 等及 noder 调用链 | 依照 1.27 实际路径纳入，不能只列传统 parse/typecheck |
| Unified IR 读写及 compiler IR | noder、ir、types、internal/pkgbits | 实例化读写器、字典、实例化队列、符号和导入缓存 |
| IR 检查及构造辅助 | typecheck | 与 types2 区分；保留 compiler IR 约束和辅助构造 |
| 去虚拟化和内联 | interleaved、devirtualize、inline | 阶段上下文和实例级跨包回调；保留依赖的导入函数体 |
| 包装函数与捕获修正 | noder.MakeWrappers、loopvar 等 | 包装函数和合成函数加入本次包队列 |
| 初始化与调用约定辅助 | pkginit、ABI wrapper 相关逻辑 | 保留包初始化语义；ABI 相关部分逐项标注，不能整段删除 |
| 死局部变量、逃逸与其他前端优化 | deadlocals、escape、slice 等 | 分析缓存与当前函数实例化；保留必要的闭包/堆栈语义 |
| walk | walk | 求值顺序、内建操作展开、runtime 调用、闭包等 |
| 函数与全局对象迭代生成 | gc 驱动、reflectdata、staticdata 等 | 本库重建工作队列，收集静态数据、类型元数据及新增函数 |
| SSA 构造 | ssagen | 指令、控制流、Phi、内存链；拆开构造与原生后端尾部 |
| 指定 SSA 优化 | ssa | 实例化选项，运行明确的阶段配置后返回 |

## 两套类型检查不是替代关系

`types2` 对 syntax AST 做源码级检查。`types` 是后续 compiler IR 使用的类型表示，`typecheck` 仍服务于 compiler IR 构造、检查和 lowering。不能只移植其中一个包，也不能将标准库 `go/types` 对象直接替换为 compiler `types.Type`。

## 包级工作队列

官方驱动在函数、全局声明、类型元数据之间循环，直到没有新增工作。生成闭包、方法包装函数、泛型实例、字典或类型描述时，可能产生新的函数或全局对象。

库保留这个语义闭环，维护本次调用的队列和去重表；每轮检查取消。不可对初始函数切片只遍历一次后返回。Dwarf/机器码输出导致的工作需要与语言语义要求分开审计。

## 两个 SSA 输出阶段

源码级 Build 已提供两个固定枚举：

| 阶段 | 截止点 | 用途 |
| --- | --- | --- |
| `Built` | SSA 构造及 Phi 补全、验证之后，进入 ssa.Compile 之前 | 第一条纵向闭环、差分基线 |
| `EarlyOptimized` | 官方 pass 顺序运行至 early fuse 之后、expand calls 之前 | 自定义后端首选的后续目标 |

Built 和 EarlyOptimized 的实现见 [M3](m3-source-build.md) 与 [M5](m5-early-optimized.md)。二者都不承诺“没有 ABI 信息”或“语言无关”；ssagen 在构造阶段已有 ABI 分析和 runtime lowering。EarlyOptimized 也并非完成了所有通用优化，后续仍有 sccp、dse 等。

保留阶段输入要求和输出不变量，禁止使用任意字符串组合 pass 的公共 API。每个配置有独立 verifier 和能力集合。

## ssagen 的拆分

上游 buildssa 并非纯 builder：insertPhis 后调用 ssa.Compile，随后进行 AllocFrame、open-defer 信息处理、参数 spill 等。

移植时抽出：

1. 基于 IR 构造完整 SSA。
2. 运行选定优化配置。
3. 收集后端需要的语义元数据。
4. 返回拥有独立存储的函数图。

原生寄存器分配、栈帧分配、genssa 和对象文件写出不属于 Build。open-coded defer、局部槽、返回值和 GC 所需信息不能随尾部删除；需要在 M5/M6 定义独立的元数据表示。

## 静态数据与 runtime

walk 后的 map/channel/interface/分配等通常已部分表示为 Go runtime 调用。库首先忠实表达 go1.27.1 的约定；采用不同 runtime 的后端必须明确转换能力。不能通过给所有 runtime 操作提供空 stub 来宣称语言特性已支持。

`obj.LSym` 等初期可以通过私有兼容实现承接，但其内容、重定位、属性和引用需要转为可公开读取的符号/数据描述。完整 linker/assembler 不作为依赖引入的默认答案。

后端所需完整输出见 [API 契约](api.md)。


Linux ARM64 现已接通同一源码阶段、原生续编译、汇编和对象写出流程，见 [M6 记录](m6-arm64.md)。Build 在两种完整目标上均保留原有阶段边界；原生指令生成与寄存器分配仍由显式 `compiler.Compile` 路径完成。
