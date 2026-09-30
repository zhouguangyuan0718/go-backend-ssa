# M5：EarlyOptimized 阶段与原生续编译

`compiler.Build` 接受 `Options.Stage = compiler.EarlyOptimized`，逐函数按固定上游的 pass 表执行到 `early fuse` 后、`expand calls` 前。空值和 `Built` 仍选择 Phi 插入完成后的分界。关闭优化时，前缀仍按上游的 required/disabled 规则执行；它不意味着跳过必需 pass，也不意味着执行所有通用优化。

## 原生算法与状态保留

生成器的 `RangeContinuationRule` 在带类型信息的原始 AST 中校验固定数组、循环、显式 defer 和词法边界，再把完整 pass 循环交给实例适配器。循环体、随机检查、dump、panic 处理和收尾语句均从上游保留。不能跨边界 return/goto、引入额外 defer，或把有副作用的局部声明移到另一作用域；常量提升也检查遮蔽。

原生 pass 数组在原始 range 表达式位置求值，初始检查和 dump 完成后才取值复制。早期阶段和后续阶段共享这份快照与原始随机数流。初始日志、图检查及 start dump 只执行一次；每次阶段调用都完成原始 HTML flush、规则统计输出和 panic 标记清理，因此显式分段模式会在两个阶段各输出一次累计规则统计。普通完整编译仍只运行一次收尾。

## 两种入口

整包 `Build` 在原生声明固定点内部推进每张独立缓存的图，随后完成静态数据和 import artifact。返回图、命名类型、初始化记录、位置、静态数据和 open-defer 槽位保持原有身份。两个阶段均不运行调用展开之后的原生 lowering、寄存器分配、帧布局、机器码输出或对象编号；调用、ABI 和 Go runtime 信息仍是构图的一部分。

低层 `ssagen.Construction.EarlyOptimize()` 在同一图上执行前缀，`ContinueNative()` 随后从 `expand calls` 接着运行，继而执行原生帧布局、defer 数据和参数 spill。不会重跑前缀。两者共享私有消费状态，复制句柄不能绕过一次性限制；优化失败后不可重试。调用方必须串行使用同一个 Context，并保持配置和前端 IR 不变。

`Package.Artifact` 保存完整原生前端 Unified IR。其 Metadata.Stage 记录生产图的阶段；导入并不要求消费方选同一 SSA 阶段。目标、实验、语言、上游 pin 和依赖身份的严格约束继续生效。Built 和 EarlyOptimized 的前端 payload 应逐字节相同；artifact 头和整包哈希因阶段不同而不同。

## 验证

- 未修改的官方 Go 1.27.1 原生 dump 对照：默认、关闭内联、关闭优化和 AMD64 v3 四种配置，每种 13 张 EarlyOptimized 图、完整函数集合及 60 个全局符号的数据/重定位一致。关闭优化时，上游最后实际执行的前缀 pass 是 middle opt，按该输出比较。
- 官方 Go 1.27.1 与 1.27.0 的独立阶段观察：每版 21 组配置，共 42 组一致，覆盖 14 种架构以及 soft-float、race 插桩、非 regabi、大量 Phi、关闭优化和指定随机检查种子。观察补丁在原生 expand calls 表项前 break，并执行原始 cleanup；不使用本库生成器变换。
- 两版各 11 组 AMD64 原生续编译对照，共 22 组一致，涵盖完整 Flush 后机器码、重定位、PC 表和 DWARF，以及随机检查种子 314159 与关闭优化组合。原生对照运行完整未分段的 SSA pipeline，确认前缀没有被重复执行、随机流没有重置。
- Built/EarlyOptimized 的前端 Unified IR payload 一致，可交叉导入；旧图保活、独立并发 Build、复制句柄的一次性消费、前缀/后缀失败恢复及配置拒绝均有回归。
- 原有四组 Built 图/静态数据、四种原生归档/链接/执行、artifact 官方导出与执行、四种 PGO 输入的归档/机器码对照继续通过。全量 `go test -race -timeout 30m ./...` 和补充生命周期竞态回归通过；`syncgo check` 验证 474 个生成文件。
- Linux/amd64、CGO_ENABLED=0、官方 Go 1.27.1 SDK 的正式 make.bash/dist bootstrap 通过（415.04 秒）。包含默认 PGO，682 个不同包经过本库；所有保留阶段及最终编译器的完整 14072 字节测试归档与官方一致；强制重编后完整 35588967 字节编译器达到二进制固定点，最终工具链程序执行结果一致。

本次 bootstrap 的库内容身份为 `63152825dae7595c0a2b651013d41a6638a3b8606e5db44ab270375670c8bb25`。

## 后续范围

该阶段不扩大完整驱动的目标范围：当前仍为 Linux/amd64。Linux/arm64、cgo/race 驱动、最低 bootstrap SDK 及更广上游回归继续分别验收。
