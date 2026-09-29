# go-backend-ssa

将 Go 1.27 官方编译器从源码解析到 SSA 的实现移植为可嵌入进程的 Go 库，供其他编译器后端直接消费。

库维护基础设施、公共数据模型、状态归属和生命周期；移植工具从固定上游提交抽取算法，通过可复现的结构改写接入这些设施。每次调用拥有独立编译状态，返回结果在后续调用期间仍有效。

当前状态：**M1 已完成，M2 已完成原生类型布局与预声明类型初始化，M3 已提供源码解析和原生 types2 类型检查入口；尚无 Go 源码到 SSA 的 Build API。** 当前可用编译器 syntax.Parse/AST、实例独立的 types2.Config.Check/Info、位置、诊断、目标布局输入、包符号查找、实例化 SSA 特殊类型、tuple/results 及指针/切片/数组/map/channel 构造、未定类型与常量映射、字段/结构体/签名构造，以及上游 CalcSize、InitTypes、类型等价与比较算法；完整前端和编译流程仍待移植。

- 语言范围：Go 1.27；其他语言版本尚不支持。
- 首个源码基线：官方 `go1.27.1`，提交 `862c888e612ac346c7c4d99c9392bdfd265f33b0`。
- 交付形式：普通 Go module，进程内调用，返回本库的 SSA 对象。
- 对外保证目标：调用隔离、无共享可变编译状态、可重入、跨调用并发安全。
- 并列验收目标：可替换官方 Go 1.27 编译器对应实现，接回原生后端并完成工具链构建。
- 初始目标配置：`linux/amd64` 和 `linux/arm64`；`linux/386` 已加入布局隔离验证，不表示支持完整的 386 编译器。

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
11. [M2 类型与 Universe](docs/m2-universe.md)：第二至七批实现、对外入口及类型布局边界。
12. [M3 原生语法树](docs/m3-syntax.md)：Parse、AST、无全局解析状态与上游差分。
13. [M3 类型检查迁移](docs/m3-types2.md)：源码类型检查入口、Context 隔离、显式导入/尺寸及完整上游差分。

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

设定 `GO_SSA_SOURCE_REPO` 时，测试额外验证官方 go1.27.1 再生成、两版上游位置/符号回归与 SSA 类型切片差分测试、源码树未变以及 go1.27.0 → go1.27.1 升级；仓库需包含两个 tag。不设置该变量时仅跳过这项集成测试，其他测试照常执行。

编译器源码从 `src/cmd/compile/` 以下保留目录、包名和文件名；辅助依赖分别归入 `internal/cmd/` 与 `internal/go/`。当前移植代码包括完整位置模块、完整前端语法解析器与 AST，以及 types/ir/objabi 的结构和算法切片；完整类型系统和编译器尚未实现。

移植后的源码直接保存在本项目，和手写适配、转换规则一起纳入版本管理。使用本库时不需要先运行移植工具；维护者更新上游版本时再重新生成。

| 当前移植代码 | 内容 |
| --- | --- |
| [internal/cmd/src/pos.go](internal/cmd/src/pos.go)、[xpos.go](internal/cmd/src/xpos.go) | 位置、压缩位置、位置表与调试标记 |
| [internal/syntax/parser.go](internal/syntax/parser.go)、[scanner.go](internal/syntax/scanner.go)、[nodes.go](internal/syntax/nodes.go) | 编译器语法解析、独立扫描状态及原生 AST |
| [internal/types/type.go](internal/types/type.go)、[sym.go](internal/types/sym.go) | 真实 Type/Sym 结构、标志位与选定构造算法 |
| [internal/cmd/obj/link.go](internal/cmd/obj/link.go) | 真实 LSym/Reloc/Attribute 数据声明 |
| [internal/types/size.go](internal/types/size.go)、[universe.go](internal/types/universe.go)、[identity.go](internal/types/identity.go) | 原生布局/延迟队列、预声明类型初始化及等价判断 |
| [internal/types/pkg.go](internal/types/pkg.go) | Universe 包表、字符串驻留与 Pkg 查找方法 |
| [internal/cmd/objabi/path.go](internal/cmd/objabi/path.go) | 符号路径编码与解码 |
| [internal/ir/func.go](internal/ir/func.go) | 链接函数名解析及辅助函数 |

手写适配示例为 [internal/types/adapter_context.go](internal/types/adapter_context.go)，生成来源和哈希记录在 [sync/generated.json](sync/generated.json)。

## 当前公开基础设施

`position` 暴露上游位置对象，`diagnostic` 提供实例诊断收集，`target` 提供显式布局输入；`types` 提供初步 Universe 和包符号查找，`gcbridge` 共用位置及 Type/Pkg/Sym 对象。见 [独立 consumer 示例](testdata/consumer/consumer_test.go)。每次编译需独立创建位置表和 base；XPos 必须用原表解码。同一实例的可变操作由调用方串行。

后端当前可直接使用实例中的上游 SSA 类型和构造函数：

```go
layout, _ := target.NewLayout("linux", "amd64")
u, _ := types.NewUniverse(layout)
value, _ := u.Scalar(types.Int)
pair := u.NewTuple(value, u.TypeMem)
results := u.NewResults([]*types.Type{u.TypeMem})
// pair.IsMemory() == true；results == u.TypeResultMem。
```

`NewTuple` 和 `NewResults` 保留上游返回 `*Type` 的签名；跨 Universe 或 nil 元素会 panic。`NewResults` 按上游语义保留输入 slice，调用后不能将其修改为外来类型。SSA 单例字段也应当作只读绑定使用。以上接口尚不生成 SSA 函数图。

复合类型也直接使用上游入口：`u.NewPtr(value)`、`u.NewSlice(value)`、`u.NewArray(value, n)`、`u.NewMap(key, value)`、`u.NewChan(value, types.SendRecv)`；通过 `Elem`、`Key`、`NumElem`、`ChanDir` 等方法读取。`u.NewPtrCacheEnabled` 只控制当前实例。指针构造已使用实例目标宽度，完整类型布局仍待实现。

未定类型使用上游名字：`u.UntypedInt`、`u.UntypedRune`、`u.UntypedString` 等；常量映射为 `u.UntypedTypes[constant.Int]` 和 `u.DefaultKinds[constant.Int]`。`Type.IsUntyped` 等分类方法直接复用上游实现，完整预声明类型初始化和类型检查仍待接入。

字段和签名使用 `u.NewField(pos, sym, typ)`、`u.NewStruct(fields)`、`u.NewSignature(recv, params, results)`。通过 `FieldType`、`Params`、`Results`、`ResultsTuple` 等原生访问器消费，公开 `types.Field` 与 `gcbridge.Field` 是同一对象类型。字段偏移和结构体布局仍待 CalcSize。

类型布局入口示例见 [独立消费者测试](testdata/consumer/consumer_test.go)。先创建 Universe，再调用原生 `u.InitTypes(defTypeName)`；回调为传入的 Sym/Type 提供宿主的 `types.Object`。随后可直接调用 `Type.Size/Alignment/Registers/HasPointers` 与 `u.CalcSize/PtrDataSize/AlgType/Identical`。当前不提供假 `ir.Name`；宿主绑定契约及失败处理见 M2 文档。

源码解析入口及可运行示例见 [syntax](syntax/syntax.go) 和 [M3 记录](docs/m3-syntax.md)。`syntax.Parse` 后可调用 `types2.Config.Check` 获得类型检查结果，示例见 [types2](types2/example_test.go)。尚无 SSA 构造入口。
