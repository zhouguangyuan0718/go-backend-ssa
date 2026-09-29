# M3 原生静态数据和函数值符号

本批迁入 staticdata/data.go 中除 fileStringSym 外的全部函数，包括字符串符号、静态地址/slice/常量初始化，以及函数值符号的登记与写入；同时迁入原生 obj.Bool2int。代码仍由固定 Go 1.27.1 源码通过 typed rules 生成，没有手写替代算法。

## 原生入口和实例归属

公共 staticdata.Context 直接使用迁入的 Context；NewContext 绑定既有 IR.Context 和 objw.Context，要求它们共享 Universe、Link 和位置表。slicedataGen、funcsymsmu、funcsyms 从包全局迁入实例；目标布局、当前包和 CompilingRuntime 标志从现有实例取得。IR 名称和常量的入口拒绝外来 Universe。原生 LSym 没有 owner 字段，调用方仍需保证符号属于同一 Link。

StringSym 保留原生命名、缓存和存储：长度不超过 100 字节的字符串直接 quote 命名，更长内容通过原生 cmd/internal/hash 算法命名；产生字节对齐的只读、可内容寻址符号。StringSymNoCommon 为链接器的具名方法引用产生独立的非内容寻址符号。

InitConst 按目标字节序写入 bool、整数、浮点、复数和字符串头；nil 初始化保持原生无写入行为。InitAddr/InitAddrOffset 保留原生地址重定位；InitSlice/InitSliceBytes 保留目标 slice 头布局。相同字节的 slice backing store 不共用，其计数器只属于当前实例。恢复性诊断写入已有 IR.Diagnostics，内部 Fatalf 改为 panic。

## 并发边界

数据写入仍由调用方串行。FuncLinksym 保留上游后端的较窄并发契约：先准备并冻结输入名称/类型图，不并发执行其他包符号查找和可变阶段，再并发登记函数值；登记全部结束后调用 WriteFuncSyms。

实例 mutex 保护原生函数集合及对应包符号查找。WriteFuncSyms 按原生链接名排序，写入指向 ABIInternal 入口的函数指针。它不是通用幂等刷新 API：上游仅在 CompilingRuntime 且目标已写入时跳过重复生成，本批原样保留；普通流程应在该阶段调用一次。

## 验证与范围

- 32/64 位、大小端检查字符串数据/命名、常量字节、地址重定位、slice len/cap 和独立 backing store。
- 96 次并发函数登记验证去重、身份、延迟写入、ABIInternal 和原生排序，随后检查 runtime 的已写入符号分支。
- 十二个并发实例验证计数器、旧符号和旧字节保活，以及外来输入拒绝和既有诊断收集。
- 独立外部模块只使用公共 API，构造原生静态字符串头与函数值符号。
- 上游快照覆盖四目标的字符串命名阈值、全部常量类别、nil、数据顺序、完整字节、属性及全部重定位，并使用两版固定源码验证同一套迁移规则。

fileStringSym 仍需后续接入显式文件输入和对象文件流式写出；本批没有导入 os.Open 或宿主文件路径依赖，不声称支持完整 embed。当前产物是原生 LSym 数据与重定位，还不是可链接对象文件。下一步接完整类型描述符、名称编码与 ITab，继续推进 reader 和最终编译器回接。

本批验证完成：208 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 237.397 秒。Go 1.27.0/1.27.1 四目标静态数据快照均一致；同一套规则升级仅 1 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-staticdata-tests.log`。
