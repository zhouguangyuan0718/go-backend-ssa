# M3：公开原生 SSA opcode 常量

公开 `ssa` 包现在提供原生 `Op`、`BlockKind`、`SymEffect` 类型别名及全部 8552 个 opcode/BlockKind 常量（8409 个 Op，143 个 BlockKind）。外部后端可以直接使用 `ssa.OpAdd64`、`ssa.OpStaticCall`、`ssa.BlockIf` 等上游名字，并调用原生属性查询。Asm 的返回值与 `gcbridge.As` 保持同一类型；没有中间枚举或编号转换。

`ssa/opGen_constants.go` 由 syncgo 与 `internal/ssa/opGen.go` 一起生成。迁移规则的 `ExportConstants` 标志让工具遍历已迁入文件中的公开常量，生成 `OpAdd64 = compiler.OpAdd64` 形式的引用。常量的命名类型、iota 数值、别名和无类型常量语义由 Go 保持，不另外维护数值清单。上游新增的公开常量会自动出现在公开包中。

生成文件纳入现有所有权记录：工具检查引用和名字冲突，删除停用规则产生的旧文件，并拒绝覆盖被手工修改过的生成文件。普通手写公开 API 文件不归这一生成规则管理。配置仍全部是 Go 代码，没有新增配置文件。

验证：

- `TestPublicConstantFacades` 从独立 module 使用生成包，验证命名类型、iota 空位、别名、布尔/字符串及大于 64 位的无类型常量；同时验证旧文件清理和手工改动保护。
- 实际独立消费者导入公开 ssa，使用上述类型/常量、Asm 返回值及查询方法，通过（5.11s，`_work/m3-ssa-public-consumer.log`）。
- go1.27.1/go1.27.0 重新生成并检查公开常量引用，完整 opcode 差分仍通过（35.72s，`_work/m3-ssa-public-source.log`）。
- `go test -race -timeout 30m ./...` 通过（internal/port 63.886s，`_work/m3-ssa-public-unit.log`）；`syncgo check` 验证 307 个生成文件，版本 syncgo-m3-v28。

本批没有改动已有生成算法的主体。公开 opcode 仍不是源码到 SSA 的构造入口；下一步继续原生 Func/Block/Value、Config 和构造流程，编译器替换尚未完成。
