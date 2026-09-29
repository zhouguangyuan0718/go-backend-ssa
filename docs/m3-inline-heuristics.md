# M3 原生内联启发式与优化日志状态

本批继续推进 readBodies 的内联依赖，迁入原生 ReassignOracle、函数属性分析、调用点评分和优化日志记录。完整 CanInlineFuncs、内联 IR 展开、生产 readBodies 和包编译入口仍待接通，尚未形成源码到 SSA 或上游替换能力。

## 状态归属与接口

原生 ir.ReassignOracle 保留已有 fn、singleDef 和 funcAssigns 字段。Init 从传入函数获取 IR 实例，其余需要状态的方法从已有 fn 获取实例，未新增 oracle 对象模型或编译器全局入口。公开 ir.ReassignOracle 是原生类型别名。函数 IR 修改后重新 Init 是原生局部分析缓存的重建，不会重置编译实例或修改旧 IR 图。

inline/inlheur.Context 复用既有 typecheck 与 pgoir 实例，构造时检查二者属于同一图。fpmap、评分调整表、参数/结果调整映射、调用点表、评分临时缓存与调试调用点集合均改为实例字段。原生参数、结果、控制流、表达式和调用点分析器保留各自数据结构，只增加所属 Context。AnalyzeFunc、ScoreCalls、GetCallSiteScore、UpdateCallsiteTable、BudgetExpansion 及评分输出使用迁入代码；FuncProps 的紧凑序列化和枚举格式化也保留原生实现。

上游默认 trace_off.go 与 reassignment 的一致性检查关闭配置一同迁入，不读取调试环境变量。命令行函数属性 dump 文件驱动尚未迁入；调用点评分文本输出走既有 IR.TraceWriter。TearDown 与 ScoreCallsCleanup 保留上游阶段语义，回收当前实例的分析临时状态；已记录在 fn.Inl.Properties 中的结果不会因此消失。用户仍需按原生编译阶段顺序调用，同一个图的分析串行，独立编译图可以并发。

logopt.Context 迁入原生 Format、loggedOpts 和互斥锁。LogOpt、LogOptRange、Enabled、NewLoggedOpt 保留原生行为。库提供 Snapshot 返回独立 slice，事件及其参数仍是原生记录；字段访问器提供位置、范围、pass、函数名、事件和参数。事件参数可能引用 IR 或嵌套原生事件，必须按原生借用约定管理生命周期。支持并发记录，记录期间不要改变 Format；不提供清空全局日志的接口。命令行目录解析和 LSP 文件输出尚未提供，没有空的 flush 实现。

noder.NewContext 现在把 typecheck.HaveInlineBody 绑定到本实例的 unifiedHaveInlineBody。查询真实的 reader 表及函数元数据，不以测试回调或固定布尔值代替生产查询。

## 移植工具

syncgo-m3-v11 增加常量表的只读 range 遍历支持，用于上游生成的枚举字符串代码，同时拒绝把表元素用作 range 赋值目标。规则仍是 Go 代码，无新增配置文件。修复外部回调状态的签名检查：两侧都忽略参数名称，同时继续严格检查参数和返回值类型。两个问题均先由回归测试复现，再修复验证。

## 验证范围

- 真实源码经过 syntax、types2、writer 和 noder reader，函数体进入原生启发式分析；与固定上游比较函数属性的序列化、属性文本及逐调用点评分。分别验证 386、amd64、mips、ppc64，及既有无同步标记/FIPS/map 分离模式。
- 为隔离启发式阶段，差分夹具给两侧提供相同的已计算可内联元数据及预算回调；这不等于 CanInlineFuncs 已迁入。生产函数体队列仍采用原有测试排空方式，未伪装成正式驱动。
- 16 个实例配置不同评分调整，检查属性、缓存、非候选函数评分及阶段清理隔离；验证内联区域的新调用点、parent 关联、标记和重新评分，检查跨图函数及依赖拒绝。
- ReassignOracle 与上游比较单定义、二次赋值、函数值赋值记录和取址失效；并发实例保留独立查询状态，拒绝跨图节点。
- 优化日志与上游比较开关、字段、范围、嵌套说明及非法模式。独立实例内分别并发写入，检查快照、参数 slice 及旧记录保留。
- 独立模块通过公开 ir、inline/inlheur、logopt 接口运行分析并读取真实记录。

下一步迁入 inline/inl.go 的原生可内联判断，处理其 AST 调试、符号初始化和 PGO hash 等依赖，接通生产 readBodies；随后继续包注册/导入和完整包 IR 入口。

本批验证完成：241 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 369.483 秒。go1.27.1 与 go1.27.0 各八组函数体/属性/评分快照一致，原生 oracle 与日志对照一致；同规则升级仍有 2 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-inline-heuristics-tests.log`。
