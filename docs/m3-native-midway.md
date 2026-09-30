# M3：原生 midway SIMD 源码重写

本阶段迁入 `cmd/compile/internal/midway` 的五个完整生产源文件，保留上游 SIMD 依赖分析固定点、声明筛选、语法深复制、目标宽度专门化和分派函数生成。它是接通原生 `noder.checkFiles` 的前置步骤；尚未接入统一源码入口。

## 实例输入与归属

`midway.Context` 绑定一个 types2 checker，接收目标架构以及实验和调试配置的值快照。`Analyzer`、`Rewriter` 和 `DeepCopier` 持有所属实例，生成的变量和函数通过该 checker 创建。构造入口拒绝外来 package、外来 analyzer，以及与 analyzer 不一致的 package/Info。输入的 Info 和 AST 必须来自同一次检查，由调用方保证；本阶段没有引入 AST 所有权注册表。

调试警告交给实例回调，内部致命错误以 panic 报告，不终止宿主进程。各实例可以独立并发，同一实例的变换由调用方串行。

## 保留的上游行为

- 只有 SIMD 实验开启且目标为 amd64、arm64 或 wasm 时，`RewriteWrapper` 才尝试分析和重写。
- AMD64 保留 0/128/256/512 的宽度集合；ARM64 和 Wasm 保留 0/128。0 表示上游软件实现专门化。
- 完整保留递归类型、方法依赖、函数体依赖传播、类型别名、泛型声明和变量重写。
- 保留原生名称规则、bridge 导入、入口分派、依赖签名原函数的 panic body，以及重写前后的源位置检查。
- 深复制后的 AST 仍须经过原生第二次类型检查，重建完整 Info；不能将复制期间的部分映射当作最终类型检查结果。

## 验证

本地 race 回归覆盖 12 个并发实例、不同目标宽度、保留旧图、复制变量定义与使用的关联、函数对象归属、调试回调、禁用分支及跨实例拒绝路径。

独立上游快照覆盖 4 个目标 × 2 个实验开关 × 7 组源码，包括别名、递归类型、方法、泛型、全局变量、SIMD Load/Broadcast、闭包、defer、循环、switch、select 和多返回值。比较完整打印 AST、每个节点位置、定义对象及类型和第二次检查后的包作用域；每个重写结果再次通过 types2 检查。SIMD 和 bridge 依赖由测试源码定义，验证的是编译器重写行为，不是硬件 SIMD 执行。

Go 1.27.1 与 Go 1.27.0 的独立原生快照对照均通过，移植侧启用 race。全量 `go test -race -timeout 30m ./...` 通过；`syncgo check` 确认全部 460 个生成文件与规则一致。

下一步接通 noder 的原生 AST 诊断与 `checkFiles`，并将 midway 后的全新 checker、依赖环境和 writer 预声明对象同步绑定，随后接入完整 unified 驱动。
