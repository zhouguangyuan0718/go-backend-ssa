# M3 原生内联调用展开

本篇记录该批提交时的边界；后续已接通 [原生去虚化与整包内联驱动](m3-interleaved.md)。

本批将已有可内联判定接通到上游真正的展开算法：`inline.TryInlineCall → mkinlcall → noder.unifiedInlineCall`。返回值是原生 `ir.InlinedCallExpr`，其局部声明、初始化语句、返回变量、闭包和内联位置信息均由迁入代码构造。本批不提供替代的整包内联循环；原生 interleaved 去虚化/内联驱动、wrapper 生成、walk 和 SSA 仍待接入。

## 实现与对外使用

- 迁入 `TryInlineCall`、`mkinlcall`、`CalleeEffects`、`unifiedInlineCall` 和 `expandInline`。保留原生调用点成本、递归限制、参数声明/赋值、结果延迟初始化、返回到标签的改写和调试声明构造。
- 上游包级 `InlineCall` 变为既有 `inline.Context` 字段。`noder.NewContext` 将其绑定到本实例的 unified reader，和已有 HaveInlineBody、PGO 查找/清理共用一个图。
- 展开使用已有实例 `inlgen`、reader 队列、类型/符号图和 `Link.InlTree`。`buildcfg.Experiment` 显式来自 `Types.Experiment`。无新增配置文件、IR 模型或全局 Reset。
- 本地函数复用可内联分析产生的声明信息；导入函数通过 `expandInline` 惰性读取抽象调试声明，保留原 callee 的 Dcl，普通导入函数不进入本地编译队列。
- 对直接闭包调用保留上游提前 InitLSym 规则；启用 GenDwarfInl 时，原生 SetPrecursorFunc 记录内联前函数。`CalleeEffects` 将嵌套 callee 表达式的初始化和函数体副作用移入调用初始化序列。

外部 module 可以使用 `noder.Context.Inline.TryInlineCall`。调用方遵循原生阶段顺序：先准备并读取包 IR，设定内联选项，调用 CanInlineFuncs；展开时在 `IR.WithFunc(caller, ...)` 内调用，并把非 nil 结果装回原调用位置。使用新内联启发式时还需按原生驱动的生命周期评分/清理调用点；启用内联 DWARF 前需提供 `gcbridge.NewDwarfFixupTable(link)`。这些阶段设置将在完整驱动接入后统一负责。

TryInlineCall 的 nil 返回表示原生策略拒绝展开。编译器不变量错误仍按低层 API 的 panic 约定中止当前编译；它不是允许失败后复用任意中间状态的事务接口。入口检查 caller、call 和 profile 的编译实例归属。

独立外部 module 已在 [consumer](../testdata/consumer/consumer_test.go) 中对真实源码的泛型调用完成展开；只导入公共包，不替换任何生产回调。

## 验证

- 本地源码覆盖单/多返回值、命名结果、分支提前返回、无结果函数、变参、泛型、直接捕获闭包及返回闭包；分别启用和关闭 NewInliner。
- 八个实例并发展开，验证返回图与 DWARF 表不共享，旧结果不受后续实例影响；外来图被拒绝。
- 真实官方 finalized export 数据覆盖普通导入和返回闭包的惰性函数体/调试声明读取；生产路径不启动编译器，官方工具仅用于测试数据。
- 嵌套 `Maker(x)()` 在测试中通过原生单调用入口完成两次展开，检查副作用只出现一次且调用消除。测试中的局部遍历仅组织夹具，不作为生产整包优化驱动。
- PGO 高成本调用在预算不足时拒绝展开，预算足够时实际展开并登记本实例的热调用状态。
- 递归函数允许首次展开，已在内联栈上的重复递归调用被原生判定拒绝，实例展开计数不再增加。
- 上游差分夹具比较展开节点、类型/类型检查状态、位置、变量归属/内联标记、抽象 DWARF 符号、内联位置树和导入声明。该层验证 IR 变换与上游一致，不宣称尚未接入的机器码执行已经通过。

Go 1.27.1 的定向差分已通过（386、amd64、mips、ppc64、amd64-newinline）。测试导入数据由固定官方编译器产生；newinline 配置使用匹配的 `GOEXPERIMENT=newinliner`。实验配置不匹配时，原生 reader 本身也无法解码，因此低层 RegisterPackage 的调用方必须保证数据与实验配置匹配。该头部/配置校验属于后续完整 artifact/驱动入口。

`syncgo generate` / `check` 通过，共 250 个迁入文件，工具版本保持 `syncgo-m3-v13`。`GOTOOLCHAIN=go1.27.1 go test -race ./...` 通过（port 包 49.114 秒）；日志见 `_work/m3-inline-expansion-unit.log`。

`TestPinnedUpstreamAndUpgrade` 通过（测试主体 401.27 秒，包总计 402.732 秒）。Go 1.27.1、Go 1.27.0 的五种新增展开配置及既有完整差分全部通过。同一套规则从 1.27.0 升级到 1.27.1 时仅两个 Go 输出文件变化，上游工作树保持原状。日志见 `_work/m3-inline-expansion-source.log`。

下一步接入原生 interleaved 去虚化/内联驱动及其剩余依赖，随后补齐完整包 IR 驱动、wrapper 与后续优化阶段，继续推进源码到 SSA 和原生编译器回接。
