# M3 完整 SSA 校验与循环分析

迁入完整原生 `check.go` 和 `likelyadjust.go`，包含 `checkFunc / memCheck / domCheck`、循环嵌套分析及分支预测。Func 的原生 loopnest 缓存也已接通。校验器的条件、诊断顺序和已知跳过条件保持上游实现。

校验覆盖块种类与控制值、CFG 双向索引、值身份与归属、opcode 参数数量和 Aux 类型/范围、常用操作的类型与宽度、Phi 前驱数量、对象是否在图中、空闲链、支配关系、循环结构及 Uses。memCheck 保留原生的内存 tuple、末尾存活内存、内存 Phi、调度后内存顺序和 Phi 顺序检查。

校验器只从经过审计的原生 opcode 表查询 argLen/auxType/name；循环预测查询 call 标记。无须为检查函数额外分配完整 opcode 表。S390X 的 `CCMask` 和 `RotateParams` 两份原生文件完整迁入，用于保留具体 Aux 类型及其运算。公共 ssa 包提供原生类型别名和旋转参数构造器，独立 consumer 已验证无需导入 internal 即可读取和操作这些 Aux。

## 编译实例与策略

`IsMergeCandidate` 使用原生判定：MergeLocals、优化开关、PAUTO、3×指针宽度阈值、取地址、禁止合并和 open-defer 标记。它通过内部 `ir.ContextOf` 取得 Name 已有的 IR 所有者，读取该实例的 Debug 和 Base.Flag；没有给 Name/Value/Func 新增所有者。未初始化名称和外来类型被拒绝。

循环分析沿用 Func 的 postorder/Sdom/cache，统计信息通过原生前端 Warnl 输出，调试文本写入既有 SSA 编译实例的 TraceWriter。四项分支预测描述词是不可变的原生常量表，读操作由已有 ReadTables 规则局部化。

## 验证

`TestPinnedSSACheck` 在 Go 1.27.1 / Go 1.27.0、386/amd64/mips/ppc64 四种布局下运行完整原生校验器并对照结果：

- 直线图、调度后的图、含 Store/内存 Phi 的菱形图，嵌套循环、不可约循环、不可达块、Exit、Call、Defer 和已有分支预测。
- 循环深度/包含关系、缓存复用和失效重算、分支预测、统计诊断。
- 错误 CFG、重复/缺失块或值、错误控制值、参数数量、Aux 类型和范围、NaN/float32 精度、内存参数位置、类型和操作宽度、Uses、支配关系、内存 Phi 和调度顺序的拒绝。
- 局部合并开关、优化禁用、目标大小阈值及名称标记；具体 S390X Aux 类型和掩码/旋转操作。

原生 memCheck 在存在未使用的内存值或内存 Copy 时会提前返回。差分用例明确记录未使用内存的跳过行为，再补上该内存值的使用以验证错误内存 Phi 确实被拒绝；没有把跳过检查当作完整内存链验证。

全仓库 race、生成一致性检查通过。独立 race 测试并发执行八个 64 位实例的 `checkFunc → numberLines → deadcode → invalidateCFG → checkFunc`，另保留一个 32 位旧图，并再次验证调度状态。它还测试损坏图拒绝、循环日志归属、名称策略隔离和外来输入拒绝。CFG 改写后显式失效分析缓存，按原生 pass 调用约定重算。

原生 verifier 自身并非完整类型证明：上游保留了更多类型检查和值环检查的 TODO，且寄存器分配后会跳过支配关系检查。通过这些测试不能推出目标机器码运行正确。Config 构造、ssagen、完整优化和寄存器分配流水线、编译驱动及上游工具链回接仍待完成。
