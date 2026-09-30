# M3 walk 的架构输入

本批开始迁入 `internal/walk`，保留原生 `walk.go` 的 mayCall 和 `range.go` 的 cheapComputableIndex。它们分别判断表达式是否可能调用函数，以及元素宽度是否适合目标架构的索引寻址。完整 Walk、排序、表达式/语句改写仍待迁入，本批没有公开可调用的 Walk API。

## 去除架构全局依赖

原生代码读取 `ssagen.Arch.LinkArch` 和 `ssagen.Arch.SoftFloat`。前者绑定到已有 `Typecheck.IR.Base.Ctxt.Arch`，后者是 walk Context 上显式的 bool 输入。类型表和 instrumentation 标志继续使用同一编译图。没有额外的架构接口或新目标描述模型。

同步迁入 `internal/go/goarch` 和 `internal/cmd/sys` 的原生架构族常量组，保留数值及别名关系，避免手写一份架构枚举。

## 迁移工具的字段规则

现有 StateRule.Variable 现在允许 `变量.直接字段`，例如 `Arch.SoftFloat`。替换以 go/types 对象身份、字段选择和根变量共同匹配，局部同名对象不受影响。规则仍在 Go 源码内，没有新的配置文件。

生成器加载用于检查的临时依赖签名时，只保留明确选择的原生结构体字段声明。未选择的原生后端回调可能依赖尚未迁入的 SSA/代码生成包，因而不进入这一临时签名。精确字段类型、来源文件和完整源码哈希仍被校验；这些裁剪后的签名结构不会输出到生产库。

限制是明确的：根变量须有显式类型，结构体须在该源码文件直接声明；仅支持直接命名字段，不支持隐含提升或多层投影。整值、整值地址和未审阅字段的访问被拒绝，不允许把同一全局的整值绑定与字段绑定混用。涉及投影字段的 unsafe.Offsetof 也被拒绝，因为它观察原结构体布局。多个消费者的字段需求先取并集，维持签名对象身份；字段删除、字段类型改变或适配字段类型不匹配都会失败。

工具回归先证明旧规则拒绝字段绑定（`_work/m3-projection-baseline.log`），再验证实际生成、运行和并发隔离，包括闭包捕获、字段地址、局部同名变量、实例初始化时读取外部字段、跨包共享签名、整值泄漏拒绝和上游字段变化。

## 验证与边界

原生对照包含 Go 1.27.0/1.27.1 的 386、amd64、mips、ppc64 布局。每个布局枚举 11 个原生架构族与七种元素宽度，并比较软件浮点和 instrumentation 开关的四种组合；表达式覆盖常量、名字、整数/浮点/复数运算、浮点比较、转换、可能 panic 的除法及带初始化的逻辑右侧。比较的是原生判定，不是目标机器码执行。

定向回归验证八实例并发及旧目标输入不变。原生 mayCall 的输入仍须经过相应的前置改写；本批不放宽其原有阶段约束。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test -timeout 5m ./internal/port -run '^TestPinnedWalkTarget$' -v
```

工具版本 `syncgo-m3-v21`，共 273 个生成文件。两版四目标对照通过（32.85s，`_work/m3-walk-target-source.log`）。generate/check 验证全部 273 个文件，完整 `go test -race -timeout 20m ./...` 通过（`_work/m3-walk-target-unit-final.log`）；本阶段未重复完整前端双版本流水线。

下一步处理原生 walk 中零值 staticinit.Schedule 的实例绑定，并把已确认的包级计数器、运行时字段缓存和预分析缓存迁到 walk 实例，接入完整 Walk 流程。

局部零值调度器绑定随后已实现，见 [m3-zero-schedule.md](m3-zero-schedule.md)。
