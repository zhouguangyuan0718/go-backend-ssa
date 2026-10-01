# M7：替换实现的验收与边界

本库已实现固定 Go 1.27.1 的源码前端、Built/EarlyOptimized SSA、AMD64/ARM64 原生后端与对象输出。隔离工具链的 `cmd/compile` 实际调用同一套库实现；没有回退到原版编译器或另起外部编译器进程。

本次目标为 `linux/amd64`、`linux/arm64`。上游源码 pin 为 `862c888e612ac346c7c4d99c9392bdfd265f33b0`，生成规则版本为 `syncgo-m7-v89`，最终生产实现对应提交 `68795db9`。验收日期为 2026-10-01；后续验收提交只更新测试与文档。

## 源码与输出契约

| 能力 | 实现和证据 |
| --- | --- |
| 源码解析、类型检查、泛型、闭包、接口、初始化、defer/panic/recover、map/channel | 原生前端及声明固定点；完整 Go 测试目录与 compiler/runtime 回归 |
| Built / EarlyOptimized | 每张 SSA 图独立 cache，后续调用不清除旧结果；固定阶段边界、官方对照、独立 consumer |
| 导入 | 严格版本化 artifact、显式原生归档及按需提供器；依赖闭包哈希、失败/取消与跨调用隔离 |
| 原生 Compile | 两种目标的机器码、重定位、PCLN/DWARF、合并/分离归档与汇编输出 |
| PGO、汇编、cgo、race | 原生 profile、SymABIs、cgo 协作与竞争检测；见对应阶段记录 |
| 调试与错误 | 原生 `-d`、`-m`、`-S`、`-live`、源码错误文本和结构化诊断；未知 panic 不吞掉 |
| 工具链替换 | 隔离源码树、生产依赖 vendor、bootstrap 适配、内容哈希及实际调用记录 |

公开 consumer 无需导入 `internal`，也无需通过反射或 unsafe 读取 Aux。原生 continuation 与独立 Build 使用同一对象模型；Build 不执行原生寄存器分配和最终机器码输出。参见 [API](api.md)、[独立后端消费](m6-independent-backend.md)。

## 正式自举

最终生产实现的两条路径均经过 `make.bash` 的 toolchain1/2/3、标准库与命令重建，保留阶段的探针归档与官方相同。随后强制重编 compiler，完整二进制与 toolchain3 相同，最终工具链构建并执行程序，与官方输出一致。

| 目标 | 启动 SDK | 本库编译包数 | 一致的探针归档 | 编译器二进制固定点 | 全程 |
| --- | --- | --- | --- | --- | --- |
| ARM64 | 最低 Go 1.24.6 | 682 | 14,196 字节 | 34,460,983 字节 | 118.131 秒 |
| 原生 AMD64 | Go 1.27.1 | 684 | 14,072 字节 | 36,737,056 字节 | 149.116 秒 |

这两次 bootstrap 使用 `CGO_ENABLED=0`；cgo 完整标准库/工具重建是单独的验收，见 [cgo 记录](m6-cgo.md)。最低 SDK 的声明限于已验证的 ARM64。

## 官方对照回归

每条替换路径均校验实际 compiler 调用记录及内容哈希，官方控制使用相同 SDK、目标和执行环境。计数包含父测试，跳过数是上游在该目标及 `-short` 下的结果。

| 套件 | ARM64 官方 / 替换 | 原生 AMD64 官方 / 替换 |
| --- | --- | --- |
| 完整 Go 测试目录 | 各 2670 通过、59 跳过；456 个包调用本库 | 各 2692 通过、37 跳过；458 个包调用本库 |
| compiler + SSA 完整短测试 | 各 2637 通过、12 跳过；388 个包调用本库 | 各 2638 通过、11 跳过；390 个包调用本库 |
| runtime/reflect/sync/ABI 完整短测试 | 各 1925 通过、101 跳过；314 个包调用本库 | 各 1939 通过、91 跳过；317 个包调用本库 |
| 选定 dist 套件 | 两条路径均通过 11 个套件；214 个包调用本库 | 两条路径均通过 12 个套件；214 个包调用本库 |

Go 测试目录明确补入固定源码中的 3400 个 Go 文件，拒绝 SDK 缺少该目录导致的静默整体跳过；codegen 使用目标默认范围，未启用全架构扩展。dist 专项包括 PIE 内部链接、外部链接、cgo/TLS/stdio、runtime 调试/采样和 sync 多 CPU；AMD64 另含 reflect 的 Spectre 变体。FIPS 变体按上游配置完成构建和运行，但默认实验设置下显示无匹配测试，不能算作 FIPS 功能验收。这是选定 dist 套件，不是所有发行构建模式的全量 dist 验收。

此前完整测试中发现的错误报告边界、未来导入归档、前端选项和分离归档问题均已修复并建立对照：见 [错误报告](m7-error-reports.md)、[按需导入](m7-lazy-imports.md)、[前端选项](m7-frontend-flags.md)、[链接模式](m7-link-modes.md)。

## 状态、生成与升级

