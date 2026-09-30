# M3：SSA 构建分界与原生 continuation

公共低层入口 `ssagen.Context.ConstructSSA(fn, isPgoHot)` 返回 `*ssagen.Construction`。`Func()` 提供原生 Phi 插入完成、SSA Compile 尚未运行的函数图。输入函数须完成前端、walk 和符号准备；本入口还不是源码级包 Build。

## 原生算法与分界

迁移规则在固定源码的 `ssa.Compile(s.f)` 调用之前建立分界。原有构图和 Phi 算法仍逐语句来自上游；原生后半段整体保留为捕获同一 state、frontend 和参数的闭包。普通 `BuildSSA` 和编译器驱动立即执行它，继续完整 SSA Compile、帧分配、open-defer 元数据与参数寄存器 spill 记录。

生成器按原始 AST 验证唯一顶层调用锚点和 state 的局部结构体声明，拒绝缺失或重复锚点、state 遮蔽、命名返回和跨闭包边界的 label/goto。生成清单记录具体函数、来源调用、adapter 方法和 state 名称。独立夹具验证上下文重写、原生 defer 次数、实例隔离及延迟执行。

## 图生命周期

原生 worker cache 会在下一函数构建时清空。ConstructSSA 为每次构建创建一份专属 cache，不随 native worker 数量倍增，退出时恢复 Context 的原 worker cache；因此同一 Context 的后续 ConstructSSA 或 BuildSSA 不会覆盖已经返回的图。

`ContinueNative()` 在同一图上执行原生后半段，成功后 `NativeCompleted()` 为 true，结果可交给 GenSSA。开始执行即消费 continuation；发生 panic 后也不能再次执行。消费状态由私有共享对象保存，复制 Construction 句柄不会复制执行权限。完成符号编号、替换 SSA 配置或嵌套捕获时拒绝恢复。调用者须在恢复前保留原编译配置及前端 IR；同一 Context 内操作须串行，独立 Context 可并发。

原生构图的 deferred popLine 在 ConstructSSA 返回时执行。保存的 continuation 恢复后半段所需的原始行号栈，退出时还原；不会重跑构图或再次执行前半段 defer。原生 DebugInfo 占位对象本来就在构图前半段创建，因此 Built 分界不以 DebugInfo 是否为 nil 判断；测试检查寄存器分配及 RegArgs 尚未产生。

## 验证

- 并行实例测试保留循环/Phi 和指针写入函数图，穿插 worker-cache BuildSSA，再恢复两张图；逐次检查图内容及 cache 独立性。
- 生命周期回归检查外来 IR 在修改前拒绝、worker cache 恢复、配置变更拒绝，以及失败的 continuation 只执行一次、复制句柄仍共享同一次恢复。
- 固定上游差分在原始 `ssagen/ssa.go` 的 SSA Compile 前直接插入观察返回，独立比较构图结果；恢复路径与未经修改的上游完整编译比较指令、机器码、重定位、帧、GC/PCLN/DWARF 和 defer 元数据。
- Go 1.27.1 / Go 1.27.0 各 19 组构图差分通过，覆盖 14 个架构以及 ARM 软浮点、AMD64 race 插桩、关闭寄存器 ABI、关闭优化和大图 Phi；各 9 组“先构建全部函数图，再逐一恢复并 Flush”的差分通过，包含默认/无优化、race 插桩、寄存器 ABI、DWARF 配置、大图 Phi、内联和 SIMD。
- 全量 `go test -race -timeout 30m ./...` 通过；多 worker 缓存恢复、失败与复制句柄回归通过；`syncgo check` 验证 474 个生成文件。
- 本批源码编译的四种模式继续通过完整归档、链接和运行对照；无 profile、raw/gzip pprof 和 preprocessed profile 的完整归档与官方一致，实际热调用内联结果通过机器码检查。
- 最终源码完整 dist bootstrap 通过（415.69 秒），记录 669 个不同包。保留阶段编译器及最终编译器的完整 14072 字节测试归档均与官方一致；强制重编译得到的编译器与 toolchain3 全部 35468031 字节一致，最终工具链编译运行程序的结果与官方一致。

完整自举和程序执行限定 Linux/amd64、CGO_ENABLED=0、官方 Go 1.27.1 bootstrap SDK。多架构构图差分不等同于其他目标的完整工具链验收。

## 后续

源码级 Build 已完成包准备、声明固定点的阶段编排与结果元数据，见[源码到 Built SSA](m3-source-build.md)。artifact 和严格版本校验已见[后续 M4 阶段](m4-import-artifact.md)。EarlyOptimized 需要独立的原生阶段截点。Linux/arm64、cgo/race 驱动、最低 bootstrap SDK 及更广上游回归继续分别验收。
