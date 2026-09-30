# M3 SSA 调用辅助信息

迁入原生 `AuxCall` 的全部方法和四种构造函数，保留 ABI 参数/结果查询、寄存器约束生成、缓存复用及寄存器编号映射。另迁入 `AuxNameOffset`、`ValAndOff` 的原生操作和字符串、类型、符号、调用的 Aux 转换；`LSym` 使用上游自己的 Aux/Sym 标记方法。

唯一需要实例状态的 `LateExpansionResultType` 从已有 ABI 配置取得类型 Universe。ABI 配置保留所属编译实例，因此无参数、无返回值的调用也能定位正确的 `TypeMem` 和规范化 `TypeResultMem`；无需从某个参数猜测归属或给 AuxCall 再添加所有者字段。查询不初始化 opcode 表。缺失 ABI 所有者或混入外部 Universe 的结果类型会在构造结果类型前拒绝。

公共 `ssa` 包用别名暴露 Aux、Sym、AuxCall、AuxNameOffset、ValAndOff，并转发四种原生调用构造函数和 StringToAux。外部模块可以直接使用公开的 ABI 配置、类型和 LinkSymbol，保留原生身份。

## 验证

- `TestPinnedSSAAux` 在 Go 1.27.1 和 Go 1.27.0 上分别对照原生输出，覆盖 386、amd64、mips、ppc64 四种目标布局。每种布局含三组寄存器预算、标量/结构体/切片/零长数组签名、四种调用构造、栈偏移、寄存器约束与缓存、命名结果、空签名、名称偏移和 ValAndOff 边界操作。
- `TestNativeAuxCallOwnership` 并发使用八个 64 位实例，同时保留 32 位实例的调用与结果类型，验证内存类型及空结果的实例归属。
- `TestNativeAuxCallRejectsUnownedResults` 检查缺失 ABI、未初始化配置和外部结果类型的拒绝，及已有结果的保留。
- 独立外部模块验证公开 Aux/ABI/符号的类型身份；全仓库 race 测试及生成一致性检查同时执行。

目标布局对照在宿主进程中进行，不代表这些目标上的机器代码执行验证。此阶段未接通源码到 SSA、完整优化流水线或编译器替换。后续原生 Value/Block 诊断与格式化仍需要完整 Fatalf/HTML 依赖，不能用忽略诊断分支的简化实现代替。
