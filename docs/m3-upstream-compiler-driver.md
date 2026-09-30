# M3 上游编译驱动接入

`gcintegrate` 导出固定 Go 1.27.1 Git 源码到新的隔离目录，复制同版本官方 bootstrap 工具和源码树中缺少的生成文件，将本库 `compiler` 的完整生产依赖闭包放入 `src/cmd/vendor/go-backend-ssa.invalid`，并替换 `src/cmd/compile/main.go`。它拒绝覆盖已有输出目录；不会修改输入 Go 仓库、宿主 GOROOT 或本库源码。

替换驱动保留官方参数、importcfg/embedcfg 和原生 ABI 配置解析。源码加载、前端、SSA、机器码、归档及汇编头全部通过 `compiler.Compile` 完成，驱动没有调用原版 `gc.Main` 或外部编译器的回退路径。隔离副本将导入路径映射为私有别名 `go-backend-ssa.invalid`，兼容 cmd 模块及正式 dist 的外部工作模块 vendor 解析；库的公开 module 不变。cmd 模块的 Go 指令设为 1.27.1。

## 产物身份与使用

准备工具生成 `go-backend-ssa.json`，记录上游 commit、本库 Git revision、原始和实际 vendored 生产文件逐一哈希、集成文件哈希、组合内容哈希及 bootstrap 路径。组合哈希还覆盖替换驱动模板、导入重写结果及 bootstrap 补丁；dirty 工作树也以实际文件内容标识。`compile -V=full` 包含该内容哈希，使 cmd/go 为不同实现生成不同构建缓存身份。

```sh
go run ./cmd/gcintegrate -repo "$GO_SSA_SOURCE_REPO" -root . \
  -bootstrap /path/to/official/go1.27.1 -out /new/isolated/go
```

随后用隔离树原版工具在其 `src/cmd` 模块中构建 `cmd/compile`，保存原版 compile 并将新产物放到隔离 `pkg/tool/<host_os>_<host_arch>/compile`。集成测试自动完成这些步骤；当前替换编译器输出目标仅 Linux/amd64，构建驱动本身可由匹配版本的宿主官方编译器构建。

支持官方构建正常使用的包路径、语言版本、工作目录/GOROOT 与路径裁剪、构建 ID、标准库/runtime 标志、优化/内联、DWARF、worker 数、显式依赖、嵌入配置、symabis、PGO profile、归档和汇编头。未支持的显式选项或调试环境变量会报错，不静默忽略或调用原编译器；当前手动调用要求显式 `-p`。symabis 描述原生汇编 ABI，汇编源文件仍由官方 asm 工具处理并由官方构建驱动组装。

## 共享库的驱动配置

`compiler.Request` 新增显式路径上下文，`Options` 新增语言版本、构建 ID、路径裁剪、本地导入控制与汇编头选择；返回值包含独立的汇编头字节。归档和汇编头都在最终包检查成功后才返回。

原生 `ParseFlags` 的包属性检测和 runtime/checkptr 规范化区间完整迁入同一实例：标准 runtime 包强制优化、禁用 checkptr；普通包把未指定的 checkptr 规范化为零。汇编头使用固定上游 `gc/export.go` 的完整生成算法。输出错误保留原始原因，调用方继续拥有输出流，失败后不能重复运行该阶段。

函数区间生成器支持以完整 if/range 内的唯一包变量赋值作为结构锚点；拒绝局部遮蔽、重复和函数闭包中的赋值。复制原生函数时保留真实源码身份，修复另一包引用该文件全局量时的重复签名声明。

## 验证

集成回归由以下显式选项开启：

```sh
GO_SSA_SOURCE_REPO=/path/to/go \
GO_SSA_DRIVER_INTEGRATION=1 GO_SSA_DRIVER_FULL=1 \
GO_SSA_LINUX_AMD64_IMAGE=goallc-x86-frame-debug-env:local \
go test ./internal/gcintegration -run '^TestPinnedCompilerDriver$' -v -count=1 -timeout 30m
```

Linux/amd64 本机执行不需要镜像。其他宿主使用已存在的镜像，禁止拉取和网络访问；自编译测试的临时挂载可写，以接收归档输出。

已完成的集成验证包括：

- 在隔离 cmd 模块构建替换版 cmd/compile，替换隔离工具目录中的原版 compiler。
- 从空缓存强制重编含 runtime、reflect、fmt、slices 和主包的 59 个包；逐条调用记录验证进入了本库及精确内容哈希。测试程序的泛型、接口/反射、排序、goroutine/channel 和 defer/recover 结果与官方 Go 1.27.1 一致。
- `go build -a std` 在 Linux/amd64、CGO_ENABLED=0 下成功；374 个标准包的调用集合与官方 `go list -deps -json std` 选定集合逐项匹配。
- 替换编译器编译出自身 Linux/amd64 版本（使用上游默认 PGO profile），执行该版本编译另一个包，完整 2386 字节归档与第一阶段编译器一致。
- 默认、禁用优化、禁用 DWARF 和驱动参数四种模式的源码测试中，依赖/主包完整归档与官方逐字节一致；汇编头逐字节一致；程序成功执行。
- 版本诊断、runtime 强制优化、汇编头布局、流所有权、写出失败/短写和生成器状态来源均有回归。
- 本阶段原有竞态、469 文件及双版本差分均通过；后续 PGO 的新增验证和最新门禁见[原生 PGO 输入](m3-native-profile-input.md)。

## 尚待完成

这里证明了真实驱动替换、无 cgo 标准库重建和一次编译器自编译后的代码生成。后续阶段已完成本文限定目标的正式 dist bootstrap 与二进制固定点；整套规定回归、cgo/race 等驱动能力、第二目标及独立 Built/EarlyOptimized API 仍需继续验收。不能把这些记录等同于全部工具链替换目标已经完成。

正式 dist bootstrap 接入、各阶段验证和当前限制见[后续自举阶段](m3-dist-bootstrap.md)。
