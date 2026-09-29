# M3 原生包初始化任务

本批迁入完整 `pkginit/init.go` 和 `pkginit/initAsanGlobals.go`。对外提供 `pkginit.NewContext(reader).MakeTask()`，沿用上游对包变量静态初始化、用户 init、导入包依赖及 ASan 全局变量登记的处理。

执行顺序与上游保持一致：整包内联、方法包装、loopvar、MakeTask，随后才是尚待接入的 ABI wrappers、deadlocals、escape、slice、walk 和 SSA。本批仍未实现完整 SSA 入口或上游编译器替换。

## 实例状态和迁移

`pkginit.Context` 引用现有 noder 实例，类型检查、目标包、符号、位置、对象写入和静态初始化都使用同一编译图。原生 `InstrumentGlobalsMap` 和 `InstrumentGlobalsSlice` 移到实例字段，初始化表达式由迁移工具原样搬入构造函数。保留切片提供的确定顺序，不通过遍历 map 生成登记数组。

MakeTask 入口检查 init 函数、外部变量、导入包及其初始化记录的图归属。独立图可并行编译，旧图和已生成数据继续有效。与原生驱动一样，每包调用一次；接口不承诺重复调用幂等，也不通过清空全局变量复用编译器。

原生 MakeTask 使用 `staticinit.Schedule{Plans: ..., Temps: ...}`。迁移工具增加按类型身份匹配的 `LiteralRule`，将该值交给 `StaticInit.BindSchedule`，保留原生映射创建、字段值及求值顺序。BindSchedule 只接受未绑定且尚未使用的调度值，为其设置已有的私有 compilation 字段，不复制或替换原生映射。

此规则只处理明确选定的结构体值，并检查来源文件及 `func(T) T` 方法签名；未命中、指针/省略类型的字面量、泛型或类型漂移会报错。别名导入和局部同名类型按类型身份区分。上下文依赖传播覆盖间接调用；嵌套字面量从内向外处理。相关测试验证初始化表达式只执行一次、嵌套结果、独立实例及错误规则拒绝。

工具版本升为 `syncgo-m3-v14`。原有生成文件随版本头更新；没有增加配置文件或更换原生 IR/初始化算法。

## 对外使用

```go
reader.Interleaved.DevirtualizeAndInlinePackage(target, profile)
reader.MakeWrappers(target)
capture := loopvar.NewContext(reader.Typecheck, reader.Inline.Log)
var transformed []loopvar.VarAndLoop
for _, fn := range target.Funcs {
    transformed = append(transformed, capture.ForCapture(fn)...)
}
initialization := pkginit.NewContext(reader)
initialization.MakeTask()
// 后续接入 ABI wrappers、deadlocals、escape、slice 后，
// 再按上游顺序调用 capture.LogTransformations(transformed)。
```

`GetRedzoneSizeForGlobal` 保持为无状态原生函数。ASan 的登记表保留在 initialization 实例，供后续对象布局/后端使用。当前验证的是原生登记 IR、数据和布局，不是执行经过 ASan 插桩的最终程序。

## 验证范围

- 从真实源码生成原生 IR，再运行此前各阶段及 MakeTask；检查静态常量数据、动态初始化和用户 init 的调用顺序、空 init 消除。
- `.inittask` 的状态、函数个数、数据大小、函数指针重定位和 `R_INITORDER`；普通空包省略记录，main/runtime 保留空记录。
- ASan 的变量筛选、linkname/cgo/internal-fuzz 排除、登记数组与位置结构、合成 init 命名及运行时登记调用；无可登记变量的路径也做差分。
- 八实例并发、旧数据保留、外来图拒绝；公开示例和独立 module 消费者直接调用公共 API。
- 原生差分比较完整初始化 IR、外部变量类型/位置、ASan 登记顺序、对象数据/属性位/重定位及 redzone 边界。参考构建仅在临时 overlay 中添加读取共享 Unified IR stub 的测试入口，不修改上游工作树。

差分配置为 386、amd64、mips、ppc64、amd64-asan、386-asan、amd64-empty、amd64-main、amd64-runtime、amd64-deps、amd64-deps-empty、amd64-fuzz、amd64-asan-empty。这里的跨目标验证是 IR 和数据布局比较，不声称在相应目标执行了程序。

`syncgo generate` / `check` 已通过，共 256 个迁入文件。Go 1.27.1 和 Go 1.27.0 的 13 种包初始化差分均通过。

首轮普通 race 测试通过（port 包 49.684 秒）；完整集成测试首轮在 600 秒触发 Go 默认时限，超时前两版的包初始化差分均已通过，未报告差分失败。日志保留为 `_work/m3-pkginit-source-timeout.log`。最终代码的 `go test -race ./...` 已通过（port 包 50.704 秒），日志见 `_work/m3-pkginit-unit-final.log`。完整回归加上 `-timeout 20m` 后通过（测试 663.81 秒，包总计 665.307 秒）；同一套迁移规则适用于 Go 1.27.0 和 1.27.1，两个 Go 输出文件随上游版本变化，上游工作树保持原样。日志见 `_work/m3-pkginit-source.log`。

下一步按原生驱动顺序接入 ABI wrappers，继续清理函数和包级共享状态，随后推进 deadlocals、escape、slice、walk 和 SSA。
