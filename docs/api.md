# 公共 API 与后端契约

已实现源码级 `compiler.Build(ctx, Request)`，返回 Built 或 EarlyOptimized 阶段的 `compiler.Package`，见[源码到 Built SSA](m3-source-build.md)。它复用 `compiler.Compile` 的显式输入，按选择的阶段运行原生流程，独立保留每张图及包元数据；当前支持 Linux/amd64 和 Linux/arm64，依赖通过 `Request.Artifacts` 传入严格版本化 artifact，或通过 `Request.Imports` 使用显式原生归档适配器。

下文是完整目标契约；其中完整能力集仍按回归逐项验收；EarlyOptimized 的实际边界见[阶段记录](m5-early-optimized.md)；artifact 的实际契约见[阶段记录](m4-import-artifact.md)。当前实际声明以 `compiler/build.go` 与 `compiler/compiler.go` 为准。原生归档输出继续使用 `compiler.Compile`；低层 `ssagen.Context.ConstructSSA` 另提供一次性原生 continuation，见[SSA 构建分界](m3-ssa-construction.md)。

## 入口

```go
func Build(ctx context.Context, req Request) (*Package, error)

type Request struct {
    ImportPath string
    Files      []SourceFile
    Target     Target
    Artifacts  map[string]*Artifact
    Imports    map[string][]byte // 显式原生归档适配器
    Options    Options
}

type SourceFile struct {
    Name string
    Data []byte
}

type Options struct {
    Stage Stage // Built 或 EarlyOptimized；按实现里程碑开放
}
```

语言默认 Go 1.27，也可通过显式选项选择已支持的较早语言版本；artifact 严格匹配所选版本，不隐式接受宿主 Go 语言版本。源文件已经由调用者按构建约束筛选，但影响文件语义的指令仍须处理。Target 显式包含 GOOS、GOARCH、CPU 特性等已支持配置；零值或不支持配置报错，不能回落到宿主环境。

Build 不修改输入；调用期间输入字节不得被外部并发改写。需要在返回后保存的输入数据由库复制或转为自身拥有的对象。所有外部内容通过输入或 resolver 获取，不读取 cwd、GOFLAGS、GOSSAFUNC 来控制语义。

## 依赖解析

当前编译单包，Build 返回 `Package.Artifact`，`Bytes` / `DecodeArtifact` 支持独立存储与加载；调用方通过 `Artifacts` 提供不可变 artifact 和精确依赖闭包，依赖编译顺序由调用者负责。后续可提供单独的 BuildGraph，复用同一套阶段设施。

artifact 包含原生类型信息以及泛型实例化、跨包内联所需的函数体/Unified IR 信息。解码进入本次 Universe，不接受另一次 Build 的可变 *types.Type / *ir.Package 作为隐式共享缓存。

artifact header 标记 format、精确上游版本/提交、语言、目标、实验、race 模式、包身份、原生对象引用能力及依赖哈希；完整容器用 SHA-256 验证。校验严格匹配，不假定不同 Go patch 可互换。官方 .a 已作为显式 `Imports` 适配器验收，同一路径的两种输入不能混用。

Package → artifact → 新 Universe 的三包往返已验证泛型、类型别名和跨包内联。纯 artifact 闭包使用名称引用；原生归档混合输入保留其原生对象身份，并通过 `NativeObjectReferences` 显式标记。处理标准库从最小依赖闭包起步；不默认与宿主标准库缓存混用。

## 输出对象

```text
Package
  Artifact       严格版本化的类型和函数体导出
  Metadata       语言、源码 pin、目标、阶段、能力集
  Functions      本库 ssa.Func 图，包含生成函数
  Types          本次调用的类型环境/可查询类型集合
  Symbols        本包定义、外部引用、ABI/可见性等属性
  Globals        数据、对齐、重定位和特殊语义属性
  Init           包初始化顺序与依赖描述
  RuntimeTypes   必要类型描述、方法/接口/字典相关信息
  Imports        本次使用的依赖身份
  Positions      源位置与内联信息
  Diagnostics    成功调用的警告/说明
```

不只返回函数列表；遗漏字符串、静态初始化、泛型字典、方法包装函数或类型元数据会让后端无法完成包编译。M3 的子集可只填充其支持的内容，遇到未支持语义必须拒绝。

## SSA 的公开可用性

直接返回本库维护/移植的 Func、Block、Value、Type、Op。保持与上游结构尽量接近，初期不额外设计另一个 IR。

Aux 的每种可达对象必须有外部可用的具体描述或 typed accessor，例如符号引用、局部槽、调用描述、类型和偏移。不可要求后端通过反射或 unsafe 解读内部 ir.Name/obj.LSym。采用原始结构公开还是 accessor 在 M2 按具体 Aux 清单决定，不能悄悄丢字段。

Built/EarlyOptimized 中的 Block.Values 不是执行顺序。消费方按照控制流、数据依赖和 memory chain 调度，并正确补全 Phi；返回阶段描述中记录是否已排序。自定义后端可做自己的调度，不强制运行原生 machine schedule。

