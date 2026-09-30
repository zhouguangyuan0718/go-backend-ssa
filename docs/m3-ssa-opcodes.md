# M3：原生 SSA opcode 与寄存器元数据

本批完整迁入 `internal/ssa/opGen.go`，包括 8409 个 opcode、143 个 BlockKind、全部 opcode 属性及各架构寄存器/参数寄存器表。`op.go` 中的原生元数据结构、`regalloc.go` 中的 regMask 运算和 `block.go` 中的 BlockKind 声明一并迁入。没有裁剪 opcode 范围，也没有替代的后端枚举。

## 分离只读查询与可变表

上游的 `Op.String/Asm/Scale/SymEffect/IsCall/IsTailCall/HasSideEffects/UnsafePoint/ResultInArg0` 直接从全局 opcodeTable 读标量字段，但 `NewConfig` 会在 shared 模式修改其中 `Op386LoweredWB` 的寄存器 clobbers。整张表不能被当作不可变对象跨实例共用，也不能给 Op 增加所属实例而改变原生整数表示和 fmt.Stringer 接口。

迁移工具新增经类型检查的 `ConstantFieldRule`：从原生数组字面量提取指定的常量标量字段，为指定查询生成不可变字符串数据及读取函数。原生查询主体只替换那次字段访问，保留签名和索引求值；完整表仍由原生初始化器在各实例中重新创建。读取使用 Go 常量，不使用 package var、全局缓存、unsafe 或进程初始化。

转换检查整个被选中的包：初始化器必须由常量及复合字面量组成；被投影字段不能被写入或取址，整个元素/数组不能通过指针、切片或其他未支持的方式逃逸，可能隐式取址的 receiver 调用也被拒绝。允许独立字段的原生访问，例如对 reg 的修改。嵌套查询从内向外转换，字段、变量和局部遮蔽通过 go/types 对象身份识别。上游违反这些条件时迁移会失败，需重新评审规则。

将大型数组简单搬进每个查询函数会产生每次调用的数组复制，本机 Go 1.27.1 汇编探针确认了这一点；因此没有将完整 opcode 表做这种局部化。整数和字符串的打包数据全部由上游常量自动计算，不在适配代码中维护第二份值列表。

## 实例状态

私有 Context 持有独立 opcodeTables 存储。opcodeTable 中的输入/输出约束切片、各架构参数寄存器切片也通过原生初始化器分别创建。寄存器结构和 regMask 运算保留原实现。

类型谓词及 `ssa.NewTypes` 使用轻量 Context，不创建 opcodeTables。这样扩展 SSA 支持不会让现有 CanSSA 调用分配后端大表。生成器对嵌入状态字段仍验证完整原生类型；错误的命名数组类型也不能绕过检查。

## 验证

- `TestNativeOpcodeQueries` 逐项核对全部 opcode 的 9 个查询与原生元数据；检查越界及零分配。
- `TestOpcodeContextIsolation` 保留旧实例，8 个独立实例并发修改 shared-mode BX clobber、嵌套寄存器约束切片、寄存器名称和参数寄存器数组；旧实例及新创建实例不受影响。
- `TestConstantFieldQueries` 对照未变换代码验证 signed/unsigned 边界、空槽、Unicode/零字节字符串、闭包、索引求值次数、类型/变量遮蔽、fmt.Stringer、嵌套读取及越界错误，拒绝写入/取址/整表逃逸/非恒定初始化等。嵌套索引回归先复现失败，再修复转换顺序；日志 `_work/m3-ssa-constant-nested-baseline.log`。
- `TestPinnedSSAOpcode` 在 go1.27.1 和 go1.27.0 对照所有 opcode 属性/查询、BlockKind、所有架构寄存器组、128 位 regMask 运算和 shared-mode clobber 修改。通过（35.19s，`_work/m3-ssa-opcodes-source.log`）。
- 完整 `go test -race -timeout 30m ./...` 通过（internal/port 63.581s，`_work/m3-ssa-opcodes-unit.log`），`syncgo check` 校验 306 个生成文件；版本 syncgo-m3-v27。
- 本机 darwin/arm64 组合查询基准消费返回值，3.163 ns/op、0 B/op、0 allocs/op（`_work/m3-ssa-opcodes-bench.log`）；用于检查查询开销，不代表整包编译性能。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test ./internal/port -run '^TestPinnedSSAOpcode$' -count=1 -timeout 10m -v
```

## 边界

本批尚未迁入 NewConfig/Func/Block/Value 构造、重写算法、Cache 生命周期或编译器回接。shared-mode 测试执行的是原生表修改，不能代替未来真实 NewConfig 的跨配置验证。下一步公开完整原生 opcode 常量，并继续完整图结构及 Config 构建的依赖闭包。
