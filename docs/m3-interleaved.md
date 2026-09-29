# M3 原生去虚化与整包内联

本批接通 `noder.Context.Interleaved.DevirtualizeAndInlinePackage` 与 `DevirtualizeAndInlineFunc`，直接执行 Go 1.27 的原生固定点驱动。此前已经可以展开一个调用；现在由迁入驱动组织全包可内联判定、调用点评分、静态去虚化、内联后的赋值分析更新、重复展开及最终清理。

完整源码到 SSA、原生编译器回接仍未完成。本批结果是优化后的原生包 IR；wrapper 生成、后续 IR 优化、escape/walk 和 SSA 构造仍按后续阶段推进。

## 迁入与状态归属

完整迁入以下三个上游文件，目录保持 `src/cmd/compile/` 以下的形式：

- `internal/devirtualize/devirtualize.go`：接口变量赋值分析、具体类型判定、静态调用改写，以及内联后缓存更新。
- `internal/devirtualize/pgo.go`：热调用目标选择、接口/函数值调用特化、输入临时变量、条件分支与原调用回退。
- `internal/inline/interleaved/interleaved.go`：整包和单函数固定点驱动、调用计数、括号标记、闭包处理与启发式生命周期。

沿用原生 `State`、IR 类型及算法；没有另建优化器模型或配置文件。`State` 的分析缓存继续由原生结构持有；上游 `noType` 哨兵移入 `devirtualize.Context`。`base`、`typecheck.Target`、`ir.CurFunc`、类型表、PGO/日志与内联回调均指向同一编译实例。

原生驱动使用局部零值 `State`。公共 `StaticCall` 在验证节点归属后绑定该 State，再调用迁入的私有 `staticCall`；同一 State 不允许改绑另一实例。这样不必改变原生驱动的声明和调用方式，也不需要新增迁移引擎规则。`State.InlinedCall` 检查 caller、原调用和展开结果属于已绑定的图。独立实例可并发；同一图的变换按阶段串行执行。

`noder.NewContext` 在绑定真实 unified reader 内联回调之后，创建共享该 Inline Context 的 Interleaved Context。外部消费者可通过 `devirtualize`、`inline/interleaved` 公共包使用同样的原生对象。

## 使用顺序

已完成源码检查、stub 写入和 `ReadLocalPackage` 的调用方可以执行：

```go
reader.Flag.LowerL = 1
reader.Interleaved.DevirtualizeAndInlinePackage(reader.Typecheck.Target, profile)
```

`profile` 可以为 nil，非 nil 时必须由该 reader 的 PGO Context 归属化。包参数必须是该实例的 `Typecheck.Target`。内联选项、实验开关和 DWARF 设施仍由宿主明确设置；启用内联 DWARF 时先提供 `gcbridge.NewDwarfFixupTable(link)`。完整 driver 的默认选项装配尚未接入。

整包入口负责原生阶段顺序，调用方无需自行调用 `CanInlineFuncs` 或遍历替换调用点。单函数入口遵守上游前置条件：被调用函数应已完成可内联分析；后续 wrapper 阶段会使用这一入口。

原生算法原地修改函数。编译器不变量失败会 panic；这里不承诺失败后可以继续使用部分变换的图。独立成功编译所保留的旧图不因后续实例而改变。

## 输出边界

补齐原生 `inlheur.DumpFuncProps` 的缓存、收集、排序和属性格式化。`dumpBuffer` 归属于实例；`emitDumpToFile` 的宿主适配沿用原生排序/输出循环，去掉文件打开和路径前缀处理，写入 `IR.TraceWriter`。`DumpInlFuncProps` 字符串仅作为启用标志；库不会据此打开文件，宿主需要文件时自行提供 writer。

普通 trace 和 PGO 的 JSON encoder 也写入实例 TraceWriter，诊断进入实例 Collector。差分测试将原生文件输出与库的 writer 输出直接比较，包括属性和调用点评分。

## 验证

- 真实源码覆盖直接接口、工厂返回接口、多结果返回、相同/不同具体类型赋值、别名循环、nil 和延迟赋值、slice/map/channel range、嵌套与重复闭包调用及递归停止。
- 单函数与整包入口、新旧内联器、关闭内联分别验证；检查最终无驱动临时括号，嵌套 callee 副作用只保留一次。
- defer/go 已由 reader 规范化为包装函数。包装函数体可优化，外层调度调用和 nil 检查保持；原生对照同时检查完整函数列表、类型断言的 UseNilPanic 和调用标志。
- PGO 使用真实预处理 profile，接口和函数值调用均形成条件分支、内联的热路径和原间接调用回退；JSON trace 进入实例 writer。
- 八实例并发、保留旧图、跨图 State/节点/package/profile 拒绝；独立外部 module 通过公共 noder 入口完成嵌套泛型调用的整包优化。
- 固定原生编译器差分比较全部函数节点、类型/检查状态、位置、局部声明/归属、内联树、抽象 DWARF 符号和函数属性输出。新增八种配置为 386、amd64、mips、ppc64、amd64-newinline、amd64-noinline、amd64-single、amd64-pgo。

Go 1.27.1 的八种定向差分已全部通过。`syncgo generate` / `check` 通过，共 253 个迁入文件；迁移引擎未改动，版本仍为 `syncgo-m3-v13`。

`GOTOOLCHAIN=go1.27.1 go test -race ./...` 已通过，port 包 50.063 秒，日志见 `_work/m3-interleaved-unit.log`。`TestPinnedUpstreamAndUpgrade` 已通过（测试主体 456.85 秒，包总计 459.329 秒），日志见 `_work/m3-interleaved-source.log`。Go 1.27.1/1.27.0 的新增八种配置及既有完整差分全部通过；同一套规则升级时只有两个 Go 输出文件变化，上游工作树保持原状。上述测试证明当前 IR 阶段与固定上游一致，不代表尚未接入的机器码执行、自举或替换验收已通过。

下一步迁入原生 `MakeWrappers`、方法值/接收者 wrapper 及其收尾流程，使用本批的单函数优化入口；随后继续补齐原生包驱动与 escape/walk/SSA 阶段。
