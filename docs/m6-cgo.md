# M6：启用 cgo 的真实驱动验证

本批在 Linux AMD64 和 ARM64 上验证已迁入的原生 cgo 路径。没有新增或替代 cgo 编译算法；实际工作是建立带系统 C 编译器的隔离环境，并验证源文件指令、ABI wrappers、对象 pragma 与原生链接流程能共同工作。

## 测试边界

`TestPinnedCGODriver` 先构建目标架构的替换 `cmd/compile`，再在 Linux 容器中运行同版本官方 `cmd/go`、cgo 工具、汇编器、链接器和 gcc。所有 Go 编译调用进入本库。官方与替换工具链使用独立缓存、同一份输入和相同 C 工具链，先建立官方基线，再运行替换版本。

每次构建强制 `-a`，验证调用日志中的库内容身份、上游 pin，以及 runtime、runtime/cgo 和主包确实经过库实现。容器禁用网络，显式指定目标平台和入口；SDK、源码树和输出均显式提供。

## 已验证场景

两种架构分别通过自动、显式内部、显式外部三种链接模式：

- Go 调 C 整数函数。
- C 调 `//export` Go 回调；回调内启动 goroutine，并触发 GC。
- C 字符串分配、Go 字符串转换与 `defer C.free`。
- 官方与替换程序均实际执行，输出 `42 21 cgo`；每种替换构建均记录 60 个不同包。

三种模式在 ARM64 合计约 37 秒，在 AMD64 合计约 98 秒。

完整 `go build -a -p=4 std cmd` 在两种架构均通过：ARM64 覆盖 783 个不同包身份，AMD64 覆盖 785 个；各自核验全部 21 个命令入口包的源码确实进入替换编译器。每种架构均先完成官方同版本基线，再完成库版本；完整测试分别约 102 秒和 336 秒。

这些验证使用 ARM64 阶段提交 `6a7d5dd0` 的生产实现，库内容身份为 `cdb251316286f8da1d7bb1b48a9303a011fccf287b4d9400623d58692c91d2f8`。后续生产实现变更需重新验证。

## 复现

```sh
GO_SSA_CGO_INTEGRATION=1 \
GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> \
GO_SSA_LINUX_BOOTSTRAP=<official-go1.27.1-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image> \
go test ./internal/gcintegration -run '^TestPinnedCGODriver$' -v -timeout 30m
```

AMD64 使用 `GO_SSA_TEST_ARCH=amd64` 和 `GO_SSA_LINUX_AMD64_IMAGE`，并提供对应的 Linux AMD64 SDK。设置 `GO_SSA_DRIVER_FULL=1` 增加完整 `std cmd` 重建及源码调用覆盖核验；SDK 必须包含 `preprofile`，因为编译器构建会使用默认 PGO。该测试不下载缺失工具或容器镜像。

这些结果不自动承诺共享库、插件、所有 cgo 回归或公开 `-race` 驱动支持。race 的安全执行、真实竞争和隐含 checkptr 行为另设验收。
