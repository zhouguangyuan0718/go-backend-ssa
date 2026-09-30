# M3 SSA 支配树与后序遍历

迁入原生 `postorder / postorderWithNumbering`、生产使用的 Lengauer–Tarjan 立即支配者算法及其 DFS/路径压缩辅助方法，并迁入完整 `sparsetree.go`。`Func.Postorder / Idom / Sdom` 保留上游缓存方式，CFG 改动继续通过原生 `invalidateCFG` 清空分析缓存。

算法使用已经迁入的 Cache 派生切片分配器；没有替换为新图结构、另一个支配算法或手写兼容层。`dom.go` 中未被生产分派使用的 `dominatorsSimple` 暂未迁入，因为其错误路径依赖完整原生诊断输出；生产 `dominators` 的分派原样保留。

## 验证

`TestNativeDominatorsAgainstReachability` 使用独立算法交叉验证：移除顶点后，从入口重新进行广度遍历，判断哪些可达顶点因此失去可达性，进而求支配关系和立即支配者。该模型不使用上游的 DFS 编号或 LT 递推。

覆盖直链、菱形合流、自环、循环、不可约控制流、平行边、不可达块及不可达前驱；另有 80 组确定性的混合图，并改变入口块编号。对每个可达顶点逐对检查稀疏树支配关系，同时检查不可达块不进入 postorder、其立即支配者为 nil。加边/删边后重新查询，验证缓存失效与重算。

`TestNativeDominatorGraphIsolation` 并发分析八个超过原生块缓存数组边界的长链，同时保留另一张图及其分析结果。

`TestPinnedSSADominators` 在 Go 1.27.1 和 Go 1.27.0 上运行同一快照，覆盖 32 张控制流图及各自的加边、删边状态，逐项比较 postorder、编号、立即支配者、稀疏树结构/区间/祖先查询和缓存复用。对不可达顶点的祖先查询也保留原生输出；独立模型仅对可达顶点主张支配关系正确性。

全仓库 race 测试及生成一致性检查一并执行。上述证据证明已迁入的图分析行为，不等价于源码到 SSA、完整优化流水线或上游编译器替换已经完成。
