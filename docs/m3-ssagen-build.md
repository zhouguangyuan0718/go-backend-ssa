# M3：完整原生 IR 到 SSA

本批新增公共 `ssagen.Context.BuildSSA(fn, worker, isPgoHot)`。输入是已经完成前端分析和 walk 的原生 IR 函数；输出经过原生 SSA Compile、寄存器分配、AllocFrame、open-defer 元数据及参数寄存器 spill 记录。算法从固定 Go 源码迁入。

## 实现

- 完整迁入 `ssagen/ssa.go` 的 IR 构图部分，以及 `phi.go` 的简单/大图 Phi 算法；调用、接口、闭包、内存和写屏障、defer、整数/浮点转换等走原生实现。
- 完整迁入 `intrinsics.go` 和 amd64、arm64、wasm SIMD 注册文件。架构对象采用实例内的 canonical 指针，配置和 builder 闭包归属本次编译；`InitTables` 同时绑定 IR 的 intrinsic 查询回调。
- 原生 SSA 标记名称保持无包符号身份，不创建虚构包或跨实例共享标记。转换、mask/vector 与 allocator size-class 表均通过原生声明生成并在 Context 初始化。
- 原生 HTML 输出的目录创建由显式宿主回调提供。移植器收集独立类型诊断，缺失声明仍使生成失败。

## 调用约束

先创建共享的 types/IR/typecheck 图及 SSA、liveness 服务，配置目标特征，再调用 `InitConfig` 和 `InitTables`。后者应早于 walk，以安装原生 intrinsic 查询。

包列表中的闭包不能直接当作根函数编译：与原生 `gc.enqueueFunc` 相同，先准备外层函数，再从 walk 产生的 `Func.Closures` 收集仍需编译的闭包。完成全部准备后逐函数调用 BuildSSA。已内联闭包可能仍留在包列表中。

函数入口在任何原生修改前检查函数、声明、隐藏 IR 节点和类型归属，以及 worker、配置和 liveness 服务。一个 Context 内串行使用；独立 Context 可并发。返回 SSA 使用原生 worker cache，必须在相同 worker 下次构建前消费完毕；源码 IR/type/symbol 身份独立保留。跨多次 Build 保留 SSA 的统一驱动尚需处理 cache 生命周期。若启用 HTML 转储，原生 BuildSSA 保留 `HTMLWriter` 供后续指令阶段使用；单独消费此入口时应关闭返回图的 writer。

## 验证

- `go test -race ./... -count=1` 通过，含生成器、图归属保护、无包 SSA 标记和独立 public consumer。消费方还在最终调整为“先全部 walk、再逐函数 BuildSSA”后单独通过竞态测试。
- `syncgo check` 验证 412 个生成文件与固定 go1.27.1 源码及移植规则一致。
- Go 1.27.1 / Go 1.27.0 各 19 组、合计 38 组原生差分全部通过。矩阵覆盖 14 个架构，以及 ARM 软浮点、amd64 race、关闭寄存器 ABI、关闭优化、大图 Phi。软浮点采用原生 ARM 后端支持的配置。
- 两侧消费含 50 个顶层函数声明的同一源码经过类型检查和 Unified IR writer 生成的输入，比较完整 SSA 文本、栈帧、RegArgs、open-defer 数据、优化后的声明及相关静态数据；同时启用原生 SSA verifier。大图输入含 260 个连续条件分支，覆盖原生 500 块阈值以上的 Phi 算法。
- intrinsic 对照包括 SIMD 关闭/开启时所有架构的完整注册键、原生 bodyless intrinsic 函数体与其编译结果，以及 amd64/arm64/wasm SIMD builder 的 opcode、类型与参数身份。独立实例并行初始化和构建的竞态测试通过。

## 尚未完成

本批输出是带寄存器和栈帧信息的 SSA，尚未通过 genssa 生成机器指令。统一源码/包驱动、正式 Built/EarlyOptimized 截点、目标代码输出、上游编译器替换、自举和标准库回归仍未完成。图差分和 verifier 通过不等价于这些源码已经执行过。
