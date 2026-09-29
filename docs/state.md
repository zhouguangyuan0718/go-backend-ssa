# 状态、并发与结果生命周期

## 状态的层级

| 层级 | 内容 | 共享规则 |
| --- | --- | --- |
| 进程只读数据 | opcode 描述、无对象指针的固定数学/语法表 | 经审计的不可变数据可共享 |
| 一次 Build | 配置、目标、Universe、包/符号表、导入、诊断、runtime 描述、工作队列 | 不跨调用共享可变对象 |
| 一次阶段执行 | reader/writer、内联缓存、逃逸分析、walk 当前函数 | 显式持有，本次调用内按阶段使用 |
| 一个函数 | SSA 图及持久存储、函数元数据 | 返回结果继续持有 |
| 临时计算 | pass 工作数组、分析缓存 | 生命周期结束可复用，不得包含对外存活对象 |

第一版单次 Build 内部串行处理可变前端阶段，多个 Build 可并行。以后再单独引入函数级并发；跨调用并发不依赖全局大锁。

## 状态迁移种子清单

这是已核对的代表项，不是完整审计。M1 必须建立含全部声明、初始化和读写者的机器清单。

| 上游位置 / 状态 | 新归属 | 注意事项 |
| --- | --- | --- |
| base.Flag、Debug、Ctxt、Pos、诊断计数 | compile 配置 / diagnostic / symbol context | 不复制命令行进程语义 |
| types.Types、预声明类型、ShapePkg | types.Universe | 根对象及其递归可达缓存都独立 |
| types.PtrSize、RegSize、MaxWidth、SliceSize 等 | Universe 的 TargetLayout | 不从宿主 GOARCH 推断目标 |
| types.pkgMap、nopkg、internedStrings | Universe / symbol context | nil receiver fallback 也必须去全局化 |
| types.defercalc、deferredTypeStack、NewPtrCacheEnabled | 类型计算上下文 | 尊重布局延迟与缓存写入边界 |
| ir.CurFunc、BlankNode、Syms | 阶段/函数上下文、Universe、runtime 描述 | CurFunc 不放共享的进程字段 |
| typecheck.Target | 本次 Package | 与导入包对象区分 |
| noder.localPkgReader、objReader、延迟函数体队列 | noder.Context | 闭包、字典和回调不能捕获其他会话 |
| inline.InlineCall、SSADumpInline、pgoir.LookupFunc | 组装时注入的 callbacks | 避免包级变量赋值 |
| inline / inlheur 的分析与打分缓存 | inline.Context | 未启用功能也要审计初始化可达路径 |
| walk/reflectdata/rttype 的 runtime 类型缓存 | 本次 runtime/type context | 不以“缓存”为由保留全局写入 |
| ssagen.Arch、ssaConfig、ssaCaches、intrinsics | ssagen.Context 与函数存储 | intrinsic 表可能含会话类型指针 |
| ssa passes 的 debug/disabled/dump 等可变字段 | 本次优化配置 | 固定函数表与可变选项分开；禁止浅拷贝共享 map |
| Debug/HTML/dump 全局 writer 与列表 | 可选诊断接收器 | 默认没有文件和 stdout 副作用 |

## Context 传播方式

优先复用上游已有 reader、state、builder 的 receiver，添加明确依赖。剩余有状态包级函数按规则转为包内 Context 方法，或增加窄上下文参数。无状态工具函数保持原样。

示意：

```go
// 上游
func makePointer(t *types.Type) *types.Type {
    return types.NewPtr(t)
}

// 移植结果；此处仅演示转换方向
func (c *Context) makePointer(t *types.Type) *types.Type {
    return c.Types.NewPtr(t)
}
```

不能只替换函数体中的全局读取。声明、调用点、函数值、接口方法、闭包捕获、递归调用均需检查。跨包 Context 构造由 frontend 完成，底层接口定义在消费方或更低层。

## 类型环境

Type 保留上游布局和缓存算法。M2 已为 Type/Pkg 添加私有 Universe owner，并将 SSA 类型身份、结果缓存及包级初始化迁入每实例状态；通过显式入口检查跨 Universe 输入，完整布局/构造路径仍待移植，详见 [第二批记录](m2-universe.md)。无论形式如何，Size/Alignment/NewPtr/PtrTo 等路径都不得回到包级目标状态。

同一个源码类型在两个目标或两次调用中是不同可变对象。不同 Universe 的 Type 混用需在构造/验证边界报错；类型等价应使用语义比较，不能跨调用比较指针。

依赖导入数据可以是不可变字节，由本次 Universe 解码。第一版不缓存跨调用的解码图。共享调用者提供的 resolver 时，resolver 必须自身支持并发并保证相同 key 的内容一致。

## 返回结果

上游 ssa.Cache 内含实际 Value/Block 数组；Reset 会清空它们。单纯保存 *ssa.Func 不能保证有效。

第一版每个返回函数持有独立持久存储，禁止在结果存活期间 Reset；分析 scratch 可以另行回收。结果保留解析 Aux、类型、符号、位置所需的全部对象，移除已结束编译的 writer、importer 等无关引用。

Build 成功后，库不在后台修改结果。公共读取方法如触发 lazy cache，必须在返回前完成预计算或实现本结果内同步，才能声称并发只读安全。调用者对同一结果的主动变更不自动线程安全，需要自行串行；后续公开优化 API 也必须遵守所有权。

第一版使用 Go GC 管理结果，不设计容易误用的 Close/Release。以后若引入内存池，必须证明不影响已经返回的图。

## 错误与取消

- 普通语法/类型/能力错误返回结构化诊断，不退出进程，不修改环境变量和当前目录。
- 若保持上游深层中止模式，可使用私有、带调用标识的 abort 类型；只在对应 Build 边界恢复该类型。
- 不吞掉未知 panic；内部错误不得伪装成普通用户代码错误。
- context.Context 显式传入，在文件、声明/函数队列、pass 和长循环的受控边界检查；不承诺即时抢占。
- 错误或取消后丢弃本次未完成环境，返回 nil 编译结果及携带诊断的 error；下次调用不需要 Reset。

## 全局与环境审计

审计生产依赖闭包中的包级变量、init 写入、lazy cache、函数变量、sync.Once/Pool、嵌套可变对象、环境读取、文件副作用和 go:linkname/unsafe 路径。只读白名单按具体声明记录理由，新增项默认失败。

静态审计并不等于完备证明：配合并发 race、相同包路径不同源码、32/64 位布局、交错调用、失败恢复和旧结果保活试验。宿主 runtime/标准库正常内部状态不属于“编译会话状态”；但不能利用它们实现隐藏的会话单例。

当前已实现位置和诊断的实例隔离。XPos 保持原始布局，不含 owner；必须连同生成它的 PosTable 使用，不能从 index 推断归属。当前受控工厂检查 owner，完整类型图的验证仍待后续补齐。
