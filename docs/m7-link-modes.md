# M7：动态链接与分离归档

上一轮完整 Go 测试目录尚有 `fixedbugs/issue58826.go` 和 `linkobj.go` 停在未支持的驱动选项。本次接通 `-dynlink` 和 `-linkobj`。

`Options.DynamicLink` 设置实例对象上下文的原生动态符号引用模式，沿用原生 Shared 归一化、代码生成和 DWARF flags 顺序。Build 的元数据记录动态链接及有效 Shared 状态；artifact 输入必须匹配这两个模式。显式 Shared 与动态链接隐含的 Shared 不构成不同身份，普通 Shared 也不能冒充动态链接。

`Options.SplitObjects` 复用已有原生归档写入器，先输出 compiler member，再输出 linker member。`Result.Archive` 和 `Result.LinkerArchive` 分别持有两份独立字节；替换驱动将其写入 `-o` 和 `-linkobj` 指定文件。默认仍返回合并归档。Build 阶段明确拒绝这一机器码输出选项。

已验证：

- amd64/arm64 × 动态/普通 × 分离/合并 × 优化/无优化共 16 组完整原生归档和汇编文本逐字节一致；分离时两份归档分别比较。
- 分离的 compiler archive 可继续作为真实导入输入，包含导出数据而不包含 linker member。
- 双架构 Built/EarlyOptimized 的 artifact 往返、有效 Shared 等价输入、错误模式拒绝和后续编译保活均通过 race 检查。

完整固定版本 Go 测试目录已通过，使用 `-short` 和目标默认 codegen 范围：

| 目标 | 官方与替换结果 | 本库编译包数 | 全程耗时 |
| --- | --- | --- | --- |
| Linux ARM64 | 各 2670 通过、59 跳过 | 456 | 180.027 秒 |
| 原生 Linux AMD64 | 各 2692 通过、37 跳过 | 458 | 190.803 秒 |

测试明确复制固定版本的 3400 个 Go 测试源文件，拒绝缺失测试目录导致的整体跳过。全部 18 个前次叶子失败已消除。全库 `go test -race ./...` 通过。
