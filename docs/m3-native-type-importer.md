# M3：原生 types2 导出数据导入器

`importer.Context.ReadExport` 将官方编译器的最终 Unified 导出数据解码到指定的 types2 环境。`Import` 可直接用作 `types2.Config.Importer`，返回已加载且完整的依赖包。生产实现只消费调用者传入的数据，不执行工具链或搜索依赖文件。

## 保留的原生算法

迁入 `cmd/compile/internal/importer/ureader.go` 全文，以及 `support.go` 的断言和字典索引声明。包描述符、预声明类型、对象惰性加载、别名、类型参数、递归字典、普通及泛型方法、延迟约束和位置记录均使用原生算法。

导入器与 checker 绑定。所有原生构造函数使用这一 checker 的 Universe；pkgReader/reader 持有导入实例。包字典在导入前验证路径及 checker 归属。原生方法和函数的签名、来源及适配均记录在生成清单中。

泛型方法的合成名称保留在原生包表中。方法内容通过 `Named.Method` 读取；普通声明遍历只解析合法 Go 标识符，避免将 `Node.Map` 这样的内部记录当作包级函数强制加载。

输入为最终 Unified bitstream，不含 `.a`/对象头和开头的 `u` 字节。包各加载一次。先导入 B 时，原生惰性加载可以按需创建 A 的声明；随后导入 A 自身的导出数据，补全同一个包及其类型身份。

## 两处实例边界

原生导入器对 builtin 对象使用 `nil` 包，并通过 `Package.Scope` 取得进程 Universe。移植规则将这个方法的直接调用绑定到 `checker.PackageScope`，把原 receiver 作为显式参数保留下来。新增的通用方法适配规则验证精确来源、完整签名和 receiver 类型；拒绝方法值、方法表达式、提升 receiver、隐式取地址、未使用规则及含直接 recover 的方法。

递归惰性约束还暴露了原有 types2 实例化边界的问题：原生 `Named.expandRHS` 可以创建一个仅用于去重的临时 `Context`。该缓存不应成为类型所属的 Universe。本地 `A[_ B[any]]` / `B[_ any]` 递归约束导入曾触发 typeWriter 的外来类型检查。修复保留原 receiver 的缓存、锁和 ID 表，使 instanceHash、lookup、update 中的类型运算从原始 Named/Alias/Signature 取得所属环境，不重置任何保留图。

## 使用边界

- 同一个导入环境及其惰性查询串行使用；独立环境可以并发运行。
- 数据应来自兼容且可信的原生导出产物。损坏数据沿原生路径 panic，调用方须丢弃该图；惰性读取的错误也可能在后续查询中暴露。
- 原生低层 `ReadPackage` 保留共享包 map 和 decoder 输入，供后续 noder 导入流程接入；公开 `ReadExport` 管理自己的包 map。
- 依赖路径解析、归档头/目标检查、文件 provider、原生 `checkFiles`、统一源码 Build 和工具链替换仍待接通。

## 验证

- 官方 Go 1.27.1 生成含 unsafe、递归泛型、泛型方法、别名、常量、结构标签和 line 指令的两包导出数据。
- 8 个独立环境并发执行 B→A 导入、类型实例化及使用依赖的源码检查；同一 import path 的两份不同常量产物保持隔离，旧类型图保活。
- 独立实例化缓存被 8 个不同类型环境并发使用，覆盖 Named、Alias 和泛型 Signature；公开 importer API 完成导出数据到源码检查的测试。
- 外来 checker/包 map、空 map、重复加载、缺失依赖及损坏数据边界测试通过；错误不影响新建环境。
- 生成器方法适配测试通过，验证 nil receiver、变参、receiver 单次求值和错误规则拒绝。
- Go 1.27.1 与 Go 1.27.0 的原生导入器差分均通过；每个 pin 的 8 组输入、408 条观察结果完全一致。
- 最终 `go test -race ./...` 通过。方法适配清单标记修正后，全部 450 个生成算法文件与差分时逐字节相同。
- 最终 `syncgo check` 验证全部 450 个生成文件，`git diff --check` 通过。

差分在临时 overlay 中运行各 pin 的原始 importer/types2/syntax/pkgbits，不使用移植规则改写参考算法。每个 pin 覆盖两种依赖加载顺序和 386/AMD64/ARM64/PPC64 四种类型布局，比较惰性对象、类型/方法签名、位置、常量、身份、结构大小及偏移和后续源码检查结果。这是类型布局验证，不代表对应目标的完整代码生成已经完成。
