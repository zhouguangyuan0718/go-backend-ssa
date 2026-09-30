# M3：完整原生 SSA Compile

本批迁入整个上游 `ssa/compile.go`，并公开 `ssa.Compile(f)`。已有 SSA 图可以经过完整原生阶段循环，完成 ABI 调用展开、通用优化、写屏障、目标 lowering、调度及寄存器/栈槽分配。

## 保留的原生行为

阶段表、必需/禁用判断、无优化模式下的阶段选择、每阶段一致性检查、按函数名和 seed 随机调整调度前 Value 顺序、HTML 阶段写入、耗时/内存统计、rule-match 汇总和 panic 阶段诊断均保留上游实现。阶段表在编译实例初始化时捕获实验策略，见 [阶段配置](m3-ssa-phases.md)。

`DumpFileForPhase` 和 `dumpFile` 也保留原生序号、文件名清理、目录拼接、打开失败告警、图输出及关闭逻辑。调用方可以提供真实文件或内存流；库不会自行读取进程环境或打开文件。

## 显式主机服务

SSA `Context` 增加四个输入：

| 输入 | 原生用途 |
| --- | --- |
| `Now func() time.Time` | 每个实际执行阶段的起止时刻；运行 `Compile` 时必须提供 |
| `ReadMemStats func(*runtime.MemStats)` | 启用阶段内存统计时读取原生计数 |
| `DumpDirectory string` | 原生 `GOSSADIR` 值；空字符串保留原生相对名称 |
| `CreateDump func(string) (io.WriteCloser, error)` | 启用转储时创建输出流 |

普通主机驱动可以显式绑定 `time.Now`、`runtime.ReadMemStats` 和包装后的 `os.Create`；测试可以绑定确定性计时、计数和内存流。缺少当前操作所需的提供器会失败，随后原生 panic 处理报告正在编译的函数和阶段。

生成器的 `HostCallRule` 仅接受上述四种已审查的标准库调用，检查真实标准库声明及适配方法签名。`os.Create` 唯一允许的返回类型调整是从 `*os.File` 收窄到其实现的 `io.WriteCloser`，保留独立的 `error` 结果。环境读取必须使用规则中指定的常量键，不能通过别名、动态键或回调绕过。

`runtime.MemStats` 只允许声明中的被动指针类型、函数内零值计数对象及其 `TotalAlloc` / `Mallocs` 字段；其他 runtime 操作仍被拒绝。原生 panic 路径的 `runtime.Stack(buf, false)` 与现有 Caller/Callers 一样保留当前 goroutine 诊断；全部 goroutine、动态布尔参数及 Stack 函数值不在许可范围内。

## 验证

- 同一套输入 SSA 图分别运行迁入的 `Compile` 和固定上游原版，记录每个实际执行阶段的图、最终物理位置、IR 局部声明、RegArgs 和分配状态。覆盖 14 个目标架构，以及 frame pointer、寄存器 clobber、AIX、无优化、两个随机检查种子、SIMD、循环抢占和带检查的无优化模式。阶段观察器只包裹入口记录结果，不替换原生阶段循环或算法。
- 独立物理位置解释器执行最终 AMD64 指令，通过实际寄存器和 spill slot 读写计算校验和；覆盖优化开关、三个 seed 设置、frame pointer/clobber 组合及每组 200 个确定性随机输入，检查别名写入及 spill/reload。它不通过 SSA Value ID 保存中间结果。
- 显式服务测试核对每阶段两次时钟读取、准确的计时/计数字段、全部 dump 的序号/名称/关闭、打开失败后的告警与继续编译，以及缺失提供器和阶段 panic 的诊断。
- 多目标并发实例验证各自图、选项和输出归属。生成器测试拒绝越权环境键、直接 host/runtime 操作、不匹配的 writer/clock 签名及未审查的 metric 使用。
- 独立 consumer 从公共包构造具有原生函数和 ABI 信息的 SSA 图，直接执行公开 `ssa.Compile` 并检查已产生 RegAlloc。

验证结果：Go 1.27.1 / Go 1.27.0 各 23 种配置，共 46 份完整流水线快照全部一致；全量 `go test -race -count=1 ./...` 通过，包含独立 consumer；最终 `syncgo check` 验证 398 个生成文件一致。

这些验证针对手工构造的真实原生 SSA 图。源码/IR 经 ssagen 建图、完整指令输出、统一驱动及替换/自举仍待完成，不能把 SSA 编译入口完成等同于完整 Go 编译器替换。
