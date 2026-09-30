# 回接 Go 1.27 原生编译器

这是 2026-09-29 用户新增的最终验收目标，与独立后端使用并列：完成库化后，使用本项目替换官方 Go 1.27 编译器对应实现，并继续生成可用的 Go 程序。

不是只把原编译器作为差分 oracle，也不是在库中重新启动原编译器。集成编译器必须实际调用本库移植实现，不同时运行两套前端或退回未替换路径。

## 目标结构

```text
独立使用：自定义驱动 → 本库前端/SSA → 自定义后端
回接使用：Go cmd/compile 驱动 → gcbridge → 同一本库前端/SSA
                                      → 原生 lowering/代码生成/对象输出
```

本库生产依赖始终不导入宿主 `cmd/compile/internal/*`。Go 源树一侧负责标志、文件/导入配置、诊断、对象输出和驱动环境适配；编译状态仍显式传递。

## 共享对象模型

类型、IR 节点、符号、源码位置、ABI 描述和 SSA 必须有明确的唯一 owner。原生后端中的 liveness、debug、栈帧和架构代码若继续消费这些对象，需要使用同一模型或经过验证的转换。

优先使用版本绑定的公开 `gcbridge` 面连接原生消费者；局部可以生成类型别名和转发函数。类型别名不能解决私有字段/私有方法访问：遇到这类边界必须扩展桥接接口，或将相关消费者连同算法一起移植，不能复制一套同名类型再用 unsafe 转换。

目录与上游对应不改变 Go 的 internal 可见性：宿主编译器通过本 module 的公开 `gcbridge` 接入；gcbridge 在库内导入 `internal/ir`、`internal/types`、`internal/ssa` 等实现。同包私有操作由 `adapter_*.go` 提供必要入口。公共别名和转发不维护第二套算法。

inspect 和后续声明审计应记录：出边依赖、宿主消费者、私有访问、对象身份和回调。M1 的候选根包含 gc，可见许多原生消费者；它不是完整的可替换边界证明。M2 要补充由核心数据模型到架构消费者的反向引用清单。

## SSA 阶段与后端继续执行

独立 API 的 Built/EarlyOptimized 边界仍保留。回接路径还需提供版本绑定的继续执行入口，让剩余优化、ABI 展开、lowering、调度、寄存器分配和原生栈帧/对象输出正确衔接。

不能为了回接而在所有独立 Build 中默认运行 native tail。应在同一阶段实现上分离驱动，禁止重复运行 escape、writebarrier、expand calls 等阶段。

第一阶段保留原始符号/类型/ABI 信息并建立 provenance；后续设计公共 Aux 与结果时，需要同时证明自定义后端与原生消费者所需的信息未丢失。

## 驱动和 bootstrap

在固定 go1.27.1 的隔离源码树中生成集成补丁，不改日常 GoALLC 工作树或宿主 GOROOT。补丁固定本库 revision / 内容 hash，按 Go 源树的 cmd module 与 vendoring 规则安排依赖，构建时不隐式访问网络。

Go dist 的 bootstrap 阶段、所需语言版本、生成器、编译器自举和标准库重建都需单独验证。独立 module 的 go test 成功不代表可直接用于 bootstrap。

## 最终验收

1. 证明编译入口进入本库：固定版本标识、受控探针或集成测试中的调用证据；不以“链接到了库”作为证据。
2. 隔离树构建替换后的 cmd/compile，通过最小 Go 源码到原生可执行程序测试。
3. 重建标准库与工具链，完成规定的 bootstrap/self-host 检查。
4. 运行 compiler/SSA/codegen、ABI、闭包、接口、泛型、初始化、defer、GC/debug 等回归；未支持目标必须明确列出。
5. 在相同目标与输入下对比官方 go1.27.1 的执行结果；布局、诊断和代码生成差异要解释。
6. 库的并发/状态隔离测试仍通过；不允许为了接回单进程驱动恢复全局会话。

M2 起做最小桥接试验，M6 后完成整条回接链路，M7 发布验收纳入替换编译器自举。这样不会到末期才发现接口无法接回原生后端。

## 当前进展

M2 第一批提供 position/gcbridge 的共享类型别名，并以独立 consumer module 验证位置编码/解码与对象身份。此试验尚未修改或构建宿主 cmd/compile，不代表完成前端、SSA、native tail 的替换。直接消费者约束及后续 Universe 方案见 [M2 记录](m2.md)。

M2 第二批增加 Type/Pkg/Sym/Object 与真实 LSym 数据结构接入，外部 consumer 验证对象身份及 Object/Aux 接口；完整原生算法尚未接回。见 [第二批记录](m2-universe.md)。

M2 第三批把上游 SSA 特殊类型绑定与 tuple/results 构造转为 Universe 成员，独立消费者可直接使用 `u.TypeMem`、`u.NewTuple`、`u.NewResults`。后续宿主中的 `types.TypeMem` 等访问必须接到对应编译实例，不能为了桥接重新建立全局别名。当前仅验证类型切片与未去全局化的上游切片一致，尚未编译宿主后端。

M3 已接通真实隔离 `cmd/compile` 驱动、Linux/amd64 无 cgo 标准库重建及一次编译器自编译后的代码生成，见[接入记录](m3-upstream-compiler-driver.md)。驱动编译使用隔离 cmd 模块的 vendor 副本，生产编译直接进入本库；完整 bootstrap 和其余最终验收仍未完成。

原生 raw/gzip/预处理 PGO 输入已接通，自编译现在使用上游默认 profile，且产物继续生成与第一阶段相同的测试归档，见[PGO 验证](m3-native-profile-input.md)。这尚未替代 dist bootstrap 的完整多阶段验收。

正式 dist bootstrap 已在 Linux/amd64、CGO_ENABLED=0 和 Go 1.27.1 bootstrap SDK 下通过；各阶段归档及强制重编后的编译器二进制保持一致，详见[自举记录](m3-dist-bootstrap.md)。独立 SSA 阶段接口、第二目标、其余驱动与回归要求仍未全部完成。


M5 已提供源码 Built/EarlyOptimized 图与严格 import artifact；M6 已把相同源码驱动、完整后端及正式 bootstrap 扩展到 Linux/arm64，见 [ARM64 记录](m6-arm64.md)。剩余验收包括完整后端元数据消费、cgo/race 驱动、最低 bootstrap SDK 与规定回归。
