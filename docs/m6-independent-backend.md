# M6：独立后端消费源码 SSA

新增独立 module 测试，经本地 module replace 使用公开 `compiler`、`ssa`、`ir`、`types` 和 `gcbridge`，不能借助本项目 internal 可见性。`TestIndependentSourceBackend` 在临时独立 module 中运行 race 检查。

## 元数据消费

同一份源码分别构建 AMD64/ARM64 的 Built/EarlyOptimized。依赖经 artifact 序列化、解码后进入新 Universe；源码包含泛型调用、接口方法、闭包、map、channel/go、defer、panic/recover、初始化及静态数据。每种配置实际返回 19 个函数、82 个全局、41 个运行时类型映射、16 个调用 Aux 和 2 个带 open defer 元数据的函数。

外部消费者逐项读取并验证：

- 函数及调用 ABI 的参数/结果类型与所属 Universe。
- 类型、局部变量及符号 Aux 的原始身份。
- 全局与重定位目标的符号闭包，以及初始化函数和跨包任务依赖。
- runtime 类型描述符、输入 artifact 哈希及原生位置，包括内联依赖文件和自动生成的位置。
- open defer bits/closure 槽与对应函数局部变量的身份。

## 独立执行

测试内含一个只支持整数控制流的小型后端解释器，直接消费公开 SSA 图。它按依赖求值，不假设 Built 的值列表已调度；Phi 使用公开边的前驱索引，并同时提交输入值。未知操作和块立即报错，超过步数上限也报错。

包含负数路径、条件、乘法、位与及循环的函数，在每种配置下执行 24 组输入，共 96 组结果，与由官方 Go 编译的直接参考函数比较。随后在同一 import path 编译另一包，验证旧图及 artifact 保持有效。

这验证了公共接口的实际消费与上述整数子集的执行；解释器没有实现 Go runtime，也不把 map、channel、defer 或 GC 的元数据检查声称为独立后端执行。完整语言执行仍由原生回接及官方差分测试验证。

```sh
go test ./compiler -run '^TestIndependentSourceBackend$' -v -count=1
```

本批仅新增验收代码，没有修改生产编译实现。
