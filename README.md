# go-backend-ssa

将 Go 1.27 官方编译器从源码解析到 SSA 的实现移植为可嵌入进程的 Go 库，供其他编译器后端直接消费。

库维护基础设施、公共数据模型、状态归属和生命周期；移植工具从固定上游提交抽取算法，通过可复现的结构改写接入这些设施。每次调用拥有独立编译状态，返回结果在后续调用期间仍有效。

当前状态：**M1 已完成，M2 已推进到真实类型结构与 Universe 归属；尚无 Go 源码到 SSA 的 Build API。** 当前可用位置、诊断、目标布局输入、包符号查找及初步 Universe；完整类型布局和编译流程仍待移植。

- 语言范围：Go 1.27；其他语言版本尚不支持。
- 首个源码基线：官方 `go1.27.1`，提交 `862c888e612ac346c7c4d99c9392bdfd265f33b0`。
- 交付形式：普通 Go module，进程内调用，返回本库的 SSA 对象。
- 对外保证目标：调用隔离、无共享可变编译状态、可重入、跨调用并发安全。
- 并列验收目标：可替换官方 Go 1.27 编译器对应实现，接回原生后端并完成工具链构建。
- 初始目标配置：`linux/amd64` 和 `linux/arm64`；32 位布局在正式保证目标隔离前加入验证。

## 阅读顺序

1. [整体设计](docs/design.md)：目标、分层、代码归属与确定的取舍。
2. [完整编译流程](docs/pipeline.md)：AST、类型检查、IR、优化、walk 和 SSA 的移植范围。
3. [状态与生命周期](docs/state.md)：Context、类型缓存、回调、返回结果和并发。
4. [源码移植机制](docs/porting.md)：抽取、符号改写、补丁、审计和升级。
5. [公共 API 与后端契约](docs/api.md)：输入、依赖导入、SSA 形态、runtime 和错误。
6. [实施路线](docs/roadmap.md)：逐步细化、实现和验收。
7. [源码基线与证据](docs/baseline.md)：本次实际检查的版本和入口。
8. [原生编译器回接](docs/go-compiler-integration.md)：共享对象模型、gcbridge、native tail 与自举验收。
9. [M1 实现与验证](docs/m1.md)：首批工具与切片记录。
10. [M2 基础设施](docs/m2.md)：第一批位置设施与归属设计。
11. [M2 类型与 Universe](docs/m2-universe.md)：第二批实现及类型布局边界。

## 当前可运行的工具

本地 module 路径为 `go-backend-ssa`，工具链为 Go 1.27.1。维护命令读取固定 Git 对象，不修改来源 Go 工作树；日常测试不需要来源仓库或网络。

```sh
export GOTOOLCHAIN=go1.27.1
export GO_SSA_SOURCE_REPO=/path/to/local/golang-go

go run ./cmd/syncgo inspect -repo "$GO_SSA_SOURCE_REPO" -out _work/inspection.json -summary _work/inspection-summary.json
go run ./cmd/syncgo generate -repo "$GO_SSA_SOURCE_REPO"
go run ./cmd/syncgo check -repo "$GO_SSA_SOURCE_REPO"
go test -race ./...
```

上游版本和移植规则统一维护在 [internal/port/rules.go](internal/port/rules.go)，不需要 JSON 配置。审计报告按需生成到 `_work/`；`sync/generated.json` 仅由工具自动维护。

设定 `GO_SSA_SOURCE_REPO` 时，测试额外验证官方 go1.27.1 再生成、两版上游位置和符号回归测试、源码树未变以及 go1.27.0 → go1.27.1 升级；仓库需包含两个 tag。不设置该变量时仅跳过这项集成测试，其他测试照常执行。

编译器源码从 `src/cmd/compile/` 以下保留目录、包名和文件名；辅助依赖分别归入 `internal/cmd/` 与 `internal/go/`。当前移植代码包括完整位置模块、前端位置与字符读取器，以及 types/ir/objabi 的结构和算法切片；完整类型系统和编译器尚未实现。

移植后的源码直接保存在本项目，和手写适配、转换规则一起纳入版本管理。使用本库时不需要先运行移植工具；维护者更新上游版本时再重新生成。

| 当前移植代码 | 内容 |
| --- | --- |
| [internal/cmd/src/pos.go](internal/cmd/src/pos.go)、[xpos.go](internal/cmd/src/xpos.go) | 位置、压缩位置、位置表与调试标记 |
| [internal/syntax/pos.go](internal/syntax/pos.go)、[source.go](internal/syntax/source.go) | 前端位置与实例化字符读取 |
| [internal/types/type.go](internal/types/type.go)、[sym.go](internal/types/sym.go) | 真实 Type/Sym 结构、标志位与选定构造算法 |
| [internal/cmd/obj/link.go](internal/cmd/obj/link.go) | 真实 LSym/Reloc/Attribute 数据声明 |
| [internal/types/pkg.go](internal/types/pkg.go) | Universe 包表、字符串驻留与 Pkg 查找方法 |
| [internal/cmd/objabi/path.go](internal/cmd/objabi/path.go) | 符号路径编码与解码 |
| [internal/ir/func.go](internal/ir/func.go) | 链接函数名解析及辅助函数 |

手写适配示例为 [internal/types/adapter_context.go](internal/types/adapter_context.go)，生成来源和哈希记录在 [sync/generated.json](sync/generated.json)。

## 当前公开基础设施

`position` 暴露上游位置对象，`diagnostic` 提供实例诊断收集，`target` 提供显式布局输入；`types` 提供初步 Universe 和包符号查找，`gcbridge` 共用位置及 Type/Pkg/Sym 对象。见 [独立 consumer 示例](testdata/consumer/consumer_test.go)。每次编译需独立创建位置表和 base；XPos 必须用原表解码。同一实例的可变操作由调用方串行。
