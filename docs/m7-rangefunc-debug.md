# M7：range-over-func 的无优化 DWARF 回归

固定 Go 1.27.1 的完整 compiler 测试首先暴露 `TestIssue77597`：`-race -gcflags='all=-N -l'` 构建依赖时，库在 reflect 的 range-over-func 调试信息生成中报 `ABI assignment has no compilation owner`。官方基线通过。

独立最小源码随后在 AMD64、ARM64 复现：启用 DWARF 的普通/race 四组都失败，关闭 DWARF 的四组对照均通过。原生 `BuildFuncDebugNoOptimized` 为闭包上下文直接构造 ABI 参数字面量，绕过了 ABI 包内分配路径的实例绑定。

修复使用已有的类型感知字面量迁移规则，在该原生字面量外围绑定当前 SSA 的 ABI 上下文。参数类型、名称、寄存器切片、偏移以及上游分解算法保持原样。绑定拒绝外来类型、外来名称和重复绑定，不放宽所有权检查。

验证结果：

- 两种指针宽度的所有权测试通过，确认值复制不修改原始字面量且保留字段及切片身份。
- AMD64、ARM64 × 普通/race × 开启/关闭 DWARF 的 8 组完整 archive 与官方编译器逐字节一致，测试本身启用 race 检查。
- Linux ARM64 和原生 Ubuntu AMD64 均运行官方及库版本 `TestIssue77597`，各 1 个通过、0 个跳过，实际构建并执行 race、无优化程序。库调用分别覆盖 201、202 个包；同一测试设施逐条验证内容身份及上游 pin。
- 全库 `go test -race ./...` 通过；生成文件一致性检查验证 490 个文件。

针对完整 compiler 套件的其余失败继续单独处理；本项通过不代表调试参数、PGO 调试输出或整个 compiler 套件已经全部通过。

```sh
GO_SSA_NATIVE_EXECUTION=1 go test -race ./compiler -run '^TestRangeFuncNoOptDebug$' -v -count=1
GO_SSA_COMPILER_REGRESSION=1 GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> GO_SSA_LINUX_BOOTSTRAP=<official-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image-with-gcc> \
go test ./internal/gcintegration -run '^TestPinnedRangeFuncRegression$' -v -count=1 -timeout=20m
```

原生 Linux 主机使用匹配架构并省略镜像变量。完整 compiler 套件入口为 `TestPinnedCompilerRegression`，聚焦入口不会替代完整验收。
