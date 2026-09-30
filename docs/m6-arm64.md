# M6：Linux ARM64 原生后端与替换工具链

本批把第二个完整目标接到现有源码 API 和隔离 `cmd/compile` 驱动。基线仍是 `go1.27.1`、`862c888e612ac346c7c4d99c9392bdfd265f33b0`；从固定 Git 对象迁移，不修改来源工作树。

## 实现

- 完整迁入 `cmd/compile/internal/arm64` 的普通/SIMD 指令生成，以及 `cmd/internal/obj/arm64` 的汇编、指令表、系统寄存器、格式器和 SVE 编码表。
- 架构状态、查表、嵌套 operand/encoder 切片、初始化和格式注册归属于实例；保留原生初始化顺序与算法。异属 Link、Prog、函数及空分配器在修改输入前拒绝。
- 公开 `arm64.NewContext` 在 `ssagen.InitConfig` 前安装目标。`compiler.Build`、`compiler.Compile` 和隔离驱动按目标选择 AMD64/ARM64，复用同一前端与包收尾流程。
- `compiler.Target.GOARM64` 使用固定上游的 `ParseGoarm64`；空值选择 `v8.0`，特征按原生 `String` 规范化，`v8.1` 及以上自动包含 LSE。ARM64 要求 `GOAMD64=0`；AMD64 不接受 `GOARM64`。
- 规范化目标贯穿对象头、Built/EarlyOptimized 元数据和 artifact 身份校验。新增 JSON 字段使用 `omitempty`，保留既有 AMD64 格式；跨架构 artifact 明确拒绝。
- 移植工具从固定 `cmd/dist/buildruntime.go` 的生成声明证明缺失输入常量的类型，再把读取绑定到实例输入；不运行生成器，不采用宿主的默认 CPU。错误声明、错误类型和输入写入均有拒绝回归。

## 已完成验证

- 五种 CPU 输入的源码编译、Built/EarlyOptimized 图、目标规范化、artifact 往返及泛型依赖；无效目标和跨架构依赖拒绝。
- AMD64/ARM64 交错 Build、失败及取消后的恢复、旧图/类型/artifact 保活，以及 A→B→A 的确定性。
- 固定官方编译器四种模式的 Built 和 EarlyOptimized 图、全局数据及重定位对照。
- 默认、禁用优化、关闭 DWARF、驱动参数四种模式：依赖和主包完整归档逐字节相等，经官方 linker 链接，在 Linux ARM64 容器实际执行并对照输出 `14 8 6 11`。
- Go 1.27.0 和 1.27.1 各九种底层 Flush 配置，比较完整机器码、重定位、PC 表和 DWARF。包含低层 race 配置；这不代表公开驱动已支持 `-race`。
- 并发汇编器隔离与异属输入拒绝；SVE `SETFFR`、`ZADD` 按固定上游测试编码验证，并检查三层嵌套表没有共享可变存储。SVE 验证属于编码，不声称运行硬件支持 SVE。
- 真实替换驱动强制重编全部 372 个所选无 cgo 标准库包；程序执行与官方一致；默认 PGO 自编译出的 Linux ARM64 编译器实际运行，输出同一份 2394 字节归档。

- ARM64 正式 `make.bash/dist` 全部阶段通过，记录到 682 个不同包；保存的各阶段编译器及最终编译器均输出与官方一致的 14196 字节归档。强制再次重编得到与 toolchain3 完全相同的 34385560 字节编译器；最终工具链编译的程序实际执行与官方一致。
- 全库 `go test -race ./...` 通过；新增跨架构生命周期测试单独通过 race 检查；`syncgo check` 验证 490 个生成文件。

AMD64 最终 bootstrap 回归也通过：记录到 684 个不同包，各阶段输出同一份官方 14072 字节归档，强制重编的 36616577 字节编译器与 toolchain3 完全一致，最终程序执行通过。ARM64 和 AMD64 此轮 bootstrap 分别耗时约 134 秒和 429 秒。两者使用同一生产实现内容身份 `cdb251316286f8da1d7bb1b48a9303a011fccf287b4d9400623d58692c91d2f8`。

## 验证入口

```sh
GO_SSA_NATIVE_EXECUTION=1 GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image> go test ./compiler -run '^TestARM64(BuildOfficialOracle|EarlyOfficialOracle|CompileLinkAndRun)$' -v
GO_SSA_SOURCE_REPO=<source-repo> go test ./internal/port -run '^TestPinnedARM64Flush$' -v -timeout 30m
GO_SSA_SOURCE_REPO=<source-repo> GO_SSA_DRIVER_INTEGRATION=1 GO_SSA_DRIVER_FULL=1 GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image> go test ./internal/gcintegration -run '^TestPinnedARM64CompilerDriver$' -v -timeout 45m
GO_SSA_SOURCE_REPO=<source-repo> GO_SSA_DIST_BOOTSTRAP=1 GO_SSA_LINUX_BOOTSTRAP=<go1.27.1-linux-arm64-sdk> GO_SSA_LINUX_ARM64_IMAGE=<installed-arm64-image> go test ./internal/gcintegration -run '^TestPinnedARM64DistBootstrap$' -v -timeout 60m
```

容器执行显式选择平台并覆盖镜像默认入口，避免基础镜像的服务启动命令截获测试程序。测试源码、标准库、自编译和 bootstrap 均在独立临时树中完成。

## 后续范围

继续后端元数据的独立消费审计、cgo/race 驱动、最低 Go 1.24.6 bootstrap SDK 和完整规定回归。两种完整目标接通不能替代这些剩余验收；386 仍仅承诺已验证的布局/SSA 路径。
