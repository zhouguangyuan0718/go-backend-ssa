# M7：源码调试设置与完整 compiler 回归

固定 Go 1.27.1 的官方 compiler 套件最初在库驱动失败：不支持 `-d`、`-m`、GOCOMPILEDEBUG、GOSSADIR 等设置，影响 SSA 检查、PGO、FMA 哈希、局部栈槽合并和调试行号测试。官方同环境基线通过。

## 接入方式

`Request.Debug` 接收按顺序应用的原生调试字符串和可选宿主服务。使用已迁移的原生参数解析器，SSA 参数进入当前编译实例的 `PhaseOption`。原生 hash、loopvar、Link 参数初始化区间通过源码规则抽取，并在插桩、并发和 runtime 标志归一化前运行，保留上游默认值及优先级。无效 hash/SSA 参数返回错误，帮助通过 `DebugHelp` 返回，未知 panic 不吞掉。

`Options.OptimizationReport` 对应 `-m`；原生 WarnfAt、addErrorMsg 和 FlushErrors 保留即时刷新、位置排序与相邻重复消息去除。边界将警告同时保存在结构化 Diagnostics 中。DebugOutput 返回原生报告文本，驱动不会再次打印同一批 warning。队列锁支持底层库调用方的并发报告，不额外禁用后端并发。

源码、SSA/AST HTML、阶段 dump、目录、计时、内存统计和可选图形命令都由显式提供器供给。库不读取进程调试环境或隐式打开宿主路径。真实驱动负责读取环境、保存原生 `-d` 的有序参数，并提供文件与其它宿主服务。解析器的原生 String 方法不保存原始输入，因此驱动保留参数值后在独立实例中重放。

原生 Compile 和两个源码 SSA 阶段都返回独立的 DebugOutput。Build 在所选阶段关闭原生 HTML writer，产生完整页尾；所有报告流在成功、写入失败和取消时清理，底层 Close 至多执行一次。报告写入失败不返回可误用的编译结果。

## 失败证据与验证

- 初始 compiler 全套失败记录保留；先完成无优化闭包 ABI 和 `-S` 修复，再接入本批调试功能。
- 双架构原生对照覆盖 SSA 检查、关闭 generic/lowered CSE、安静/详细 FMA 哈希、两级优化报告和 prove 诊断。首次详细报告差分暴露遗漏的 warning 出口及原生排序差异，改用上游报告队列后文本和完整 archive 一致。
- 双架构 genssa dump 文件和完整 archive 与官方逐字节一致。
- Built/EarlyOptimized 的 HTML 流先复现未关闭/无页尾，修复后与 Compile 一样关闭一次并生成完整文档；写入失败和三个阶段的取消均检查清理及不返回部分结果。
- 首轮全库 race 检查通过已有并发诊断回归复现原生队列的竞态；加锁后该回归与全库 `go test -race ./...` 均通过。
- 首轮完整 compiler 短测试：Linux ARM64 官方及库均 2637 个通过、12 个跳过，库调用覆盖 388 个包；原生 Ubuntu AMD64 官方及库均 2638 个通过、11 个跳过，库调用覆盖 390 个包。没有添加跳过项。
- 最终源码再次完整复验：ARM64 仍为 2637 个通过、12 个跳过、388 个库调用包；原生 AMD64 仍为 2638 个通过、11 个跳过、390 个库调用包，官方与库结果相同。完整生成检查验证 491 个文件；双架构归档/报告对照、HTML 生命周期、写入失败、取消及并发诊断的聚焦 race 检查全部通过。

```sh
GO_SSA_NATIVE_EXECUTION=1 go test -race ./compiler -run '^TestDebug' -v -count=1
GO_SSA_COMPILER_REGRESSION=1 GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> GO_SSA_LINUX_BOOTSTRAP=<official-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image-with-gcc> \
go test ./internal/gcintegration -run '^TestPinnedCompilerRegression$' -v -count=1 -timeout=30m
```

原生 Linux 使用匹配的架构和 SDK，省略镜像变量。该套件包含 `cmd/compile/internal/ssa` 与 `cmd/compile/internal/test` 的完整短测试；不将其等同于整个 Go 测试目录、全套 dist test 或所有目标架构已经验收。
