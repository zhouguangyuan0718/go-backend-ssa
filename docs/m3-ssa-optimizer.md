# M3：原生 SSA 优化阶段

本批基于官方 `go1.27.1`，使用 `syncgo-m3-v41` 迁入已有 SSA 图的优化算法。`Compile`、调用展开、写屏障、调度、寄存器分配和源码到 SSA 的入口仍未接通；这里验证的是明确列出的优化阶段及其与已迁入 lowering 的组合。

## 迁入范围

完整迁入 `branchelim.go`、`checkbce.go`、`cse.go`、`deadstore.go`、`fuse.go`、`fuse_branchredirect.go`、`fuse_comparisons.go`、`known_bits.go`、`memcombine.go`、`phiopt.go`、`sccp.go`、`shortcircuit.go`、`uses.go`、`zcse.go`、`cpufeatures.go`、`lca.go`、`prove.go`、`loopbce.go` 和 `downward_counting_loop.go`。

另迁入：

- `nilcheckelim` 及 `schedule.go` 中原生 `storeOrder/valuePosCmp` 依赖；调度后的 `nilcheckelim2` 留待后续阶段。
- `poset.go` 的完整关系推理、别名合并、检查点、撤销和完整性检查，以及 bitset。没有被这些阶段调用的 `DotDump` 文件诊断未迁入。
- `Func.DebugHashMatch`、偏序关系缓存的 `Func.newPoset/retPoset`；哈希筛选直接绑定当前 IR 的 base 服务。
- `compile.go` 的原生 `checkEnabled` 默认值，用于 `factsTable.cleanup` 检查。

所有阶段保留上游算法、操作码、原生图与缓存布局。未添加新的 SSA 表示或替代优化算法。

## 状态和初始化

原生不可变的数字、字符串与操作码查找表通过已有规则局部化；`domainRelationTable`、`checkpointBound`、`debugPoset` 和 `checkEnabled` 保留原生初始化表达式，存于编译 Context。CSE 的直接输出绑定实例 TraceWriter；编译器诊断仍调用原生 Frontend 契约。

`factsTable` 的状态相关私有方法和 `poset` 的状态相关方法显式传入本次编译 Context，缓存中不增加旧 Context 指针。这样，空偏序关系缓存可以按原生契约复用。`Cache.Reset` 仍会使其承载的旧函数图失效；需要保留旧图时，应使用独立 Cache。

本批只选取 `compile.go` 的 `pass` 声明和 `checkEnabled`。该文件的 `init` 验证未迁入的 `passes/passOrder` 表，不初始化 `checkEnabled`。选择规则显式使用 `DeferInit`，生成记录包含 `deferred-unselected-init`；该规则禁止用于整文件迁入或与 `Init` 同时使用，并拒绝 `init` 直接或经本文件 helper、变量初始化表达式引用所选状态和函数的情形。其他文件的副作用仍需显式审查。将来迁入完整阶段表时，必须恢复原生阶段顺序验证。

## 验证

本批实际执行结果：两个源码版本 × 五种目标布局的 10 组差分全部通过；全量 `go test -race -count=1 ./...`（含独立消费者）通过；`syncgo check` 验证 386 个生成文件一致。

`TestPinnedSSAOptimizer` 对官方 `go1.27.1` 和 `go1.27.0` 分别重生成，并在 `386/amd64/arm64/mips/ppc64` 五种目标布局上比较逐阶段图、日志和 CPU 特征。两个源码版本均使用固定 Go 1.27.1 构建工具链。覆盖：

- 交换律和有序运算 CSE、相邻与不相邻内存定义后的 Load CSE、零参数 CSE。
- 完整及部分覆盖的死存储、局部变量清理与 `VarLive` 保留、大小端内存合并。
- 已知位常量折叠、SCCP 的可达 Phi 输入、条件选择、短路 Phi、早期范围合并和后期分支重定向。
- 有符号和无符号比较的传递关系、归纳变量、向下计数循环、冗余 nil 检查。
- CPU 特征探测的正反分支与汇合交集。

常规图在阶段执行前后运行原生 verifier，并继续经过复合类型拆分、泛型改写和目标 lowering，最终运行 `checkLower`；CPU 特征探测案例单独比较各块和函数的特征集合。局部整数解释器和字节存储解释器另验证优化后的值与内存语义，不把图差分等同于机器码运行。

额外单元测试包括所有 8 位区间的单目运算边界、8/16/32/64 位算术样本、已知位组合枚举、偏序关系的具体模型与嵌套撤销、比较分支的独立求值、并发实例及旧图保持、空偏序关系缓存跨编译复用。迁移器测试覆盖显式延后无关 init 的范围、默认拒绝行为及直接/间接依赖拒绝。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test ./internal/port -run '^TestPinnedSSAOptimizer$' -count=1 -v
GO_SSA_SOURCE_REPO= GOTOOLCHAIN=go1.27.1 go test -race -count=1 ./...
GOTOOLCHAIN=go1.27.1 go run ./cmd/syncgo check -repo /path/to/go
```

完整 SSA Compile、前端 ssagen、统一驱动和上游替换/自举验收仍是后续工作。
