# 原生函数编译入口与工作队列

本阶段接通 `gc/compile.go` 的完整函数准备和后台工作队列，以及 `ssagen/pgen.go` 的完整 `Compile`。调用方不再需要手动组合 `BuildSSA → GenSSA → Flush` 才能编译一个函数；闭包由原生队列调度。包级类型/声明固定点、完整数据收尾及统一源码驱动仍是后续阶段。

## 保留的原生实现

- `enqueueFunc` 跳过空标识符和独立闭包入口，处理 bodyless intrinsic、程序集声明的 ABI0 栈图及参数位置数据，并顺序准备顶层函数与其闭包。
- `prepareFunc` 初始化函数符号、记录全局 map 初始化函数、计算参数布局、运行完整 Walk，然后清空 CurFunc。Wasm 包装函数的完整实现也随依赖迁入；本阶段没有验证 Wasm 代码生成后端。
- `compileFunctions` 保留函数长度排序/随机排列、mutex 与 WaitGroup、后台 worker 及闭包追加调度，按原生顺序设置并恢复 `CalcSizeDisabled` 和 `InParallel`。
- `ssagen.Compile` 保留 PGO 热函数判断、完整 SSA 构造、两次超大帧检查、指令生成及 Flush，以及 map 初始化弱重定位和有序字段跟踪重定位。`CheckLargeStacks` 保留原生排序与诊断。

算法来自固定上游源码，生成器只绑定类型、状态和服务。随机排列使用实例内随机数源；`RaceEnabled` 是显式的编译器调度配置，与被编译包的 `Flag.Race` 分开。它选择上游随机调度分支，不读取进程全局随机状态。

## 接口和状态

先完成前端、内联、包装函数、初始化任务、ABI 包装、逃逸与 slice 分析。配置目标、SSA、liveness 和 DWARF 服务，执行 `InitConfig`/`InitTables`，并把 backend 的 `Inline` 绑定到同一 noder 实例。

`gc.Context.BindPackage` 后调用 `BindBackend(backend, lowering)`；对尚未 Walk 的顶层函数调用 `EnqueueFunc(fn, symABIs)`，然后调用 `CompileQueued(profile)`。该方法运行配置好的 worker 并等待全部完成，调用方随后执行 `backend.CheckLargeStacks()`。队列允许包固定点循环补入新生成的顶层函数，但同一函数不能重复准备。

worker 的编译器 panic 在适配边界保存，原生队列等待全部 worker 并恢复阶段标志后，在调用方 goroutine 重新抛出原值。失败实例随后拒绝继续编译、全局输出或收尾；这不改变正常队列调度与代码生成。

公共阶段调用仍须串行。入队拒绝外来函数、外来 ABI 输入、重复/已准备函数、活动 CurFunc 和已编号对象；运行前检查 worker 配置、profile 归属及已有错误。有未编译函数时，`AddGCLocals` 拒绝提前收尾。

writer 的 IR 所有者在启动 workers 前固定，后端不再重复写入同一绑定。诊断收集器支持同一实例内的并发写入及快照：32 个函数并发报告 defer 诊断的回归，在修复前复现数据竞争且仅保留 29 条；加锁后保留全部 32 条并通过竞态检查。

## 验证

- 8 组独立实例各使用 4 个 workers，覆盖随机调度、嵌套闭包、空函数名、真实 symabis 的 bodyless ABI0 栈图、字段跟踪重定位及旧机器码保活。
- 2 个 workers 编译超大参数帧与普通函数：前者不进入汇编器，顺序检查报告原生大栈帧错误，后者仍生成机器码。
- 跨实例/重复输入、提前收尾和 worker 配置变化在入口拒绝；正常实例可继续完成编译。
- 注入后台目标回调 panic：修复前测试进程被 worker panic 终止；修复后调用方收到同一值，阶段标志恢复，失败图拒绝复用，新实例正常编译。
- 独立公开 consumer 使用生产队列完成源码到完整归档，并检查机器码和 PC 栈表。
- `TestPinnedCompileQueue` 在 `go1.27.1` 与 `go1.27.0` 上各通过 13 组完整归档逐字节对照，共 26 组。覆盖优化/非优化、race、ABI、DWARF 配置、大 Phi、内联、分离归档、cgo、build ID 和 4 个 workers；各组均包含空函数名与 bodyless ABI0 声明。
- 固定上游的归档与对象 reader 继续验证成员、表及符号引用；完整 `go test -race ./... -count=1` 和 446 个生成文件的一致性检查通过。

本阶段的归档差分运行完整上游队列及函数编译入口。它尚未覆盖 `gc.Main` 的类型/全局声明/函数固定点；归档能够逐字节匹配这些测试配置，仍不能代替完整程序链接、执行或编译器替换验收。