公开图版本绑定本库 v0 与精确前端能力，不承诺直接等同官方 cmd/compile API。底层原始 opcode 数值不作为长期存储格式保证。

## runtime 与 ABI

第一版采用 Go 1.27.1 编译器的源码语义和 runtime lowering 约定。目标为在机器指令选择和原生栈帧分配之前交付；这不代表 IR 中没有 ABI 分析、SP/SB、closure context 或 runtime 调用。

| 库负责 | 后端负责 |
| --- | --- |
| 正确的 Go 前端语义、初始化与 SSA 内存依赖 | 目标指令/LLVM IR 等生成 |
| 调用的逻辑签名及已有 ABI 元信息 | 将调用和返回映射到实际 ABI |
| 分配、闭包、defer、runtime 调用及相关元数据的完整描述 | GC、栈、safepoint、异常/恢复与运行时集成 |
| 静态数据、符号引用与类型元数据需求 | 数据布局输出、重定位、对象与链接 |

替换 runtime 的后端可实现明确转换，但 Build 第一版不宣称任意 runtime 可插拔。若某项 Go 语义无法由当前输出完整表达，需完善适配或拒绝该能力，不能标记成功。

## 错误、确定性与并发

用户错误返回带有源码位置、类别、消息的 error；失败不返回可被误用的半成品 Package。内部未知 panic 不吞掉。详细策略见 [状态设计](state.md)。

对于固定源码、依赖 artifact、目标、阶段和工具版本，输出的规范化图及诊断排序应可重复；不要求对象地址或上游临时 ID 完全相同。不依赖 map 遍历或绝对源码根目录实现稳定顺序。

跨 Build 调用可并发；同一结果的只读并发需要完成 lazy-cache 审计后才宣布支持。主动修改同一结果由调用方同步。取消后库不能继续异步写入已返回对象。


## 已实现的完整目标

源码 `compiler.Build` / `compiler.Compile` 支持 Linux AMD64 和 ARM64。AMD64 使用 `GOAMD64=1..4`；ARM64 使用 `GOARM64` 的原生版本/特征语法，空值规范化为 `v8.0`，并要求 `GOAMD64=0`。返回元数据及 artifact 均记录规范化后的目标；混合架构字段或不匹配的依赖会报错。低层 ARM64 处理函数通过公开 `arm64.NewContext` 安装。实现和端到端验证见 [M6 记录](m6-arm64.md)。

`Options.Race` 在两种源码阶段及原生编译入口均采用固定上游的插桩、checkptr 和包排除策略。`Metadata.Race` 保留请求模式；import artifact 的模式必须匹配当前请求。`InstallSuffix` 是显式驱动输入，不会自动查找任何宿主依赖。见 [race 验收](m6-race.md)。

`Options.Shared` 请求原生可共享代码生成，`Metadata.Shared` 及 artifact 保留该模式并严格匹配依赖。原生 `Compile` 支持该选项的归档输出；PIE 归档和真实链接验收见 [M7 记录](m7-runtime-regression.md)。

`Options.AssemblyListing` 为 `Compile` 选择原生 `-S` 的输出详细级别，结果通过 `Result.AssemblyListing` 返回。零值不打印；1、2、3 逐级包含普通符号/指令、DWARF 辅助符号及符号索引。该输出独立于 `AssemblyHeader`；`Build` 不接受非零级别。详见 [汇编输出验收](m7-assembly-listing.md)。

`Request.Debug` 使用上游 `-d` 语法，明确提供调试输出文件、源文件、目录、计时及其它宿主服务。`Options.OptimizationReport` 对应 `-m` 详细级别。`Result.DebugOutput` 和 `Package.DebugOutput` 保存独立的原生报告文本；结构化 Diagnostics 仍保留 warning，展示原生文本时不应重复打印这些 warning。源码错误不返回部分结果，调试帮助返回 `*DebugHelp`。完整行为及验证见 [compiler 调试验收](m7-compiler-debug.md)。

`Options.ClobberDead` 和 `Options.ClobberDeadReg` 对应原生死栈槽、死寄存器调试开关；`Debug.ClobberDeadHash` 显式提供函数 hash 后缀筛选。库不读取 `GOCLOBBERDEADHASH`，替换驱动负责传入。见 [Go codegen 验收](m7-codegen-regression.md)。

`Options.Spectre` 接受原生 `all,index,ret` 设置语法，并保留目标限制、错误及 DWARF producer 记录。见 [Spectre 验收](m7-spectre.md)。

`compiler.Error.Report` 保存独立的原生诊断文本，`Error()` 使用该文本；`Diagnostics` 保留结构化坐标与消息。`Options.UnlimitedErrors`、`OmitErrorColumns`、`ActualErrorFiles` 分别对应 `-e`、`-C`、`-L`，列号开关仅影响文本。详见 [错误报告验收](m7-error-reports.md)。
