# M3 原生静态初始化调度与包初始化读取

本批完整迁入 staticinit/sched.go，并接通 noder 的 pkgInit/pkgInitOrder。包变量初始化、显式 init 函数、大 map 初始化分离和声明读取均使用原生算法。函数体队列仍由测试驱动排空，生产 readBodies 的内联依赖尚未迁入，因此本批不等于完整的包编译入口。

## 状态与公共能力

staticinit.Context 绑定已有 typecheck.Context 和同一图的 reflectdata.Context；静态数据写入复用其中既有 staticdata/objw。构造器拒绝另一编译实例的依赖。statuniqgen、mapinitgen、varToMapInit 和 MapInitToVar 改为实例字段，配置、当前位置、目标包、类型和诊断直接使用既有 IR。

原生 Schedule 保留 Out、Plans、Temps 和 seenMutation，增加 compilation 指针。公开 staticinit.NewContext 和 Context.NewSchedule 构造这一原生对象；Schedule.StaticInit/StaticAssign 产生实际静态数据，并把必须运行的语句按原顺序保留到 Out。Context.StaticName、StaticLoc、OutlineMapInits 和 AddKeepRelocations 直接使用移植实现。AnySideEffects 是原生纯函数的公共转发。单个编译图内串行操作，独立实例可并发。

原生调度保留运行时修改屏障：一旦初始化表达式可能修改本包全局变量，后续初始化不能再复制该变量的初始静态值。FIPS 包继续按上游规则把需要全局地址重定位的初始化保留为动态语句。StaticAssign 识别内联调用的原生 IR 形状，保留参数地址/副作用检查、常量截断和表达式替换，不依赖另建的初始化模型。

map 初始化分离保留原生阈值、禁用和强制模式及副作用判断。MapInitToVar 保留初始化函数到全局 map 的关系；AddKeepRelocations 写入真实 R_KEEP，消费其原生待写集合。这个阶段应在所有需要分离的 map 初始化登记完成后执行。

## 验证

- 实际源码依次经 syntax、types2、writer、noder 的包初始化读取和函数体读取，再由 Schedule 调度。检查标量复制、数组/指针/slice/string/bytes、接口和函数值、map、调用与动态复制的顺序。
- 八个并发编译实例保留旧符号、字节、map 登记和静态名称；检查运行时修改后的全局复制不能被提前静态化，重复消费 R_KEEP 集合不重复写入。
- 外部 consumer 通过公共 staticinit API 构造原生 Schedule，验证实际静态数据；另检查外来类型、名称及反射图拒绝。
- 原生差分扩展既有 noder_body 快照：386、amd64、mips、ppc64，以及 amd64 的无同步标记、FIPS、禁用分离、强制分离，共八组。比较原始和保留的动态初始化 IR、调度计划数量、临时变量、全部已涉及的符号数据和重定位，以及既有函数体/字典/作用域结果。
- 额外构造真实 AS2/INLMARK/BLOCK/GOTO/LABEL 形状，比较内联静态折叠的禁用、取址拒绝、整数截断及原输入保留。这是折叠算法验证，完整内联优化流水线仍未接通。

下一步继续包注册/导入、readBodies 及内联依赖，随后提供完整的包 IR 构造入口。pkginit 的链接器任务生成、方法包装、优化与 SSA、上游回接及自举仍待完成。

本批验证完成：211 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 346.978 秒。go1.27.1 与 go1.27.0 各八组扩展初始化/函数体快照一致；同一套规则升级有 2 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-staticinit-tests.log`。
