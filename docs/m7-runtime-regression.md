# M7：官方运行时回归与 PIE 驱动

本批用固定 Go 1.27.1 的官方测试直接验收替换编译器，先在同一目标与环境运行官方基线，再运行库驱动。每组测试使用独立缓存；runtime、reflect、sync、sync/atomic、internal/abi、internal/runtime/atomic 的源码及测试程序均经相应编译器构建，库调用日志核对内容身份和固定上游提交。

## 从失败回归修复的问题

### walk 错误被队列检查掩盖

官方 `internal/abi.TestFuncPCCompileError` 正常产生三条 ABI 不匹配诊断。库在已有 walk 诊断时拒绝整个编译队列，产生 panic，掩盖了源码错误。

移除这个额外限制，继续遵循原生 `enqueueFunc`：发生 walk 错误的函数不会入队，已排队的合法函数继续处理，包级入口最后返回完整诊断。调用方的错误图、ABI、worker 和生命周期检查仍保留；未知 panic 不会被当成源码错误吞掉。

真实驱动对结构化源码错误复用上游 `base.ErrorExit`，保留原始诊断、不加 `compile:` 前缀，退出 2 并移除指定的旧输出。独立回归先复现 panic，以及随后发现的退出码/旧输出差异，再核验修复。

### PIE 缺少 shared-code 输入

ARM64 的官方 `runtime.TestCgoPprofPIE` 会传入 `-shared`，原驱动明确拒绝该选项。新增 `Options.Shared`，将显式状态传给原生 Link/SSA/汇编器，并按原生顺序记录 DWARF 标志。没有重写 PIE 或重定位算法。

Build 元数据与 artifact 记录 shared-code 模式，拒绝模式不同的依赖；未启用时省略 JSON 字段，原有编码保持兼容。`-shared` 的支持不等于已支持 `-dynlink`、插件或 Go 共享库的全部工作流。

### 原生 AMD64 验收工具构建

原生主机首先暴露了生成规则文件名 `rules_arm64.go` 被 Go 当作架构限制的问题；改名为 `rules_arm64_emit.go`，使 ARM64 迁移规则也参与 AMD64 上的工具构建。该文件中的迁移规则和算法未改动。

隔离测试设施现在支持原生 Linux 主机。虚拟输入根只映射路径前缀，缓存后缀不作错误替换；其它宿主仍显式使用目标容器。

## 验证

- 两种架构、Compile/Built/EarlyOptimized 六组源码错误回归，均保留三条诊断且不输出部分结果；随后新实例编译成功。
- 双架构真实驱动三条 ABI 诊断逐字节匹配官方。额外直接驱动检查核验退出 2、删除预先创建的旧输出。
- 双架构 shared、shared+race、不优化六组完整归档逐字节匹配官方；双阶段 artifact 往返、模式隔离和旧图保活通过 race 检查。
- 双架构 cgo PIE 的内部/外部链接均完成官方及库版本的真实执行，均输出 `42`；使用 ELF 读取器确认产物为 ET_DYN。每个库版本构建记录 60 个包。
- Linux ARM64 完整 `-short` 运行时套件：官方和库版本均为 1925 个测试通过、101 个跳过；库版本记录 314 个包。没有为通过这组测试添加跳过项。
- 库本身全量 race 检查通过，生成文件检查验证 490 个文件。

原生 Ubuntu Linux AMD64 的完整 `-short` 套件也已通过：官方和库版本均为 1937 个通过、93 个跳过，317 个包记录到库调用；同一原生主机还通过 ABI 诊断、退出/旧输出检查及两种 cgo PIE 链接。runtime 套件约 136 秒，包含 ABI 与 PIE 的整次验收约 189 秒。没有添加针对测试的跳过项。

原生验证使用独立源码、SDK、缓存和临时目录，启动环境由 `env -i` 显式提供。传输的库输入及 SDK 分别校验 1222 和 12082 个文件的 SHA-256，源仓库 pin 核验为 `862c888e612ac346c7c4d99c9392bdfd265f33b0`。

Rosetta 环境的官方与库版本均出现 TestPanicRace、TestCheckFDs 和 TestMmapErrorSign 失败，其中 mmap 测试触发模拟器中止；这些结果保留为环境基线，不用来归因编译器。

## 复现

```sh
GO_SSA_RUNTIME_REGRESSION=1 \
GO_SSA_TEST_ARCH=arm64 \
GO_SSA_SOURCE_REPO=<source-repo> \
GO_SSA_LINUX_BOOTSTRAP=<official-go1.27.1-linux-arm64-sdk> \
GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image-with-gcc> \
go test ./internal/gcintegration \
  -run '^TestPinned(RuntimeRegression|ABIDiagnostic)$' -v -timeout 40m
```

原生 Linux AMD64 使用对应 SDK、`GO_SSA_TEST_ARCH=amd64`，无需设置容器镜像。PIE 测试使用 `GO_SSA_PIE_INTEGRATION=1` 和 `TestPinnedPIEDriver`。SDK 除编译/汇编/链接/cgo 工具外，须包含固定版本的 addr2line、cover、nm、objdump、pack、test2json、vet、preprofile；不隐式下载缺失项。

compiler/SSA 的完整官方回归另行推进；本批不把其中尚未接通的调试驱动选项或新发现的 rangefunc 调试参数归属问题标记为完成。
