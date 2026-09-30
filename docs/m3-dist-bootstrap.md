# M3 正式 dist bootstrap 接入

隔离 Go 1.27.1 源码树现在包含 `cmd/dist` 的 bootstrap 接入补丁。正式 `make.bash` 保持上游构建阶段和工具链选择：官方 bootstrap 先构建 toolchain1，后续编译持续进入本库；不是通过预先构建一个替换二进制跳过这些阶段。

## 两个实际接入问题

上游 bootstrap 模块以 Go 1.24 语言规则编译源码。库驱动原先在 `obj.Link` 字面量中指定嵌入字段，使用了 Go 1.27 新语法。已改为等价的独立字段赋值，保留原生 bootstrap 语言要求；生产依赖闭包通过 Go 1.24 语言模式编译。这里的验证 bootstrap SDK 仍为 Go 1.27.1，不将语言模式检查等同于最低 Go 1.24.6 SDK 验收。

上游 cmd/go 从外部工作模块构建工具链时，仅对首段含点号的导入路径做标准库 vendor 查找。原始 `go-backend-ssa` 因而在 toolchain2 被误当作标准库路径。隔离副本使用私有模块别名 `go-backend-ssa.invalid`，按 AST import spec 精确重写引用；公开 module、原始文件、非导入字符串和注释均保持原样。不会联网解析该别名。

## bootstrap 模块与来源记录

正常构建的库副本位于 `src/cmd/vendor/go-backend-ssa.invalid`。dist 的独立 bootstrap 模块不使用这个嵌套 vendor 目录，接入补丁将其复制到 bootstrap 模块根下，并与上游其他复制包一样重写为 `bootstrap/go-backend-ssa.invalid/...`。原有标准包和 `cmd/...` 导入重写继续使用上游规则。

补丁只应用于已验证的固定源码锚点；源码变化、重复应用都会失败。`go-backend-ssa.json` 同时记录原始生产源码、实际 vendored 源码和四个集成文件的哈希。编译器身份哈希覆盖固定上游、本库内容、驱动模板、模块别名、导入重写结果与 bootstrap 补丁。

成功调用记录增加源码文件列表，使自举回归能区分主编译器源码的普通构建和默认 PGO 构建。构建期间使用独立缓存和隔离树，不写原始 Go 仓库或宿主 GOROOT。

## 验证入口

```sh
GO_SSA_SOURCE_REPO=/path/to/pinned/go \
GO_SSA_DIST_BOOTSTRAP=1 \
GO_SSA_LINUX_BOOTSTRAP=/path/to/go1.27.1-linux-amd64 \
GO_SSA_LINUX_AMD64_IMAGE=goallc-x86-frame-debug-env:local \
go test ./internal/gcintegration -run '^TestPinnedDistBootstrap$' \
  -v -count=1 -timeout 60m
```

Linux/amd64 本机不需要镜像，bootstrap 默认当前 GOROOT；其他宿主显式提供 Linux 工具链与已有镜像。容器禁止拉取和网络，bootstrap 挂载只读。当前验证用官方同版本源码和宿主官方编译器离线交叉构建 Linux bootstrap，已在 Linux 中完成版本、源码构建和程序执行验证。

集成回归运行原生 `make.bash` / `dist bootstrap -a -d`，保留各阶段编译器；检查库身份、runtime/fmt 调用及编译器源码的有/无 PGO 调用记录。各阶段和最终编译器生成的完整测试归档与官方对照；随后使用正式工具链的发行构建参数强制重编编译器，检查二进制固定点，并用重建的 go/compiler/linker/runtime 编译运行程序。

已完成验证：

- 原生 make.bash / dist bootstrap 完整通过（395.20 秒），包含 toolchain1、go_bootstrap、toolchain2、启用默认 PGO 的 toolchain3，以及最终标准库和工具安装。
- 成功调用记录覆盖 669 个不同包；编译器主包分别有关闭 PGO 和默认 PGO 的构建记录，所有调用固定到同一本库及上游内容身份。
- compile1、compile2、compile3、compile4 和最终 compile 均能运行，完整 14072 字节测试归档全部与官方编译器一致。
- 强制重新编译得到的编译器与 toolchain3 的完整 35458896 字节二进制相同。
- 正式安装的工具链强制重建并运行包含 fmt、反射和 goroutine/channel 的程序，输出 `42 true`，与官方工具链执行一致。
- 带模块别名的既有驱动集成复跑通过（43.68 秒），包括 59 包程序、374 个无 cgo 标准包及默认 PGO 自编译。
- 全量 `go test -race -timeout 30m ./...`、固定上游补丁检查、导入重写与来源哈希回归均通过；Go 1.24 语言模式的 compiler 和集成包测试通过。

全部构建和执行限定 Linux/amd64、CGO_ENABLED=0、官方 Go 1.27.1 bootstrap SDK。

## 后续范围

独立 Built/EarlyOptimized API、Linux/arm64、更多上游 compiler/codegen/ABI/GC/debug 回归、cgo/race 驱动和最低 bootstrap SDK 仍需分别验收。
