# 原生 Go 对象序列化

本阶段接通 `Link.NumberSyms`、完整 `obj.WriteObjFile` 和 `goobj.Writer`，把函数机器码、数据、重定位、PC 表及 DWARF 序列化为原生 Go 对象二进制载荷。公开入口为 `objw.Context.WriteObject(io.WriteSeeker)`。载荷尚不含编译器文本头、导出区或 `.a` 容器；完整包驱动、链接执行和替换工具链仍需继续完成。

## 原生实现与实例边界

- 保留原生符号分类和编号、builtin ABI 索引、内容哈希、字符串表、文件表、符号表、重定位、辅助信息、函数信息以及对象头回填算法。
- 每个 Link 独立持有 builtin 表和索引、编号及输出阶段状态。目标的 RegabiWrappers 设置在建立表前确定，不读取宿主进程的实验配置。
- 原生 `bio.Writer` 保留缓冲、Flush、Offset 和 MustSeek 的函数体。生成规则仅将审核过的私有 `*os.File` 字段收窄为所需的流接口；文件工厂、其他方法和非流操作不会自动获准。
- 输出流由调用方提供并负责关闭；文件支持的数据符号通过 `Link.OpenObjectInput` 显式提供 `io.ReadCloser`，由原生写入流程关闭输入。未配置读取服务会返回诊断，不回退到宿主文件系统。
- 原生流操作中的致命 I/O 诊断转换为可返回的错误，保留底层错误身份。普通编译器不变量 panic 继续传播。可恢复的 Link 诊断保留既有回调，并由 WriteObject 汇总返回。

同一 Link 只可编号、输出一次，失败也会消耗相应阶段。失败后可能已写出部分内容，由调用方处置。实例内操作仍由调用方串行；不同编译实例可以并发。

## 调用顺序

对象写入前须完成所有函数及包级数据：

1. 完成源码读取、前端分析、walk、BuildSSA、GenSSA 和完整 Flush。
2. 使用绑定同一 `pkginit.Context` 的 `gc.Context`，对全局声明调用原生 `DumpGlobal` / `DumpGlobalConst`；这会注册静态临时变量、保留 ASan/coverage/linkname 处理并生成对应 DWARF。完成类型和静态数据生成，包括已注册的 `reflectdata.WriteGCSymbols`、`staticdata.WriteFuncSyms`，以及适用的其他包级数据阶段；仅生成函数体不足以构成完整包。
3. 完成包末尾的 DWARF 修正。调用 `gc.NewContext(writer).AddGCLocals()`，执行上游完整的顺序收尾函数，把函数 GC、栈对象、参数、defer 和跳转表等辅助符号加入数据定义。
4. 调用 `writer.WriteObject(output)`；该方法配置实例 builtin 表，按原生顺序编号并序列化符号，最后刷新流。

`gc.Context` 目前暴露本阶段迁入的全局声明和函数辅助数据收尾，并非完整的 gc 编译驱动。上面的顺序是显式阶段接口的使用约束，统一驱动将在后续阶段承接。

## 验证

- `go test -race ./... -count=1` 通过，包括公开 consumer、8 组并行对象输出、输出偏移及保活、重复写入拒绝、文件输入和 I/O 错误隔离。
- `syncgo check` 验证 441 个生成文件与固定上游一致。
- `TestPinnedObjectFile` 在 `go1.27.1` 和 `go1.27.0` 各验证 9 组配置，共 18 组通过：默认、noopt、race、noregabi、DWARF4、DWARF4 BASE entries、无位置表、大 Phi 和内联。两端输出的完整对象载荷逐字节一致。
- 官方固定版本的 `goobj.Reader` 回读符号、文件、数据、重定位、辅助信息和引用表，并检查符号引用范围。既有机器码、PCLN 和 DWARF 差分继续执行。
- 官方参考侧直接运行固定源码中的完整全局声明及 GC 辅助数据收尾函数；仅把入口名称改为测试可调用名称，函数体不变。
- 新增测试覆盖全局变量的只读段、ASan redzone、linkname、coverage 类型和外来实例拒绝；生成规则测试拒绝未审核的文件工厂、字段类型和流操作。

对象字节与官方一致及官方 reader 回读验证，证明的是本阶段的二进制编码；尚未证明整个对象可链接、生成程序可运行或编译器可自举。
