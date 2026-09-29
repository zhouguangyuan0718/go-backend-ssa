# M3 原生类型描述符与 ITab

本批接入 reflectdata 的完整描述符生成路径，包括名称/包路径编码、方法信息、各类类型描述符、map 元数据、ITab、基础类型和插件表，以及接口方法的链接器使用标记。helpers.go 和 map.go 现完整迁入；reflect.go 除已有的匿名包 makefield 适配外，所有原生函数均由工具迁入。makefield 继续通过现有 Universe 提供原生 nil Pkg 所表示的匿名包，不另建字段模型。

同时迁入原生 TypeHash、IsDirectIface、Type.IsPtrElem 和 compare.IsRegularMemory。比较谓词从输入 Type 的既有 Universe 取状态，不保留另一份可变编译状态。typeSig、ptabEntry 和 typeAndStr 的结构与原生声明直接对照。

## 使用和状态归属

reflectdata.NewContext 现在显式接收三个已有实例：typecheck.Context、已 Init 的 rttype.Context、staticdata.Context。构造器验证它们使用同一 IR、Universe、Link 和对象写入器。调用方应先完成 typecheck.InitUniverse，并在生成描述符/ITab/相等函数前调用 InitRuntime。当前包和目标选项需在使用相关缓存前设置。

调用 NeedRuntimeType 登记原生 Type，然后 WriteRuntimeTypes 按上游排序并处理生成过程中追加的队列。生成数据仍附着在同一 Type/Sym/LSym 上；可通过 TypeLinksym 和 Link.Data 取得字节、属性、目标对齐以及原生重定位。原生类型缓存和 Siggen 标志仍属于 Universe 中的对象，不复制类型图。

ITabLsym 保留上游允许不实现接口时生成 dummy ITab 的语义；ITabAddrAt 是已通过类型检查的转换入口。ConvIfaceTypeWord 现在覆盖 T2E、T2I 和 I2I，并保留已提供的字典 TypeWord。WriteBasicTypes 保留只在编译 runtime 时使用的前提，WritePluginTable 消费现有 typecheck.Target.PluginExports。

dnameCount 进入 Context，原生 kinds 查表按实例初始化；已有队列、map 布局和算法缓存继续复用。名称数据保持原生 varint 编码、tag/embedding/exported 标志、包路径重定位及内容寻址属性。不会把缓存改为全局或跨编译共享。

这些阶段按实例串行，不与 GCSym 并发登记阶段交错；不同实例可并发。ITab 生成按原生 defer 恢复调用前的 Pos 和 CurFunc。其他原生生成路径可能改变当前位置，仍由编译阶段管理。失败生成不是事务回滚：内部 panic 后实例可能保留部分符号，调用方应丢弃失败实例，不复用它继续编译。

## 保留的编译器分工

方法表和 ITab 引用真实方法/包装方法符号，方法指针使用原生弱重定位；本版本 methodWrapper 只选择符号，包装函数体由 Unified IR 创建。本批没有用占位函数填充这些符号。reader 的类型/对象/函数体实例化尚未接通，因此这还不是可链接编译结果。

类型描述符使用前面迁入的 GC 和 hash/equality 路径；普通 GC mask 从待生成集合移除，按需 mask 保留原生 BSS 单元及后端完整位图路径。map 描述符保留 split/interleaved group 布局、间接键值标志和 hasher。导入类型索引、别名归一化、泛型实例/shape、Noalg、typelink 和动态链接处理均来自上游。

## 验证

- 32/64 位检查具名类型、哈希、相等闭包、队列消耗、普通/按需 GC mask；检查 map 的间接键值标志和两种 group 布局。
- 真实多方法 ITab 验证原生大小、ABIInternal 弱方法重定位、符号身份和调用环境恢复；dummy ITab、严格转换失败和跨实例拒绝另行验证。
- 十二个并发实例保留旧描述符、ITab、函数列表、名称计数器和字节，检查新实例不会污染旧结果。
- 外部 consumer 只通过公共 API 生成类型描述符、ITab 和接口转换 type word，并验证对象身份。
- 原生差分覆盖 386、amd64、mips、ppc64，以及 amd64 的 split-map、runtime 基础类型和动态链接配置。比较全部数据字节、属性、对齐、类型哈希、导入索引、重定位和生成的函数 IR，覆盖递归生成、具名/匿名/导入类型、别名、泛型实例、shape、方法提升、可变参数、插件表和具名接口方法标记。
- ABI Kind 比较底层数值，避免依赖当前未迁入的 abi.Kind.String；没有因此省略实际描述符数据。

下一步继续原生 reader 的类型/声明实例化和函数体读取；完整源码到 SSA、原生编译器回接和自举验收仍待完成。

本批验证完成：209 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 270.731 秒。两版固定源码各七组描述符快照一致；同一套规则升级仅 1 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-descriptors-tests.log`。
