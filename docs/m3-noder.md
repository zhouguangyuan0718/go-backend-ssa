# M3 第五批：原生 noder 位置映射

已移植 noder 的 `posMap`、`trimFilename` 以及 objabi 的 `AbsFile/ApplyRewrites` 算法。公共 `noder.NewPosMap` 将 syntax.Pos 转为指定原生 Link.PosTable 中的 XPos，保留文件 base 缓存、`//line` 的层级和上游路径重写规则。

```go
link := &gcbridge.Link{Pathname: "/work/src"}
positions := noder.NewPosMap(link, "/work=>example", "/sdk/go")
xpos := positions.XPos(file.Pos())
native := link.PosTable.Pos(xpos)
```

传入的 Link 及其位置表归当前编译实例所有；不要在并发调用间共享可变表，也不要复制正在使用的 PosMap。此前返回的 XPos 仍由原 Link.PosTable 解码，后续 map 不会清空旧表。

## 去掉的宿主耦合

- `base.Ctxt` → 当前实例的原生 `*obj.Link`，没有新的位置表表示或简化 Link。
- `base.Flag` → 当前实例的原生 CmdFlags；本入口只设置 TrimPath。
- `buildcfg.GOROOT` → PathContext 的显式 GOROOT 字符串。工作目录来自 Link.Pathname，不调用 Getwd。
- 路径算法保留上游按构建宿主解释 filepath 的行为，包括编译期常量 runtime.GOOS；没有可变全局、环境读取、文件打开或进程退出。

规则在 [rules_noder.go](../internal/port/rules_noder.go)，手写输入装配在 [adapter_context.go](../internal/noder/adapter_context.go)，原生算法保留来源目录和文件名。PathContext 仅存储 AbsFile 原先读取的 GOROOT 输入。

移植工具现在可从直接函数调用的单一、非泛型返回值推导外部变量类型，例如 `GOROOT = os.Getenv(...)` 的 string。只移植类型契约，不复制或执行初始化表达式；无法证明的表达式仍拒绝。函数返回类型漂移、多个返回值及 adapter 字段不匹配均会报错。

## 验证

- 真实 parser → syntax.Pos → 原生 XPos：文件位置、`//line`、已 trimmed 文件、GOROOT 和 trimpath、未知位置、缓存稳定及旧表保留。
- 16 个独立 Link/PosMap 对同一个只读 syntax.PosBase 并发映射，使用 race detector。
- 两套固定补丁源码各 100 组目录、文件名、重写规则和 trimmed 标志组合；使用未改写的上游 posMap/trimFilename/AbsFile 算法作参考，比较压缩位置、相对行列、line base 和 AbsFilename。参考端仅重定位 import、适配 NoXPos 零值，并显式配置测试宿主全局。
- 独立 consumer module 只导入公共包，复用 gcbridge.Link；既有类型检查、codec、对象声明及升级检查继续通过。

位置链路之上的完整 writer 已在[第六批](m3-writer.md)接入。Unified IR reader、compiler IR 和 SSA 构造仍在后续阶段。
