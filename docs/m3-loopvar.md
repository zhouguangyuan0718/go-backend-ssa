# M3 原生循环变量捕获

本批迁入完整 `internal/loopvar/loopvar.go`，对外提供 `loopvar.Context.ForCapture` 和 `LogTransformations`。它使用原生 IR 的 DistinctVars、声明和闭包捕获信息，将需要分离的循环变量改写为每次迭代独立的变量。执行顺序保持在整包内联及方法包装之后、escape 分析之前。

当前尚未接通完整 escape/walk/SSA 和上游编译器替换。本批验证的是循环捕获 IR 变换及日志，不将测试提供的逃逸分类视为真实 escape 分析结果。

## 状态和适配

算法整个文件由 syncgo 迁入。原生 `VarAndLoop`、循环改写、声明搬移、continue 重定向和日志逻辑保持；辅助遍历仍为原生实现。

`loopvar.Context` 仅绑定现有 typecheck 图和 logopt 记录器。Ctxt、Pos、Debug、LoopVarHash、类型表和所有节点构造都来自该实例。原生函数内部的映射、序号和变换列表继续保持局部，不提升为共享缓存。没有增加新 IR 模型、配置文件或 Reset 流程。

入口检查函数和变换记录的图归属，拒绝另一个编译实例的节点。警告进入该图的 Diagnostic Collector，优化事件进入显式传入的 logopt Context，hash 调试使用既有 LinkContext 的实例 writer。独立图可并发；同一图按阶段串行变换。

## 对外顺序

```go
reader.Interleaved.DevirtualizeAndInlinePackage(target, profile)
reader.MakeWrappers(target)
capture := loopvar.NewContext(reader.Typecheck, reader.Inline.Log)
var transformed []loopvar.VarAndLoop
for _, fn := range target.Funcs {
    transformed = append(transformed, capture.ForCapture(fn)...)
}
// 后续接通原生 escape 分析后，再调用：
// capture.LogTransformations(transformed)
```

返回记录直接引用原生变量、循环和最后位置，应与所属图一同保留。LogTransformations 根据变量的最终 Esc 分类报告栈/堆分配，不能提前调用并将默认值视为 escape 分析结论。无需另建 noder 级封装；[公开示例](../noder/example_test.go)和[独立消费者](../testdata/consumer/consumer_test.go)均直接使用此包。

ForCapture 消耗原生 DistinctVars 标记，原生行为是再次执行不重复改写。它原地修改图，仍遵循现有低层 API 的编译器不变量 panic 约定，不承诺失败后任意复用中间图。

## 覆盖与差分

- 普通三段 for、无 post、无 cond、多初始化变量、嵌套循环；外层带标签 continue 和内层普通 continue 各自保留正确目标。
- slice、数组指针、map、channel、string range；地址捕获和闭包捕获，包括内联后的循环和嵌套闭包函数。
- 非声明式 range、无捕获循环、直接返回地址或闭包的路径按原生判定保留，避免无条件改写。
- 声明搬移后每个捕获变量只有一份声明；函数归属、改写标记、当前函数恢复和重复调用行为均检查。
- hash 的两组互补选择合起来覆盖基线；排除配置不改写，强制配置改写无逃逸候选的循环。调试输出只使用实例 writer。
- 日志测试显式设置 EscHeap/EscNone，分别覆盖堆/栈记录、循环范围记录、内联位置还原和静默强制配置。这仅验证报告接口。
- 八实例并发及旧图保留、外来函数/变换记录拒绝；独立 module 通过公共包检查真实源码的捕获改写结果。

上游差分比较完整 IR、声明/捕获归属、continue 标签、条件与 post 搬移、变换记录及末尾位置、hash trace、诊断和 logopt 事件。参考编译器只在临时 overlay 中增加两个只读观察函数，读取其已有诊断和事件缓冲，不替换算法或修改上游工作树。

新增九种配置：386、amd64、mips、ppc64、amd64-newinline、amd64-hash0、amd64-hash1、amd64-exclude、amd64-force。Go 1.27.1 和 Go 1.27.0 的新增差分全部通过。

`syncgo generate` / `check` 通过，共 254 个迁入文件；迁移引擎未修改，版本仍为 `syncgo-m3-v13`。`GOTOOLCHAIN=go1.27.1 go test -race ./...` 已通过（port 包 50.021 秒），日志见 `_work/m3-loopvar-unit.log`。

`GO_SSA_SOURCE_REPO=/Volumes/Disk1/00.Work/00.Code/goallc/go GOTOOLCHAIN=go1.27.1 go test -race -v ./internal/port -run '^TestPinnedUpstreamAndUpgrade$'` 已通过（测试 579.25 秒，包总计 580.795 秒）。完整差分覆盖 Go 1.27.1 和 Go 1.27.0；同一套选择与上下文规则从 1.27.0 升级到 1.27.1，仅两个 Go 输出文件变化，上游工作树保持原样。日志见 `_work/m3-loopvar-source.log`。

下一步接入原生包初始化任务和所需实例状态，再推进 ABI wrapper、deadlocals、escape、slice、walk 与 SSA，继续保持上游阶段顺序和算法。
