# M3：原生 AMD64 指令生成

本批将 `ssagen.genssa`、完整 GC/参数活跃性计算、变量位置列表构建，以及 AMD64 的普通和 SIMD 指令处理迁入同一编译实例。`ssagen.Context.GenSSA` 向原生 `objw.Progs` 写入指令和函数元数据。机器码汇编、目标文件封装、链接执行和替换后工具链自举仍未接通。

## 原生实现与状态

迁移 `ssagen/ssa.go` 全文件、`ssagen/nowb.go`，并保留 `State` 的调用准备、分支和跳转表修正、frame 指令、参数 spilling、内联标记及 HTML 输出路径。`nowritebarrierrecChecker` 归编译实例所有，调用边仍按上游放入各函数的 `NWBRCalls`。

完整迁移 `liveness/plive.go`、`liveness/arg.go` 和 `ssa/debug.go`，包含原生 GC 栈图、栈对象、参数 liveness、优化/未优化变量位置分析及 DWARF 4/5 位置列表写入。补入原生平衡树、LEB 编码、边界异常编码及必要对象方法。调试结果持有 SSA 实例；公开元数据入口在写入前核对函数、Link 和输出缓冲区归属。

AMD64 保留 `galign.go`、`ggen.go`、`ssa.go` 和生成的 `simdssa.go`。目标初始化使用宿主提供的 `LinkArch`，安装原生编译器回调并绑定原生 AMD64 DWARF 寄存器映射；汇编器回调不在本批实现。寄存器/操作码格式化注册表和 x86 后缀表各归一个实例，原生初始化按源码顺序运行。

`objw.Context.NewProgs` 保留上游 worker 划分和指令池。`Free` 清除已用指令及缓冲区内容。调用方须在复用同一 worker 或 Free 前消费指令、重定位及使用它们的调试 PC 回调；此处的 `Prog.Pc` 仍是汇编前序号，不能当成机器码偏移。统一 Build API 的长期结果生命周期仍在后续驱动阶段处理。

## 接入顺序

在类型/runtime、SSA 和 liveness 服务就绪后，调用 `amd64.NewContext(backend)` 安装目标处理函数，再调用 `backend.InitConfig()`、`backend.InitTables()`。宿主显式设置目标操作系统、`Link.Headtype`、架构特征和调试选项。函数完成前端准备及 Walk 后，依次调用 `BuildSSA`、writer 的 `NewProgs` 和 `GenSSA`。

`Prog.String`、`InstructionString` 保留上游签名，通过 `Prog.Ctxt` 使用注册表。独立 opcode 和地址没有所属实例，新增格式化入口 `As.String(link)`、`Addr.WriteNameTo(link, writer)` 显式接收 Link；不提供依赖全局注册表的 opcode Stringer。属性字符串保留原签名，使用源初始化的独立只读格式表。

## 工具规则

`ContextParamExports` 单独列出经过审阅的公开无 owner 值方法；原有 `ContextParams` 继续拒绝隐式修改公开类型的公开方法签名。直接调用、方法值和方法表达式均传递对应实例。

外部状态可从具名结构体字面量提取声明类型，不移入其初始化表达式或宿主回调。固定数组移入 Context 后，原有常量 `len`/`cap` 使用 go/types 校验的常量值，保留常量声明与数组尺寸语义。

## 验证

- `go test -race ./... -count=1` 通过，包含独立 public consumer、8 组并行编译实例、指令/寄存器映射隔离、递归写屏障检查及函数/缓冲区归属拒绝。外来调试 Link 的用例调整为先绑定本实例结果，确保直接覆盖 Link 检查；随后 AMD64 包竞态测试再次通过。
- `syncgo check` 验证 426 个生成文件与固定 go1.27.1 源码和规则一致。
- Go 1.27.1 / Go 1.27.0 各 9 组、合计 18 组原生差分通过。配置包括 amd64 默认、关闭优化、race、关闭寄存器 ABI、DWARF 4、DWARF 4 base entries、关闭位置列表、大图 Phi 和内联。
- 两侧将同一含 50 个顶层函数声明的源码经过类型检查、Unified IR、前端分析、walk 和 SSA Compile，再实际调用原生 genssa；对照完整指令文本、函数属性、GC/参数/open-defer 数据及重定位、跳转表目标、调试槽/变量和位置列表编码。另比较标量 intrinsic 及原生 Int32x4.Add 的 SSA 与 SIMD 指令。
- 首次原生对照发现宿主架构描述符缺少 AMD64 DWARF 寄存器映射，导致位置表达式错误；修复为从上游声明初始化每个 x86 实例的映射，并在 AMD64 初始化时绑定。最终 18 组对照覆盖该修复。

上述检查未运行机器码，也未替换或自举上游工具链。
