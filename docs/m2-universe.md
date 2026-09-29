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

保留 Type 字段并不等于完成类型系统。当前标量节点 width 为上游 BADWIDTH，align 尚未计算；没有伪造 Size/Alignment 结果。InitTypes、预声明名字、CalcSize、递归布局、接口/方法集、泛型、完整命名类型及包选择器仍待移植。

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

## 第四批：复合类型构造与指针缓存

已移植上游 NewPtr/NewSlice/NewArray/NewMap/NewChan 及 Elem/Key/NumElem/ChanDir/PtrTo 等消费者接口。PtrSize 来自当前 Universe，NewPtrCacheEnabled 也是每实例字段。保留上游缓存语义：关闭开关只禁止写入新的指针缓存，已有缓存仍可命中；切片仍使用独立的元素缓存。指针的 width/align/ptrBytes/intRegs/alg 由原 NewPtr 算法初始化，32/64 位差分均通过；其他复合类型的完整布局仍等待 CalcSize。

`base.Fatalf` 按上游真实函数签名绑定到同目录 `Universe.fatalf`，后者 panic(error)，不会退出宿主进程。该入口用于编译器不变量失败，不代替用户源码诊断。工具只将从固定源文件读取的签名放入移植前类型检查器，不生成假的 base 包；转换按 go/types 对象身份进行，且移植后核对 adapter 的完整签名。签名来源哈希写入现有生成账本。

上游 Kind.String 也已移植。其常量整数索引表移动到使用它的函数内部，保留数组类型和查表算法；工具只允许索引读取和 len，拒绝写入、取地址、slice 或其他逃逸。

验证：完整 race、独立 consumer、两版参考切片差分及升级检查通过；指针缓存开关、跨 Universe 拒绝、失败后继续构造均有覆盖。参考切片保留上游全局状态与构造算法，仅对宿主 Fatalf 退出行为使用测试 panic 适配，差分执行合法输入路径。`syncgo check` 验证 18 份产物，16 份移植 Go 文件在 go1.27.0 → go1.27.1 间无内容变化。日志：`_work/m2-constructors-tests.log`。

## 第五批：未定类型与常量映射

上游 UntypedBool/String/Int/Rune/Float/Complex 六个类型身份，以及按 go/constant.Kind 索引的 UntypedTypes 和 DefaultKinds 表，均改为 Universe 字段。保留原初始化表达式，未新增常量类型抽象。UntypedRune 与 UntypedInt 保持两个不同对象。

上游 IsKind/IsBoolean/IsInteger/IsSigned/IsUnsigned/IsFloat/IsComplex/IsString/IsUntyped 可直接从公开 Type 调用。有状态分类方法通过 Type.owner 读取相应身份，`(*types.Type)(nil).IsUntyped()` 和 IsKind 的 nil 行为保持原样。转换工具仅对直接的、无副作用的 nil receiver 常量返回保留前置分支，其他复杂 nil 语义仍需单独审阅。

外部消费者可直接读取 `u.UntypedTypes[constant.Int]`、`u.DefaultKinds[constant.Int]`；不需要新包装接口。表和对象仍遵循上游可变模型，同一实例内修改须串行；不同实例没有共享节点或表。

验证：全量 race、独立 consumer、两版参考切片的类型分类/身份映射/初始化差分、生成与升级检查通过。nil receiver、方法表达式、回调保活和 32 个并发 Universe 均有覆盖。`syncgo check` 验证 18 份产物；16 份 Go 文件跨补丁版本无内容变化。日志：`_work/m2-untyped-tests.log`。

第五批结束时的剩余主线：移植 Field/Struct/Signature 构造及必要访问器，再接入 CalcSize、延迟布局队列、算法分类和 InitTypes。完整预声明类型绑定仍未实现，现有 Scalar 是未命名节点入口；本批常量身份不等于已经完成 typecheck，更不等于源码到 SSA。

## 第六批：Field、Struct、Signature 与后端访问器

已直接移植 NewField/NewStruct/NewSignature、Field 的复制与标记方法，以及字段、参数、结果和结果 tuple 的访问器。公开 types/gcbridge 只增加原生 Field/Struct 的类型别名，未引入第二套字段或签名对象。

Field 保持上游结构，不添加独立 owner；其 Type 和非 nil Sym 必须属于当前 Universe。NewField、NewStruct、NewSignature 在执行上游构造体之前检查字段图。通过同包 adapter 检查 nil 字段、外来类型和外来符号；NewSignature 的 nil receiver 和空参数/结果仍然合法。这里检查的是输入 Field 的 Type/Sym 归属，不是任意可变图的递归所有权证明。

保留上游存储语义：NewStruct 接管字段 slice；NewSignature 将输入 slice 的条目复制到 allParams，但不复制 Field 对象；ResultsTuple 与 Results 共用对应字段区间。调用者不得事后通过公开 Field、Struct 或 slice 字段拼接外来对象。Field.Copy 是上游浅复制，类型和符号身份保持不变。

后端可通过 `Fields/Field/FieldType/NumFields`、`Recvs/Params/Results`、`RecvParamsResults/RecvParams/ParamsResults`、`NumRecvs/NumParams/NumResults`、`Recv/Param/Result`、`IsVariadic/ResultsTuple` 读取真实对象。FieldType 也支持原有 tuple/results，无需手写解包模型。

