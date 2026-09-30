# M3 原生比较构造

完整迁入 `internal/compare/compare.go`，保留上游的类型判定、连续内存段分析、比较成本、字符串/接口/结构体比较 IR 构造。工具版本 `syncgo-m3-v20`，仍为 271 个生成文件；本批扩展已有文件，没有新增生成器机制或配置文件。

## 实例绑定与对外入口

`compare.IsRegularMemory(t)`、`Memrun(t, start)` 和 `EqStructCost(t)` 保持原生函数签名，从输入类型取得 Universe 和已绑定的 Link。IsRegularMemory 仍可用于没有 Link 的类型实例；成本和内存段分析需要目标架构输入。EqCanPanic 保留原生无状态递归。

构造比较节点使用 `compare.NewContext(tc)`，复用现有 typecheck、IR、类型表、位置表和运行时声明。公开 Context 是实现类型的别名；没有新的比较 IR、算法或共享缓存。调用方设置当前函数及位置，完成运行时声明初始化，并遵守原生输入/阶段约束。同图修改串行，独立图并发。

- EqString 返回长度相等判断和 memequal 调用。必须先判断长度并短路求值。
- EqInterface 返回类型/itab 相等判断和 efaceeq/ifaceeq 调用。必须先判断类型/itab 并短路求值。
- EqStruct 返回有序条件列表及是否可能 panic。只允许在原生认可的区间内部重排；接口比较前后的顺序保留。空白字段跳过。
- Memrun 和成本计算保留上游对字段填充、目标对齐、寄存器宽度和 CanMergeLoads 的处理。调用方须先完成结构体布局。

构造入口在修改前拒绝其他编译图的节点/类型。原生类型查询直接使用输入的 owner，不依赖最近创建的 Context；旧图不被后续编译重置。

## 固定版本的已知边界

本次真实测试发现：直接向 EqStruct 提供高成本内存段，会走入原生 eqmemfunc 的类型替换路径；Go 1.27.1/1.27.0 的 memequal 声明已经使用 unsafe.Pointer，没有对应占位类型，因此原生报 `SubstArgTypes: too many argument types`。库保留相同失败。

上游 walk 在决定调用 EqStruct 前检查 EqStructCost ≤ 4，这条高成本内存段路径不由该正常调用链进入。本库不将 EqStruct 扩大为任意结构体都可调用的高级比较 API，后续 walk 必须保留这个原生门控。测试在临时原生进程和库进程中分别确认失败，不替换 Fatalf、不修补参考算法，也不把预期失败统计成正常 IR 构造成功。

## 验证

源码测试从 syntax/types2/noder 读取真实结构体、字符串和接口函数，再运行原生比较构造。覆盖命名字符串、常量字符串长度选择、空/非空接口、字段填充、空白接口字段、嵌套结构体和接口数组、成本分组与 panic 顺序；另验证架构输入、八实例并发、保留旧图、无 Link 的既有类型判定、外来节点拒绝和已知失败。

独立 module 消费者已经通过公共 compare 入口生成原生字符串比较节点。固定 Go 1.27.0/1.27.1 的对照包含 386、amd64、mips、ppc64、amd64-aligned、amd64-nomerge 六组正常 IR 快照，以及单独的 amd64-large 失败一致性检查。比较完整条件 IR、比较成本、内存段和 panic 标记。它们是 IR/元数据验证，尚不是可执行程序的比较语义验证。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test -timeout 5m ./internal/port -run '^TestPinnedCompare$' -v
```

双版本对照通过（57.35s，`_work/m3-compare-source.log`）；完整普通 race 回归通过，internal/port 用时 55.840s（`_work/m3-compare-unit.log`），补充嵌套结构/接口数组后定向 race 回归再次通过（`_work/m3-compare-focused.log`）。generate/check 均验证 271 个文件；269 个既有生成 Go 文件中，仅 compare 的算法主体扩展，另外 268 个仅工具版本头变化。本批执行普通全量 race 回归和新增比较切片的双版本对照，未重复完整前端双版本矩阵。

下一步绑定 walk 的原生架构字段和零值静态调度器，再接入完整 walk 排序、表达式与语句改写，随后推进 SSA 构造和原生编译器回接。

后续进展：walk 架构字段绑定及首批目标判定已迁入，见 [walk 架构输入](m3-walk-target.md)。
