# M3 原生类型、对象与函数体读取

本批迁入 reader 的类型实例化、对象定义和函数体读取主体。helpers.go 与 types.go 完整迁入；函数体的表达式、语句、闭包、泛型字典、接口元数据及作用域路径均保留上游算法，没有用空函数或回调绕过依赖。

## 状态与适配

对象索引、函数体索引、导入函数体索引、待处理字典和函数体、方法包装登记、localPkgReader、内联标签编号及 init 重命名计数进入已有 noder.Context。原生 basics 表指向当前 Universe 中的类型槽，dirs 表也按实例初始化。配置、当前位置、目标包和当前函数继续使用既有 IR/typecheck；运行时字典写入复用同一图的 reflectdata、rttype 和 objw。

pkgReader 和 reader 保留原有字段加 compilation 所有者；pkgReaderIndex、readerDict 不新增字段，访问实例状态的私有方法显式接收 Context。原生 reader 构造函数仍由工具迁入。

ScopeMarker 增加一次性 Bind，供原生 reader 的零值 marker 在首个 scope 绑定既有位置表。重复绑定同一实例不重置已记录的 scope，绑定另一个实例会被拒绝；openScope 的 ContextFrom 使用这一小段适配。其他 Push/Pop/Unpush/WriteTo 算法不变。

泛型实例仍在原生 Type/Sym/Name/Func 图中缓存。闭包 addBody 在当前函数非空时立即读取函数体，原生字典写入真实 LSym 数据和重定位；方法包装仍登记到原生队列，后续驱动负责实际生成包装函数。没有另建类型或 IR 模型。

## 验证

实际源码经 syntax、types2、rangefunc、writer 再由 reader 读取。局部测试检查递归指针与别名身份、递归接口、常量精度转换、即时闭包体、泛型函数和方法、类型断言、控制流、原生作用域及函数状态恢复。八个并发实例检查旧函数体、符号、字典字节和 init 编号不被其他实例污染。

原生差分使用移植 writer 生成的同一 bitstream，分别交给原始编译器和移植 reader。writer 的完整字节另由已有双版本差分测试验证。函数体快照覆盖 386、amd64、mips、ppc64 和 amd64 关闭同步标记的配置，比较声明、函数签名、逐节点 IR、位置、局部变量、闭包捕获、作用域、包装登记、字典数据及重定位；额外覆盖可变参数、defer/go、select、带标签控制流、泛型约束和泛型闭包。

原始编译器使用其 readBodies 驱动；移植侧目前由测试代码排空原生队列，以单独验证本批 reader 算法。这个测试驱动不是生产 API，也不作为完整编译流程的替代实现。

## 尚待接通的阶段

当前公共入口仍是已检查 AST 到 Unified IR stub，以及既有原生对象操作。本批未提供新的完整包读取入口。下一步需要迁入包注册/导入、包初始化（包括原生 map 初始化分离）、函数体队列驱动及其内联依赖，再对外提供完整的包 IR 构造能力。方法包装生成、优化与 SSA 构造、上游回接和自举验收仍待完成。

本批验证完成：210 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 328.811 秒。go1.27.1 与 go1.27.0 各五组函数体快照一致；同一套规则升级有 2 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-reader-bodies-tests.log`。原有 writer 差分测试改用外部测试包，解除 noder → reflectdata → compare 与测试包之间的导入环，未削减比较用例。
