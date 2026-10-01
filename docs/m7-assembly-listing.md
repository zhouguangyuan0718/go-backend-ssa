# M7：原生汇编输出

固定上游的 `TestDashS` 和 `TestIssue16214` 在官方编译器通过，原库驱动因拒绝 `-S` 失败。现由 `Options.AssemblyListing` 将详细级别传入原生 Link，原生对象写出流程调用已有的 `debugAsmEmit`，结果保存在独立的 `Result.AssemblyListing` 字节切片中。驱动将该结果写到标准输出，与官方行为一致。

级别 0 不打印；1 包含指令、符号数据及重定位；2 增加 DWARF 辅助符号及 ABI 版本；3 及以上还包含符号索引。编译器不读取进程输出设置，两个独立调用的缓冲区互不共享。`Build` 不生成机器码，因此明确拒绝非零汇编输出级别；负数级别也会报错。

验证：

- 两种架构 × 四个详细级别 × 优化开关，共 16 组并发测试：汇编文本和完整 archive 都与官方 Go 1.27.1 逐字节一致，测试启用 race 检查。
- Linux ARM64 官方及库版本的两个上游回归均通过、无跳过，库调用覆盖 199 个包。
- 原生 Ubuntu AMD64 官方及库版本同样通过两个上游回归、无跳过，库调用覆盖 199 个包。
- `go test -race ./compiler ./internal/gcintegration` 通过。

```sh
GO_SSA_NATIVE_EXECUTION=1 go test -race ./compiler -run '^TestAssemblyListing' -v -count=1
GO_SSA_COMPILER_REGRESSION=1 GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> GO_SSA_LINUX_BOOTSTRAP=<official-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image-with-gcc> \
go test ./internal/gcintegration -run '^TestPinnedAssemblyListingRegression$' -v -count=1 -timeout=20m
```

原生 Linux 验收使用对应架构并省略镜像变量。这一项恢复汇编输出；其余 compiler 调试功能另行验收。
