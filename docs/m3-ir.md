# M3 原生 IR 图与实例状态

本批迁入原生 `ir` 的节点定义、表达式/语句/名称/函数构造、复制和遍历、常量、格式化、符号表及配置。`internal/ir` 保留上游文件名；公共 `ir` 包通过类型别名暴露同一批对象。这里没有第二套 IR 数据模型。

## 当前入口

```go
layout, _ := target.NewLayout("linux", "amd64")
link := &gcbridge.Link{}
u, _ := types.NewUniverseWithPositions(layout, &link.PosTable)
c := ir.NewContext(u, link)

// 原生 typecheck.InitUniverse 所用的类型名称回调。
// 完整 InitUniverse 将在后续迁入；本例只初始化 compiler types。
u.InitTypes(func(sym *types.Sym, typ *types.Type) types.Object {
    n := c.NewDeclNameAt(position.XPos{}, ir.OTYPE, sym)
    n.SetType(typ)
    n.SetTypecheck(1)
    sym.Def = n
    return n
})
v := c.NewInt(position.XPos{}, 1)
expr := c.NewBinaryExpr(position.XPos{}, ir.OADD, v, v)
copy := ir.DeepCopy(position.XPos{}, expr)
_ = copy
```

构造函数通过 `*ir.Context` 调用。节点方法及 `Node` 接口保持原生签名；不需要实例状态的 `Copy/DeepCopy/Visit/EditChildren` 等仍是普通函数。`types.Type.Obj()` 可以指向真实 `*ir.Name`，`TypeNode` 返回同一个对象。

`miniNode` 增加私有实例引用；`miniExpr/miniStmt` 和具体节点经嵌入继承它。生成器向原生构造表达式补入该引用，原生结构复制自动保留引用。`CurFunc`、闭包编号、内建节点、intrinsic/escape 格式化回调、已知符号/包及常量表均为实例字段。独立实例使用独立 Universe 和 Link；同一实例上的可变操作需要串行。

节点字段仍可直接修改，调用方须保证一个图中的节点、类型、符号、位置和函数来自同一实例。具体对象参数会执行生成的归属检查，但这不是能阻止一切错误图编辑的隔离容器；不能通过修改公开字段拼接不同实例的图。

## 位置归属

`NewUniverseWithPositions` 在创建任何类型之前安装位置表，保留已有表中的位置。`Universe.PosTable` 改为指针；原有 `NewUniverse` 仍创建自己的独立表。IR 必须使用与类型系统相同的 `&Link.PosTable`，接入不同表会被拒绝。不能事后复制表、替换该指针或让两个编译实例共享表。原有 `BindLink` 只绑定符号命名空间，不隐式搬迁位置。

noder 的 `PosMap` 可以继续写入同一个 Link。原生 `FmtPos/AllPos`、类型布局诊断和 IR 调试输出由此解释同一套 XPos。原生 base.Pos 绑定到 `Context.Types.Pos`；`WithFunc` 在调用期间安装当前位置，并在正常返回或 panic 后恢复，因此类型布局诊断和 IR 共用同一个当前位置信息。

## 移植和验证

- 原生 `OpNames/OpPrec` 是常量元素的 slice。只读表规则增加 slice 支持，只允许索引读取和 len；写入、取地址、append、整体返回等继续拒绝。
- 原生 `Sym.Format/String` 从符号所属 Universe 取得格式化状态，nil 符号保留原生 `<S>` 行为；`Pkg.Selector` 和 `FloatForComplex` 同时迁入。
- `obj.TrimInlineHash` 的小型原生算法暂置于适配器，唯一变化是将 `inlHashLen` 初始化移入函数。它和 IR 行为一起纳入官方源码差分；不以复制的算法作为测试预期。
- 真实 IR 测试覆盖名称与类型身份、复制/编辑/遍历、位置、参数、闭包、函数作用域异常恢复、不同布局并发及保留旧图。
- 差分测试将不可变 Git blob 中的原生 ir/types/base/obj 源码放入临时 Go build overlay；对官方实现和库运行同一套图操作，比较输出、对象关系、位置、常量和闭包名称。overlay 只用于测试，不修改 GOROOT 或来源 checkout，库运行时不启动编译器子进程。
- 独立 consumer module 仅导入公开包，构造和遍历真实 IR。

## 尚未完成

这些入口可以构造和消费原生 IR；尚未把已检查源码经 noder reader 还原为此图。完整 `typecheck.InitUniverse`、IR 类型检查、reader、优化、walk、SSA 构造和原生后端回接仍待推进。`OKForConst` 和 `IsSimple` 等在官方 typecheck 阶段填充的表目前保留其阶段前的值，不冒充已完成该初始化。ABI 文本符号初始化及 HTML/文件 dump 驱动也未迁入。

下一批先迁入真实 typecheck universe 与所需辅助函数，再接 reader 的类型/声明读取；保持现有对象身份，逐步扩展到函数体。

本批验证完成：167 份生成文件通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，包含独立 consumer 和两版官方差分。相同规则从 go1.27.0 到 go1.27.1 仅有 1 份 Go 产物内容变化，来源工作树状态保持不变。验证日志：`_work/m3-ir-tests.log`。验证宿主为 darwin/arm64；这仍不构成源码到 SSA、原生后端执行或工具链自举完成的证据。
