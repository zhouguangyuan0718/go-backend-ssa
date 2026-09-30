# 原生 Unified 导出与包归档

本阶段接通原生 Unified IR 的最终导出，以及带 `__.PKGDEF` 和 `_go_.o` 成员的 Go `.a` 归档。它承接已完成的符号编号和对象二进制编码；完整源码编译驱动、包级工作队列收尾、链接执行及工具链替换仍未完成。

## 原生实现

`noder/linker.go` 保留完整的导出元素重定位、去重、泛型记录、可内联函数体选择，以及 ABI、逃逸信息、内联成本和符号索引更新。`writeUnifiedExport` 按原生顺序建立公共/私有根并计算 Link fingerprint；`WriteExports` 写出原生 `$$B` 导出区。

`gc/obj.go` 的 `dumpobj1`、编译器/链接器成员写入、文本头、cgo JSON 段、归档占位、对齐和头部回填均保留上游函数体。`archive.FormatHeader`、`objabi.HeaderString`、`buildcfg.GOGOARCH` 和实验选项列表也使用原生实现。

文件创建绑定到调用方传入的 seekable 输出。原生 `bio.Writer.Close` 仍刷新缓冲区，但借用的底层流由调用方负责关闭。没有隐式文件系统输出或环境参数回退。

## 接口与顺序

调用方须先完成全部函数、类型及静态数据，以及会生成新函数/类型的工作队列。已有函数 Flush 和 GC 辅助符号注册不等于完成整个包的收尾。

- `objw.Context.NumberSymbols()` 完成原生符号编号；只有成功返回才允许导出。`WriteObject` 在已经编号时复用编号，允许先导出再写原始载荷。
- `noder.Context.ExportUnified(io.Writer) error` 写出最终 Unified bitstream，并设置 Link fingerprint。结果可交给另一个实例的 `RegisterPackage(..., false)`。它不包含 `.a` 或 `$$B` 包装。
- 使用 `gc.Context` 的归档接口时，先绑定同一个 `pkginit.Context`，完成全局声明和其他包级数据、DWARF 修正及 `AddGCLocals`，再调用 `SetObjectHeader(gc.ObjectConfig{...})` 提供目标参数和编译器版本。实验选项从同一 Universe 复制。
- `WriteArchive(output, gc.CombinedObject)` 输出两个成员；分离输出依次调用 `CompilerObject`、`LinkerObject`，共享同一份导出 fingerprint 和符号编号。方法在需要时完成编号。

导出和每个归档成员只能写一次。导出失败也消耗导出阶段；归档写入失败后拒绝继续写其他成员。I/O 错误保留底层错误身份，普通编译器不变量 panic 继续传播。失败后的部分字节仍由调用方处置。实例内操作需要串行，不同实例可以并行。

## 配置输入的生成规则

同包的环境派生配置使用显式、只读的 host-input 规则。生成器保留原生变量的类型签名，将读取绑定到 Context 字段，不选择或执行环境初始化表达式。引用类型、写入、取地址和指针接收者调用会被拒绝；适配器字段仍须通过类型校验。

上游 `buildcfg.Version` 来自未纳入 Git 的 bootstrap 常量。生成器从固定 `cmd/dist/buildruntime.go` 中的字符串常量输出模板证明其类型，并记录该源文件哈希；不读取宿主工具链的版本值，不运行环境生成器。模板变化或无法证明类型会报错。

## 验证

- `TestPinnedArchive` 在 `go1.27.1` 与 `go1.27.0` 固定源码上各通过 12 组对照，共 24 组完整归档逐字节一致。覆盖前一阶段的 9 组 AMD64 配置，以及分离归档、cgo 段和 build ID。两端使用同一显式实验设置；对象头版本使用验证工具链的版本。
- 官方固定版本的归档 reader 解析成员，官方 goobj reader 继续检查链接器载荷的全部表和符号引用。
- 原生导出在 8 组并行实例间往返，验证不同常量、可内联函数体、泛型类型和旧结果保活；未编号、重复导出及 I/O 失败路径有覆盖。
- 8 组并行归档验证合并成员与分离成员字节相同、结果隔离和调用方流所有权；写入/定位失败后拒绝复用，并验证新实例正常工作。
- 独立公开 consumer 已从源码走到包含原生导出与对象载荷的归档。
- 完整 `go test -race ./... -count=1` 通过；444 个生成文件的一致性复查通过。

这些结果验证的是导出/容器格式、元数据和实例边界。尚未用本阶段输出完成程序链接、执行或替换编译器自举。
