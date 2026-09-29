# M3 原生包注册、导入查找与本地包读取

本批迁入 `noder.readPackage`、`LookupFunc`、`lookupFunction` 和 `lookupMethod`，把原生对象注册表、导入内联体索引与现有 reader 接通。外部 Go module 现在可以把已检查源码写成 stub，再读取为原生 `ir.Package`，无需访问 internal 或编写函数体排空循环。这是前端读取阶段的 IR；完整 unified 驱动、wrapper 生成、内联展开、优化、walk 和 SSA 仍待接入。

## 接口与状态

- `Context.RegisterPackage(pkg, data, localStub)`：薄适配入口，创建原生 decoder/reader 并调用迁入的 readPackage。data 是去掉 archive/export 头部的 Unified IR bitstream；true 对应本地 WritePackage stub，false 对应已完成重定位的导入数据。每个编译实例中每个包只注册一次；本地 stub 只能对应本实例 LocalPkg，重复注册或外来 Universe 会被拒绝。
- `Context.ReadLocalPackage(data)`：调用原生对象注册、pkgInit 和 readBodies，返回原有 `Typecheck.Target`。调用方先配置 LocalPkg、目标、runtime 声明、ReflectData 与 StaticInit；这些依赖必须属于同一图。该入口不执行尚未移植的统一驱动尾部和 wrapper 生成。
- `Context.LookupFunc(fullName)`：保留原生完整符号名解析、普通函数查找和方法查找。接收者符号、alias/interface 拒绝及泛型缺少实参等行为来自上游代码，不新增函数模型。成功返回本实例原生 `*ir.Func`；失败返回原生 error。
- `Context.PostLookupCleanup()`：使用上一批迁入的 readBodies 处理查找期间产生的待办项。普通导入函数仍保持导入身份，不被误加到本地编译列表。

`noder.NewContext` 同时绑定本实例的 PGO LookupFunc 和 PostLookupCleanup。PGO 对额外导入函数的归属现在使用真实 reader；调用方需事先注册依赖数据。完整 types2 导入器和依赖加载驱动尚未提供，不能据此宣称完整多包编译可用。

objReader、importBodyReader、bodyReader、包缓存和工作队列仍属于既有 Context/Universe。`types.PkgMap` 也由原生全局函数转换为 Universe 方法。多个实例可读取相同不可变导出字节，同时拥有不同的包、类型、符号和函数；返回对象保留其所属图，无需 Reset。

原生 `init` 函数不在普通对象注册表中，而是通过私有根的包初始化记录读取。对应测试改用 ReadLocalPackage 验证 init.0 命名和保留，不人为把 init 填入普通函数注册表。

## 宿主边界

RegisterPackage 不访问文件系统，也不启动编译器。导入数据由宿主提供，官方 `.a` 文件的读取/头部剥离仍是宿主工作。decoder 的反同步诊断输出绑定既有 TraceWriter。

readPackage 的原生导入路径诊断进入实例 Collector；ErrorExit 以 panic 中止当前调用，保留解码器既有 panic 约定，不退出进程。该低层 API 不承诺失败实例可继续编译；失败之后的新实例不受影响，最终 Build 入口的统一错误收敛仍在后续阶段。

可运行的源码到读取阶段 IR 示例见 [noder 示例](../noder/example_test.go)，独立模块验证见 [consumer](../testdata/consumer/consumer_test.go)。

## 验证

- 使用固定官方 Go 1.27.1 编译器产生真实 finalized export 数据，测试注册的对象根、内联体索引、包名、`.inittask`、函数和两种接收者方法，及不适用的查找路径。编译器和 archive 工具只出现在测试中。
- 并发八个实例读取同一导出数据，检查缓存身份、旧函数保留、普通导入函数不进入本地编译列表；本地源码产生 IR 后，PGO 通过正式 noder 绑定查找导入函数。
- 本地源文件覆盖泛型、闭包、包初始化和 rangefunc；对照夹具增加原生 readPackage、注册对象集合与函数查找。
- 独立外部 module 和公开示例完成 syntax → types2 → writer → ReadLocalPackage，检查原生函数体及保留的 types2 图。
- 导入路径错误仅中止当前实例；外来包、重复本地 stub 被拒绝；下一实例仍可成功读取同名包。

`syncgo generate` / `check` 通过，迁入文件总数仍为 245。`GOTOOLCHAIN=go1.27.1 go test -race ./...` 通过（其中 port 包 54.621 秒），包含独立外部 module 验证；日志见 `_work/m3-package-reader-unit.log`。

`TestPinnedUpstreamAndUpgrade` 通过（测试主体 361.85 秒，包总计 363.518 秒）。Go 1.27.1 和 Go 1.27.0 的原生源码对照均通过，包括 11 种本地 reader/body 配置和新增 finalized import 对照。两个版本的 reader 读取同一份由官方 Go 1.27.1 生成的数据；这不表示导出夹具由 Go 1.27.0 编译器生成。同一套迁移规则从 1.27.0 升级到 1.27.1，只有两个 Go 输出文件变化，上游工作树保持原状。日志见 `_work/m3-package-reader-source.log`。

下一步迁入统一内联展开及其符号初始化依赖，随后接通完整 unified/包 IR 驱动和剩余前端阶段，继续向 SSA 与原生编译器回接推进。
