# M3 SSA 值改写、常量缓存与类型比较

迁入完整原生 `value.go`、`block.go`、`copyelim.go`，以及 Func 的常量构造/缓存、值和块回收、`spSb`、`LogStat`。此前仅为图构造和诊断选取的 Value/Block 方法现在由整文件迁移覆盖。

常量缓存继续使用上游的每函数 map、特殊常量 magic key、opcode 和 `Type.Compare` 匹配。浮点常量按位缓存，保留正负零和 NaN payload。值 reset/copy/invalidate/free 使用原生缓存清理和引用计数顺序；空闲链复用原生 ID。`copyInto` 和 `copyIntoWithXPos` 仍先构造副本，再检查内存参数并报错，未把检查提前而改变失败状态。

`copyelim` 保留先执行 `phielim`、再更新控制值与命名值的流程，包含零大小值转为 `OpEmpty`、重复 Phi 输入消除、自引用 Phi 保留、Copy 链压缩和不可达 Copy 环的终止处理。它不负责从块的 Values 列表删除死值，后续仍需原生 deadcode pass。

## 类型与实例归属

常量缓存依赖原生 `Type.Compare / Type.cmp / Sym.cmpsym / cmpForNe`，本阶段迁入这些完整比较算法，并公开原生 `Cmp` 和三种比较常量。原生 nil、自身比较、byte/rune/any 别名、复合类型、符号排序和 SSA 特殊类型处理均保留。

生成规则新增受审计的 `ContextAfterGuards`：只有指针参数/接收者的身份或 nil 比较，以及常量返回，才可以放在实例校验之前。Type.cmp 的三个原生提前返回因此不会先解引用 nil 接收者；正常比较仍校验类型归属。`StatelessFunctions` 让 `cmpForNe` 保持无状态的包级函数；上游如果为它增加编译状态依赖，生成器会拒绝，不能静默改变接口。

Value 的 `LackingPos` 从现有 Func.Config.Types 获取编译实例，无 Block 的已初始化 Value 则使用自身类型归属。没有新增 Value/Block/Func 所有者字段；已归属图中的外来类型被拒绝。

## 验证

`TestPinnedSSAMutations` 纳入完整升级套件，并可独立执行。它在 Go 1.27.1 和 Go 1.27.0、386/amd64/mips/ppc64 四种布局上比较原生输出：

- 类型比较矩阵：nil、别名、命名类型、不同包的符号、结构字段/tag/embedded、非空接口、函数接收者和 variadic、数组/map/channel、tuple/results 和 SSA 特殊类型。
- 全部常量构造器、重复构造身份、特殊 key 碰撞、浮点位模式、结构相等但身份不同的类型，以及缓存损坏的原生诊断。
- 复制的位置标记、内存参数报错后的副本/引用计数、reset、递归失效、值/块复用和非法释放诊断。
- 前驱与 Phi 参数删除、分支交换、Copy/Phi 消除、命名值和控制值更新、自引用 Phi、不可达 Copy 环。
- 寄存器读取、spill slot 查询及失败诊断、MemoryArg、LackingPos、removeable、AddrSinkArg、sp/SB 复用和统计输出。

图改写测试同时复算 Uses 并检查反向 CFG 边。寄存器查询使用显式 location 输入，不代表已经运行寄存器分配；部分操作属性探针不是完整指令，不声称通过 SSA verifier。四种目标布局均在宿主进程内模拟，不是目标机器码执行验证。

独立测试并发操作八个 64 位编译实例并保留旧的 32 位常量图，验证缓存和类型归属隔离；类型比较另检查反对称、传递性、别名等价和外来实例拒绝。规则测试覆盖提前返回顺序、状态访问、外来参数，以及不纯条件/返回/分支和 helper 新增状态依赖的拒绝。全仓库 race、独立 consumer 和生成一致性检查通过。

## 下一步边界

本阶段完成了 SSA 图的原生值操作闭包和第一个完整 Copy/Phi 清理 pass。Config 构造、完整 SSA 校验/优化流水线、ssagen 源码到 SSA、统一编译驱动及官方编译器回接仍未完成；不能据此声称可以替换上游编译器。
