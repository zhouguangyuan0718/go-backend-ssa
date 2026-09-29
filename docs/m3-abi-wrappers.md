# M3 原生 ABI wrappers

后续已接入 [原生 deadlocals](m3-deadlocals.md)，以下记录本批完成时的范围与验证。

本批从 `ssagen/abi.go` 迁入原生 `SymABIs`、汇编 ABI 信息解析与查询、`GenABIWrappers`、wrapper ABI 遍历和 IR 构造。对外增加 `ssagen.Context`，绑定已有 typecheck 图；不引入新函数模型或 ABI 描述模型。

GenABIWrappers 在 pkginit.MakeTask 后运行。这里只接入这一原生驱动阶段，尚未接入 SSA 构造、机器码生成和上游编译器替换。该文件的 Wasm import/export 机器码包装与布局函数依赖后续 ABI 分配、objw.Progs 和后端初始化，继续留在对应阶段实现；没有添加空实现。

## 输入、状态与行为

- 原生 defs/refs 映射仍在 SymABIs 中，构造函数和完整解析循环来自上游。迁移仅增加私有编译实例归属；使用 `backend.NewSymABIs()` 创建，不使用未绑定的零值。
- Ctxt、Pos、CurFunc、Debug、Target 和 Experiment 都从同一 typecheck 图读取。生成器保留原生函数 ABI、引用集合、参数、variadic 转发、尾调用、Nosplit/Dupok、linkname 属性及 cgo 导出 pragma 处理。
- `ReadSymABIs` 通过 `Context.ReadFile func(string) ([]byte, error)` 获取输入。宿主可以提供内存内容或自行选择文件读取方式，库没有默认文件系统读取；没有汇编输入时可以不调用 ReadSymABIs。
- 迁移引擎增加显式 `RedirectReadFile` 规则：按 Go 对象身份将原生 os.ReadFile 绑定到实例方法，并校验原始签名。其他文件操作仍由环境审计拒绝。原有 log.Fatalf 转入可捕获的编译错误，调试输出进入现有 IR.TraceWriter。
- 汇编重复 def/ref、未知符号查询、Go/汇编重复定义诊断，以及禁止 cgo 导出函数产生 ABI wrappers 的规则均保留原生行为。解析错误可能已处理前面的记录；生成诊断也可能已经修改图，不承诺失败后任意复用中间状态。
- GenABIWrappers 在修改目标前检查所有函数归属，HasDef 拒绝外来图符号。独立图可并发，已生成的旧 wrapper 图保持有效。

SymABIs 可以提前读取，供前端检查汇编定义；生成 wrapper 的时机仍保持在包初始化任务之后。含无 Go 函数体的汇编声明使用现有 `Flag.Complete=false`，与原生前端约束一致。

```go
backend := ssagen.NewContext(reader.Typecheck)
backend.ReadFile = readInput // 显式的 func(string) ([]byte, error)
symABIs := backend.NewSymABIs()
symABIs.ReadSymABIs("package.symabis") // 没有汇编 ABI 输入时跳过
// 前端、内联、方法包装、loopvar、pkginit 完成后：
symABIs.GenABIWrappers()
```

之后的原生 deadlocals、escape、slice、walk 和 SSA 仍待接入。[公开示例](../noder/example_test.go)与[独立消费者](../testdata/consumer/consumer_test.go)已调用此阶段；后者从公开接口生成真实函数的 ABI0 wrapper。

## 迁移与验证

原生 ssagen 路径保持为 `internal/ssagen/abi.go`，规则位于 `internal/port/rules_ssagen_abi.go`。补入原生 Attribute.IsLinkname/IsLinknameStd 查询。工具版本升为 `syncgo-m3-v15`，共 257 个迁入文件；generate/check 已通过。

本批还修正插入私有所有权字段时的 AST 位置，使字段留在原生字面量内部，避免后一个声明的文档注释被打印进前一个构造函数。过滤注释后补回仍附着于保留声明的文档，防止移动状态初始化表达式时丢失后一函数的文档，同时保留文件尾部指令的原有审计。回归先复现丢失再验证修复。额外检查 253 个既有生成 Go 文件（排除新增 ABI 文件和补入方法的 obj/link.go），去除注释及位置信息后 AST 完全相同，日志见 `_work/m3-abi-ast.log`。既有生成文件的相应差异是字段/注释位置和版本头；所有权算法及原生数据结构保持。测试覆盖相邻注释、显式输入绑定、函数值、同名局部对象、错误签名和未经授权的文件操作。

定向测试覆盖 ABI0/ABIInternal 正反向 wrapper、空参数尾调用、amd64 和 ppc64 动态链接的尾调用限制、变参、cgo 导出及 unsafe args、linkname 属性传播、关闭 regabi 时只应用定义/引用、错误输入与重复定义诊断。八实例并发和旧图保留、外来函数/符号拒绝也有回归。

原生差分比较整个函数序列、ABI/引用/pragma/属性、IR 节点与位置、参数归属、诊断和最终状态。参考读取入口仅在临时 overlay 中提供共享 Unified IR stub；ssagen 使用外部测试包，避免测试导入 noder 经 dwarfgen 形成环。

Go 1.27.1 的十种定向配置已通过：386、amd64、mips、ppc64、amd64-noregabi、ppc64-dynlink、amd64-cgo、amd64-linkname、amd64-conflict、amd64-duplicates。386/mips 保持关闭 RegabiWrappers，只比较 ABI 信息应用。这些是 IR 差分，不是相应目标机器码的执行结果。

完整 `go test -race ./...` 通过，internal/port 用时 54.101s，日志为 `_work/m3-abi-unit-verified.log`。完整双版本原生差分与升级测试通过（测试 720.00s，包 721.306s），日志为 `_work/m3-abi-source.log`；Go 1.27.0 → 1.27.1 使用同一套迁移规则，两个生成 Go 文件变化，上游工作树保持不变。完整集成测试使用 `-timeout 20m`。

下一步接入原生 deadlocals，随后继续 escape、slice、walk 与 SSA，按原生执行顺序推进。
