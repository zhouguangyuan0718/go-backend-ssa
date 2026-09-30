# M3 walk 依赖：原生 CanSSA

迁入 `ssa/value.go` 的完整 CanSSA 函数，以及 `ssa/decompose.go` 的 MaxStruct 常量。两者保留上游文件名和原生算法，由现有选择、ContextFrom、StateRule 和 CallRule 生成。工具版本 `syncgo-m3-v19`，共 271 个生成文件；没有新增生成器机制或配置文件。

对外使用 `ssa.CanSSA(t)`。目标指针宽度、SIMD 实验开关、类型布局和直接接口判定，全部来自 `t.Universe()`。适配只在调用内取得已有 Universe，不维护可变缓存，不要求用户额外创建 Context。类型尺寸计算仍会写入所属图，因此同图操作由调用方串行；不同图可并发。

这个函数决定原生值是否适合 SSA 表示，供 walk 的静态复合字面量和临时变量处理使用。它不是源码到 SSA 的构造入口，也不表示 SSA 图、优化或机器码生成已完成。

回归覆盖 32/64 位大小限制、单元素/多元素数组、零大小值、嵌套结构、四/五字段边界、可直接放入接口的多字段结构，以及 SIMD 开关下按寄存器数量判定的放宽条件。内部 SSA 向量特殊类型也与上游逐项比较，不能据此推断语言层 SIMD 的调用约定或运行时正确性。

独立 module 消费者通过公开 API 查询切片和数组；并发回归保留旧图并交错运行不同目标。原生对照在 Go 1.27.1 和 1.27.0、386/amd64/mips/ppc64 上运行，相同输入比较判定、尺寸、对齐和整数/浮点寄存器数，每种目标同时覆盖 SIMD 开关。

定向对照可独立复现，也已加入完整升级测试：

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test -timeout 5m ./internal/port -run '^TestPinnedSSAValue$' -v
```

两版四目标定向差分通过（39.56s，`_work/m3-canssa-source.log`）。完整 `go test -race -timeout 20m ./...` 通过，internal/port 用时 56.481s（`_work/m3-canssa-unit.log`）；补充的旧图实验开关隔离回归也通过 race（`_work/m3-canssa-isolation.log`）。generate/check 均验证 271 个文件，267 个既有生成 Go 文件仅工具版本头变化。上一阶段完整流水线验证见 [slice 记录](m3-slice.md)；本阶段单独验证新增谓词及全部普通 race 回归，未再次运行完整流水线双版本矩阵。

下一步继续迁入 walk：补齐 compare 的原生比较构造、架构输入绑定和零值静态调度器的实例归属，再连接原生排序及表达式/语句改写。

后续进展：原生 compare 整文件已迁入，见 [比较构造记录](m3-compare.md)。
