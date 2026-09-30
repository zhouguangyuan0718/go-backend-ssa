# M3 原生 SSA 配置与架构 lowering

完整迁入上游 `config.go`，保留 `NewConfig` 的 14 个架构名称、ABI0/ABIInternal 配置、寄存器与参数寄存器集合、字节序/软浮点策略，以及全部架构重写回调。对应 19 份生成的 rewrite 文件完整迁入，包含常规 lowering、late lowering 和 386/AMD64 splitload 回调。`lower.go` 和 `magic.go` 也完整迁入。

公共 rewrite 依赖保留原生实现，包括 rewrite 迭代器、图改写辅助函数、常量运算、类型/符号判定和诊断。`logRule` 是上游重新生成 `-log` 规则时使用的文件日志函数，在两个固定版本的生产 rewrite 文件中没有调用，因此未纳入当前依赖闭包。它的缺席不改变这些固定文件的重写行为。没有以替代回调或空实现绕过架构依赖。

`Config.buildRecipes` 的完整乘法强度削减算法及生成闭包原样保留。生成器只对经过 `StatelessFunctions` 显式声明、并通过传递状态分析的泛型函数开放迁入，用于原生 `oneBit` 和 `isPowerOfTwo`；未知泛型形式、直接或间接访问编译状态的已声明纯函数仍会被拒绝。

## 实例绑定和公共入口

`ssa.NewContext(abiContext, reflectData, logContext)` 绑定已有编译服务。ABI、反射数据、运行时布局、IR、类型 Universe 和 Link 必须属于同一编译图。它复用现有 `Context`，不引入另一套 SSA/IR/目标配置表示。

调用方在 `Context.NewConfig` 前显式设置目标对应的原生特性字段：GOAMD64（1–4）、GOARM（5–7 及软浮点模式）、GOPPC64（8–10）或 GORISCV64（20/22/23）。GOOS 来自对象写入器已经绑定的 Link。目标设置在编译期间保持不变；同一实例上的阶段操作按顺序执行。

上下文持有独立的原生 opcode/寄存器表；386 共享库模式对 LoweredWB 的 BX clobber 修改不会写入其他实例。类型辅助操作、反射数据、ABI、FMA/转换 hash 策略、优化日志和诊断均绑定已有服务。Config/Func 通过现有 Types 所有者取得编译上下文，没有给图节点另加所有者字段。

公共 `ssa` 包现在提供原生 `Config / Func / Block / Value / Cache / Edge` 别名，外部后端可以使用原生 `NewConfig → Cache.Reset → NewFunc → NewBlock/NewValue` 构造图。`ssa.NewTypes(u)` 仍可用于独立类型查询；构造配置时使用对应 `Context.NewTypes()`，确保类型集合与回调属于同一实例。

构造配置会检查服务归属、完整类型集合的规范身份、目标名称、指针/寄存器宽度和字节序，以及相关 CPU 档位。边界检查在原生配置写入表之前执行。原生 Config 和架构重写算法不被这些校验替换。

## 验证和边界

`TestPinnedSSALower` 在 Go 1.27.1 和 Go 1.27.0 上分别比较 18 组配置：14 个架构名称，加上 amd64-v4、arm-v5、ppc64-power10、riscv64-rva23。每组包含普通和软浮点模式，比较：

- 配置标记、全部寄存器集合/掩码、ABI 编号/栈偏移和完整回调组合。
- 共享库模式的原生 LoweredWB clobber 修改。
- 带整数运算和内存 Store 的图，依次运行 `checkFunc → numberLines → lower → lateLower → deadcode → checkFunc → checkLower`，比较最终图。
- AMD64 ANDN、ARM 乘加、PPC64 字节翻转、RISC-V MAX 的档位条件选择；低档位下允许单条规则保持未改写，未将该探针当作完整 lowering。
- 完整乘法 recipe 集合的成本，以及每个闭包构造出的 opcode、参数和 AuxInt 序列。
- 有符号/无符号除法与整除检测常量计算的边界值。

本地测试同时保留 32 位旧图并并发运行八个独立实例，检查图、类型、opcode 表和诊断日志不串扰；四类 CPU 特性回调分别验证低档位 → 高档位 → 原低档位。错误服务/类型/目标输入和残留 generic opcode 会被拒绝。除法 magic 的独立数值测试穷举 8 位有符号/无符号输入，并对 16/32/64 位边界及确定性样本用大整数验证乘法移位恒等式。独立消费 module 仅通过公共包构造和读取原生配置及图。

这些测试验证迁入行为、实例隔离及指定数值恒等式，没有生成并运行目标机器码。splitload 回调已绑定，但完整调度/寄存器分配流水线尚未接通；完整 generic 优化、ssagen 从 IR 构造 SSA、编译驱动以及工具链替换/自举仍待完成。公开的手动图构造入口不等于源码到 SSA 的 Build 接口。
