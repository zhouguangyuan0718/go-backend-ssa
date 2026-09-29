# M3 第一批：Go 源码到编译器 AST

已提供公开的 `syntax.Parse` 和官方 Go 1.27.1 的原生 AST、位置、遍历与打印接口。输入是显式 `io.Reader`、PosBase、错误/pragma 回调和原有 Mode；不引入新的 Parser/Session 抽象。`syntax.Type` 仍是 AST 中供 types2 填写的原始接口，与后续 IR 的 `types.Type` 不混为一套对象。

```go
file, err := syntax.Parse(
    syntax.NewFileBase("sum.go"),
    strings.NewReader("package p; func Sum(x, y int) int { return x + y }"),
    nil, nil, syntax.CheckBranches,
)
if err != nil { return err }
fn := file.DeclList[0].(*syntax.FuncDecl)
ret := fn.Body.List[0].(*syntax.ReturnStmt)
add := ret.Results.(*syntax.Operation)
// add.Op == syntax.Add
```

公开节点是内部编译器节点的别名，不拷贝为另一套 AST。gcbridge.SyntaxFile/SyntaxNode/SyntaxPos/SyntaxPosBase 与它们保持对象身份。结果及位置基对象由调用方持有，之后的 Parse 不会清空或覆盖旧树。不同调用可以并发，错误/pragma 回调也可重入解析另一份源文件；共享 Reader、回调或 AST 的同步仍由宿主负责。

## 移植范围与状态归属

保留上游 nodes/type/tokens、scanner/parser/branches、positions/walk、printer/dumper/testing 的文件和算法。syntax.go 仅选取 Parse 及其声明；读取文件路径的 ParseFile 不进入生产库，调用方提供 Reader。测试辅助的 CommentsDo/CommentMap 也直接接受 Reader，可作为普通库函数使用。

四处原有全局状态移入已有结构：

| 状态 | 存放位置 | 初始化 |
| --- | --- | --- |
| keywordMap | scanner.keywordMap | scanner.init 执行原来的表初始化循环，重新使用 scanner 时先清零 |
| invalid 分支目标哨兵 | labelScope.invalid | 每个 labelScope 分配独立 LabeledStmt，保留该 scope 内的身份 |
| tabBytes/newlineByte/blankByte | printer 对应字段 | 原有 printer 构造字面量中分配 |
| indentBytes | dumper.indentBytes | 原有 dumper 构造字面量中分配 |

Token/Operator 的 stringer 索引表按已有 ReadTables 规则局部化。关键字 hash 内的 `len(keywordMap)` 是上游固定数组的编译期常量，工具通过类型检查得到该常量值；hash 仍保留原函数签名，不添加全局表或 Context 参数。没有新增手工配置文件。

迁移工具的 ReceiverStateRule 按 go/types 对象身份处理已有 receiver；原有算法函数和方法签名保持不变。未知 free-function 状态访问、丢失初始化、字段冲突、不受支持的构造形式会停止生成，避免上游升级后静默丢失状态。

## 错误契约与上游差异

公开 Parse 在错误回调为 nil 时返回第一个错误和 nil 树；提供回调时尽量返回部分树，并通过回调报告全部错误。启用 CheckBranches 可检查 break/continue/goto/标签等语法层约束。调用方应先检查错误；部分树可能含 nil/BadExpr，不能假定 Walk/Inspect 接受每种损坏结构。

固定上游存在一个已复现的接口边界：分支检查直接调用 errh，因此 CheckBranches + nil errh 遇到错误会发生 nil 调用；提供 errh 时，分支错误也未写入 parser.first。移植的内部 Parse 保留该算法。公开入口通过一层局部回调收集第一个错误，并在无用户回调时触发原 Parse 的 Error 恢复路径，使分支错误进入正常返回契约。该差异有原始上游与移植内部源码的复现测试，也有公开接口回归。其他宿主回调 panic 仍向调用方传播。

pragma 完全交由宿主解释；Parse 能保留泛型、闭包、接口等语法节点不表示已经检查它们的类型或具备相应 SSA 降低能力。不会解析 import、构造真实 ir.Name、执行 noder/typecheck/walk 或产生 SSA。

## 验证

- 外部独立 module 使用公开语法树、运算节点和 gcbridge 别名，验证无需导入 internal。
- 错误与 Reader 失败、部分树、行指令、pragma、泛型语法、分支 Target 身份、打印往返、A→B→A 回调、24 个并发调用和旧树保活通过 race 测试。
- 对固定 go1.27.0/go1.27.1 执行上游可移植 syntax 回归测试，保留其断言及 testdata。只排除依赖 internal/testenv 的标准库遍历与优化关闭环境下的分配测试；错误测试中确认 fixture 已存在后去除 MustHaveGoBuild 环境门槛。ParseFile 仅从原始源码加入临时测试文件。
- 另行编译保留全局状态的原始上游 parser，对全部 syntax/testdata Go 文件和额外标签/泛型/指令案例比较打印树、结构 dump、诊断及 pragma；合法树额外比较所有遍历节点的类型与位置。仅规范化 Fdump 在 SliceExpr.Index 数组中打印的进程地址；诊断排序后比较，以容纳上游 map 遍历的报告顺序。
- 全量 race、两版差分、同输入再生成与升级检查通过，来源 Go 工作树保持不变。40 份生成产物含 38 份 Go 文件；跨补丁版本的生成 Go 文件内容无变化。日志：`_work/m3-syntax-tests.log`。

接下来是 types2 的预声明类型与 checker 状态解耦，接入显式目标 Sizes 和导入能力，再连到 noder/IR。M3 的 Go 源码到 SSA 验收以及原生编译器回接/自举尚未完成。
