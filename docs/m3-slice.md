# M3 原生 slice 优化

完整迁入 Go 1.27.1 `slice/slice.go`，这是该包唯一的生产文件。保留原生 sliceInfo、独占性分析、循环权重和转移节点插入算法，目录保持 `internal/slice`。执行顺序为 pkginit → ABI wrappers → deadlocals → escape → slice；walk、SSA 构造和编译器回接仍待实现。

## 对外入口与状态

```go
analysis := escape.NewContext(reader.Typecheck, reader.Inline.Log)
analysis.Funcs(reader.Typecheck.Target.Funcs)
reader.Typecheck.IR.Debug.VariableMakeThreshold = 64
slice.NewContext(analysis, reader.ReflectData).Funcs(reader.Typecheck.Target.Funcs)
```

阈值由宿主显式提供，单位与上游相同，表示候选元素大小的字节上限。分析只在能保持语义、且原生成本条件满足时，将 append 标为不逃逸，并在唯一的转移位置前插入原生 MoveToHeapExpr。容量可能被观察时，保留 PreserveCapacity 和 append.UseBuf；运行时类型引用由既有 reflectdata 实例提供。参数是否泄漏由 escape 的原生参数摘要判定。

Context 只绑定已有 escape 和 reflectdata 实例，不新建分析状态模型。Flag、Debug、VariableMakeHash、节点构造和 AST 调试输出使用同一 IR 实例。分析的临时 sliceInfo 原本就在函数内部，执行后按原生逻辑清理 Name.Opt。

构造时拒绝来自不同编译图的依赖；Funcs 在修改前检查全部函数归属。同一图的可变操作需串行，独立图可并发；后续实例不会重置旧图。按编译阶段运行一次，不承诺重复执行的幂等性或失败回滚。

公共包 `slice` 暴露 Context 别名和构造函数。公开 noder 示例、独立 module 消费者已经串接此阶段；消费者检查真实源码产生的 MoveToHeapExpr、类型引用和节点归属。

## 移植与验证

规则位于 `internal/port/rules_slice.go`，适配位于 `internal/slice/adapter_context.go`。沿用已有 StateRule、CallRule 和入口归属检查，生成器没有新增转换机制或配置文件。工具版本 `syncgo-m3-v18`，生成文件总数 269。

源码回归覆盖循环 append、容量观察、命名返回、赋值/range 转移、零大小元素、静态函数/方法参数摘要，以及别名、间接调用、元素地址、多转移、低收益和元素过大的保守排除。另验证 -N、hash 禁用、元素阈值、八实例并发、旧图保留、实例调试输出和跨图输入拒绝。

固定 Go 1.27.1 的定向差分通过九组配置：386、amd64、mips、ppc64、amd64-noopt、amd64-small、amd64-exclude、amd64-all、amd64-inline（35.63s，`_work/m3-slice-probe.log`）。相同源码包含泛型实例；比较完整节点、位置、参数摘要、append 标记、MoveToHeapExpr 的类型引用/容量标记、临时状态清理和最终位置状态。测试检查启用配置确实产生转移，禁用配置没有转移；内联配置必须实际产生内联 IR。

参考算法直接读取固定 Git 对象，通过临时 overlay 执行，上游工作树保持只读。本阶段验证的是 IR 改写与元数据，尚未执行 move2heap 的 lowering 或机器码，不能据此声称已完成栈分配的运行时验证。

完整 `go test -race -timeout 20m ./...` 通过，internal/port 用时 67.399s（`_work/m3-slice-unit.log`）。完整双版本差分及升级测试通过，测试用时 851.28s（`_work/m3-slice-source.log`）；Go 1.27.0 与 1.27.1 的九组 slice 配置均一致，同规则升级有两个生成 Go 文件变化，上游工作树保持不变。generate/check 均验证 269 个文件。额外逐文件比较确认，266 个既有生成 Go 文件除工具版本头外完全相同。

## 下一步

先接入 walk 所需的原生 CanSSA 类型判定与架构输入，再迁入完整 walk 生产包。walk 的预分析缓存、运行时字段缓存、select 类型和接口计数器归实例；已有 typecheck/IR/reflectdata/staticinit 的数据继续共享。随后连接 SSA 构造、对外结果及原生后端消费，最终以实际替换后的编译器构建、可执行程序和自举作为验收。

后续进展：CanSSA 已迁入，见 [walk 类型表示判定依赖](m3-canssa.md)。