生成器对所选包及手写适配进行类型化审计，拒绝剩余包级变量、init、隐式环境依赖和未登记生产文件。公开 compiler 的实际依赖闭包额外检查 91 个 module 包、567 个生产文件，覆盖公开组合代码；这是静态约束，与 race、A→B→A、先失败后成功、跨目标并发和旧图保活测试共同使用。

全库 race 测试已通过。`syncgo check` 验证全部 491 个生成文件。完整双版本源码矩阵通过，耗时 3860.871 秒：Go 1.27.1 重复生成结果一致；Go 1.27.0 使用相同选择与状态改写规则完成生成。每版均通过 checker 的 48 个用例、writer/rangefunc 的 48+96 次字节比较，以及位置、指令、导入、归档和对象声明对照；每版的 74 组原生阶段测试包含 521 次一致快照和 1 次预期错误对照。

Go 1.27.0 → Go 1.27.1 升级影响 3 个生成 Go 文件；先安装旧版再升级到新版，两次一致性检查均通过，来源工作树保持不变。该测试验证这两个固定 patch 的升级，不代表任意后续版本无需调整规则。

升级参考中的 parser pragma 回调不属于 writer 的比较边界，现仅保留 writer 所需的原生 function-pragma mask；完整 pragma 行为由独立 directive 对照覆盖。这样修复参考切片的依赖错误，没有修改生产算法或替换官方预期。

对象声明对照逐项核验新增的实例状态字段，再比较所有原生字段；方法对照先严格核验实例绑定和所有权检查，仅归一化这些已知差异，继续比较完整算法体。

## 性能与分配基线

Apple M4 Max、darwin/arm64、Go 1.27.1、`GOMAXPROCS=4`。每次重新创建编译实例并返回拥有独立状态的结果；固定约 6.7 KB 源码包含 64 个普通函数及泛型、闭包、接口、map、defer。下面为每组 3 次、每次 1 秒采样的中位数。

| 输出目标 / 阶段 | 毫秒/次 | MB 分配/次 | 分配次数/次 |
| --- | --- | --- | --- |
| amd64 / Built | 5.51 | 34.46 | 88,578 |
| amd64 / EarlyOptimized | 8.03 | 45.21 | 105,021 |
| amd64 / Compile | 12.65 | 40.63 | 139,661 |
| arm64 / Built | 6.79 | 35.24 | 96,511 |
| arm64 / EarlyOptimized | 9.72 | 45.99 | 112,954 |
| arm64 / Compile | 14.41 | 41.39 | 147,679 |

MB 为十进制累计分配量，不是保留堆大小或峰值。这是库自身后续维护的微基准，未与官方 compiler 的进程开销比较，也不包含依赖 I/O 和链接。

## 复现入口

使用匹配的 Go 1.27.1 宿主工具链，来源仓库包含 `go1.27.0` 和 `go1.27.1` 两个 tag。

```sh
go test -race ./...
GO_SSA_NATIVE_EXECUTION=1 go test -race ./compiler \
  -run '^Test(FrontendFlags|LinkModes|DynamicLink|SplitObjects)'
GOMAXPROCS=4 GO_SSA_SOURCE_REPO=/path/to/go go test -v ./internal/port \
  -run '^TestPinnedUpstreamAndUpgrade$' -count=1 -timeout=90m
GOMAXPROCS=4 go test ./compiler -run '^$' \
  -bench '^BenchmarkSourcePipeline$' -benchmem -benchtime=1s -count=3
```

原生 Linux 集成测试设置 `GO_SSA_SOURCE_REPO`、`GO_SSA_LINUX_BOOTSTRAP`（同目标官方 SDK）和 `GO_SSA_TEST_ARCH`。分别开启 `GO_SSA_LANGUAGE_REGRESSION`、`GO_SSA_COMPILER_REGRESSION`、`GO_SSA_RUNTIME_REGRESSION`、`GO_SSA_DIST_REGRESSION`，运行对应 `TestPinned*Regression`；跨宿主还需已存在的匹配镜像。正式自举及最低 SDK 参数见 [自举](m3-dist-bootstrap.md) 与 [最低 SDK](m7-minimum-bootstrap.md)。

`cmd/gcintegrate` 生成可审阅的隔离 Go 源码树与 `go-backend-ssa.json` 来源清单，拒绝覆盖已有输出目录。具体使用见 [驱动接入](m3-upstream-compiler-driver.md)。库源码和生成文件已经提交，普通使用方不必执行生成器。

## 支持边界

模块下载、构建标签选择、cgo 预处理、外部汇编和最终链接由调用者或原生构建驱动负责。替换驱动要求显式 `-p`，未接入的选项会报错。其他 GOOS/GOARCH、MSan/ASan、完整覆盖率驱动、共享库/plugin 全流程及任意实验组合不在本次验收声明内；`DynamicLink` 的代码生成与 artifact 模式校验不等同于完成共享库发行流程。

本次交付是本地库、隔离集成工具和可复现测试，未发布 module 或工具链发行包，也未推送远端。编译实例之间可并发；对同一结果的主动修改或触发惰性缓存的操作由调用者同步。
