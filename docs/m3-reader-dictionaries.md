# M3 reader 字典与共享编译状态

本批将 noder 绑定到已有的 IR/typecheck 实例，并迁入原生泛型字典读取、shape 复用、实例名称和 shape 函数签名算法。没有新增独立的字典模型或编译会话抽象。

## 状态归属与公共入口

`noder.NewContext(checker, tc)` 接收已有的 types2.Context 和 typecheck.Context。调用方先初始化原生类型 Universe，并保证 Universe 与 Link 使用同一个 PosTable。构造器检查目标宽度、位置表和类型初始化状态。

Flag、Debug、Diagnostics 分别指向现有 IR 的 Base.Flag、Debug、Diagnostics，trace 也使用 IR.TraceWriter。构造器保留调用方配置；Lang、Complete、SyncFrames、RangeFuncCheck 等应由调用方显式设置。原来分别传入 Universe/Link 的调用方式已更新，公共示例和外部 consumer 展示新的用法。

原生初始化只在构造器执行一次。WritePackage 不再重新初始化 reader 缓存或清空共享诊断，只将本次新增的错误作为返回错误；先前诊断与已创建的类型、shape、符号继续有效。单个实例按编译阶段串行使用，不同实例可以并发。

## 原生算法

迁入 reader.go 的 Shapify、shapeSig、objDictIdx、类型参数名称读取，以及 readerDict 的 mangle、段偏移、字数和 varType。加入原生 typed 和 SplitMethSuffix 依赖。所有实现由移植工具生成。

Shapify 使用既有 Universe 的 ShapePkg 缓存，保留基础约束的指针合并、NotInHeap 例外、递归实例化类型及超长名称哈希。输入的类型和符号必须属于当前编译实例。公共 Context 可直接使用 Shapify；字典仍是 reader 内部的原生结构。

readerDict 不增加所有者字段，需访问实例状态的私有方法显式传入现有 Context。名称拼接、方法/字典/运行时类型/ITab 段偏移和 `.dict` 参数签名均保留原生语义。原生 objDictIdx 的切片借用行为也保留：只有隐式类型参数、没有显式参数时，shape 替换可能更新传入切片；不能将其描述为不可变输入接口。

## 验证范围

- 从实际源码经 syntax、types2 和原生 writer 生成 bitstream，读取函数/类型/方法约束字典，检查符号复用、参数数量和段布局。
- 检查共享配置/诊断/trace 身份、重复写入后旧 shape 与诊断保留、跨实例输入拒绝，以及十二个并发实例的结果隔离。
- 使用原生 codec 编码字典记录，在移植实现与原始编译器间逐项比较 shape、实例名称、字典内容、偏移、签名和输入切片变化。覆盖 386、amd64、mips、ppc64，以及带/不带同步标记的记录。
- 原生声明对照保证 readerDict、readerMethodExprInfo、pkgReaderIndex 没有引入额外所有者字段；外部 consumer 检查公共入口的共享状态和 shape。

字典记录差分不是完整泛型实例化验证。typIdx、objIdx 和函数体读取仍未接通；原生 addBody 在闭包路径会立即读取函数体，必须作为真实依赖链迁入，不能用空回调绕过。下一步继续这条链，之后仍需接通编译驱动、SSA 生成、上游回接与自举验收。

本批验证完成：209 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 294.295 秒。go1.27.1 与 go1.27.0 各四组字典快照一致；同一套规则升级仅 1 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-reader-dictionaries-tests.log`。