```go
field := u.NewField(position.XPos{}, nil, value)
record := u.NewStruct([]*types.Field{field})
signature := u.NewSignature(nil, []*types.Field{field}, []*types.Field{field})
// signature.Result(0) == field
// signature.ResultsTuple().FieldType(0) == value
```

工具规则仍在 Go 中。Checks 将选定声明绑定到同目录参数校验方法；校验方法必须保留原始完整参数签名且无返回值。未命名/空白/variadic 参数尚不支持。receiver 校验也通过原对象 owner 调用，不创建全局 validator；上游签名变化或过于宽松的 adapter 类型会被拒绝。

验证覆盖：位置/符号/类型身份、Field.Copy 元信息、HasShape/NotInHeap 传播、空签名、receiver、variadic、结果 tuple 存储共享、跨实例与 nil 输入拒绝、回调保活及 24 个并发 Universe。参考切片继续保留上游构造算法和全局初始化，比较 go1.27.0/1.27.1 下的 32/64 位字段与签名快照；参考 Fatalf 仍仅使用测试 panic 适配。

完整 race、独立 consumer、两版上游差分、生成和升级检查通过；`syncgo check` 验证 18 份产物，16 份 Go 文件跨补丁版本无内容变化。日志：`_work/m2-signature-tests.log`。

**边界：** Field.Offset 和 Struct/ResultsTuple 的布局仍未计算，保持上游 BADWIDTH/零 align；只加入了 widthCalculated 的查询及禁止改写已计算字段的原有保护。下一步是 CalcSize 的依赖闭包：延迟布局队列、算法分类、必要接口/方法集和诊断状态，然后接入 InitTypes。尚未打通源码到 SSA。


## 第七批：原生类型布局与预声明类型初始化

已移植完整 size.go 算法、alg.go 的比较/哈希分类、identity.go 的类型等价，以及 universe.go 的 InitTypes。Type 的 Size/Alignment/Registers/HasPointers/ArgWidth、接口方法集和命名类型 SetUnderlying 可由公共别名直接调用。类型格式化沿用 fmt.go 的实现，支持 `%L`、LinkString/NameString 及 nil Type，不自行拼接类型描述。

PtrSize/RegSize/MaxWidth、SimType、slice/string 布局、defercalc/deferredTypeStack、算法优先级表、预声明类型及假 receiver、格式化缓冲池、包名消歧表全部归 Universe。跨包的 base.Pos/AutogeneratedPos 和 buildcfg.Experiment 也绑定为实例字段。仅移植真实 ExperimentFlags/Flags 数据声明，不带入环境变量读取；实验开关由宿主显式设置。base.EnableTrace 保留上游常量。

布局诊断通过实例的 PosTable 与既有 diagnostic.Collector 保存；ErrorfAt 的原生错误码以数字字符串保留。Errorf/错误位置和格式化参数均来自原算法。编译器不变量通过 panic 终止当前调用，不退出宿主进程；Fatalf 发生在布局中途时可能留下未完成节点/队列，该 Universe 必须丢弃，不能把所有 panic 都当成可恢复输入校验失败。

初始化保留上游入口 `u.InitTypes(func(sym *Sym, typ *Type) Object)`。宿主需返回绑定**所传 Sym/Type** 的对象，后续前端将提供真实 ir.Name。本库没有增加占位名称对象。每个 Universe 只能初始化一次，须在 Scalar/布局操作前完成；跨实例回调可以重入初始化另一 Universe，同一 Universe 的初始化/变更不能重入。NewUniverse 默认给出实例内 BuiltinPkg/UnsafePkg 和匿名 LocalPkg；编译宿主可在 InitTypes 前设置自己的 LocalPkg。

初始化后 Scalar 返回 Types 中的规范预声明对象。旧的构造用途仍可在未初始化 Universe 中获取未命名 Scalar，但这种使用不允许再调用 InitTypes 替换其身份。公开表与类型图仍按上游可变结构保留，宿主不能自行接入外来对象或覆盖 canonical binding；入口校验不是对任意回调及公开字段修改的递归所有权证明。

验证涵盖：386/amd64/arm64 布局；结构体对齐/尾部零大小字段、数组寄存器分类/指针前缀、not-comparable 类型、递归命名类型、参数宽度、接口方法展开/去重、byte/rune/any 身份、延迟队列嵌套/去重、禁止布局后的已计算节点、独立诊断与位置恢复、nil 输入、跨实例拒绝、A→B→A 回调及并发旧图保活。公开独立 module 直接消费相同 Type/Field 对象并查询布局。

差分测试另外编译保留全局变量和初始化的官方源码切片及原始位置包，使用相同输入比较 32/64 位布局、算法、字段偏移、参数、命名/递归格式、atomic align64、SIMD 开关/标记与诊断。参考宿主仅替换进程退出、错误收集和环境实验输入；其类型算法不走本库 Context 改写。go1.27.0/go1.27.1 两版均通过，全量 race、独立消费者、再生成/升级检查通过；26 份生成产物中 24 份 Go 文件跨补丁版本无内容变化。日志：`_work/m2-layout-tests.log`。

下一步接入前端 syntax 的完整解析入口与实际 IR 对象，继续按依赖移除 compiler 状态；尚未提供 Go 源码 → SSA，也未完成原生编译器替换或自举。类型构造/布局切片通过不代表完整语言类型检查已实现。
