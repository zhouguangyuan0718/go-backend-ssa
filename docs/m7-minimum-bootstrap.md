# M7：最低 bootstrap SDK 验收

已在 Linux ARM64 上使用官方 Go 1.24.6 SDK 完成固定 Go 1.27.1 替换工具链的正式 bootstrap。源码与算法没有为了旧 SDK 改写；本批扩展隔离验收，将启动 SDK 与官方结果对照 SDK 分开。

`TestPinnedMinimumDistBootstrap` 精确核对启动 SDK 为 `go1.24.6`，参考 SDK 为同架构 `go1.27.1`。前者仅用于启动 `make.bash/dist`；各阶段编译器的结果和最终程序仍与 Go 1.27.1 对照，避免用旧版本对象格式作为错误基线。源码、生成文件和库内容仍从固定基线以及当前库实现准备，不修改 SDK 或来源工作树。

## 结果

- 正式 toolchain1、go_bootstrap、toolchain2、toolchain3、标准库及工具安装全部完成，包含默认 PGO。
- 682 个不同包记录到本库的实际编译调用；生产实现内容身份为 `cdb251316286f8da1d7bb1b48a9303a011fccf287b4d9400623d58692c91d2f8`。
- 各保留阶段及最终编译器输出的 14196 字节归档均与官方 Go 1.27.1 逐字节相等。
- 强制再次编译出的 34385560 字节编译器与 toolchain3 完全相同。
- 最终重建的 cmd/go、编译器、链接器与 runtime 共同生成并执行程序，结果与官方一致。
- 完整验收约 131 秒；当前仅声明 Linux ARM64、`CGO_ENABLED=0` 的最低 SDK bootstrap。启用 cgo 的发行包重建是另外的验收，不能与此混为一次测试。

## 复现

```sh
GO_SSA_DIST_BOOTSTRAP=1 \
GO_SSA_MINIMUM_BOOTSTRAP=1 \
GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> \
GO_SSA_LINUX_BOOTSTRAP=<official-go1.24.6-linux-arm64-sdk> \
GO_SSA_LINUX_REFERENCE=<official-go1.27.1-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image> \
go test ./internal/gcintegration -run '^TestPinnedMinimumDistBootstrap$' -v -timeout 60m
```

原有 Go 1.27.1 启动 SDK 的双架构测试继续保留。本项完成不代表完整 M7 发布验收完成；race 驱动、其余规定回归及完整能力矩阵仍继续推进。

## compiler 调试接口接入后的复验

提交 `8bd2c405` 后，使用相同 Go 1.24.6 ARM64 启动 SDK 再次完成正式自举。生产内容身份为 `e458aee87378b15f33c14b04245856cea0d098b57300c70207788389cfd4ea67`：682 个包调用本库，所有保留阶段的 14196 字节归档与官方相同，强制重建的 34391338 字节编译器与 toolchain3 完全相同，最终程序实际执行与官方一致。完整验收耗时 121.795 秒。此记录针对该提交的生产源码，不自动覆盖后续变更。
