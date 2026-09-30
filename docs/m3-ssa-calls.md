# M3：原生 SSA 调用、写屏障和循环抢占检查

本批以官方 `go1.27.1` 为基线，通过 `syncgo-m3-v42` 完整迁入 `expand_calls.go`、`writebarrier.go`、`loopreschedchecks.go` 和原生 `Func.NewLocal`。这些阶段在已有 SSA 图上运行，并与泛型改写、目标 lowering 组合验证。完整 Compile、调度、寄存器分配、源码到 SSA 和替换/自举验收仍待后续完成。

## 算法与编译实例

调用展开保留原生寄存器游标、参数/返回值拆分、栈传递、间接调用和尾调用处理。参数类型及 ABI 信息使用同一编译实例的 types 和 ABI 服务，展开后执行原生 `postExpandCallsDecompose`。

写屏障保留原生已知零区域分析、屏障需要判定、存储排序与分组、每次最多八个指针的缓冲申请、旧/新指针记录、Move/Zero 和 CgoCheck2 路径。类型符号直接使用现有 reflectdata 服务，runtime 符号通过原生 Frontend.Syslook 契约提供。可能被调用参数覆盖的 Move 源，按上游算法先复制到真实 IR 临时变量。

循环抢占检查保留原生回边分析、内存定义传播、Phi 插入与修复、栈界限比较和 `runtime.goschedguarded` 调用；NoSplit 仍使用原生提前返回条件。

`Func.NewLocal` 继续调用原生 `typecheck.TempAt` 并设置 NonMergeable。适配器先检查函数 IR 和类型的实例归属，再调用类型检查服务；外来函数/类型被拒绝时不改动函数声明列表。未新增 SSA 表示、优化算法或虚构的临时变量构造过程。

## 验证

全量 `go test -race -count=1 ./...`（含独立消费者）通过；`syncgo check` 验证 388 个生成文件一致。

`TestPinnedSSACalls` 对 `go1.27.1` 与 `go1.27.0` 的原生源码分别重生成、构建，在 `386/amd64/arm64/mips/ppc64` 五种布局上运行相同输入。两个源码版本均使用固定 Go 1.27.1 构建工具链。10 组逐阶段图与元数据快照全部一致，覆盖：

- ABI0/ABIInternal，整数、字符串、slice、interface、complex、struct、空/单元素数组的输入与返回值，以及不可放入 SSA 的大数组传参和结果存储。
- 超过整数/浮点参数寄存器容量的参数列表，以及 Static、Closure、Inter、Tail、TailInter 五种调用形式。
- 连续指针存储、Move/Zero、栈目的地址、关闭屏障、易被覆盖的栈源，以及 CgoCheck2 开关。
- 带内存 Phi 的循环、无写入的循环、没有初始内存链的循环和 NoSplit。

样例使用真实 IR 函数、DeclareParams 和 ABIAnalyze，而非无关联的参数名称。执行原生 verifier、相关转换、泛型改写和目标 lowering 后，再执行 `checkLower`。

独立单元检查另外验证：

- 24 个不同 64 位整数经过调用展开后的寄存器值，以及大小端栈参数的每个字节和无重叠写入。
- 写屏障开关两条路径的实际写入；启用时缓冲分批为 8+3，记录一个共享新指针和十个不同旧指针。
- 已知零区域的精确范围、对齐、内存版本与新指针来源边界。
- 循环快慢路径在 0、1、2、37 次迭代中的写入顺序、终止行为和调度调用次数。
- 临时变量的归属、声明和标志，拒绝外来输入后的可用性，以及并发实例与保留的旧实例互不影响。
- 独立消费者仅通过公开类型和方法构造原生临时变量。

这些字节和内存效果检查使用测试内解释器；本批未执行产出的机器码或真实 GC/调度器。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test ./internal/port -run '^TestPinnedSSACalls$' -count=1 -v
GO_SSA_SOURCE_REPO= GOTOOLCHAIN=go1.27.1 go test -race -count=1 ./...
GOTOOLCHAIN=go1.27.1 go run ./cmd/syncgo check -repo /path/to/go
```
