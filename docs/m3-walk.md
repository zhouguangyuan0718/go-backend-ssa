# M3 原生 walk 流程

本批迁入 Go 1.27 的全部 14 个 walk 生产文件，并公开 `walk.NewContext(nd, esc)` 和原生 `Context.Walk(fn)`。解析、类型检查、Unified IR writer/reader、前置优化、逃逸分析、slice 优化到 walk 的流程现在可以在库内运行。尚无 SSA 函数图构造入口，也尚未把库接回宿主编译器。

## 原生算法与实例状态

保留原生排序、表达式/语句改写、map/range/select/switch、闭包和接口转换算法及文件名。orderState、exprSwitch、exprClause 和 typeSwitch 仍使用上游结构，仅为需要状态的私有方法传递已有 Context。

包级类型断言/接口 switch 计数器、scase 和运行时字段缓存、预分析缓存、map 函数名表、libfuzzer hook 表及 math.MulUintptr 符号均属于当前实例。math 符号初始化在编译图输入绑定后运行，保留原生符号构造与包名。每次 Walk 仍按原生 defer 清理 staticValues/shapeConvSources；不会通过重置全局变量实现隔离。

架构字段使用上一批的直接字段规则；类型、诊断、当前函数、runtime 符号和静态数据使用同一图。NewContext 验证 noder、typecheck、escape、reflectdata 和 staticinit 共享输入；Walk 在修改前拒绝外来函数。

补齐的依赖为原生 types.IsReflexive、Type.ToUnsigned、Type.SoleComponent，以及 abi.ZeroValSize。unsignedEType 表由 Universe 持有。没有手写这些算法的替代版本。

## 原生结构字面量的归属

orderState.addrTemp 的局部零值 Schedule 通过上一批规则绑定，保留 nil maps。类型 switch 中 `&ir.CaseClause{List: ..., Body: ...}` 则使用 LiteralRule 的显式 Pointer 选项，生成 `context.BindCaseClause(&ir.CaseClause{...})`。

指针规则验证原生类型来源和 `func(*T) *T` 适配签名，只接受显式地址字面量。它保留一次分配、嵌套字段求值顺序和返回指针身份；局部同名类型不受影响。BindCaseClause 只绑定 owner，保留原生零值 op/position 和子节点 slice；外来子节点、外来类型、已绑定节点在修改前被拒绝。没有为迁移便利放宽 NewSwitchStmt 对未绑定节点的检查。

## 调用边界

前置顺序仍由宿主驱动负责：读取包、整包内联/去虚化、包装函数、loopvar、包初始化、ABI wrappers、deadlocals、escape、slice，然后按 gc.prepareFunc 的约定准备函数并运行 walk。

```go
lowering := walk.NewContext(reader, escapeContext)
lowering.SoftFloat = softFloat

// fn 是尚未准备/Walk 的函数；闭包也须按原生顺序分别处理。
graph.InitLSym(fn, true)
graph.Types.CalcSize(fn.Type())
lowering.Walk(fn)
graph.CurFunc = nil
```

Walk 原生会留下 CurFunc，完整驱动应在调用后清除并推进自己的函数处理队列。一个编译图内串行使用，同一函数只运行一次；独立图可以并发。节点、类型和符号保持原图归属，后续编译不使旧图失效。

公开接口的独立 module 验证见 `testdata/consumer/consumer_test.go`；它只导入公开包。对外没有增加第二套 IR 或目标描述抽象。

## 验证

固定 go1.27.1 与 go1.27.0 使用同一迁移规则，完成源码经过前置流程再到 walk 的差分。测试在临时目录构建只读 Git 对象 overlay，不改来源工作树或宿主 GOROOT。

- 四种目标布局：386、amd64、mips、ppc64。
- 七组额外输入：软件浮点、禁用优化、内联、FIPS、跳转表、map split group、libfuzzer。
- 输入覆盖求值顺序、多返回值、字符串/结构体/大数组比较、map 访问与更新、make/copy/append、string 转换、各类 range、select/channel、defer/go、闭包/方法值、接口断言/转换/类型 switch、普通 switch、泛型和浮点/整数除法。
- 比较改写后节点、调用、局部变量、捕获、位置、静态数据字节/重定位及阶段状态；单独处理函数的闭包。
- 真实图的八实例并发、旧图保活、外来函数拒绝；生成器检查指针字面量的嵌套求值顺序、指针身份、闭包归属和适配签名漂移。

定向双版本 22 组对照通过（83.90s，`_work/m3-walk-source.log`）。独立 module 通过（`_work/m3-walk-consumer.log`）。完整 `go test -race -timeout 20m ./...` 通过（internal/port 70.128s，`_work/m3-walk-unit.log`）；generate/check 验证全部 286 个文件。完整 `TestPinnedUpstreamAndUpgrade` 通过（1016.15s，`_work/m3-walk-full-source.log`），同一规则完成 go1.27.0 到 go1.27.1 安装/检查，两个 Go 输出发生上游变化，来源工作树保持不变。函数缓存清理定向回归也通过（`_work/m3-walk-caches.log`）。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test -timeout 10m ./internal/port -run '^TestPinnedWalk$' -v
```

工具版本 syncgo-m3-v23，生成文件共 286 个。相比上一提交，已有生成代码仅 types/type.go 与 walk 的两个先前切片扩展了主体，其余 268 个 Go 输出只更新生成器版本头。IR 差分不等于目标程序执行验证；这里没有宣称完成 SSA、代码生成、自举或原生编译器替换。下一步按原生 SSA 对象及 ssagen 构造依赖继续移植，保留函数级状态和宿主后端消费边界。
