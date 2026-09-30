# M3 SSA 派生切片分配器

完整迁入上游 `allocators.go`，包括 Block、int64、int、int32、int8、bool、ID、uint 和 knownBitsEntry 九类派生切片的分配/归还方法。基础池继续按 Cache 隔离。`knownBitsEntry` 和 `internal/unsafeheader` 原生声明作为依赖迁入；没有替换分配算法或新增运行时代理。

## 对原生转换的分类

生成器此前只允许声明中的被动 `unsafe.Pointer` 类型。本步增加 `SliceReinterpretationRule`，明确列出派生分配/归还方法及其基础分配/归还方法。规则检查的是已通过类型检查的原生结构：

- 方法签名必须成对一致；基础和派生切片元素类型从签名取得。
- 元素必须同为单个指针，或均不含 GC 指针。验证 gc 支持的 32/64 位宿主布局中的整除、非零尺寸和对齐关系。
- 原生切片头必须按顺序为 `Data unsafe.Pointer`、`Len int`、`Cap int`，保持 GC 可见性。
- AST 必须保留原生尺寸检查、向上取整、容量计算、首元素地址和成对归还结构。局部变量、内建函数、方法和 unsafe 操作均按类型对象匹配，不能靠同名函数绕过。
- 只将通过该结构检查的方法内确切 unsafe 操作交给审计器；其他函数、其他表达式和改变后的公式继续拒绝。

这些规则用于分类固定上游算法，不向产物输出另一份算法，也不宣称验证了编译器中所有 unsafe 使用的内存安全。原生方法对不合法输入的行为保持原样，例如派生分配器/归还方法会对空切片取首元素地址并 panic。

## 验证

`TestNativeSliceReinterpretation` 先证明没有显式规则时审计拒绝转换，再验证原生与迁入方法的容量、清理、GC 后内容及归还。测试在 race 和 `checkptr=2` 下运行，并拒绝偏移首元素、扩大长度/容量、错误取整、缺少尺寸检查、混合 GC 布局、错误头结构、额外 unsafe 和被遮蔽的内建函数等变体。

`TestPinnedSSAAllocators` 对 Go 1.27.1、Go 1.27.0 的九类原生方法运行相同快照，覆盖不同容量桶、复用清理、GC 后内容和空切片 panic 文本。`TestNativeDerivedAllocatorIsolation` 同时使用八个独立 Cache，并保留另一个缓存中的指针/数值切片；全仓库 race 与专门的 `checkptr=2` 检查验证迁入代码。

本步仅补齐后续图分析依赖的真实分配器。SSA 配置工厂、优化流水线、ssagen 及编译器回接仍未完成。
