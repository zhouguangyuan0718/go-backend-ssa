# M3：原生 x86 汇编器

本批完整迁入原生 x86 普通/AVX/EVEX 编码、目标预处理、栈增长与 TLS 改写、跳转布局和 Windows SEH 构造。`amd64.NewContext` 现在安装实例自己的原生汇编回调，并运行原生 `instinit`。源码差分已走到函数机器码及重定位；正式 `Progs.Flush`、PCLN/DWARF 汇总、对象文件封装和链接执行仍待接通。

## 实现与归属

- 完整保留 `asm6.go`、`obj6.go`、`avx_optabs.go`、`evex.go`、`ytab.go` 和 `seh.go`；追加原生 `ld.go`、`pass.go`、栈界限常量、浮点常量符号及所需对象方法。
- 操作数表、普通/AVX 指令表、opcode 索引、寄存器编码表、NOP 序列、目标描述符、DWARF 映射和 Plan 9 私有符号均归 x86 Context。指令索引仍指向本实例中原生初始化的表；不共享可变初始化状态。
- `Linkamd64`/`Link386` 保留原生描述符初始化式。目标架构指针使用调用方提供的 canonical 架构，操作系统从 Link 显式输入。AMD64 编译器处理函数和汇编器处理函数由同一实例串联。
- Init、Progedit、Preprocess、Assemble、ErrorCheck 在修改前核对 Link、函数或指令归属及分配器。日志和致命诊断接入实例输出/异常边界。

## 跨包值方法

原生汇编器会显式调用 `As.String()`。该值没有实例 owner，而其格式化表属于 Link，因此移植规则通过 `ContextParamCalls` 为调用传入当前 Link。规则必须指向目标单元中已明确列出的 `ContextParamExports`，并记录源码哈希。

转换保留直接调用、方法表达式、方法值的接收者和实例捕获、指针隐式取址及变参签名。嵌套调用一次遍历绑定各自的实例。直接 deferred 调用保留 recover 语义；需要额外闭包的 recover 方法值会被拒绝。

## 验证

- `go test -race ./... -count=1` 通过，包含独立 public consumer 和新汇编实例测试。生成器在最后补充跨包 recover 检查后另行运行完整竞态测试。
- 8 组并行 AMD64/386 实例各自生成 MOV/RET 机器码；检查本实例表指针、寄存器/操作数状态、旧输出保留、重复初始化及外来输入的写入前拒绝。386 在本批仅作汇编器实例验证，不表示已接通其源码代码生成器。
- Go 1.27.1 / Go 1.27.0 各 9 组、合计 18 组源码到 AMD64 汇编差分通过。矩阵覆盖默认、关闭优化、race、关闭寄存器 ABI、DWARF 4、DWARF 4 base entries、关闭位置列表、大图 Phi 和内联。
- 两侧对含 50 个顶层函数声明的源码执行类型检查、Unified IR、前端分析、walk、SSA 和 genssa，再运行原生 `Flushplist` 的汇编段。比较机器码字节、重定位、汇编后指令 PC、跳转表字节、GC/参数元数据和变量位置列表。另比较标量 intrinsic 及 Int32x4.Add 的实际 SIMD 机器码。
- 临时测试观测器从固定源码提取完整 `Flushplist` 函数，仅明确移除后续 `linkpcln` 和 `populateDWARF` 调用；两侧保留相同的指令整理、分支修正、目标预处理及汇编顺序。该观测器不进入生产代码。
- `syncgo check` 最终验证 432 个生成文件与固定源码和规则一致。

这些检查比较生成的机器码，尚未执行它们，也未产出可链接的完整 Go 对象文件。后续将迁入 PCLN/DWARF 输出并接通原生 `Progs.Flush`，再完成对象输出、统一驱动及替换后的工具链自举。
