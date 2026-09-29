# M3 原生可内联判定与函数体驱动

后续进展：原生包注册、函数查找和公开本地包读取已在[下一批](m3-package-reader.md)接通。下文保留本批完成时的范围与证据。

本批接通 `inline.CanInlineFuncs` 和生产 `noder.readBodies`。测试中手写的函数体队列循环已删除；源码经 syntax、types2、writer、reader 后，由迁入的原生驱动读取函数体，再计算真实的内联成本、局部声明列表及函数属性。本批不包含内联调用展开、完整包入口、walk 或 SSA 构造。

## 实例状态与对外入口

`inline.Context` 直接绑定同一图的 typecheck、inlheur、pgoir 和 logopt 实例。原生热点 callee/callsite 表、已有热点调用记录、CDF 阈值及预算都归本实例。保留上游 SCC 遍历顺序、hairyVisitor 成本计算、SIMD/闭包预算、编译指令拒绝原因、调用目标解析、递归/插桩检查及 PGO hash 决策；没有另写内联模型。

公开包 `inline` 提供原生 Context 别名及构造函数，可调用 `CanInlineFuncs`、`CanInline`、`InlineImpossible`、`InlineCallTarget`、`PGOInlinePrologue` 等已迁入方法。构造器检查依赖归属，入口拒绝外来函数/调用节点和 profile。优化事件进入传入的 logopt collector，文字诊断进入已有 `IR.TraceWriter`。

`noder.NewContext` 创建绑定同一图的内联状态。`noder.Context.ReadBodies(duringInlining, profile)` 对自身的 `Typecheck.Target` 执行原生私有驱动：字典优先，队列动态扩展，泛型声明登记，内联阶段的可内联分析与函数体释放，linkshared 方法保留，以及正常返回时的 LowerM 恢复，均为移植算法。原生 `PostLookupCleanup` 也已绑定给本实例 PGO；任意导入函数的 LookupFunc 尚待后续包注册/导入闭环。

同一实例的阶段操作保持串行。每个独立编译应创建自己的 Context 及依赖；没有 Reset 或全局串行锁。原生局部阶段清理不影响其他实例或已保留的 IR 元数据。

## 依赖和宿主边界

`base.HashDebug` 的匹配、包含/排除、位置和内联栈 hash 算法完整迁入。八个 hash 状态归现有 LinkContext；HashDebug 关联其创建实例的 Link.PosTable。原生 `NewHashDebug` 已有显式 pattern 和 writer 参数，因此只用同目录适配方法替换日志的进程环境/文件打开部分：保留锁、bisect marker 和传统 gossahash 两种格式，nil writer 丢弃输出。宿主需要文件时自行打开并传入 writer；库不读取 GSHS_LOGFILE，也不默认写 stdout。对外类型名为 `gcbridge.HashDebug`。

`objabi.LookupPkgSpecial` 所需的原生分类列表、惰性 OnceValue 缓存迁入 objabi 实例，由 Universe 持有。已有内部 PathContext 扩展并更名为 Context，原有路径算法及参数保持不变；没有共享可变的包属性缓存。types 的 runtime/reflect 名称及特殊包判定继续调用原生算法。

`ir.MatchAstDump`、`MatchPkgFn` 保留原生匹配。AstDump 的文件/HTML 驱动以小适配方法调用原生 FDump，输出到实例 TraceWriter；这与命令行自动生成 `.ast`/HTML 文件的行为有明确区别，宿主文件驱动尚未接入。

## 移植规则

`syncgo-m3-v12` 增加经审核的 `AdapterMethods`，用于上述 HashDebug 日志方法。工具先类型检查选中的上游原始方法，再移除明确指定的方法体；输出检查要求同目录适配文件确实提供同名方法，接收者指针/值形式及完整签名必须一致。上游文件和适配文件仍进入已有生成账本的校验，没有新增配置文件。

工具还支持给已检查的依赖包追加宿主函数签名，复用已检查的原生类型身份，同时记录本地签名的真实来源。补充签名不生成占位声明到库中，类型漂移或缺少真实适配实现仍然报错。

## 验证范围

- 原生 hash 对照覆盖 nil、bisect、传统包含/排除、函数名、位置、内联位置栈及后缀模式；单元测试覆盖输出隔离、不读取环境路径、并发实例和旧对象归属。
- 原生调用成本政策对照覆盖冷热调用、普通/大函数、成本边界、PGO hash 开关和 runtime/race/checkptr 特殊包判定。
- 源码函数体对照使用真实 CanInlineFuncs，比较 inlinability、cost、Dcl、CanDelayResults、属性和调用点评分。四种目标布局及已有模式之外，增加 duringInlining、linkshared 和 NewInliner 模式。
- 源码单元测试覆盖并发实例、原生拒绝原因与优化事件、AST dump、泛型函数体释放/方法保留、PGO 预算和跨实例拒绝。
- 独立 consumer module 通过公开 inline/gcbridge 接口调用原生分析和 hash gate。

验证完成：245 份产物通过 `syncgo check`；全量 `go test -race ./...` 通过（port 包 59.696 秒）。带来源仓库的 `TestPinnedUpstreamAndUpgrade` 通过（测试 351.20 秒，包 352.712 秒），两版各十一组函数体快照、hash 与成本政策快照均一致；同规则升级仍有 2 份 Go 产物变化，来源工作树保持不变。日志分别为 `_work/m3-native-inlining-unit.log` 和 `_work/m3-native-inlining-source.log`。

验证时修正了两处旧参照夹具的范围：类型对照使用原始 objabi 包属性实现；writer 对照只选择 writer/freePackage，reader 驱动由完整上游 overlay 对照覆盖。

下一步接通原生包注册与导入函数查找，再迁入统一内联展开及其符号依赖，形成完整包 IR 入口；其后继续优化、walk、ssagen 和上游编译器回接。
