# M3 局部零值的实例绑定

原生 walk/order.go 的 orderState.addrTemp 用 `var s staticinit.Schedule` 完成常量静态赋值。该路径不需要 Plans、Temps；不能把它替换为分配这两个 map 的 NewSchedule。

本批扩展既有 LiteralRule：按 go/types 对象匹配局部显式类型的零值声明，生成 `var s T = context.Bind(T{})`。规则仍由 Go 源码表达，不增加配置文件。显式复合字面量保留原来的处理。命名局部变量、多个变量、类型别名、循环、闭包和间接调用均保留声明位置与实例归属；指针零值保持 nil，同名局部类型不受影响。包级零值和空白变量被拒绝，包级状态需要专门审阅初始化顺序。

staticinit.Context.BindSchedule 现在同时接受全零值与原有的新建空表值。绑定只增加所属 Context，保留 nil 字段和原有 map 身份；部分初始化、使用中的表和已绑定值仍被拒绝。原生 StaticAssign 算法未修改。零值调度器仍限于原生支持的常量路径，不把它变为通用初始化调度器。

## 验证

- 基线工具拒绝局部零值规则：`_work/m3-zero-schedule-baseline.log`。
- 生成工具回归执行生成代码，验证局部别名、多个变量、闭包、循环、私有 receiver 方法传参、指针 nil 和八个并发实例；保留旧闭包仍访问原实例。
- 实际 Schedule 回归验证常量输出、保留 nil 字段、八实例并发和旧符号不变，以及部分初始化拒绝。
- Go 1.27.0/1.27.1 的 386、amd64、mips、ppc64 对照通过（31.89s，`_work/m3-zero-schedule-source.log`）：整数、布尔、浮点、复数、字符串、nil 和无显式值的原生 StaticAssign 结果、符号字节及重定位一致；同时覆盖 FIPS 路径。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test -timeout 5m ./internal/port -run '^TestPinnedZeroSchedule$' -v
```

工具版本 syncgo-m3-v22，生成文件数量保持 273，本批未修改任何已有生成算法。generate/check 验证全部 273 个文件；完整 `go test -race -timeout 20m ./...` 通过（internal/port 60.303s，`_work/m3-zero-schedule-unit.log`）。完整 Walk 仍待下一批迁入，本批是其局部 Schedule 绑定前置依赖。

后续完整 walk 已迁入，见 [m3-walk.md](m3-walk.md)。本文件保留该前置阶段的验证范围。
