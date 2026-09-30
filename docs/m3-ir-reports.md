# M3：完整原生 IR 文本与 HTML 诊断

完整迁入上游 `ir/dump.go` 和 `ir/html.go`。`Context.AstDump` 现在保留原生文件名、首次创建和后续追加模式、阶段标题处理、HTML 内容快照与相同内容阶段合并。此前写入 trace 的文本适配已移除；trace 继续接收原生报告位置和输出错误消息。

## 宿主输出与状态

调用方设置 `Context.OpenAST(path, flags, mode)`，返回 `io.WriteCloser`，并在使用相对 HTML 路径时提供 `ASTWorkingDirectory`。提供器须遵守原生文件模式：文本首次创建/截断，后续追加；HTML 创建/截断。IR 实例持有锁、文本文件记录、HTML writer 表、关闭顺序、缩进字节与阶段正则表达式，不使用进程级报告注册表。

报告阶段完成后调用 `Context.CloseHTMLWriters()`；它按原生顺序输出待写阶段并关闭流，清理 HTML 表。正常关闭后的再次关闭为空操作。文本文件的追加历史按原生实现保留到实例结束，不随 HTML 关闭重置。独立 Context 可以并发使用；同一个图及其 Context 仍由调用方串行操作。

`HTMLWriter` 保留原生节点遍历、反射字段/标志、内联位置、节点编号、CSS 和 JavaScript。共享 `HTMLWriterBase` 继续从原生缓冲对象取得 owner，兼容 noder 的语法报告。公开 `ir.HTMLWriter` 和 `ir.HTMLWriterBase` 为原生类型别名。

`FDumpAny` 和 `DumpAny` 也保留完整原生算法。位置使用当前实例的 PosTable，根节点按归属检查；带类型的 nil 节点指针保留原生 nil 输出行为，不尝试读取 owner。通用结构体内容和字段筛选行为不变。

## 错误边界

外来函数、HTML 节点和通用转储根节点在打开输出前被拒绝。未绑定提供器或提供器返回 nil writer 时产生明确错误。文本打开/关闭失败时仍按原生算法尝试 HTML 输出，并把文本错误写入 trace。HTML 错误和宿主 panic 通过编译实例的 panic 边界传播；此阶段不改变上游缓冲流的错误处理算法。

## 移植机制

原生文本报告先在局部变量中计算打开标志，再调用文件工厂。生成器新增四个显式宿主常量绑定：`os.O_APPEND`、`os.O_RDWR`、`os.O_CREATE` 和 `os.O_TRUNC`。它们绑定到 IR 实例字段，并由宿主常量初始化；仅列出的常量通过审计，未绑定常量和其他 os 操作仍被拒绝。生成器回归验证创建/追加两条路径、实例值差异及未授权常量拒绝。

## 验证

- 八个实例并发测试文本追加、内联前后 HTML 快照、相同内容阶段合并、报告关闭及关闭后的再次追加。
- 验证位置表、nil/带类型 nil、外来图拒绝、未配置或空提供器、文本打开/关闭失败后 HTML 仍能完成。
- 早期 inlining、deadlocals、slice 和源码入口测试已改为通过提供器捕获报告，并核对原生文本内容和流关闭。
- Go 1.27.1 / Go 1.27.0 各六组完整快照均一致。每组比较 11 份报告：三份语法 HTML，以及四个函数（含 rangefunc helper）的文本和 IR HTML；同时核对前端 IR、覆盖率符号与保活标志。HTML 仅对语法报告中的作用域地址做原有规范化。
- 全量 `go test -race -timeout 30m ./...` 通过；`syncgo check` 验证全部 468 个生成文件。

下一步接通整包原生前端顺序与统一 Build；本阶段不代表完整编译器替换、链接运行或自举已经完成。
