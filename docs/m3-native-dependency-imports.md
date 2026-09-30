# M3：原生 noder 依赖导入

本阶段迁入完整原生 `noder/import.go`。同一次归档读取同时注册编译器的包/对象/内联体和 types2 的惰性声明，保留路径映射、查找顺序、兼容对象头检查、直接导入标记、缓存一致性和链接指纹登记。

## 显式依赖输入

`Context.BindImports(ObjectConfig, hostOS, open)` 在首次导入前绑定目标、工具链版本、宿主路径语法及归档提供器。配置中的 GOOS/GOARCH 必须与当前编译器匹配；实验位来自当前 Universe。`hostOS` 与编译目标独立，替代原生 `runtime.GOOS` 在 Windows 盘符判断中的读取。`Base.Flag` 保留原生 importmap/packagefile、ImportDirs、-D、安装后缀和 race/msan/asan 等输入。

提供器接收原生 `openPackage` 选择的文件名，返回含 Read、ReadAt、Seek、Close、Name 的 `DependencyFile`。它可以返回内存归档，不要求访问真实文件系统。原生查找仍尝试 `.a` 后 `.o`，遵循显式 packagefile、搜索目录和 GOROOT 的优先级。`readImportFile` 在成功打开后始终关闭句柄；惰性声明和函数体保留独立数据，不依赖该句柄继续存活。

`Context.Import` 可直接用作 `types2.Config.Importer`，同时将依赖加入本编译器 IR 命名空间。重复导入复用包对象，不重复打开归档。失败后原生直接导入状态可能已部分改变，因此公开入口将此实例标记为失败，要求丢弃；提供器 panic 保留原值。

## 原生实现与生成边界

- `readExportData` 保留完整原生函数体，包括对象头的精确字符串比较、export 格式和尾标记校验。
- `base.MapFile` 明确选择上游 `mapfile_read.go` 的便携读取版本，通过原生 `io.NewSectionReader` 复制区段并返回独立字符串。生成规则确认原始 `!unix` 构建条件后将该版本用于库；这一选择记录在来源清单中。它不使用 Unix 的永久 mmap；截断/异常区段行为应以所选便携实现为准。
- 流接口仍由原始 `*os.File` 方法签名推导。`os.Open` 的适配结果必须严格匹配所列方法集合；额外能力、缺失方法、变更签名均被拒绝。
- 宿主常量绑定只开放已审阅的 `runtime.GOOS`，要求原始常量确实被使用，且适配字段保持 string 类型。
- 生成器允许经过类型检查确认的导入包限定名（如 `types2.Context`）与本包生成的 `Context` 同名；本地标识符冲突仍被拒绝。

## 验证

- 真实官方编译器生成两包依赖归档；八个独立实例从内存导入，验证延迟完成的包身份、重复导入缓存、指纹、关闭次数，以及关闭时清空原始缓冲后仍能进行源码类型检查、原生 IR 读取和内联。
- 错误回归覆盖对象头不匹配、归档截断、尾标记破坏、提供器错误/panic/nil 结果、外来 checker，以及失败实例拒绝复用。
- 路径和查找测试覆盖宿主 Linux/Windows、盘符、本地路径、别名、自导入、保留路径和非法字符，以及目录、GOROOT、安装后缀和 race/msan/asan 的尝试顺序。
- 生成器回归覆盖宿主值、流工厂、构建变体及限定类型名的成功编译与拒绝路径。
- Go 1.27.1/1.27.0 的完整原生对照均通过：对象头/格式错误、路径、包身份、对象与函数体、指纹、unsafe 和收尾快照一致。另以原始便携 MapFile 的独立副本对照六组区段边界，包含零长度、EOF 和截断。移植快照启用 race。
- 全量 `go test -race -timeout=30m ./...` 通过，`syncgo check` 验证全部 455 个生成文件。

下一步接通原生 `checkFiles` 和 unified 前端驱动，让源码入口直接消费这些依赖，再继续整包编译、链接运行和工具链替换验收。
