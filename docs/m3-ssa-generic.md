# M3 原生通用重写与复合值拆分

完整迁入 `rewritegeneric.go`、`rewritedec.go`、`rewritedec64.go`、`rewritedivisible.go`、`rewritedivmod.go`、`opt.go`、`decompose.go`、`softfloat.go`、`rewritetern.go` 和 `tern_helpers.go`。这包含 generic opt、整除判断/常量除法重写、用户与内建复合值拆分、32 位目标上的 64 位值拆分、原生软浮点转换和 SIMD 三输入逻辑优化。

对应依赖仍使用上游实现：Func 的全部 SplitString/Interface/Slice/Complex/Int64/Struct/Array/Slot 方法及拆分缓存、Type.FieldName，以及通用规则需要的原生 `isVolatile`。这里只迁入 writebarrier.go 中被通用规则调用的 isVolatile，完整 write-barrier pass 仍待后续接入。

## 状态与接口

通用重写继续使用现有 SSA Context 和已有 IR、类型、ABI、反射数据服务。`ir.Syms.Zerobase` 绑定到同一 IR 编译实例，empty makeslice 的优化不会取到其他实例的运行时符号。需查询类型或诊断的 Func 拆分方法从 Config.Types 的既有所有者取上下文；纯 SplitSlot/Struct 方法保留普通原生方法。

ternary 逻辑优化的三项真值表只读输入使用已有 ReadTables 规则局部化。CPU feature 位仍由原生 Block/Func 字段表达；此阶段没有替代 CPU feature 数据流算法，也没有据宿主 CPU 或环境变量推断目标能力。

拆分方法通过已公开的 Func 别名直接可用，继续调用原生 Frontend.SplitSlot 契约，并复用 CanonicalLocalSplits。独立消费模块仅通过公共包构造参数槽位、调用 SplitString 并验证缓存身份和偏移。生产代码没有加入测试前端或解释器。

## 差分与语义验证

`TestPinnedSSAGeneric` 使用固定 Go 1.27.1 / Go 1.27.0 源码，以 Go 1.27.1 工具链分别编译参考端和迁入端，覆盖 386、amd64、mips、ppc64、arm64，共 10 个差分快照。覆盖内容包括：

- 有符号常量除法、余数等于零的判断，以及优化后的图和后续架构 lowering。
- empty makeslice 到 Zerobase 地址的真实通用规则，保留调用 ABI、SelectN 和返回 memory chain 的原生表示。
- 嵌套 struct/单元素 array、空 array、string、slice、interface、complex64/128 和 int64 的双前驱 Phi，命名值拆分、重复命名值清理、大小端分量偏移。
- 软浮点负零和 NaN payload，经类型转换、取反、Round、内存存储和后续 lowering 的图。
- 显式给定 Block/Func CPU 位时，SIMD 逻辑表达式的三输入真值表替换与架构/特性门控。

非 SIMD 图按测试所需阶段运行原生 `checkFunc → numberLines → decomposeUser → opt → divisible → divmod → decomposeBuiltin → softfloat → opt → deadcode → checkFunc → lower → lateLower → deadcode → checkFunc → checkLower`。软浮点用例在转换前不调用会拒绝浮点值的 soft-mode verifier；SIMD 用例单独检验 ternary 阶段，不把预置 feature 位描述为 CPU 数据流分析已经完成。

另外有独立于重写规则的语义验证：

- 测试专用整数 SSA 解释器执行优化后的 32/64 位、有/无符号除法与整除判断，比较 Go 整数运算的结果。输入包括边界值和确定性随机样本，覆盖 32/64 位目标寄存器宽度选择；还确认常量除法/余数节点确实消除。
- 软浮点测试重建 Store 的实际字节，包括 32 位目标拆开的高低字。零、负零、普通数、无穷和 NaN payload 均与独立的 IEEE 位模式取反结果比较，覆盖大小端。另检验 Arg、Load、Phi、Store、Zero、Move 的类型转换及后续 lowering。
- SIMD 用例穷举三个布尔输入的八种组合，检查优化前表达式与 ternary 真值表等价，并验证禁用 feature 或非 AMD64 架构不进行该替换。
- 保留一个 32 位 string Phi 图，并发处理八个其他实例，验证图、命名槽位、类型、偏移和诊断日志隔离；测试拆分缓存复用、失败后继续使用，以及运行时符号归属。

全仓库 race（`-count=1`）、独立消费模块和 368 个生成文件的一致性检查通过。

这些是固定源码差分、图校验和上述范围内的 SSA 执行验证；没有生成并运行目标机器码。测试使用参数槽位这一明确的 Frontend 分支，不代替尚待迁入的完整 ssagen 前端。完整优化 pass 序列、调用展开、CPU feature 分析、写屏障、调度/寄存器分配、源码到 SSA、编译驱动和工具链替换/自举仍待完成。
