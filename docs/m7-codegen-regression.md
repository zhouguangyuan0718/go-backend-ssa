# M7：Go 测试目录与 clobber 调试选项

Go 1.27.1 发行 SDK 不包含 `GOROOT/test`，直接运行 `cmd/internal/testdir` 会跳过整套测试。验收现在复制独立参考 SDK，再加入固定提交的完整测试目录，共 3400 个 Go 源文件。输入 SDK 和来源工作树保持只读。独立副本允许上游运行用例创建临时链接输出，避免把只读挂载错误误认为编译失败。官方编译器先运行，替换工具链随后运行相同用例。

首轮 ARM64 codegen 对照中，官方通过，替换驱动的 `clobberdead.go` 因拒绝 `-clobberdead` 失败。库内原生 liveness 和寄存器分配算法已经存在；本次将 `Options.ClobberDead`、`Options.ClobberDeadReg` 接入对应原生开关，并将显式 `Debug.ClobberDeadHash` 传给 liveness。驱动接入 `-clobberdead`、`-clobberdeadreg` 与 `GOCLOBBERDEADHASH`，库本身不读取进程环境。

`TestClobberCompilerOracle` 在 amd64/arm64 上各运行默认、栈槽、寄存器、两者同时、hash 后缀 0/1 六种模式。12 组完整归档、汇编及调试报告与官方 Go 1.27.1 逐字节一致，测试并发运行并开启 race detector。

ARM64 完整 codegen 对照均为 83 项通过、6 项跳过；146 个包记录到本库调用。修正外部测试包身份核验后的整套验收耗时 45.483 秒。全库 `go test -race ./...` 通过。AMD64 首轮因 `-spectre` 未接入而失败；接入后的完整验收也已通过，官方与替换编译器均为 86 项通过、3 项跳过，见 [Spectre 验收](m7-spectre.md)。测试保留上游默认的架构选择和短测试策略，不强制扩大到其它目标。验收要求测试实际执行，拒绝整套静默跳过，同时验证 `cmd/internal/testdir_test` 和测试源码 `command-line-arguments` 确实调用本库。

```sh
GO_SSA_CODEGEN_REGRESSION=1 \
GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> \
GO_SSA_LINUX_BOOTSTRAP=<official-go1.27.1-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image> \
go test ./internal/gcintegration -run '^TestPinnedCodegenRegression$' -v -timeout 30m
```

`GO_SSA_LANGUAGE_REGRESSION=1` 和 `TestPinnedLanguageRegression` 运行整个测试目录。修正独立参考副本的只读挂载后，ARM64 官方完整基线为 2670 项通过、59 项跳过（101.441 秒）。替换驱动首轮仍因 `-e`、`-C` 等选项及诊断差异失败，尚不声明替换后的完整测试目录通过。
