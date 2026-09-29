# M3 第七批：原生 rangefunc 前端重写

已移植完整 `rangefunc/rewrite.go` 及原生 `internal/abi/rangefuncconsts.go`。原生循环状态机、跨循环分支/return、defer、生成闭包及 runtime helper 标记均保留；没有另写一套 lowering。公开入口沿用现有 noder.Context：

```go
// pkg、info、files 来自一次成功的原生 types2.Config.Check。
closures, err := writer.RewriteRangeFuncs(pkg, info, files)
if err != nil { /* 处理错误 */ }
stub, err := writer.WritePackage(pkg, info, files, closures)
```

完整可运行例子见 [noder/example_test.go](../noder/example_test.go)。独立 consumer module 使用相同入口验证外部调用不需要导入 internal 包。

## 状态归属

- 上游的全局 `runtimePkg` 是重写器为 `deferrangefunc/panicrangestate` 构造的符号包，并非完整 runtime 包。其原生初始化表达式现在绑定调用者已有的 types2.Context，每次重写创建独立 helper Package；无进程共享类型图。
- `types2.Universe` 和 NewVar/NewParam/NewFunc/NewSignatureType/NewTuple/NewPackage/CoreType 等原生入口，均使用相同 checker 实例。
- 原生 rewriter 保存 Context 所有权。零位置 nopos 改为零值表达式；CmdFlags/DebugFlags 来自当前 noder.Context；AST 调试输出写入共享 IR.TraceWriter，nil 丢弃，不写进程 stderr。
- `Debug.RangeFuncCheck` 默认值为 1，与上游 flag 初始化一致。显式设为 0 时保留上游关闭迭代器误用检查的行为；调试选项不再通过全局传播。

## 调用契约

RewriteRangeFuncs 直接修改 AST 和 Info。必须在成功类型检查后、写入前调用一次，并将返回的原生 closure map 传给 WritePackage。重写过程中不能并发读取或修改这些输入；不同 checker/AST/Link 实例组可并发。重写不清空已返回的包或类型，后续调用产生的新 helper 包也不改变旧图。

入口检查所属 checker、完整 Info 与非 nil files；它不代替源程序的类型检查或整个 checkFiles 驱动。重复重写会丢失上一遍返回的 closure 分类，因此调用者应保留第一次返回的 map；已重写的 AST 可以反复写入。未重写的函数 range 仍被 WritePackage 拒绝。

## 验证

- 12 个独立实例并发重写并编码，再使用同一 checker 重写新源码；检查 helper Package 归属、旧类型/对象保留、闭包分类和重复写入字节稳定。
- 迭代器误用检查开关、实例 AST trace、非法输入拒绝和原生 writer 的既有失败恢复测试。
- Go 1.27.1 与 1.27.0 各 96 组完整 Unified IR stub 字节对照：12 个源码样例 × 386/amd64 × SyncFrames=-1/0 × RangeFuncCheck=0/1。覆盖嵌套循环、带标签 break/continue、goto、跨循环 return、defer、捕获迭代变量、零/双参数 yield、泛型、函数内闭包及 panic/recover。
- 参考端使用未做 Context 改写的完整上游 rangefunc 实现，仅重定位 import。原生 checker → rangefunc → writer 的整条编码链路比较，包括生成闭包和 runtime helper 信息；此前的 48 组普通 writer 用例每版继续通过。

这些测试证明前端变换和 Unified IR 编码与固定上游一致，尚不代表本库已执行这些程序。Unified IR reader、compiler IR/typecheck、walk/escape/ssagen、实验性 midway 集成及 compiler directives 仍待接入。原生后端回接、自举和运行语义验证仍属于最终验收。
