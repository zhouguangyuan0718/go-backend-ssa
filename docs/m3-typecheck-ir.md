# M3 原生 IR 表达式与语句检查

本批将固定 go1.27.1 的 typecheck 生产源码补齐，包括 const、dcl、expr、func、iimport、stmt、subr、typecheck 等文件。源码继续通过 typed migration 生成，目录、文件名、原生节点和改写算法保留；没有另写表达式检查器。mkbuiltin.go 是上游生成工具，不属于迁入的生产包。

## 对外能力与调用方式

公共 `typecheck.Context` 现在提供原生 `Expr/Stmt/Exprs/Stmts/Call/AssignConv/DefaultLit`、`DeclFunc/FinishFuncBody/TempAt`、嵌入成员查找及闭包捕获布局。`ClosureStructIter` 和 `RangeExprType` 同时从公共包导出。初始化沿用 [已有实例装配](m3-typecheck.md)：IR、types 和 Link 共用位置表，再调用一次 `InitUniverse`。

调用者必须使用返回的节点：原生检查可能将 OCALL 改写为内建专用节点，将接口赋值变为 OCONVIFACE，或补出嵌入字段选择链。`Exprs/Stmts` 则直接更新传入 slice。输入是原生 IR，通常应由 noder reader 构造；这里并未增加 AST 到 IR 的替代模型。

`DeclFunc(fn)` 按上游规则声明形参，将函数加入当前 `Target.Funcs` 并压入函数声明栈。完成函数体后必须调用 `FinishFuncBody()`，随后可用 `CheckFuncStack()` 检查栈是否平衡。同一实例的整个 IR/typecheck 生命周期由调用方串行；不同实例可以并行。`TempAt` 创建真正的函数局部 Name，保留原生符号编号、逃逸和使用标记。

## 状态移出全局

| 原生状态 | 当前归属 |
| --- | --- |
| Target、函数声明栈 | typecheck.Context |
| autotmpnames 与锁 | typecheck.Context |
| dotlist、slist | typecheck.Context |
| typeSymIdx、HaveInlineBody | typecheck.Context |
| traceIndent、诊断细化记录 | typecheck.Context |
| types 类型表、当前源码位置、布局和 tracing 标记 | 共享的 IR.Types |
| CurFunc、IR 构造和诊断 | 共享的 IR Context |
| Flag、Timer、链接位置表 | 共享的 IR.Base/Link |
| typeSet、ClosureStructIter | 原生对象附带当前 Context |

`ptrSize` 是由现有 Universe 目标布局初始化的私有整数，仅供原生闭包迭代器跳过函数入口地址，不读取宿主架构。原生 `_typekind` 只读表局部化，不保留包变量。`HaveInlineBody` 保持上游的“尚未绑定时失败”回调，后续由 reader/inliner 绑定，没有用恒假回调掩盖未接入能力。

## 诊断边界

原生 Errorf/ErrorfAt 进入已有 IR.Diagnostics，保留错误码、相对文件位置和格式化消息；原生 Errors 同时观察 IR 和 types 的布局诊断。选择器错误细化通过比较最后一条快照再更新，之前取得的快照保持不变。Fatalf 仍转为 panic；普通检查错误留在结构化诊断中。

这是库宿主适配：未复刻命令行编译器的 stderr 缓冲、重复错误过滤、十条错误后退出或删除输出文件策略。也不承诺任意 panic 后可继续复用实例。直接修改原生图时仍须遵守实例归属与上游 IR 不变量；所有接口参数尚未具备完整跨实例图校验。

## 验证范围

- 真实函数声明、形参/临时变量、加法与赋值/return 的类型检查和节点标记，嵌套声明栈恢复。
- 普通函数调用及 len/cap/append/make/new、map/slice 索引、默认常量类型、接口转换。
- 嵌入字段选择链和提升方法集合；类型 switch 重复接口 case 的实例错误收集。
- 按值/按引用闭包捕获布局，导入类型和指针类型索引。
- 12 个交错的 32/64 位实例并发运行，之后继续读取原来的函数体、类型、符号和索引。
- 外部 consumer 仅导入公共包调用真实 Expr 和 RangeExprType。
- 相同有效 IR 操作同时在未迁移的官方编译器与库中执行，比较格式、opcode、类型、标记、声明关系、嵌入成员及捕获布局。无效输入的库诊断行为另行回归，不把原生 Fatalf 退出当成可恢复行为。

完整生产包可构建不代表每个语言构造已验证。reader、walk、escape、SSA 构造与原生工具链替换尚未完成；本批也不输出机器码或运行编译产物。

本批验证结果：`syncgo check` 核对 184 份产物；`GO_SSA_SOURCE_REPO=... GOTOOLCHAIN=go1.27.1 go test -race -v ./...` 全部通过。官方 go1.27.0 与 go1.27.1 均通过相同 IR/typecheck 快照；同一套规则跨两版仅 1 份 Go 产物变化，来源工作树保持不变。日志：`_work/m3-typecheck-ir-tests.log`。
