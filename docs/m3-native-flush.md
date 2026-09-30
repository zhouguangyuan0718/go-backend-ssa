# 原生函数 Flush、PC 表和 DWARF

本阶段把 `objw.Progs.Flush` 接到完整的上游 `obj.Flushplist`。前一阶段只在测试观察器中运行到汇编；现在生产代码依次执行指令整理、分支修正、目标预处理、机器码汇编、PCLN 和 DWARF。尚未把这些符号序列化为可交给链接器的对象文件。

## 原生实现与实例边界

- `cmd/internal/obj/{plist,pcln,line,inl,dwarf}.go` 保留原生函数体，包含栈深、文件、行号、内联 PC 表、行号程序、函数 DIE、位置及范围表和内联修正。
- `cmd/compile/internal/dwarfgen/{dwarf,scope,dwinl}.go` 保留变量选择、SSA 位置列表、闭包偏移、词法作用域、内联范围及抽象函数生成；原有 ScopeMarker 继续共享同一位置表。
- `cmd/internal/dwarf` 的编码器使用每个 Link 独立的 `Emitter`。缩写表、目标相关缩写展开、七位编码缓冲区和日志状态均归属于实例。构造前确定目标 OS 和实验选项。
- `dwarfgen.NewContext(backend, reflectData)` 验证编译器图及已有 DWARF 修正表归属，必要时创建原生修正表，再绑定 `Link.DebugInfo` 和 `Link.GenAbstractFunc`。`Info` 拒绝外来 Link、IR 函数、声明和 SSA 调试结果。
- `Progs.Flush` 在修改原生指令之前验证活动缓冲区、函数和符号归属；`obj.Flushplist` 检查整条指令链属于同一 Link 且无环。汇编源入口仍保留上游允许 nil 分配器的语义。

`dwarfgen.Context.RecordFlags` 保留上游 producer 字符串算法，通过显式传入的 `Flags` 查询接口读取选项；可直接赋予 `*flag.FlagSet`。生成器只允许绑定后的 `flag.Lookup` 和描述字段、Getter 读取，仍拒绝进程级解析、全局选项集合和 `Value.Set`。

底层原生 `dwarf.FnState.PutRanges` 的目标实验参数通过显式 `Emitter` 参数传递，沿用已有的公开方法签名变换许可机制。AIX 外部链接器探测 `IsDWARFEnabledOnAIXLd` 不属于该编码路径，未迁入。

## 调用顺序

在完成原生类型、IR、运行时和 SSA 初始化后：

1. 在 `backend.InitConfig` 前调用 `amd64.NewContext`。
2. 固定 DWARF 版本和位置表选项，调用 `dwarfgen.NewContext(backend, reflectData)`。若先执行内联阶段，应预先调用 `Link.EnsureDwarfFixups`，让内联阶段记录的前驱函数保留在同一修正表中。
3. 完成整个包的 walk，再对每个函数调用 `BuildSSA`、`writer.NewProgs`、`GenSSA`、`pp.Flush`。
4. 读取机器码、重定位和元数据后再 `pp.Free`；共享工作缓冲区的复用约束保持原样。
5. 所有函数完成后调用原生 `Link.DwFixups.Finalize`，完成可能延迟到包末尾的抽象函数和 DIE 引用修正。

同一编译实例的可变操作由调用方串行。不同实例可以并行；本阶段没有增加同一实例内并行后端的承诺。

## 验证

- `go test -race ./... -count=1` 通过，包括生成器、公开 consumer 和 8 组并行完整 Flush 隔离测试。
- `syncgo check` 验证 437 个生成文件与固定上游来源一致。
- `TestPinnedFlush` 在 `go1.27.1` 和 `go1.27.0` 各验证 9 组配置，共 18 组通过：默认、noopt、race、noregabi、DWARF4、DWARF4 BASE entries、无位置表、大 Phi 和内联。
- 两端执行各自的生产 `Progs.Flush`，对比机器码和重定位、PCLN 字节及引用、函数 DWARF 符号、包末尾修正后的 DIE 引用和缩写表。临时观察器只读取私有字段，没有替换或删减生产 pass。
- 外来编译器服务、函数、调试结果、指令链和 DWARF 修正表的归属检查，以及已释放缓冲区拒绝路径均有覆盖。选项查询测试还验证全局解析、全局 FlagSet 和 Value.Set 不会被生成器误放行。

本阶段验证机器码和元数据输出，没有执行生成的机器码。对象文件写出、统一源码 Build 驱动、第二个完整目标后端及替换编译器/自举仍在后续阶段。
