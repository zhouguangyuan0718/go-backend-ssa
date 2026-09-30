# M3：原生 B.Loop 保活与覆盖率修正

完整迁入上游 `bloop/bloop.go` 和 `coverage/cover.go`，提供 `bloop.NewContext(tc).Walk(tc.Target)` 与 `coverage.NewContext(tc).Fixup()`。这两个阶段消费已检查的 Unified IR，在整包内联之前执行；覆盖率修正还必须早于包初始化任务生成。公开源码示例已按此顺序接入。

## B.Loop

保留原生 `testing.(*B).Loop` 识别、表达式寻址、空白赋值临时变量、多返回值与实参保活，以及嵌套语句遍历。生成的 `runtime.KeepAlive` 调用保留 `IsCompilerVarLive` 和 `NoInline`，临时声明仍位于循环体，供后续逃逸分析使用。

原生值接收者 `editor` 持有实例 Context；类型、临时变量与诊断均归属于输入 typecheck/IR 实例。入口在改写前校验目标包身份与全部函数归属。阶段的 AstDump 调用现已接入[完整原生 IR 文本和 HTML 报告](m3-ir-reports.md)，由宿主提供输出流。

## 覆盖率

`coverage.Config` 是原生 `covcmd.CoverFixupConfig` 的公开别名，设置于 `tc.IR.Base.Flag.Cfg.CoverageInfo`。配置来自覆盖率插桩后的源码及其配置文件；此阶段自身不负责源码插桩。

原生算法完整保留：元数据只读标记、包 ID 与计数器符号分类、模式和粒度验证、元数据哈希解析、`runtime.addCovMeta` 注册，以及 main 包尾部的 `runtime/coverage.initHook` 调用。固定 runtime 包 ID 的查表算法与原始表来自同一固定源码，表局部化为函数内常量数据，无全局可变注册表。

覆盖率入口在任何变更前校验全部外部变量与函数归属。原生坏配置诊断通过实例 panic 传播；未设置配置时保持原生无操作行为。两个 Context 均与同一编译实例串行使用，不可对已经改写的 IR 重复运行；独立编译实例可并发。

## 验证

- 八个实例并发编译包含真实 `testing` 依赖归档的源码，覆盖赋值、空白赋值、多返回值、变参、if/for/range/switch/select；核对保活调用、类型检查状态和普通循环/循环外调用不被改写。依赖归档是测试定义的最小 B 类型，验证识别及 IR 算法，不代表真实基准计时验收。
- 三种包路径、五种模式、两种粒度，共 30 组覆盖率实例验证符号分类、元数据参数、runtime 固定 ID、main 初始化钩子与顺序。
- 验证无配置、坏模式/粒度/哈希以及外来函数和变量的提前拒绝。
- Go 1.27.1 / Go 1.27.0 各六组完整原生快照均一致：普通包、main、testmain、runtime、atomic/perfunc、未开启覆盖率。对照包括完整 IR、保活标志、局部变量、符号分类、初始化调用与语法报告。
- 全量 `go test -race -timeout 30m ./...` 通过；额外的覆盖率外来图提前拒绝回归通过；`syncgo check` 验证全部 468 个生成文件。

统一包编译驱动、源码 Build API、真实链接执行及工具链替换/自举仍待完成。
