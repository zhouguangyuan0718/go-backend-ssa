# M3：原生 SSA 前端与目标初始化

本批迁入上游 ssagen 的 `ssafn` 及全部 Frontend 方法、`InitConfig`、软浮点运行时表、`deferstruct` 和 ABI 选择策略。公开 `ssagen.Context.InitConfig`、`SSAConfig`、`NewFrontend`；调用方提供同一编译图的 SSA 服务及目标 ArchInfo，再由原生配置初始化后端。

## 保留的行为

- 原生 InitConfig 预热指针类型、关闭后端指针缓存、生成 SSA Config 和工作缓存，并绑定全部运行时函数/变量。Wasm 的边界检查函数表以及软浮点操作表保留原生条件和符号身份。
- 原生 `_defer` 类型保留字段顺序、布局、匿名包字段和实例缓存；ABI 选择保留 RegabiArgs、函数 ABI、RegisterParams 及无函数体栈图策略。
- StringData 使用原生 staticdata.StringSym 并保留每函数字符串缓存。SplitSlot 对可拆分局部变量新建未驻留的 Sym 和真实 IR 局部声明，保留 Used、EscNever、SplitOf 和偏移；参数或取过地址的变量继续引用原槽。
- Syslook 保留上游六项严格分支及未知名称错误。日志进入编译实例的 TraceWriter，警告进入诊断集合；Fatalf 保留函数名前缀和当前位置，经既有实例错误通道返回 panic。
- ArchInfo 保留完整原生定义，其回调依赖的 State、Branch、objw.Progs 和 liveness.Map 只迁入声明。指令生成及活跃性分析入口尚待迁入。

## 生成器和边界

`LiteralRule.NilPointer` 对经过类型解析的 `(*T)(nil)` 调用显式实例绑定器，检查真实内置 nil、原始命名结构体来源及 `func(*T) *T` 签名。不匹配的表达式不能借此变成隐式默认值；普通字面量和已有指针字面量规则保持原行为。Universe.BindPackage 将 native nil 包接到该实例已有的匿名包，已有同实例包直接保留，外来包被拒绝。

ssagen 检查 SSA、IR、typecheck、reflectdata 和 staticdata 服务共享同一编译图；初始化要求相同目标、正工作缓存数及运行时声明。新建前端要求已 DeclareParams 的本实例函数，槽拆分检查节点和类型归属。一个编译图内的阶段操作需要串行执行，独立实例可并发编译。

## 验证

- 独立测试覆盖 14 架构的硬/软浮点初始化、运行时符号身份、defer 布局与匿名字段、ABI 模式和前端全部服务。拆分测试检查相同名字的多个新 Sym 仍具有不同身份，未改变包的驻留符号。
- 8 个并发实例在 AMD64、ARM64、RISC-V64、Wasm 上通过原生前端执行完整 SSA Compile；输入图含字符串常量、局部字符串拆分、需要写屏障的指针写入及普通调用，启用每阶段 verifier。检查字符串缓存、写屏障符号、真实局部声明和最终寄存器分配，外来函数/类型被拒绝。
- 固定上游两版差分记录完整运行时符号表、边界检查表、软浮点表、defer 字段、ABI 选择、字符串存储、槽拆分以及经原生前端完成的 SSA 图。覆盖全部 14 架构，加 AMD64/ARM 软浮点、无优化、Race 和无 RegabiArgs 模式。
- 独立 consumer 通过公共包调用 InitConfig 和 NewFrontend 后执行 SSA Compile。

验证结果：Go 1.27.1 / Go 1.27.0 各 19 种配置，共 38 份快照一致；全量 `go test -race -count=1 ./...` 通过，包含独立 consumer。补充字符串/写屏障/调用用例后，`internal/ssagen` 竞态测试再次通过；nil 指针绑定的拒绝用例也通过独立竞态测试。最终 `syncgo check` 验证 402 个生成文件一致。

本阶段完成 SSA 所需的原生前端服务与目标初始化。IR 经 buildssa 建图、完整 intrinsic 路径、指令输出、统一驱动及编译器替换/自举仍待完成。
