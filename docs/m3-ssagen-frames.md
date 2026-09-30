# M3：原生栈帧分配与局部变量合并

本批迁入 `ssafn.AllocFrame`、`cmpstackvarlt`、`needAlloc`、`StackOffset`，以及它们依赖的完整 MergeLocals 和区间算法。原生 buildssa 在 SSA Compile 后调用 AllocFrame，因此这一步补齐后续源码建图入口的必需依赖。

## 原生行为

AllocFrame 从寄存器分配位置及 SSA 指令引用识别使用中的局部变量，保留 open-coded defer 槽，稳定排序声明，合并允许共享的栈槽，分配负偏移并计算总尺寸、指针区域尺寸和栈对齐。指针类型优先、Needzero 分组、零大小对象隔离、defer 索引顺序、寄存器返回值和未使用声明转移至 FuncDebug.OptDcl 均保留上游代码。

局部变量合并保留候选筛选、地址的间接使用追踪、跨块地址逃逸剔除、保守写入、活跃性不动点、区间构造与贪心分区、hash 筛选和诊断。迁入所需的原生 Liveness 声明、初始化、写屏障不可抢占区域识别、clobber 选择策略及数据流求解。完整 GC 栈图计算/输出和 clobber 指令插入尚待迁入，不能把这些依赖闭包视为完整 liveness.Compute。

StackOffset 保留调用方参数偏移、目标 FixedFrameSize、frame pointer、局部槽内偏移及寄存器结果槽的规则。FuncDebug 保留完整原生结构；本批使用其 OptDcl 字段，尚未迁入完整 BuildFuncDebug 算法。

## 实例接口

公开 `liveness.NewContext(ssaContext)`，将其设置为 `ssagen.Context.Liveness`。调用方为真实 IR 函数准备 `ssa.FuncDebug`，完成 SSA Compile 后调用 `ssagen.Context.AllocFrame(f)`，获取原生的总尺寸、指针区域尺寸和对齐要求。

入口要求原生前端、配置、活跃性服务和 SSA 图来自同一编译实例。声明、分配位置和指令辅助数据中的变量也在改写前检查归属，拒绝外来对象时不改变布局。原生字段及变量对象继续使用共享 IR，不引入另一套 frame/variable 表示。

`liveness.Context.ClobberDeadHash` 显式提供原生 GOCLOBBERDEADHASH 输入。生成器检查该环境访问的常量键及方法签名；库不读取进程环境。全部诊断走实例 TraceWriter 和已有错误通道。

## 切片分配绑定

原生区间分析通过 `make([]IntervalsBuilder, count)` 创建状态数组。新增 `LiteralRule.MakeSlice` 将原始 builtin make 的结果交给签名严格为 `func([]T) []T` 的实例绑定器，保留原始分配及参数求值顺序。绑定器给已分配容量中的元素附加上下文后返回原切片，长度、容量、零值和底层数组身份保持不变。

规则核对真实 builtin make、非泛型命名结构体元素及声明来源。指针元素、命名切片、普通字面量、同名函数、重复规则、不兼容模式和放宽的绑定器参数类型均有拒绝测试。8 个并发实例验证每个容量内元素的归属、两种 make 形式、空切片、求值次数及分配失败先于绑定。

## 验证

- 独立栈帧测试在六个架构上验证相邻生命期共享、重叠生命期不共享、合并禁用、指针区域、defer 顺序、未使用声明及 StackOffset。8 个并发实例验证隔离，负例覆盖外来声明、寄存器位置和指令辅助对象。
- 区间算法用 1,000 组确定性随机输入对照独立的稠密布尔集合，检查交集、双向并集及结果区间的不重叠性；另测非法区间和倒序扫描约束。
- 上游差分用真实 SSA Compile 及原生 AllocFrame，比较最终图、每个局部变量的类型/偏移/标志、未使用声明及帧尺寸。每组包含相邻/重叠生命期、非指针局部数组、分支、循环、混合布局、间接地址和地址逃逸八种图。
- 独立 consumer 从公共包配置原生前端和活跃性服务，完成 SSA 编译后分配栈帧。

Go 1.27.1 / Go 1.27.0 各 21 种配置，共 42 份差分快照、336 个函数场景全部一致。最终 `syncgo check` 验证 406 个生成文件一致。

全量 `go test -race -count=1 ./...` 已通过，包含独立 consumer；补充跨实例拒绝用例后，ssagen 与 liveness 的竞态测试再次通过。

IR 经 buildssa 建图、intrinsics、完整 GC 栈图和调试位置、指令输出、统一驱动及替换/自举仍待完成。
