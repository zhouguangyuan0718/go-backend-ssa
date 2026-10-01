# M7：原生 Spectre 参数与 AMD64 codegen

AMD64 官方 Go 1.27.1 codegen 基线通过后，替换驱动的 `spectre.go` 和 `retpoline.go` 因拒绝 `-spectre` 失败。这两条路径的 SSA 与汇编算法已经迁入，本批补齐原生参数解析及驱动入口。

`Options.Spectre` 接受上游的逗号分隔设置，迁入完整 `base.parseSpectre` 处理空白、重复项、`all`、`index`、`ret`、未知项与目标限制。解析使用当前编译实例的目标和 linker 状态；错误返回调用者，不退出宿主进程。DWARF producer 记录原始选项，与上游一致。驱动将 `-spectre` 原样传入。

`TestSpectreCompilerOracle` 并发运行 amd64/arm64 各六种设置，开启 race detector。AMD64 五种有效设置和 ARM64 默认设置的完整归档与汇编逐字节匹配官方；另外六种无效/不支持设置的错误与官方匹配。测试还明确核对间接调用生成 retpoline、索引生成条件移动。

原生 Linux AMD64 完整 codegen：官方与替换编译器均为 86 项通过、3 项跳过，148 个包记录到本库，耗时 51.948 秒。生成检查核对全部 491 个文件通过；全库 `go test -race ./...` 通过。
