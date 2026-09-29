# M3 第六批：原生 Unified IR writer

已移植 Go 1.27.1 的完整 `noder/writer.go` 和 `writePkgStub`，保留原生声明、类型、表达式、函数体、泛型字典及 relocation 编码算法。生成文件仍放在 `internal/noder/`，源码 pin 与全项目一致。

公共入口为 `noder.NewContext(checker, compilerTypes, link)` 和 `Context.WritePackage(pkg, info, files, rangeClosures)`。可运行示例见 [noder/example_test.go](../noder/example_test.go)，独立 module 用例见 [consumer_test.go](../testdata/consumer/consumer_test.go)。返回值是上游原生 package stub 字节串，包含函数体与位置数据；它还不是 compiler IR、SSA、完成链接的 export data 或可直接交给 `go tool compile` 的 `.a` 文件。

## 输入和生命周期

- AST 必须成功通过同一个 `types2.Context` 检查；检查失败不能只根据 `Package.Complete()` 判断可用。
- `Info` 必须开启 `StoreTypesInSyntax`，并初始化 `Defs/Uses/Instances/Selections/Implicits/Scopes/FileVersions`。调用者负责让 Info、AST、Package 对应同一次检查。
- 目标 Link 和 compiler Types 的指针/寄存器宽度必须一致；checker 的 Sizes、语言版本以及 writer 的 GOARCH/Flag 也必须由调用者保持一致。构造函数默认语言为 go1.27、Complete=true、SyncFrames=-1。
- 当前低层入口不执行 rangefunc/midway 重写，也没有集成编译器的 directive 收集。未重写的 range-over-func 明确返回错误。当前公开调用仅覆盖无需 compiler directives 的源码；linkname、embed、cgo、wasm 指令等尚不能从此入口完整传入，不能将 parser 忽略这些注释当成支持。
- 同一 Context 和它的 checker、Types、Link、AST 由调用者串行操作；独立对象组可以并发调用。外部共享可变 AST 或 Link 不在并发保证范围内。
- 普通 writer 诊断记录在 `Context.Diagnostics`，失败不返回半成品。内部不变量失败仍 panic；没有进程退出。下一次有效写入重置当前诊断，已经取出的诊断快照保留。
- 上游命令驱动的 `freePackage` 会清零 checker graph。库在此处只检查包身份并解除临时输入引用，保留调用者持有的 Package、Type 和 Object，后续写入不使旧图失效。

## 全局状态改造

writer 的 any/comparable/rune 预声明类型缓存、运算符表、Link、原生 CmdFlags/DebugFlags、GOARCH 和路径输入均绑定 noder.Context。types2 的 Universe/Unsafe/Typ 直接指向已有 checker 的实例字段；类型谓词及函数值绑定该 checker 的方法，不复制类型模型。

初始化改为实例方法，在 checker/Types/Link 装配后执行原生初始化表达式，避免缓存获得其他实例的预声明对象。`pkgWriter/writer/writerDict/declCollector` 保存当前 Context，保留原生调用关系和允许 nil writerDict 的语义。

上游 `obj.Pkg().Scope()` 在 Pkg 为 nil 时读取全局 Universe。库提供显式 `checker.PackageScope(pkg)`：nil 返回该 checker 的 Universe，非 nil 检查所属实例。writer 的 `isGlobal` 仅将这处隐式全局读取改为显式调用。公共 `Package.Scope()` 需要非 nil Package；库没有引入默认全局 checker。

移植规则在 [rules_noder.go](../internal/port/rules_noder.go)，输入装配和诊断适配在 [adapter_context.go](../internal/noder/adapter_context.go)。移植器新增类型检查后的嵌套字段/方法绑定、依赖已迁移包的原始类型身份复用、实例初始化方法及同包驱动函数绑定；仍由 Go 规则维护，没有新增配置文件。函数绑定校验来源和签名，原生 helper 可以保留用于输入类型检查但不进入最终生成代码。

## 验证与边界

- 原生 parser/checker/writer：普通控制流、泛型、闭包、接口、type switch；字节可重复、旧图保留。
- 12 个独立实例并发写入相同包路径、随后写入不同源码；race 检查、诊断失败后恢复、错误 checker 身份及未重写 rangefunc 拒绝。
- 每套固定上游源码 48 组完整字节比较：12 个样例 × 386/amd64 × 无同步标记/同步标记。覆盖泛型别名、导入泛型、unsafe、初始化、slice/integer range、map/channel/select、go/defer/recover 及 line pragma。
- 参考端使用原生完整 writer、pkgbits、checker 和原始全局状态，保留原生 freePackage；仅测试驱动注入已检查输入和诊断，重定位 import，并适配已经验证的位置零值。比较完整 stub，包括其 fingerprint。
- go1.27.0 和 go1.27.1 使用同一套移植规则。该字节对照验证编码兼容性，不能替代后续程序执行、runtime 或后端语义验收。

下一步接入原生前端准备步骤和 Unified IR reader，构造真实 compiler IR/typecheck；随后推进 walk/escape/ssagen。替换官方编译器对应实现、接回原生后端和工具链自举仍是未完成的最终目标。
