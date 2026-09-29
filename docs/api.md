# 公共 API 与后端契约

本文件为拟议接口，不是已存在的 Go 声明。

## 入口

```go
func Build(ctx context.Context, req Request) (*Package, error)

type Request struct {
    ImportPath string
    Files      []SourceFile
    Target     target.Config
    Imports    ImportResolver
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

语言固定为 Go 1.27，文件内版本约束仍按该版本规则检查；不隐式接受宿主 Go 语言版本。源文件已经由调用者按构建约束筛选，但影响文件语义的指令仍须处理。Target 显式包含 GOOS、GOARCH、CPU 特性等已支持配置；零值或不支持配置报错，不能回落到宿主环境。

Build 不修改输入；调用期间输入字节不得被外部并发改写。需要在返回后保存的输入数据由库复制或转为自身拥有的对象。所有外部内容通过输入或 resolver 获取，不读取 cwd、GOFLAGS、GOSSAFUNC 来控制语义。

## 依赖解析

第一版编译单包，Imports 返回匹配本库前端的不可变 artifact；依赖编译顺序由调用者负责。后续可提供单独的 BuildGraph，复用同一套阶段设施。

artifact 需要包含类型信息以及泛型实例化、跨包内联所需的函数体/Unified IR 信息，不能仅用公开导出符号代替。解码进入本次 Universe，不接受另一次 Build 的可变 *types.Type / *ir.Package 作为隐式共享缓存。

artifact header 至少标记 format、upstream revision、语言版本、目标和实验配置；内容使用 hash 验证。初期严格匹配，不假定任意 Go 1.27 patch 的 gc export data 都可互换。直接导入官方 .a 是未来单独验收的适配器，不是默认承诺。

为打通第一条跨包路径，库需有 Package → 自身 import artifact 的生产能力，并验证泛型函数体信息未丢失。处理标准库从最小依赖闭包起步；不默认与宿主标准库缓存混用。

## 输出对象

```text
Package
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
