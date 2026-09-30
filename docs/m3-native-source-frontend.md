# M3：原生源码检查与 unified 前端

`noder.Context.CompileParsedPackage` 现在接通完整原生 `checkFiles` 与 `unified` 主体。输入是同一 Context 的 `ParseFile` 成功结果，输出是已构造并检查的原生 `ir.Package`。入口执行依赖导入、语言版本诊断、匿名接口循环和 not-in-heap 限制检查、midway 重写及二次检查、rangefunc 重写、Unified stub 写入、读取、字典与函数体队列、遗漏检查断言，以及 runtime 来源函数的 Norace 标记。

## 使用与生命周期

先初始化原生 types/IR/typecheck、runtime、reflection 和 static-init，并通过 `BindImports` 配置目标对象头和依赖流；再解析源文件并调用 `CompileParsedPackage`。更新后的 [公开示例](../noder/example_test.go) 使用这个入口，并继续运行内联、包装、loopvar、pkginit、ABI wrappers、deadlocals、escape 和 slice。

此入口消费一个全新的编译实例一次。重复调用、已有本地 reader、已开始的公开依赖导入、已有目标函数/导入、nil/外来/重复文件和缺少的必需服务在原生变换前被拒绝。进入原生流程后发生诊断或 panic，也不能再次调用此入口。输入 AST 会被原生算法修改；同一编译实例由调用方串行使用，独立实例可以并发。

源码诊断以 `SourceErrors` 返回，包含独立的诊断快照。宿主提供器 panic 和内部编译器 panic 保持区别，原值传播。报告收尾发生在退出入口时；如主流程和收尾都 panic，保留主流程的 panic。

`ParseFile` 保持原有行为：解析失败返回 `ParseErrors`，不产生可编译的 ParsedFile，也不把失败解析混入其他成功解析的诊断。`CompileParsedPackage` 接收的是已成功解析的文件集合；原生文件打开、解析并发调度和命令行驱动尚未接入这个 API。

## 两次 checker 的绑定

上游在 `checkFiles` 中创建新 checker，并在 midway 修改 AST 后再次创建。适配器在每次创建时同步更新 noder 的 checker、类型导入器、导入包环境，以及 writer 的 any/comparable/rune 预声明对象。writer 的三个原生初始化表达式被分到独立初始化组，只重建该组，不重置已建立的后端符号、IR reader 或诊断状态。

上游对后端 `Pkg.Direct` 和目标 `Imports` 的重置保持在原始 `checkFiles` 函数体中。测试通过真实归档验证：首次读取 simd，重写后再次读取 simd，并加载新引入的 bridge 包；第二次检查后的基本类型属于新的 checker，专门化函数进入已检查的 IR。

## 原生 AST 诊断

完整迁入 `noder/dump.go` 和 `noder/html.go`，以及它们使用的原生 `ir.HTMLWriterBase`、缓冲输出、CSS 和 JavaScript。输出通过 `OpenSyntaxHTML` 和 `SyntaxWorkingDirectory` 提供；报告表、锁、节点编号和正则表达式均由实例持有。

共享 HTML 基类通过原生 Init 创建的缓冲对象获得 IR Context，保留原始 Init/输出方法算法与 noder 的嵌入关系。无需进程级 writer 注册表。报告覆盖 checked、midway 前后和 rangefunc 阶段，保留原生函数选择、文件名转义、列内容和关闭顺序。

## 生成器修正

两个独立回归先证明生成失败，再验证修正：

- 初始化组的依赖检查正确识别单参数和多参数的泛型实例化调用，包括 `reflect.TypeFor[T]()`；函数数组索引调用仍被拒绝，泛型 helper 跨组访问状态仍被拒绝。
- 外部结构体字段键（如 `types2.Config{Context: ctxt}`）通过类型对象确认归属，不与本包 Context 类型冲突；本地类型、变量和构造器名字冲突仍被拒绝。

## 验证

- 八个独立实例并发执行多文件源码、真实依赖归档、泛型、rangefunc、指令、IR 构建和内联；验证旧 checker 保活、writer 预声明对象切换、依赖句柄关闭以及报告输出和收尾。
- 真实 SIMD 实验归档验证重写、依赖重载、直接导入表重建、四种 AMD64 专门化、二次检查类型身份和中间 AST 报告。依赖是测试源码定义的普通类型，验证重写流程，不代表硬件 SIMD 运行验收。
- 八组错误与官方 Go 1.27.1 编译器逐条核对，包括语言版本、go:build、line 指令、匿名接口循环，以及不允许的 not-in-heap 类型实参、map key/value 和 channel。
- 入口边界、提供器 panic、报告创建和收尾同时失败时的主错误保留均有回归。
- Go 1.27.1 / Go 1.27.0 的完整原生源码入口快照均一致：完整 IR 节点/类型/位置、局部变量、对象登记、导入指纹、PGO 查找和完整 AST HTML 内容；HTML 比较只规范化作用域指针地址。
- 全量 `go test -race -timeout 30m ./...` 通过，公开源码入口示例通过；`syncgo check` 验证全部 464 个生成文件。

下一步接通包级前端阶段和完整 Build 驱动，再进行真实对象链接、程序执行、第二目标后端及工具链替换/自举验收。本阶段的源码到 IR 入口不等同于完整编译器替换。
