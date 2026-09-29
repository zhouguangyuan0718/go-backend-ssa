# M2 类型结构、Universe 与全局状态解耦

第二批（提交 `7112883`）在 M1（`45e3b0c`）和 M2 第一批（`fe701ed`）之后继续开发。仍固定官方 Go 1.27.1，保留上游相对目录，转换规则写在 `internal/port/rules.go`。没有新增人工配置文件。

## 第二批实现记录

从上游抽取完整的 Type、Pkg、Sym、Object 和必要附属结构的声明，保留它们的原始字段；为 Type/Pkg 增加私有 `universe *Universe`。Object 仍保留 Pos/Sym/Type 三个方法。算法和字段均有生成来源记录，不以简化同名结构满足依赖。

Pkg.Pathsym 仍为真实 `*obj.LSym`，该 LSym、Attribute、Reloc 及依赖的 SymKind/RelocType 声明也从同一固定上游移植。**这里只移植数据声明，不代表 obj 的对象输出、链接、FuncInfo 等消费者已实现。**

| 对象/操作 | 当前行为 |
| --- | --- |
| Universe | 保存本次编译的布局输入、包表、匿名包、驻留表和标量节点表 |
| NewPkg | 上游查找/创建/名字冲突/符号路径算法转为 Universe 方法，分配的 Pkg 自动带 owner |
| Lookup/LookupOK/LookupBytes/LookupNum | 保持 receiver，通过 Pkg.owner 使用本实例状态；保留驻留与输入生命周期行为 |
| nil Pkg | 原始 nil receiver 明确拒绝；通过 `u.Lookup(nil, name)` 解析到本 Universe 的匿名包 |
| Scalar | 按 Kind 缓存本 Universe 的未命名标量节点，调用上游 newType，设置 owner；未计算布局、未绑定预声明符号 |
| NewTuple | 检查两个节点均属于本 Universe，再调用上游 NewTuple；保留子节点身份和 HasShape 传播 |
| Sym | 保留真实 Def/flags/Pkg 字段，owner 经 Pkg 获取；复用上游 CompareSyms 和标志位算法 |
| 公开接口与桥接 | types、gcbridge 使用同一内部 Type/Pkg/Sym/Object 定义的别名；LSym 无复制转换 |

公开入口为 `types.NewUniverse(target.Layout)`，零布局会返回错误。Universe 零值不可直接使用；除字符串驻留外，同一 Universe 内的可变操作由调用方串行。各 Universe 的状态互不共享。

接口示例：

```go
layout, err := target.NewLayout("linux", "amd64")
if err != nil { return err }
u, err := types.NewUniverse(layout)
if err != nil { return err }
pkg := u.NewPkg("example/p", "p")
sym, err := u.Lookup(pkg, "Value")
if err != nil { return err }
scalar, err := u.Scalar(types.Int)
if err != nil { return err }
// sym.Universe() 和 scalar.Universe() 均为 u。
```

符号与类型公开保留上游可变模型。通过公开原始字段人为拼接非法图，或并发修改同一图，不在安全保证内；受控工厂和查找入口检查外来 owner。尚未完成整个编译器图的跨 Universe 验证。

## 转换工具扩展

- Declarations 按名字选择 type、整组 const 和 `Receiver.Method`。const 以组为单位保留，防止 iota/省略表达式变化。
- OwnerRule 为选中的真实 struct 注入私有 owner；同名局部类型通过 go/types 对象身份排除，原结构存在同名字段则拒绝。
- 在具有 Context 的构造路径中，为 `new(T)` 和 keyed `T{...}` 注入 owner，包括包级状态初始化的根表达式。
- receiver 方法中的包级状态/函数引用改为通过 receiver.owner 访问；方法值、方法表达式和闭包保留绑定。只分配 owned 对象而未引用全局的 receiver 方法也会接入 owner。
- 不支持的 unkeyed owned literal、未显式构造的局部零值 owner 对象、缺少 receiver owner 等情况明确失败。

第二批时 Type 的上游 newType 保留为内部原始算法，公开的 Universe 工厂在其返回时设置 owner；Pkg 的 Context 构造路径由工具自动注入。不能把 OwnerRule 理解成任意代码的完整所有权推导：跨文件状态传播、任意容器/泛型分配、整个类型图的构造仍需后续受控规则和审计。

## 验证范围

- 并行创建 amd64/arm64/386 Universe，同包路径、匿名包、标量节点和标志位互不共享。
- `u.NewTuple` 拒绝外来/空节点；`u.Lookup` 拒绝外来包；后续创建其他 Universe 不破坏旧对象。
- 包路径编码、名字冲突、LookupOK、LookupBytes 输入保活、LookupNum、method value/expression、闭包和符号排序。
- 独立 consumer module 只导入公开包，实现 types.Object，并验证 Type 的 SSA Aux marker、原生别名和 LSym 字段访问。
- 生成规则执行级测试覆盖初始化根节点、同名局部类型、只分配对象的方法、iota，以及未知形式拒绝。
- 固定 go1.27.0/1.27.1 再生成和升级；两版分别执行上游位置测试及 SymCompare 原始断言。符号测试只把全局 NewPkg 改为显式测试 Universe。

