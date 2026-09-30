# M3：原生 SSA 调度和寄存器分配

本批基于官方 `go1.27.1`，使用 `syncgo-m3-v43` 完整迁入 lowering 后的优化、调度、寄存器及栈槽分配。结果仍是原生 `Func` 图、`RegAlloc` 位置表和 `LocalSlot`；完整 Compile 入口、物理栈帧布局、指令发射、源码到 SSA 及替换/自举验收仍待后续接通。

## 原生阶段与状态

完整迁入 `addressingmodes.go`、`pair.go`、`licm.go`、`tighten.go`、`tuple.go`、`critical.go`、`layout.go`、`schedule.go`、`flagalloc.go`、`regalloc.go`、`stackalloc.go`、`looprotate.go`、`trim.go` 和 `nilcheck.go`，包括原先尚未迁入的后期 nil 检查。测试在原生前置转换之后，按上游顺序执行从 lowering 到 trim 的阶段，并保留原生 verifier。

地址合并表、成对访存表、空寄存器数组、临时值哨兵和加载故障策略属于本次 SSA Context。原生 `combineFirst` 的 init 循环随实例初始化执行。寄存器分配及边界搬移的方法显式接收当前 Context；缓存中的栈分配状态保留原生 `f` 生命周期，不增加旧 Context 指针。

`FramePointerEnabled` 是驱动传入的原生平台策略，应与 GOAMD64、GOARM 等目标输入一起，在构造配置和执行阶段前设置。测试覆盖开启与关闭帧指针。目标 OS 从关联 Link 捕获，执行原生 `faultOnLoad` 初始化表达式；后续通过 NewConfig 修改为不一致的目标会被拒绝。单独查询 opcode 元数据不需要创建类型 Universe。

## 原生架构身份

迁入完整 `sys/arch.go`，由实例 `sys.Context` 初始化 14 个原生架构描述符及 Archs 表。公开 `gcbridge.NewArchitectures()` 返回该集合，调用方用其中的描述符构造 LinkArch。

SSA Context 从描述符所属集合取得 RISC-V 和 Wasm 的规范指针，保留上游的指针相等判断。RISC-V 的压缩寄存器偏好和 Wasm 的栈处理继续使用原始算法。描述符副本不获得规范指针身份；不同集合的描述符互不共享，原生 InFamily 查询保持不变。

## 迁移审计

寄存器分配器原有的 `unsafe.Sizeof(regMask{})` 保持原样。新增显式 `FixedSizeofRule`，仅接受指定文件和函数中，对指定类型空复合字面量的一次 Sizeof 观察。类型只能由固定宽度标量、数组和结构组成，并逐一验证 14 种目标布局的大小一致；本地 int、指针、slice、随宿主变化的填充、非空字面量、额外或重复观察均被拒绝。生成记录包含该审查项。

外部配置变量的签名提取现在也识别比较表达式产生的未定类型 bool，以及这些结果之间的逻辑组合，用于原生 FramePointerEnabled。它只提取类型，不运行或输出宿主初始化表达式；无法确定类型的逻辑操作数仍被拒绝，适配器字段必须与原生签名一致。

## 验证范围

两个源码版本 × 18 种配置的 36 组差分全部通过；全量 `go test -race -count=1 ./...`（含独立消费者）通过；`syncgo check` 验证 398 个生成文件一致。

`TestPinnedSSAAllocation` 对官方 `go1.27.1` 和 `go1.27.0` 分别生成、构建，两个源码版本均使用 Go 1.27.1 构建工具链。每个版本覆盖 14 种架构，以及 amd64 关闭帧指针、清理死寄存器、关闭优化和 ppc64 AIX 四种附加配置。

对照内容包括所有原生架构描述符的字段、阶段图、寄存器/栈槽分配、参数寄存器和新增局部变量。样例覆盖跨调用存活的大量整数与浮点值、ABI0/ABIInternal、关键边及 Phi、循环内存与归纳值、循环不变量、相邻访存、索引地址和后期 nil 检查。AIX 参考样例在设置目标后，重新执行从该固定版本源码提取的 faultOnLoad 初始化表达式，避免误用测试进程启动时的宿主 OS 状态。

独立检查包括：

- amd64 高压力图的分配位置解释：通过实际寄存器及抽象栈槽读写执行，验证强制溢出/重载后的结果和别名内存写入。200 组输入 × 帧指针及死寄存器清理四种组合，并检查保留的帧指针寄存器未被分配。
- 规范及复制的 RISC-V 描述符选择差异、独立架构集合的身份和状态。
- 八个并发实例在 amd64、arm64、riscv64、wasm 上分配，保留旧 386 图及分配结果，检查临时哨兵、表、栈槽归属和缓存释放。
- 两条新增迁移审计规则的接受、拒绝和类型漂移测试，以及仅使用公开接口的架构集合消费者。

这里的独立执行使用测试解释器，栈位置仍是 LocalSlot；本批未执行产出的机器码。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test ./internal/port -run '^TestPinnedSSAAllocation$' -count=1 -v
GO_SSA_SOURCE_REPO= GOTOOLCHAIN=go1.27.1 go test -race -count=1 ./...
GOTOOLCHAIN=go1.27.1 go run ./cmd/syncgo check -repo /path/to/go
```

下一阶段接通完整原生 Compile 的阶段表、选项、初始化顺序与输出服务，再继续 ssagen、统一驱动和替换/自举验收。
