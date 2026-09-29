# M3 原生函数符号与 DWARF 内联修正

这是该批次的历史记录；后续内联调用展开见 [M3 原生内联调用展开](m3-inline-expansion.md)。

本批补齐实际内联展开依赖的 `ir.InitLSym` 和 `setupTextLSym`，并迁入它们调用的 `obj.Link.InitTextSym`、原生 FuncInfo、DWARF 符号初始化和 DwarfFixupTable 全部方法。尚未接入 `unifiedInlineCall`、整包内联驱动或机器码/DWARF 输出，不能据此认为完整编译器后端已可用。

## 迁入实现与状态归属

- `ir.Context.InitLSym(fn, hasBody)` 使用原生函数 ABI、Systemstack、Nosplit、wrapper、闭包上下文、包 init 和 runtime/reflect 特殊处理规则。`base.Ctxt` 改为该 IR 实例的 `Base.Ctxt`；入口在任何写入前拒绝其他实例的函数。重复初始化仍按上游报错。
- `obj.Link.InitTextSym` 保留文本队列、属性、FuncID/FuncFlag、经 line pragma 和内联位置解析的起始行，以及原生重复声明诊断。无函数体时 InitLSym 只建立符号，不登记文本和 DWARF 函数项。
- 原生 `DwarfFixupTable` 持有所属 Link、内联前函数与抽象 DWARF 符号；引用、子 DIE 偏移、延迟修正与锁均保留上游实现。`Finalize` 在后端并行工作结束之后运行一次，使用宿主设置的 `Link.GenAbstractFunc` 补齐缺失定义。它不生成机器码，也不替代真正的 DWARF 发射器。
- 保留原生 `LookupDerived` 的普通/静态命名空间区分。静态命名空间需要按上游 Linknew 约定初始化 `statichash`；本批不提供完整 Linknew 的宿主参数化入口，静态符号对外构造仍待该步骤接通。现有公共函数符号路径使用 ABI 命名空间，不依赖该私有静态表。
- `objabi.GetFuncID` 的只读常量 map 用既有 ReadTables 规则局部化。生成器现在接受常量元素 map，继续拒绝写入、删除、clear、取地址和整体逃逸；不把该 map 暴露为共享可变状态。工具版本变为 `syncgo-m3-v13`，因此已有生成文件的头部也随之更新。

对外继续使用原有 `ir.Context` 和 `gcbridge.Link`，新增的 FuncInfo、DWARFVar 都是原生类型别名。`gcbridge.NewDwarfFixupTable(link)` 只是上游构造器的公开转发。宿主在使用内联 DWARF 前显式将该表绑定到 Link；最终统一驱动仍需负责生命周期配置。

## 验证

- 八个实例并发创建同名函数，检查 ABI 符号、文本登记、函数属性、无函数体函数、DWARF precursor、旧图保留和跨实例拒绝。
- 原生 DWARF 测试覆盖位置列表开关、初始化之前/之后登记 precursor、派生静态符号、并发引用记录、立即与延迟重定位修正、缺失抽象定义回调及符号缓存身份。
- 双版本上游夹具覆盖九个文本标记的全部 512 种组合，每种再测试 Go/汇编属性，共 1024 种；同时对照 IR 属性映射、runtime/reflect 特例、函数 ID、line pragma、无名/无函数体符号、DWARF 修正及重复/未限定名称诊断。
- 独立外部 module 对真实源码读取所得函数调用公开 InitLSym 和 DWARF 表构造/查询，无需引用 internal 包。
- 常量 map 局部化验证命中/未命中、comma-ok、遍历和 len，并拒绝修改和逃逸。

`syncgo generate` / `check` 通过，共 250 个迁入文件。`GOTOOLCHAIN=go1.27.1 go test -race ./...` 通过（port 包 56.071 秒），其中包含独立 module 的公开接口验证。日志见 `_work/m3-text-symbol-unit.log`。

`TestPinnedUpstreamAndUpgrade` 通过（测试主体 375.75 秒，包总计 377.414 秒）。Go 1.27.1 和 Go 1.27.0 的符号夹具、完整类型声明/原生方法比较及既有差分全部通过。同一套规则从 1.27.0 升级到 1.27.1 时仅两个 Go 输出文件变化，上游工作树保持原状。日志见 `_work/m3-text-symbol-source.log`。

下一步接入原生 `inline.TryInlineCall` / `mkinlcall` 与 `noder.unifiedInlineCall`，让内联判定实际产生上游 InlinedCallExpr，并保持调用初始化副作用、返回处理和内联位置树。
