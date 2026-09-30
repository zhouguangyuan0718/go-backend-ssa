# M3 原生 ABI 参数布局

本批完整迁入 `internal/abi/abiutils.go`，作为后续 SSA Config/Func 的直接依赖；这是编译器参数分配算法，和此前 ssagen 中已迁入的 ABI wrapper 生成是不同步骤。

对外提供 `abi.NewContext(graph)`。Context 的 NewABIConfig、RegisterTypes，以及原生 ABIConfig/ABIParamResultInfo/ABIParamAssignment 方法保留原生参数分配、栈偏移、溢出区、寄存器类型分解和 padding 算法。没有新的 ABI 模型或手写寄存器分配器。

## 状态与对象归属

原生 synthOnce、synthSlice、synthString、synthIface 转为 Context 成员。32/64 位的合成头部使用当前 IR 图的 Universe，不会被另一目标的首次调用固定。每个实例仍按上游 setup 的顺序初始化，保留 sync.Once 的原始用途。

ABIConfig 和 ABIParamAssignment 增加所属 Context。ABIParamResultInfo 已通过其原生 config 字段保留归属，无需第二个 owner。私有 assignState 只传入处理本次函数的 Context，不改原生寄存器余量与栈游标结构。

参数类型列表和签名中的 Name 在分析前完成归属检查；错误输入不会先初始化缓存或更新前面参数的 FrameOffset。参数对象的公开 Type/Name 字段若被改为外来对象，后续寄存器分解也会拒绝。分析结果的类型、符号、Name 和所属编译图保持一致。

## 调用示例

```go
layout := abi.NewContext(graph)
config := layout.NewABIConfig(integerRegs, floatRegs, fixedFrameSize, whichABI)
info := config.ABIAnalyzeFuncType(signature)
registerTypes := layout.RegisterTypes(info.InParams())
```

`ABIAnalyzeTypes` 接受类型列表；`ABIAnalyzeFuncType` 包括 receiver、参数和结果；`ABIAnalyze(signature, true)` 还按原生约定更新 Name.FrameOffset 及寄存器返回标记。同图中的这些操作串行进行。寄存器数量和 frame size 由宿主原生架构配置提供，这些接口不会推断目标寄存器集合。

## 验证与边界

- 普通回归覆盖聚合参数寄存器不足时整体回退到栈、后续标量继续使用剩余寄存器、参数/返回值独立分配、零宽参数、padding、string/slice/interface 分解以及 32 位 int64 拆分。
- 八个并发 64 位实例与保留的 32 位结果交错，验证旧结果、合成类型和目标布局未变化。
- 外来类型、签名 Name、参数分配和 FrameOffset 结果归属在使用前被拒绝。
- 独立 module 只通过公开包完成 ABI 布局查询。
- Go 1.27.0/1.27.1 源码对照覆盖 386、amd64、mips、ppc64 布局，四组显式寄存器预算/固定 frame size，嵌套结构体/数组、接口、复数、SIMD 类型，以及实际 Name 偏移/返回寄存器标记。

对照是算法布局验证，不是四种架构机器码执行。测试还特意给 32 位布局设置正的浮点寄存器预算；这种人工组合不对应当前上游 32 位架构的参数 ABI。原生 appendParamTypes 在部分结构体中将 64 位浮点分解为两个字，而分配器记录一个浮点寄存器，导致 ComputePadding 的内部数量检查 panic。两版来源和移植代码行为一致；测试记录并比较这个精确失败，没有修复或绕开原生算法。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test -timeout 5m ./internal/port -run '^TestPinnedABI$' -v
```

定向双版本对照通过（31.36s，`_work/m3-abi-source.log`），独立 module 通过（`_work/m3-abi-consumer.log`）。`syncgo check` 验证全部 287 个生成文件，完整普通回归 `go test -race -timeout 30m ./...` 通过（internal/port 71.477s，`_work/m3-abi-unit.log`）。本批未重复上一批已通过的完整前端双版本流水线；本批没有更改已有生成代码的算法主体。

工具版本 syncgo-m3-v24，共 287 个生成文件。后续继续迁入原生 SSA Config、Func、Block、Value 及构造依赖；当前仍无完整 SSA 图构造或上游编译器替换能力。
