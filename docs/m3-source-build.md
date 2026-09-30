# M3：源码到 Built SSA

`compiler.Build(ctx, req)` 接受与原生 `compiler.Compile` 相同的显式源码、目标、依赖归档、嵌入文件和 PGO 输入，返回 `*compiler.Package`。`Options.Stage` 的空值或 `compiler.Built` 选择 Phi 插入完成、SSA Compile 之前的分界；其余阶段返回错误。当前完整入口支持 Linux/amd64、GOAMD64 v1–v4。

## 原生流程与生命周期

Build 与 Compile 共用同一前端配置和准备过程。Build 通过原生声明固定点反复生成运行时类型、全局声明、方法包装和闭包，沿用原生函数队列；每个函数使用独立 cache 构图。Build 固定使用一个 worker，同一编译上下文中的前端状态保持串行，独立 Build 可并发。

返回前不执行 SSA Compile、寄存器分配、帧布局、GenSSA、机器码生成或对象符号编号。原生前端优化、内联、逃逸、walk、ABI 分析和 runtime lowering 仍照常执行。原生 DWARF 生成关闭，源码位置和内联位置保留。Built 的值列表不是机器执行顺序，后端须处理控制流、Phi、数据依赖和 memory chain。

Package 拥有函数图、IR 声明、Universe、符号及位置的共享身份。后续 Build 不复用其 cache；返回后修改原始源码、依赖或 profile 字节不会改变已有结果。同一 Package 的操作仍需串行，未承诺含 lazy cache 的只读并发安全。

取消在同步阶段之间及每个构图任务前检查，所有 worker 退出后才返回；错误或取消均无 Package 结果。provider 的未知 panic 原样传播。底层 BuildDeclarations 消费当前编译器状态，之后禁止原生 FinishPackage 和设置归档头，避免将不完整 native tail 的结果输出为原生对象。

## 包与后端信息

- Metadata：精确源码版本/提交、语言、目标、实验配置及阶段。
- Functions、Declarations、Types：按名称/ABI 排序的 SSA 图、完整原生声明及本次 Universe，包括生成函数。
- Symbols、Globals、RuntimeTypes：原生符号身份、静态数据/对齐/重定位及类型描述符映射。Symbols 包含已驻留符号和定义/引用的递归闭包；同名匿名符号保持不同身份，后端不能按名称合并。
- Init：原生初始化任务、按任务指针重定位顺序排列的函数、R_INITORDER 依赖及 map 初始化对应关系。
- Imports：直接导入的包名/路径、显式归档 SHA-256 和原生链接指纹。输入仍是原生归档适配器，尚不是独立 artifact。
- Positions、Inlining、Diagnostics：源码位置、内联树和诊断。
- OpenDefers：每张图的 defer bits 局部变量及按 bit 索引排列的 closure 槽。后端自行分配帧，并据此生成运行时所需元数据。

上游将大于 1 KiB 的嵌入文件延迟到归档输出读取。Build 在返回前经显式 provider 读取并检查长度，将其转换为结果自身持有的符号数据；不要求后端保留文件 provider。小文件与普通静态数据仍走原生生成流程。

GC、safepoint、栈、寄存器、运行时集成和最终对象输出由消费后端负责。此结果没有原生 finish 入口；需要完整原生机器码归档的调用方继续使用 Compile。

## 验证

- 八组并发源码 Build 覆盖依赖泛型、循环/Phi、接口、方法包装、闭包、map、初始化和双 open defer；重复 Build、同路径不同源码和输入字节清空后验证旧图保活。
- 初始化函数与任务重定位顺序/身份一致；全局符号及其重定位目标均包含在 Symbols。低层 continuation 后，defer 元数据中的帧偏移与返回的实际局部槽逐项一致。
- 错误、取消、失败后新实例、嵌入数据保活、二次读取失败/长度变化、provider panic 和 raw/gzip/preprocessed/空/错误 PGO 输入均有回归。
- 未修改的官方 Go 1.27.1 通过原生 `ssa/build/dump` 输出 Built 图；独立官方 goobj.Reader 检查归档的静态数据。默认、关闭内联、关闭优化及 AMD64 v3 四种配置，每种 13 张图、完整函数集合及 60 个全局符号的数据/重定位均一致。图比较仅剔除官方 dump 本来不打印的 DEAD 节点/块；重定位按完整元组比较多重集，匿名符号保留重复项。
- 完整原生 Compile 的四种模式通过归档逐字节、链接及运行对照；PGO 各输入格式的完整归档和热点内联机器码继续与官方一致。
- 全量 `go test -race -timeout 30m ./...` 通过，`syncgo check` 验证 474 个生成文件一致。
- 最终源码完整 dist bootstrap 通过（401.34 秒），682 个不同包经本库编译；各阶段的完整 14072 字节测试归档与官方一致。强制重编得到的编译器与 toolchain3 的全部 35519640 字节一致，最终工具链程序运行结果与官方一致。此项限定 Linux/amd64、CGO_ENABLED=0、官方 Go 1.27.1 bootstrap SDK。

严格版本化 import artifact 的生产/消费已在[后续 M4 阶段](m4-import-artifact.md)完成。当前尚未包含 EarlyOptimized、第二目标及完整 cgo/race 驱动。这些仍按路线继续验收。