完整验证命令：

```sh
GO_SSA_SOURCE_REPO=/Volumes/Disk1/00.Work/00.Code/goallc/go \
GOTOOLCHAIN=go1.27.1 go test -race -v ./... > _work/m2-universe-tests.log 2>&1
GOTOOLCHAIN=go1.27.1 go run ./cmd/syncgo check \
  -repo /Volumes/Disk1/00.Work/00.Code/goallc/go
```

验证结果：上述全量 race 和生成检查通过。独立 consumer、两版上游位置/符号测试均通过；go1.27.0 → go1.27.1 的 14 份移植 Go 文件内容无变化，来源工作树状态保持不变。验证宿主为 Go 1.27.1 darwin/arm64，不能据此扩大为完整目标编译器支持。

## 尚未实现

保留 Type 字段并不等于完成类型系统。当前标量节点 width 为上游 BADWIDTH，align 尚未计算；没有伪造 Size/Alignment 结果。InitTypes、预声明名字、CalcSize、递归布局、NewPtr/NewSlice 缓存、接口/方法集、泛型、完整命名类型及包选择器仍待移植。

32/64 位测试目前证明布局输入和图的实例隔离，**不是完整类型大小/对齐语义验收**。还没有源码到 SSA 的 Build、原生 lowering 或工具链自举。后续继续处理布局、预声明类型表和构造函数的全局依赖，再扩展编译阶段。

## 第三批：跨文件状态与上游 SSA 类型入口

本批优先拆除现有上游全局耦合，沿用 Universe 和公开类型别名，没有新增编译框架或自定义 IR 抽象。

- `type.go` 的十个 SSA 特殊类型绑定（TypeInvalid/Mem/Flags/Void/Int128/Vec128/Vec256/Vec512/Mask/ResultMem）成为 Universe 字段。
- 上游 newType/newSSA/newSIMD/newResults/NewTuple/NewResults 直接转为 Universe 方法；newType 分配时自动带 owner，不再在返回之后手工补 owner。
- 上游 init 中的 int128、vector、mask 宽度和对齐赋值转为每实例初始化；未改动这些上游值。这不表示完整 CalcSize 已完成。
- Type.IsMemory/IsFlags/IsVoid 通过自身 owner 获取身份，IsTuple/IsResults/IsSIMD 复用原方法。
- NewResults 的单一 memory 结果复用本 Universe 的 TypeResultMem；不同实例的缓存和节点均不共享。
- 转换器合并同包状态/函数的对象身份映射，支持跨文件初始化依赖、同名 import 别名和 owner 分配。初始化顺序及限制见 [移植机制](porting.md)。

**接口调整：** 删除第二批手写的 `NewTuple(*Type, *Type) (*Type, error)`，使用上游形式 `u.NewTuple(*Type, *Type) *Type`。新增 `u.NewResults([]*Type) *Type`；非法 owner 或 nil 元素在构造前 panic。两者是编译器内部图构造 API，不作为用户源码诊断入口。NewResults 保留上游的输入 slice 存储语义，调用者不得事后往 slice 写入其他 Universe 的类型；公开 SSA 单例字段当作只读绑定使用。Universe 零值仍不支持。

```go
value, _ := u.Scalar(types.Int)
pair := u.NewTuple(value, u.TypeMem)
results := u.NewResults([]*types.Type{u.TypeMem})
// pair.IsTuple() && pair.IsMemory()
// results == u.TypeResultMem && results.IsResults()
```

验证包含：反序规则下的跨文件初始化与 init/defer/return 顺序、跨文件回调和 receiver 绑定、非法输入拒绝、32 个独立 Universe 并发构造、旧对象保活，以及独立 consumer module 的 tuple/results/memory 消费。

额外将同一固定上游切片编译为保留原始全局变量和 init 的参考包，仅重定位 imports；与移植包对比 SSA 身份分类、缓存、HasShape 传播、width/align 和 SIMD 标记。go1.27.0 与 go1.27.1 均执行该差分。这是类型切片验证，不是 Go 源码到 SSA 的端到端验证。

第三批验证结果：全量 `go test -race -v ./...`（设置 `GO_SSA_SOURCE_REPO`）通过，`syncgo check` 验证 16 份产物通过；go1.27.0 → go1.27.1 的 14 份移植 Go 文件内容无变化，来源工作树状态不变。日志位于本地忽略目录 `_work/m2-state-tests.log`。
