# M3 SSA 原生诊断、格式化与存活性

迁入完整 `print.go`，以及 `findlive / ReachableBlocks / liveValues` 的原生实现。Value 的 Aux 查询、LongString、LongHTML，Block 的长格式和 HTML，以及 Func 的文本/HTML、哈希、崩溃诊断均已接通。

`Func.Fatalf` 保留原生顺序：启用日志时打印 pass 和函数；存在 HTMLWriter 时写入并刷新崩溃阶段；最后调用前端 Fatalf。`Block.Fatalf` 委托此路径；`Value.Fatalf` 按上游实现直接调用前端，不额外触发函数或 HTML 转储。

## 元数据与宿主操作

不可变 opcode 标量查询现在支持同包跨文件消费者。转换器继续从经过类型检查的原生表提取列，审计全部所选文件中的写入和逃逸，然后生成无可变全局状态的查询。Value 使用 auxType 列，存活性分析使用 call、hasSideEffects、nilCheck 列。格式化不因查询这些列而分配整份 opcode 表。

HTML 的原生 DOT/SVG 算法完整保留：生成 DOT、管道输入、启动/等待渲染器、错误处理以及 SVG 属性注入。`exec.Command` 通过类型身份绑定到已有 Types 编译实例中的命令提供者；诊断输出也写入该实例的 TraceWriter。Context 未配置命令提供者而主动请求 CFG 渲染时，明确拒绝；不隐式使用进程环境寻找或启动命令。

生成器的 `HostResourceRule` 当前限定 `os/exec.Command`。它验证实例方法与原生函数签名相同，仅接受直接绑定到局部变量的资源结果，审计每次引用，且只允许规则列出的字段和直接方法调用。重新赋值、别名逃逸、方法值、其他成员、直接资源字面量及外来资源操作均被拒绝；其余环境审计照常生效。

已有 Types 的编译归属承担诊断状态，无额外 Config/Func/Value 所有者字段。HTML 的 nil 接收者及禁用/损坏渲染器的原生提前返回保持可用。

## 验证

`TestPinnedSSADiagnostics` 在 Go 1.27.1 和 Go 1.27.0、386/amd64/mips/ppc64 四种布局上对照原生输出：

- 可达/不可达分支、Phi 的不可达前驱、调用和 nil check 的存活根、无用循环依赖、调度前后及寄存器分配后的存活性和打印。
- 函数文本、HTML、hash/rewriteHash、局部名称、寄存器位置、每类 Aux 的格式化、无 Block 的 Value 和 nil Value。
- HTML 起止、源码/AST 列、相同阶段合并、修改后的刷新、函数崩溃和 Value 直接报错路径。
- 使用确定性可执行渲染器的实际 DOT 输入、子进程调用和 SVG 输出。

原生差分中的 Aux 探针是格式化输入，部分没有完整指令参数，不代表通过 SSA verifier。目标布局在宿主进程中模拟，不能当作目标机器码执行验证。

`TestNativeDiagnosticOrderingAndIsolation` 并发处理八张独立 64 位图，保留并随后重新打印 32 位旧图，检查类型、缓存和输出隔离。打印会使用图的 Cache，因此并发测试不在多个线程中同时打印同一张图。

`TestNativeSVGResourceBinding` 使用真实管道和子进程覆盖成功、StdinPipe 失败、启动失败、非零退出、畸形 SVG、禁用阶段和损坏状态。另检查 HTML 写入失败和 Fatalf 位置。当前环境未安装 Graphviz；上述测试验证渲染器协议及原生管线，不声称实际 Graphviz 渲染质量已经验证。

转换规则自身包含跨文件查询、求值次数、跨文件写入拒绝和宿主资源绑定/拒绝测试。全仓库 race 测试及生成一致性检查同时执行。

## 尚未迁入的部分

`NewHTMLWriter / newDotWriter / passIdxByName` 尚未迁入：原生构造器需要完整 pass 表，以及文件创建、工作目录和可执行路径查找绑定。本阶段迁入了构造后使用的原生 HTML 方法，包括完整 Func.Fatalf 依赖，没有引入替代构造器。`deadcode.go` 当前仅包含打印所需的三个分析函数，不包含 deadcode 改写阶段。源码到 SSA、完整优化流水线和编译器回接仍未完成。
