# M3：原生编译器指令

`noder.Context.ParseFile` 将原生 syntax parser 接到 noder 的原生指令处理器。返回的 `ParsedFile` 同时保存语法树和文件级记录；类型检查、rangefunc 改写后，`WriteParsedPackage` 将这些记录交给原生 writer。原有仅接受 AST 的 `WritePackage` 仍用于无文件级指令的输入。

## 源码与状态

迁入锁定上游 `noder.go` 的 `noder.error`、`checkUnusedDuringParse`、`pragma`、`isCgoGeneratedFile`、`safeArg`、`parseGoEmbed`，以及完整 `lex.go`。指令识别、参数解析、权限检查、标志蕴含关系和诊断内容保留原算法。

每个原生 `noder` 持有编译实例。标准库指令表在实例初始化时创建；目标 GOARCH/GOOS、包路径、Std/CompilingRuntime 和 FieldTrack 来自显式实例配置。解析前须配置目标操作系统和 Link.Pkgpath，不读取宿主环境。

原生算法覆盖函数 pragma、linkname/linknamestd、embed 参数、wasm import/export，以及 cgo 记录。保留 cgo 指令解析不代表完成 cgo 预处理、外部链接或完整平台支持；这些仍属于独立验收范围。

## 生命周期与调用边界

- 输入是显式 `io.Reader`；文件打开和关闭由调用者负责。
- `ParseFile` 保留原生错误通道，由独立收集器排空；返回前等待其结束。reader/parser panic 在收集器结束后继续向调用者传播。
- `ParseErrors` 保存完整原生错误和位置。失败不返回可提交给 writer 的半成品，也不修改共享位置表；包驱动可按文件顺序汇总错误。
- 同一实例可以并发解析多个文件，期间配置必须固定。类型检查、writer 和 reader 阶段仍须串行。
- `ParsedFile.Syntax()` 返回原生语法树，供该实例的 checker 和 rangefunc 使用。writer 验证文件归属，保留私有 linkname/cgo 记录，不重建仅含 AST 的 noder。
- 解析记录、检查后的类型图和原生 IR 在其他独立编译实例运行后仍有效。

这一步接通指令与 reader/writer 的实际数据路径。完整 `LoadPackage`、原生 `checkFiles`/导入器、unified 尾部及统一源码入口仍待接入。

## 验证

- 本地原生 parser → types2 → Unified writer → reader → IR 测试，检查 noinline、nosplit 隐含 nocheckptr、显式/隐式 linkname 和 cgo 数据；8 个独立实例并发执行。
- 单实例 32 个文件并发解析；语法/指令错误后继续成功；输入 reader panic 身份与收集器结束；外来/空解析记录在 writer 前拒绝。
- writer 对缺失 unsafe、重复 linkname、实例化目标以及不存在声明发出原生错误。
- Go 1.27.1 与 Go 1.27.0 各 320 组原生指令差分全部一致；原生和移植端均运行 race 检查。
- `go test -race ./...` 通过；公开 API 示例改用 ParseFile/WriteParsedPackage 后单独通过 race 检查，确认 noinline 保留到 IR。
- `syncgo check` 验证全部 448 个生成文件，`git diff --check` 通过。

差分参考直接从每个 pin 抽取原生方法，使用原生包级状态；不经过 Context/Owner/State/Call 改写。320 组输入组合覆盖 AMD64、WASM、AIX 的指令政策、标准库/runtime/FieldTrack、普通及 `_cgo_` 文件、line 重定向、合法及非法参数。比较完整诊断、位置、flags、linknames、cgo 字段、embed 模式和 wasm 记录；这不是这些架构的完整代码生成验证。
