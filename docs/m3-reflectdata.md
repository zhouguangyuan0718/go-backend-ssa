# M3 原生类型符号、引用和 map 内部布局

本批接入 reflectdata 的运行时类型登记与引用链路，为 reader 的类型实例化、泛型字典和后续描述符生成准备真实符号和 IR。迁入原生 `reflect.go` 的 TypeSym/TypeLinksym/TypePtrAt、NeedRuntimeType/NeedEmit、接口使用标记、TrackSym 和队列排序；`helpers.go` 中除 ConvIfaceTypeWord 外的全部函数；`map.go` 中 MapGroupType、MapType、mapTableType、MapIterType 的完整布局算法。

## 对外能力

公开 `reflectdata.Context` 是实例化后的原生实现别名。`NewContext(typecheckContext, runtimeLayouts)` 要求两者来自同一编译实例且已初始化，继续使用既有 IR、Universe、Link、rttype 和 objw。它不创建另一套类型或符号模型。

- TypeSym/TypeLinksym 保留原生符号命名、noalg 区分和 TypeInfo 绑定。TypePtrAt 返回经过原生 typecheck 的 `*ir.AddrExpr`，其中 LinksymOffsetExpr 指向同一个 LSym。
- NeedRuntimeType 维护原生去重集合及待生成队列；NeedEmit 保留 builtin/runtime、匿名类型、本包类型、已实例化类型和 shape 的生成归属判断。
- MarkTypeUsedInInterface/MarkTypeSymUsedInInterface 在原生符号上添加 R_USEIFACE 重定位，TrackSym 保留原生字段跟踪名称。
- append/copy/delete/map index、make、range、unsafe.Slice 及接口转换源值的 RType helper 保留原生操作码检查，静态类型引用和已有字典表达式的优先级。
- MapGroupType 使用实例的 MapSplitGroup 选项，保留两种 group 布局、大键值间接存储、零大小元素尾部填充和 Noalg 标志。MapType/MapIterType 返回以真实 ir.Name 绑定的运行时内部类型，重复调用保持身份。

运行示例在独立 consumer 测试中。这里只登记类型并建立引用，尚未实现 WriteRuntimeTypes/writeType/ITabLsym；不会把未生成的符号冒充完整描述符。ConvIfaceTypeWord 的非空接口路径依赖 ITab，因此整体留待后续迁入，未用空表或回调占位。

## 去除全局状态

原生 types.typepkg 和 typepkgmu 进入已有 Universe，TypeSym/TypeSymLookup/TypeSymName 成为其方法。并迁入原生 IsFullyInstantiated/SetIsFullyInstantiated 访问器。

reflectdata 的 signatmu、signatset、signatslice，以及 cachedMapTableType、cachedMapType、cachedMapIterType 均为 Context 字段。MapGroup 缓存仍保存在原生 Type.MapType().Group 中，类型本身属于 Universe。没有 Reset，也不为新的编译清理或覆盖旧实例的数据。

原生 makefield 的 nil Pkg.Lookup 隐含全局匿名包，本库用一个 typed CallRule 对接到适配方法，显式选择已有 Universe 的匿名包，再调用原生 NewField；上游函数签名仍被工具核对。其余算法由脚本迁移。规则集中在 `internal/port/rules_reflectdata.go`，无新配置文件或迁移引擎机制。

同一 Context 及依赖图由调用方串行操作；原生锁仍保留，但不是任意图操作可并发的保证。不同实例可并发。公共类型入口拒绝跨 Universe 类型，构造器拒绝混合编译实例；原生 IR/LSym 可变字段和 helper 已缓存表达式仍由调用方保证属于该实例。目标和实验选项应在使用对应缓存之前确定，不能在活跃实例中任意改写。

## 验证范围

- 16 个并发实例交错使用 32/64 位和两种 map group 配置，检查独立的符号、队列、TypeInfo、重定位和 map 缓存；保留旧 IR 引用与布局，在新实例创建后继续使用。
- 外来类型、未定类型、方法签名、不允许的 shape 接口标记与 helper 错误操作码/类型失败；验证队列、重定位和锁仍可使用。
- 独立 consumer 只通过公共包取得真实类型引用、R_USEIFACE 标记和 386 map 内部布局。
- 同一快照在两版固定源码的原编译器与移植实现上运行，覆盖四种目标、符号身份/noalg/别名、NeedEmit 分支、helper 字典表达式复用、原生队列排序和 map 布局/缓存。

下一步继续迁入描述符写入所需的 GC 位图、静态数据和 hash/equality 生成，再接完整 writeType/ITab，推进 reader 的类型/声明实例化与函数体。源码到 SSA 和替换上游编译器仍未完成。

本批验证完成：204 份产物通过 `syncgo check`，带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 214.008 秒。两版固定源码的四种目标 reflectdata 快照全部一致；同一套规则升级时仍仅 1 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-reflectdata-tests.log`。

集成时修正了旧符号测试对保留路径 `type` 使用包名 `p` 的夹具：原生 typepkg 初始化现已按 `NewPkg("type", "type")` 迁入，测试应沿用这一名字。参考测试重置包表时同步重建其原生 typepkg，并增加 TypeSymLookup 与包表查找返回同一符号的差分断言；生产代码的包名冲突检查保持原生行为。
