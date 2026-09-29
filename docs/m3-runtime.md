# M3 原生 runtime 声明与参数替换

迁入完整 `typecheck/builtin.go`、`bexport.go`、`export.go`、`syms.go`，以及原生 `types.SubstAny/substFields/Type.copy`。当前 228 条 runtime 声明和 coverage 的 initHook 声明均来自固定上游的生成代码，未重新描述其签名或创建替代符号模型。

```go
// graph 和 tc 按 m3-typecheck.md 的示例创建。
tc.InitUniverse()
tc.InitRuntime()
// 按需要初始化原生 coverage 声明；此处不代表完成插桩。
tc.InitCoverage()

alloc := tc.LookupRuntime("newobject", graph.Types.Types[types.Int])
// alloc.Type() 是原生 func(*byte) *int 签名。
// alloc.Linksym() == tc.LookupRuntimeFunc("newobject")
```

`InitRuntime/InitCoverage` 各调用一次，且位于 `InitUniverse` 之后。它们只加载编译器内置的运行时声明；不导入 runtime 源码、不加载 `.a`，也不执行运行时实现。原生 `LookupRuntimeFunc/Var/ABI` 使用实例 Link 的 ABIInternal/ABI0 命名空间。

## 状态和身份

`ir.NewContext` 按原生 gc/main.go 的驱动初始化协议建立 Runtime、Coverage、InternalMaps、Itab 和 Go 伪包，沿用原始路径和链接前缀。Runtime 使用 `go.runtime` 的源身份与 `runtime` 的链接前缀，避免与真正导入的 runtime 包声明冲突。这段驱动装配位于手写适配器，包和符号仍由原生 Universe 创建。

runtime/coverage 的声明数组归 typecheck.Context 所有。`importfunc/importvar/importsym` 原样创建真实 `ir.Func/Name` 并绑定到符号表；多个使用点返回相同 canonical 声明。

`LookupRuntime(name, types...)` 沿用原生逐项替换旧式 `TANY` 占位类型的算法。专用 Name 仍指向原来的 Sym 和 Func，但有独立的替换后类型；原生 Type.copy 复制相应 extra 数据，substFields 复制所有字段，结果 tuple 也独立复制。原始声明及其他实例化调用的字段偏移不被改写。替换输入另有实例归属检查，拒绝外来 Universe 的类型。

原生编译器不变量的 Fatalf 通过已有适配方式变为本进程 panic，不退出库宿主；这不代表任意失败后都能继续复用所有编译状态。测试验证了参数数量不匹配和外来类型拒绝后，既有 runtime canonical 声明不变。

## 原生计时

`base/timings.go` 全文迁入，原先的 Timer 全局归入 `base.LinkContext.Timer`。Start/Stop/AddEvent/Write 等方法保留原生实现；使用宿主时钟记录耗时，只向调用方给定的 io.Writer 输出。这些测量值不参与 IR 或声明生成。不同编译实例不共享计时记录。

## 验证和边界

- 全部 runtime 声明的类型、Name/Func/Sym 关系、ABI 和链接名；coverage 声明与真正 runtime 包的区分。
- newobject、mapaccess1、chanrecv1 参数替换，以及数组/slice/map/channel/struct 的替换顺序和复制关系。
- 同一声明的多个实例化签名具有独立字段与结果 tuple；手动改写一组字段偏移不影响其他组或原始声明。
- 12 个 32/64 位实例并发初始化并保留类型和符号；独立 consumer 通过公共 API 使用真实 runtime 声明。
- 两版官方源码差分比较完整有效路径；原生计时方法另有完整签名/函数体对照。无效参数的 panic 行为由库回归验证，官方 Fatalf 的进程退出不被表述为可恢复失败。

后续的 [原生 IR 检查](m3-typecheck-ir.md) 已补齐 typecheck 生产包；reader、walk、SSA 构造、原生后端与工具链替换仍未完成。

本批验证完成：174 份产物通过 `syncgo check`；带来源仓库的全量 race、独立 consumer、两版原生 IR/typecheck/runtime 快照和计时方法对照全部通过。相同规则跨 go1.27.0/go1.27.1 只有 1 份 Go 产物内容变化，来源工作树未改变。日志：`_work/m3-runtime-tests.log`。这些结果不代表已经执行 runtime 调用、生成 SSA 或完成原生编译器替换。
