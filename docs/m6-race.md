# M6：race 编译与原生检测运行

公开 `compiler.Options.Race` 已接入 `Build`、`NativeCompile` 和 `Compile`，隔离 `cmd/compile` 驱动支持 `-race` 及显式 `-installsuffix`。插桩算法继续使用已迁入的原生 walk/ssagen/SSA；本批补全驱动状态及固定上游初始化顺序。

## 原生规则与实例归属

从固定 `base.ParseFlags` 提取 race 隐含 checkptr 和并发后端限制，再执行已有运行时包归一化；从 `gc.Main` 提取 NoInstrument 包排除和 `Cfg.Instrumenting` 设置。由生成器保留完整原生语句，不在适配层重写这些策略。抽取锚点新增限定右值字面量及导入包变量，测试覆盖包别名、遮蔽、错误来源、表达式冒充字面量及歧义拒绝。

目标 race 模式与运行编译器自身的 race 构建相互独立；本次目标选项不改变后者的随机调度状态。DWARF 记录原始请求的 race 标志，即使包策略随后关闭插桩，也与上游的记录顺序一致。

`Package.Metadata.Race` 和 import artifact 记录请求的模式，跨 race 模式的 artifact 被拒绝。未启用时该字段从 JSON 省略，原有非 race artifact 编码保持兼容。原生归档输入依旧由调用者显式选择匹配的依赖。

## 已完成验证

- Linux AMD64 与 ARM64，Built 和 EarlyOptimized 的 race 图、完整函数集合、60 个全局及重定位与官方编译器一致；同一批也重验原有默认、不内联、不优化及 CPU 特征模式。
- 两种架构的普通包、`sync`、`internal/runtime/atomic` 和 `runtime/race`，分别在默认和不优化模式下完成 16 组完整归档逐字节对照，涵盖 NoRaceFunc、NoInstrument、`//go:norace`、隐含 checkptr 与 DWARF 标志。
- 两种架构的 Built/EarlyOptimized 并行生命周期测试：导出与解码 artifact、同模式依赖消费、拒绝跨模式依赖，以及后续普通编译不改变保留的 race 图和 artifact。
- 两种真实 Linux 驱动先运行官方基线，再通过库驱动强制重建和执行 race 程序。安全程序均输出 `safe 200`；竞争程序均报告 `WARNING: DATA RACE` 并退出 66；越界指针转换均触发隐含 checkptr。实际编译调用覆盖 ARM64 的 61 个包和 AMD64 的 62 个包，包括 runtime、runtime/cgo、runtime/race、sync 和主包。

## 最终生产实现的普通编译回归

库自身全量 `go test -race ./...` 与 490 个生成文件的 `syncgo check` 均通过。生产内容身份为 `9433d56ac29943ccbf83465086bdc6cc7aa5c4b9f3b692304d644d78cfd42f89` 的最终实现重新通过双架构正式 bootstrap：ARM64 使用最低 Go 1.24.6 SDK，682 个包、全部阶段 14196 字节官方一致归档、34385982 字节编译器固定点；AMD64 使用 Go 1.27.1 SDK，684 个包、全部阶段 14072 字节一致归档、36621095 字节编译器固定点。两者均含默认 PGO，最终重建工具链均实际生成并执行与官方结果相同的程序。验收限定 `CGO_ENABLED=0`，耗时分别约 145 秒和 444 秒。

## 复现

```sh
GO_SSA_NATIVE_EXECUTION=1 go test ./compiler \
  -run '^Test(RaceOfficialArchives|SourceBuildOfficialOracle|SourceEarlyOfficialOracle|ARM64BuildOfficialOracle|ARM64EarlyOfficialOracle)$' -v

GO_SSA_RACE_INTEGRATION=1 \
GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> \
GO_SSA_LINUX_BOOTSTRAP=<official-go1.27.1-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image-with-gcc> \
go test ./internal/gcintegration -run '^TestPinnedRaceDriver$' -v -timeout 15m
```

AMD64 切换架构、对应 SDK 和 `GO_SSA_LINUX_AMD64_IMAGE`。容器禁用网络，缺失的镜像或工具不会自动下载。

完整官方 compiler/codegen/runtime 回归仍另设验收；本项不声明已支持 MSan、ASan 或所有调试标志。
