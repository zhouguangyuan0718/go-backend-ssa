# M3 原生运行时布局和写入游标

为 reflectdata 与 reader 的类型元数据路径迁入完整 `compile/internal/rttype/rttype.go`。`internal/go/abi` 使用上游原始 Type、各类描述符、ITab、接口 switch/assert 及缓存结构，配套完整 `compiletype.go`；没有手写替代 ABI 结构或布局公式。类型描述符的完整生成仍由后续 reflectdata 接入。

## 入口和状态

公开 `rttype.Context`、`Cursor`、`ArrayCursor` 是内部原生类型的别名。先在已有 Universe 上执行 `typecheck.InitUniverse`，再创建共享同一 Universe/Link 的 objw，最后 `rttype.NewContext(writer)`、`Init()`。后端可直接读取 `Type`、`MapType`、`ITab` 等目标布局，使用 `FromReflect` 转换原生支持的宿主类型，以及 `NewCursor`、`NewArrayCursor` 写入原生 LSym。

16 个原始全局布局变量均进入 Context，目标宽度来自已有 Universe。`Init` 每个 Context 仅能执行一次，重复调用在改变旧布局前失败。没有全局重置，也没有依赖当前进程架构来选择目标布局。

Cursor 保存创建它的 Context；派生 Field/Elem 游标保留这一归属。ArrayCursor 从内含 Cursor 取得同一 Context，不重复保存所有者。原生 ModifyArray 只改变游标的虚拟数组范围并返回大小差，不修改原 Type；整数、指针、slice 和强弱偏移重定位最终调用同实例的 objw。旧游标可在新实例创建后继续使用。

原生 reflectToType 为字段分配新 Sym，未填写 Pkg。本库在它调用 NewStruct 的唯一适配点，把这些新符号绑定到已有 Universe 的匿名包，保留原 Sym/Field 身份。一般字段构造仍拒绝外来或未初始化的符号；不放宽整个 types 的所有权约束。原生类型的 String/LinkString 也纳入差分验证。

同实例的 Context、Universe、writer 和 LSym 操作需串行；独立实例可并发。新建 Cursor 会验证类型归属，但 LSym 仍是原生无所有者字段结构，调用方必须保证符号来自同一 Link。不可直接改写活跃实例的目标和依赖绑定。

## 移植规则

规则集中在 `internal/port/rules_rttype.go`，生成文件保留上游目录与文件名。使用既有 typed Call/State/Owner/ContextFrom 规则完成状态和写入调用绑定；仅增加需要的原生 Type.OffsetOf 方法。

工具版本为 `syncgo-m3-v10`。输出审计新增对 ABI struct 字段和函数类型中的被动 `unsafe.Pointer` 声明的识别，仍拒绝 unsafe 转换、指针值表达式和数组长度中的 unsafe 操作。这里只用 reflect 检查宿主 ABI 的形状，不读取运行时内存。专门回归验证允许声明而不放行操作。ABI 的完整类型体另与固定源码逐一对照。

## 验证

- 16 个并发实例交错使用 32/64 位和大小端，保留旧类型、字节和游标并继续写入。
- Init 重复调用、跨实例类型、nil symbol、零值 Cursor、错误字段/类型、数组越界和不支持的反射 kind 均失败；验证相关失败未改变已有对象数据。
- 独立 consumer 只用公共包创建 386 目标布局，写入 Hash 并生成扩展 ITab 函数项的弱重定位。
- 两版原编译器各在独立测试进程中以 386、amd64、mips、ppc64 目标运行同一快照，比较 16 类布局的大小、对齐、字段偏移、类型名，以及写入字节、重定位、slice 和虚拟数组行为。独立进程仅用于参考测试，不是库的编译实现。

本批仍不生成完整类型描述符、可链接对象或 SSA 函数图，也未完成原生编译器替换。下一步接 reflectdata 的类型符号登记及类型描述符生成，再推进 reader 的类型/声明实例化和函数体。

本批验证完成：201 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 197.847 秒。同一套规则在 go1.27.0/go1.27.1 下的四种目标 rttype 差分全部一致，两版间仍仅 1 份 Go 产物变化，来源工作树保留。日志：`_work/m3-rttype-tests.log`。被动指针审计用例随后加严为必须先通过 Go 类型检查，并单独重新通过 race 测试。
