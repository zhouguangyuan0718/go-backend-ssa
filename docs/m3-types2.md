# M3 类型检查迁移

当前完成的是 types2 所需的迁移能力和上游基本类型表验证，尚未发布 types2 检查入口。syntax.Parse 仍只产生原生 AST。

## 复用上游 Context

计划让原有 types2.Context 同时持有编译实例的预声明类型、Universe/Unsafe 包、类型参数编号和初始化表，不新增并行的 Session/Universe 公共对象。原 Context 的泛型实例缓存、互斥锁及构造函数实现保留；迁移工具将源包变量的实际类型追加为字段，公开构造函数先调用原构造函数，再按 go/types.InitOrder 和源文件顺序执行变量初始化与 init。

规则使用 ContextRule.Existing，仍写在 Go 代码中。需要的字段类型和跨文件 import 从已检查的源声明推导，不另写重复的 Context 结构定义。引用状态的原 Context 方法直接使用自身；其他对象仍通过已有 owner 规则获取所属实例。原构造函数若依赖迁移状态、分配需绑定 owner 的对象或调用本包函数/方法，工具会拒绝，需要先审查初始化依赖。

## 保留源码形式的改写

- 点导入按 go/types 对象身份分析；仍使用的导入保留，选择声明后不再使用的导入删除。最终输出继续严格检查导入和环境依赖。
- 无字段名的结构体字面量按原类型字段顺序转成键值字面量，再添加 owner，保持表达式求值顺序。含空白字段的这种字面量被拒绝，不能丢弃可能有副作用的表达式。
- 切片、数组和 map 中省略类型的值/指针字面量，根据原检查结果补 owner，避免手工重写 types2.Typ 和 basicAliases。
- whole-file 规则可以显式指定 Context 和 init；未分类的包变量仍被拒绝，不静默删除。

## 验证边界

合成跨文件用例覆盖原缓存构造、外部字段类型 import、初始化依赖、原 Context 方法、派生节点 owner、旧节点保留、多实例并发及无效规则拒绝。另从固定 go1.27.1 和 go1.27.0 Git 对象读取真实 Basic 声明、Typ/basicAliases 初始化表达式，与未改写的原表比较枚举、信息位、名字及别名身份。这个表用例是显式截取的迁移测试夹具，不跳过生产 universe.go 的 init，也不提供简化的 Basic.String 或伪造 Type 实现。

下一步移植完整 types2 类型图、预声明 Universe 与 checker，接入显式 Sizes/Importer/GoVersion。之后衔接 noder/IR。源码到 SSA、原生编译器回接和自举仍待实现。

## 按实际依赖传递 Context

`ContextRule.Selective` 根据 go/types 对象身份计算函数依赖的闭包：只有读取实例状态、调用依赖实例的函数、捕获这类函数作为回调，或构造需绑定 owner 的对象时，才将自由函数变成 Context 方法。纯函数、泛型纯函数、枚举方法及局部同名函数保持原签名。

对不能挂 owner 的切片 receiver，以及允许 nil receiver 的私有辅助方法，`Unit.ContextParams` 显式列出候选方法。只有其中实际需要状态的方法才增加 Context 参数，并改写直接调用和方法表达式调用。保留原 receiver 与可变参数，禁止将签名已变化的方法静默用作原类型的方法值；需要时另作明确适配。这不修改公开的 Type/Sizes 等接口。

`OwnerRule.Existing` 可复用原结构体中的 `*Context` 字段，例如 Config.Context 和 Checker.ctxt。工具检查字段确实存在且类型符合规则；原先已写出的字段初始化表达式保留，新分配对象缺少该字段时才填入实例。新建和外来参数检查沿用现有 owner 机制，不再增加另一根重复的 owner 指针。

只含常量的字符串索引表也可通过 ReadTables 局部化。索引写入、取址、切片或其他逃逸仍被拒绝；实际 types2.VarKind.String 无需因为名称表而依赖 Context。

这些能力的测试覆盖传递依赖、闭包与回调、方法表达式、可变/未命名参数、nil receiver、纯方法值、既有字段类型校验和跨实例输入拒绝。完整 types2 原源码的试跑已通过原始包的类型检查，尚未完成全部状态改写；nopos、临时 operand 与诊断输出等仍需继续适配。试跑不等于可用的类型检查 API。
