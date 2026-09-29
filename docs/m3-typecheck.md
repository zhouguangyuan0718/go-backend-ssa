# M3 原生 typecheck universe

迁入 `typecheck/universe.go` 全文，以及 `subr.go`、`syms.go` 中所需的原生辅助函数。内建类型名称、函数、常量、blank/nil 节点和操作合法性表现在均由原生 `InitUniverse` 初始化，不再要求使用者自行实现类型名称回调。

```go
layout, _ := target.NewLayout("linux", "amd64")
link := &gcbridge.Link{}
u, _ := types.NewUniverseWithPositions(layout, &link.PosTable)
u.LocalPkg = u.NewPkg("example/p", "p")
graph := ir.NewContext(u, link)
tc := typecheck.NewContext(graph)
tc.InitUniverse()

integerName := u.Types[types.Int].Obj().(*ir.Name)
appendName := u.BuiltinPkg.Lookup("append").Def.(*ir.Name)
_ = integerName
_ = appendName.BuiltinOp // ir.OAPPEND
```

`InitUniverse` 在新 Universe 上调用一次，替代单独调用 `types.InitTypes`；重复初始化在改动既有对象之前拒绝。先确定 LocalPkg，再执行初始化，以保留原生本包 `_` 与 builtin `_` 的不同符号身份。同一个 IR/typecheck/types 实例的操作需统一串行，绑定之后不要替换它们的关联字段。

## 实例状态

- typecheck.Context 只新增原生 typecheck 所拥有的操作表和内建名称表，通过 `IR` 使用既有实例的类型、符号、节点、当前位置与当前函数。
- `okfor` 的 slice 保留原生对相应数组的引用，包括 `types.IsOrdered`；不同实例的数组及其引用互不共享。
- `IsSimple/IsOrdered/OKForConst` 由原生算法填充。原生布尔/字符串/数值常量、内建类型 `ir.Name`、内建函数和 unsafe 函数均绑定到实例的包符号表。
- `Lookup/LookupNum/NewFuncParams/NodNil/AutoLabel` 使用原生实现。`WithFunc` 期间 `AutoLabel` 使用当前函数计数器，`NodNil` 使用同一当前位置；嵌套调用返回后恢复外层函数状态。

没有增加 typecheck 抽象接口或第二套节点。公共 `typecheck.Context` 是内部原生状态适配的类型别名；规则仍是 Go 代码，移植结果随项目提交。

## 验证与边界

本地回归覆盖内建对象身份、blank 命名空间、操作表引用关系、重复初始化拒绝且旧图不变、参数复制、嵌套函数标签/位置共享，以及 16 个 32/64 位实例并发和保留旧对象。独立 consumer 使用公共 API 初始化原生 universe。

官方源码差分对比所有 opcode 的操作表、完整分类 flags、内建与 unsafe 函数、类型/常量/blank/nil 身份、表引用关系、函数标签/位置和参数复制。参考代码来自固定 Git blob，以临时 overlay 编译；不复用迁移后的算法作为预期。两版升级使用相同规则。

当前提供的是原生 IR typecheck 的 universe 和这些辅助函数；尚无完整 Expr/Stmt 类型检查，[隐藏 runtime/coverage 声明](m3-runtime.md) 已接入，但尚未接通 noder reader。此前的 `types2.Config.Check` 继续负责源码层检查，两者不是同一阶段。下一步迁入 reader 所需的完整 IR 检查能力。

本批验证完成：170 份生成文件通过 `syncgo check`；带来源仓库的全量 race、独立 consumer 和 go1.27.0/go1.27.1 原生快照差分全部通过。相同规则跨补丁版本只有 1 份 Go 产物内容变化，来源工作树保持不变。日志：`_work/m3-typecheck-tests.log`。这些结果尚不能证明源码到 IR/SSA 或原生编译器替换完成。
