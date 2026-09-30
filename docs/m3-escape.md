# M3 原生 escape

本批整体迁入 Go 1.27.1 escape 的十个生产文件：alias、assign、call、escape、expr、graph、leaks、solve、stmt 和 utils。保留上游目录、文件、私有批次/位置/数据流结构以及原生算法，没有新增逃逸 IR 或自定义摘要格式。

执行顺序继续使用原生流程：pkginit → ABI wrappers → deadlocals → escape。后续 slice、walk、SSA 和上游编译器回接尚未完成。

## 对外能力和状态

```go
analysis := escape.NewContext(reader.Typecheck, reader.Inline.Log)
analysis.Funcs(reader.Typecheck.Target.Funcs)
```

Funcs 按原生调用图的自底向上批次分析，写入原生节点 Esc、参数 Note、闭包捕获 Byval/Addrtaken 和分配表达式的改写结果。调用一次；对已经分类的函数重新运行会保留上游的失败行为。失败不承诺回滚已修改的图。同一图由调用方串行操作，独立编译图可以并发。

Context 绑定现有 typecheck 与优化日志实例，将原生 leakTagCache 放在实例内。原生 ReassignOracle 缓存仍在 Funcs 调用内，batch、escape、location、hole、aliasAnalysis 和 leaks 保留原生布局；依赖外部状态的私有 receiver 方法由工具显式传入 Context，纯方法保持原签名。原生 Batch 作为生成的私有 context 方法由 Funcs 驱动，不公开需要外部提供内部 oracle 缓存的入口。

- Flag、Debug、Pos、Ctxt、类型表、目标指针宽度、栈大小阈值及 LiteralAllocHash 使用对应图的输入。
- 诊断写入 IR.Diagnostics，调试输出写入 IR.TraceWriter，优化事件写入现有 logopt.Context；没有进程退出、隐式文件输出或全局日志。
- IR.EscFmt 绑定原生无状态 Fmt。原生逃逸格式和参数摘要编码保持。
- Funcs 在修改前拒绝其他实例的函数；HeapAllocReason 拒绝外来节点，同时保留原生合法的 nil 输入。ParseLeaks 暴露原生摘要读取能力，返回的仍是上游数组类型。
- RuntimeFreegc 由该图的 Types.Experiment.RuntimeFreegc 控制；别名分析及 AppendNoAlias 标记算法也来自原生文件。本阶段只产生 IR 元数据，不实现对应实验的运行时或机器码支持。

公开 noder 示例和独立模块消费者已运行到本阶段；消费者检查返回局部变量地址导致的 EscHeap 分类。

## 迁移机制

迁移规则位于 `internal/port/rules_escape.go`，适配层位于 `internal/escape/adapter_context.go`。工具版本为 `syncgo-m3-v17`，共 268 个迁入文件。

这次补齐两种已有替换机制的适用范围，没有新增配置文件：

1. ContextParams 允许显式指定私有 receiver 类型的导出方法，用于原生 leaks 数组的 AddHeap、Encode 等方法；通配符仍只选择私有方法，公开类型的导出方法不能这样改变签名。方法值和方法表达式仍经过类型信息绑定，回调签名保持。
2. StateRule 可以把上游字面量布尔构建常量绑定到实例 bool 字段。RuntimeFreegc 的来源是固定版本的实验关闭构建文件；使用原始常量对象身份替换其引用，校验目标字段类型，不做字符串替换。非布尔常量不在这项能力范围内。

工具回归先复现旧实现拒绝私有数组导出方法的失败，再验证实例选项、方法值/表达式、缓存并发隔离及无效规则拒绝。

## 验证

源码测试覆盖栈/堆、参数返回与堆摘要、互递归保守摘要、闭包按值/引用捕获、defer/go、聚合对象、常量长度分配改写、只读与可变 string→[]byte 转换、外部函数和 Noescape。另验证摘要编解码/优化/饱和边界、非法深度、缓存隔离、八实例并发、旧图保留、实例诊断和输出、外来函数拒绝，以及无别名与存在别名的 append 区分。

Go 1.27.1 定向原生差分的十组配置已通过：386、amd64、mips、ppc64、amd64-nocopy、amd64-diagnostics、amd64-runtime、amd64-freegc、amd64-inline、amd64-inline-runtime（58.65s，`_work/m3-escape-probe-final.log`）。比较完整 IR、参数 Note、捕获和别名标记、局部临时状态清理、诊断、嵌套优化日志和最终位置状态。这是 IR/元数据差分，不是目标架构机器码执行。

参考测试使用固定 Git 对象和临时 overlay。为观察原生立即刷出的诊断，在 FlushErrors 入口加入仅复制缓冲内容的测试观察调用，保留排序、打印、清空、计数和分析流程。参考优化日志由测试访问器读取。freegc 配置仅在临时参考源码中把原生实验门控设为 true，以比较该分支；完整算法不改写，也不据此宣称实验运行时验证通过。上游工作树不被修改。

内联用例要求实际产生 OINLMARK，并检查被内联堆分配的诊断。差分先复现适配层将警告指到被内联函数行的问题（`_work/m3-escape-inline-baseline.log`），随后按原生 FmtPos 路径使用 OutermostPos；warning/error 均验证指向调用点。

完整 `go test -race ./...` 通过，internal/port 用时 54.882s，日志为 `_work/m3-escape-unit-final.log`。完整双版本差分与升级测试通过（测试 844.72s，包 846.431s），日志为 `_work/m3-escape-source.log`；两版的十组 escape 配置均通过。同一套选择/实例规则完成 Go 1.27.0 → 1.27.1 升级，两个生成 Go 文件变化，上游工作树保持不变。generate/check 均验证 268 个文件；额外逐文件比较确认 256 个既有生成 Go 文件除工具版本头外完全相同。

下一阶段接入原生 slice，之后是 walk 和 SSA 构造；当前尚无 Go 源码到 SSA 的 Build API，也未实现替换上游编译器。
