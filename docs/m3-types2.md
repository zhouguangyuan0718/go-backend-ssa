# M3 第二批：原生源码类型检查

已移植固定 Go 1.27.1 的完整 types2 生产源码，并提供普通 Go 库入口 `types2.NewContext`、`Config.Check`、`Info` 和原生类型/对象的别名。使用方式见可运行的 [示例](../types2/example_test.go)。没有用标准库 go/types 实现替代 checker，也没有重写或删减检查算法。

源码到类型检查链路已经可运行；noder/Unified IR、后续编译器 IR、SSA 构造和原生编译器回接/自举仍待实现。这里的 `types2.Type` 与后续 compiler IR 使用的 `types.Type` 是两套原生对象，需要继续通过上游 noder 转换。

## 调用与归属

1. 使用 `syntax.Parse` 解析显式 Reader，解析成功后再检查。
2. 每个独立编译实例调用 `types2.NewContext()`，通过 `Config.Context` 显式传入，不能用 Context 的零值替代构造函数。
3. 指定 `Config.GoVersion` 和目标 `Config.Sizes`。`ctx.SizesFor("gc", "amd64")` 等调用保留上游尺寸算法；选择动态架构名时要检查返回值非 nil。低层 API 保留上游 Sizes=nil 时默认 amd64 的行为。
4. `Config.Check` 接收包路径、原生 AST 和可选 Info。检查失败以返回的 error 为准；`Config.Error` 可收集原生 Error 的位置、Code、Msg/Full。上游在完整遍历后即使有诊断也会置 Package.complete，不能只用 Complete() 判断成功。
5. 导入完全由显式 `Importer` / `ImporterFrom` 提供，不搜索宿主文件系统。导入包须与当前检查使用同一个 Context；`unsafe` 也由 importer 返回该实例的 `ctx.Unsafe`。外来 Context 的导入包会被归属检查拒绝。

Context 保存自身的 Typ、Universe/Unsafe、泛型缓存、类型参数编号和辅助表。不同实例没有共享可变编译状态；后续检查不会清空已有结果。公共别名和 `gcbridge.CheckerContext/CheckedPackage/SourceType` 指向同一批原生对象，不额外复制图。

调用方需维护输入、导入和结果图的生命周期，不能在检查期间并发修改共享 AST、Config、回调或类型对象。并发与重入验收使用独立 Context；同一实例内的任意并发变更不在本批保证范围内。手工拼接异属对象违反调用契约，归属守卫以 panic 拒绝；源码语义错误仍走上游 error 路径。

## 状态与输出改写

- 直接扩展上游 Context，保留原缓存、锁和原构造函数。迁移状态的字段类型从已检查的源码推导，按原 InitOrder 和源文件顺序初始化。
- Config.Context、Checker.ctxt 复用原字段。必要的类型节点绑定 owner；临时 operand 的零值声明保留数据字段零值并绑定实例。
- 纯函数、纯泛型帮助函数及不依赖状态的方法保持原签名。自由函数只按实际依赖改成 Context 方法；不能挂 owner 或允许 nil 的私有 receiver 按规则显式传入 Context。
- 同一 receiver 的私有候选方法可用 `Receiver.*` 指定，但只有依赖闭包内的方法才改签名。方法值保留原回调类型、receiver 的一次求值和绑定时机；直接含 recover 的方法值和提升方法值仍需单独审查。原始直接 defer/recover 调用保留语义。
- nopos 等已审查零值变成独立值，并保留导入类型身份及所需可寻址性；invalidTypeSet 等身份 sentinel 保存在实例中，不当作普通零值替换。
- 上游允许的 nil 参数传给原算法处理；非 nil 的异属参数仍被拒绝，包括变参列表。nil Tuple 的格式化也保留 `()` 行为。
- fmt 的标准输出及 os.Stdout/Stderr 的诊断输出改接 `Context.TraceWriter`。nil writer 丢弃跟踪输出；Config.Trace 仍控制原跟踪开关。回调和 writer 都由实例显式提供。runtime.Caller 仅保留读取当前调用栈的断言位置功能，其他进程环境依赖继续审计拒绝。

规则在 [rules_types2.go](../internal/port/rules_types2.go)，对接代码在 [adapter_context.go](../internal/types2/adapter_context.go)，生成源码保留 `internal/types2/` 的上游文件结构。无新增配置文件；来源与哈希仍只在现有生成清单中记录。

## 已验证范围

- 普通函数、分支/循环、泛型函数和类型、泛型别名、接口/方法、递归类型、range-over-func 的源码检查与原生 Info。
- 显式包导入和类型身份、异属导入拒绝、旧结果保留、失败后再次检查、诊断回调中重入。
- 独立 Context 的并发检查、独立跟踪 writer；race 检测覆盖真实 checker 路径。
- 386/amd64 的 unsafe.Sizeof/Offsetof 和常量溢出；GoVersion 语言门控；错误位置、错误码、类型信息及初始化顺序。
- 单独编译完整未改写的上游 syntax/types2，只重定位 import 作为差分基准。Go 1.27.0 与 1.27.1 各执行 48 个源码/尺寸/版本组合，比较 Scope、Defs、Types、Instances、AST 内类型记录、诊断、初始化顺序和空 tuple 格式。
- 相同规则的可复现生成与两补丁版本迁移；生成代码无包变量/init，来源工作树未被修改；独立外部 module 可仅通过公共包消费 checker 和 gcbridge。

这些是类型检查验收，不代表泛型机器码、运行时 lowering 或完整语言到 SSA 已验证。下一步接入 noder/Unified IR 的导入导出与 compiler IR 构造，继续移除相关包状态，再进入后续分析及 SSA builder。
