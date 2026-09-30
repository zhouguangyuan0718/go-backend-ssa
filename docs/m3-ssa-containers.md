# M3 SSA 稀疏容器和缓存池

迁入完整原生 `sparsemap.go`、`sparsemappos.go`、`sparseset.go`、`biasedsparsemap.go` 和 `xposmap.go`。泛型 `genericSparseMap[K,V]` 及 `sparseMap` 别名保留上游声明和方法；没有重写为另一种集合。`Func` 的六个稀疏容器分配/归还方法也保持原生代码。

`allocators.go` 本步迁入五组基础分配/归还方法：Value slice、limit slice、sparseSet、sparseMap 和 sparseMapPos。五个原生 `[27]sync.Pool` 由包变量迁入 `Cache`。原生容量分桶、清零、slice header 复用和算法保持不变；仍可使用零值 `Cache`，无需新增 context 工厂或额外初始化。缓存的 `Reset` 保留原生行为，池跨该缓存的多次使用存在；不同缓存不共享池。

## 生成规则

现有 `ReceiverStateRule` 扩展为支持同一已选中包内、不同文件的接收者声明。跨文件只接受零初始化状态；迁入字段的类型由 `go/types` 对象生成，并为目标文件选择无冲突的导入别名。原生字段和 API 保持原样，生成后再次检查新增字段与原始状态类型完全一致。

零初始化状态允许原生 `var cache Cache`。需要非零初始化的既有规则仍拒绝漏掉初始化的零值声明。规则也可显式选择方法；变量选择由相同 receiver rule 驱动，不允许未选中的方法混入产物。

先前失败回归为：状态声明位于 `state.go`、`Cache` 位于 `cache.go` 时，生成器报 `receiver state target is not a selected struct: Cache`。回归还覆盖导入别名冲突、零值/`new`/键值字面量构造、并发实例及拒绝跨文件非零初始化和自由函数访问状态。

## 验证

`TestPinnedSSAContainers` 在 Go 1.27.1 和 Go 1.27.0 上比较同一个原生快照：泛型 map 和原生别名、稀疏容器插入/覆盖/删除/清空、偏置和越界、列位置归并、位置映射遍历、分配容量分桶、归还后清理及异常边界。容器使用宿主索引宽度，单次快照即可；没有将重复运行称作额外目标架构覆盖。

`TestPinnedSSAGraph` 同时重跑两版四布局建图差分。字段快照仍比较所有原生字段，只跳过五个明确迁入的池字段；新增池由生成后的类型检查及实例测试验证。

边界测试使用禁止内联的运行时下标输入。官方 Go 1.27.1 darwin/arm64 的独立泛型稀疏 map 小程序表明，固定负数下标在普通/race 构建下的 panic 文本分别含 `[1]`/`[0]`，运行时下标均为 `[-1]`；此测试调整不改容器实现，也不忽略 panic 文本差异。

race 测试覆盖八个独立缓存并发分配/归还、保留存活切片的内容不变、池内指针/数值清理和泛型稀疏 map 与普通 map 的混合操作对照。

## 后续

原生分配器中的派生切片重解释方法尚未迁入，当前审计器继续拒绝未分类的 `unsafe` 操作。下一步需要按原生方法的确切数据/布局约束审计它们，再接入支配树等图分析；本步没有放宽审计或替换这些方法的算法。

完整 SSA 配置工厂、rewrite/优化流水线、ssagen 和编译器回接仍未完成。
