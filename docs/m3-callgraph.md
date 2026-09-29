# M3 原生调用图与 profile 基础

本批为 readBodies → inline.CanInlineFuncs 的依赖迁移。迁入 ir/scc.go、cmd/internal/pgo 的原生数据与预处理流格式，以及 pgoir/irgraph.go 的调用图算法。完整内联分析、生产 readBodies、包导入驱动和源码到 SSA 仍待完成；本批不启用完整 PGO 编译流程。

## 原生算法与实例状态

- ir.Context.VisitFuncsBottomUp 使用上游强连通分量算法。递归函数按原算法分组，闭包跟随外层函数。bottomUpVisitor 仍是每次遍历的局部状态，只增加所属 IR Context，用于调用实例上的 MethodExprName 和拒绝跨图函数。
- pgo.Profile、NamedCallEdge、NamedEdgeMap、WeightInPercentage，以及 IsSerialized、FromSerialized、Profile.WriteTo 直接由脚本迁入。输入输出是 io.Reader/io.Writer，不读取全局文件路径，不解析原始 pprof。原生格式、顺序和错误语义保留。
- pgoir.Context 复用已有 typecheck.Context；目标函数列表、类型、位置表、调试配置和输出均来自同一 IR 图。上游 LookupFunc、PostLookupCleanup 两个跨包回调改为实例字段，未绑定仍按原生行为报错。生产 noder 的回调装配尚未迁入，测试绑定不能充当生产实现。
- 原生 IRGraph、IRNode、IREdge、CallSiteInfo 保留数据结构。Profile 增加 compilation 指针，使其 PrintWeightedCallGraphDOT 使用所属实例的函数列表和 TraceWriter。DOT 的 map 遍历顺序保留上游行为，不承诺稳定排序。
- Context.NewProfile 接受已处理的原生 pgo.Profile，调用迁入的 createIRGraph；只替代上游 New 的文件打开和解析外壳。空权重返回 nil。复制输入 map/slice，让返回结果保留自己的 profile 数据。独立实例可以读取同一份不变输入并发建图；单个 IR 实例内仍须串行，调用期间不得修改输入。
- 间接边按原生 ByWeight 顺序查找，缺少可导入 IR 的目标仍保留带权重的无 AST 节点。这是上游用于避免误选次热目标的算法行为。缺失回调不会伪装成查找失败；绑定回调显式报告的不可用目标才进入该分支。

对外提供 pgo、pgoir 包，均使用原生类型别名；调用图遍历通过已有 ir.Context 暴露。没有另建函数图、profile 模型或迁移配置文件。迁移规则在 internal/port/rules_pgo.go，原生 SCC 纳入既有 rules_ir.go。

## 验证

同一快照在固定上游和迁移库执行，比较互递归、自递归、闭包、普通调用边、负行偏移、未命中权重、导入查找顺序、不可见目标、非本包 caller 过滤、cleanup 次数，以及 profile 编解码和格式错误。测试使用真实原生 IR 构造器，差分测试中只绑定上游原有回调，不替换调用图算法。

16 个编译实例并发建图、打印各自诊断和 DOT；保留第一次结果后再次建图并修改原始 profile 输入，检查旧图、节点身份和权重仍独立。验证未绑定回调、跨实例 lookup 结果和 SCC 依赖拒绝。外部 consumer 通过公开包构建并消费原生图。流测试覆盖 reader/writer 错误传递。

下一步继续迁入 inline 的日志与启发式分析依赖，接通 CanInlineFuncs 和生产 readBodies，再完成包注册/导入及完整包 IR 入口。SSA 构造和原生编译器回接验收仍未完成。

本批验证完成：216 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 356.965 秒。go1.27.1 与 go1.27.0 的调用图/profile 快照一致；同一套规则升级仍有 2 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-callgraph-tests.log`。
