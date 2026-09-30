# M3：原生 SSA 类型集合与位置

继 ABI 参数布局之后，本批迁入 `ssa/config.go` 中的 `Types`、`NewTypes`、`SetTypPtrs`、`Logger`、`Frontend`，以及完整 `ssa/location.go`、`ssa/id.go`。目录和声明保持上游形式，没有引入另一套 SSA 或后端接口。固定 go1.27.1；升级对照仍为 go1.27.0。

## 实例所有权与公开入口

公开 `ssa.NewTypes(u)` 接受已经完成 `typecheck.InitUniverse` 的 Universe，原生初始化算法把标量、指针、向量和 mask 类型绑定到该实例。指针继续使用 Universe 的原生指针缓存。

`Types` 增加私有的所有权引用，供原生 `SetTypPtrs()` 使用。Config 后续将按上游形式内嵌 Types 值；值复制会保留所有权，重新初始化时不会转向其他编译实例。零值 Types 不具备所属实例，必须从 NewTypes 创建。入口预检 Universe 的原生类型，错误输入在字段写入和指针缓存操作前被拒绝。已有 CanSSA 仍直接从参数 Type 获得所属 Universe。

`ssa` 同时公开原生 Logger、Frontend、Location、Register、LocalSlot、LocPair、LocResults、Spill、ID 的类型别名。Frontend 方法及其参数、返回值保持上游声明；LocalSlot 直接保存原生 ir.Name/types.Type 和分解关系，未复制成摘要。

## 验证

- `TestNativeTypeCollection`：完整原生字段、规范类型身份、Types 值复制和 SetTypPtrs 恢复。
- `TestTypeCollectionIsolation`：保留 32 位集合，同时运行 8 个独立 64 位实例，race 下验证类型和指针缓存不串用。
- `TestTypeCollectionRejectsUnboundInputs`：nil、未初始化 Universe、零值集合、跨实例标量及向量输入；失败前不修改集合。
- `TestPinnedSSATypes`：在 go1.27.1 和 go1.27.0 分别对照 386、amd64、mips、ppc64 布局，检查全部原生字段的类型/大小/对齐/寄存器数/身份，位置和分解关系、寄存器映射、Spill，以及原生 ID 分配/溢出行为。37.41s 通过，日志 `_work/m3-ssa-types-source.log`。
- 独立 module 从公开 ssa 包构建集合和位置，通过 gcbridge 消费者测试；全量 `go test -race -timeout 30m ./...` 通过（internal/port 61.417s，日志 `_work/m3-ssa-types-unit.log`）。
- `syncgo check` 验证 290 个生成文件；工具版本 syncgo-m3-v25。未修改此前已生成算法主体。本批未重跑上一批 walk 已通过的完整前端双版本矩阵。

运行定向上游对照：

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test ./internal/port -run '^TestPinnedSSATypes$' -count=1 -timeout 10m -v
```

## 后续依赖与当前边界

尚无 Config/Func/Block/Value 构建入口，也不能替换上游编译器。下一步迁移完整 opcode/register 元数据及其汇编常量依赖，再接 Config 的真实架构重写依赖闭包。

源码审计确认两个必须在接入时处理的生命周期问题：NewConfig 会修改 opcodeTable 的 386 shared-write-barrier clobbers，不能在不同编译实例间共享这项可变状态；Cache.Reset 会清空已分配的 Value/Block，公开返回并保留的 SSA 图不能复用同一 Cache。后续实现需用不同 shared 配置及保留旧图的测试证明隔离。
