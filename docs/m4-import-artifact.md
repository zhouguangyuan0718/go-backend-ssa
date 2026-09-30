# M4：严格版本化 import artifact

`compiler.Build` 现在同时返回 Built SSA 包和 `Package.Artifact`。artifact 保存原生 Unified IR 类型、泛型及跨包内联函数体；调用方通过 `Request.Artifacts` 导入，不需要先生成原生机器码归档。

```go
dep, err := compiler.Build(ctx, depRequest)
// 检查 err 后，可持久化 dep.Artifact.Bytes()。
artifact, err := compiler.DecodeArtifact(dep.Artifact.Bytes())
// 检查 err 后，将每个依赖的精确输入一并提供。
req.Artifacts = map[string]*compiler.Artifact{"example/dep": artifact}
pkg, err := compiler.Build(ctx, req)
```

## 格式和输入校验

当前容器为 format 1：固定 magic、32 位小端 header 长度、规范 JSON header、原生 Unified IR payload、末尾 SHA-256。header 最大 1 MiB；校验覆盖整个 header 和 payload。`DecodeArtifact` 拒绝错误格式/源码 pin、损坏校验和、非规范 JSON、未知字段、错误长度、无效原生索引表或重定位边界，以及 header 与 payload 不同的包身份。

header 记录精确 Go 版本和上游提交、语言、目标与规范实验配置、阶段、包名/路径、原生对象引用能力及直接依赖的种类和 SHA-256。当前只接受本库锁定的 Go 1.27.1 / 提交 `862c888e612ac346c7c4d99c9392bdfd265f33b0`；不以“同为 Go 1.27”代替精确匹配。

Build / Compile 在进入前端前检查每个 artifact 的包路径、语言、GOOS/GOARCH、GOAMD64 与有效实验配置，并要求调用方提供其完整依赖闭包。缺失或替换依赖、同一路径同时提供 artifact 与原生归档、nil/零值 artifact 均返回错误。依赖检查按路径排序，错误选择不依赖 map 遍历顺序。

`Artifact` 的内部数据不可变，可由并发编译共享；`Bytes`、`UnifiedIR` 和 `Info().Dependencies` 均返回副本。每次导入仍通过原生类型/函数体 reader 解码到本次 Universe，不共享其他 Build 的可变 IR。`SHA256` 返回完整容器的内容身份，`Fingerprint` 返回原生 Unified IR 的 Go linker 指纹。

## 原生编码与对象边界

独立导出复用整个原生 Unified IR linker 和 encoder。适配层在 `linker.lsymIdx` 入口选择上游已有的名称回退路径，不执行本地对象编号；原生 Compile 仍使用原来的编号规则。原生归档输入和 artifact 输入在读取 export payload 的边界分流，随后进入同一个 package、types2、IR、泛型及函数体 reader。

纯 artifact 依赖闭包使用名称引用。`Request.Imports` 是已验收的原生归档适配器：上游会原样复制某些导入对象的扩展记录，因此混合输入可能保留归档中的对象索引。`ArtifactInfo.NativeObjectReferences` 显式标记这一能力，并沿依赖链传播；这些对象必须与记录的归档哈希一致。不能把原生归档输入描述成完全独立于原生对象的 artifact 闭包。

artifact 不包含机器码。消费后端需要另外生成对象、运行时信息及链接输入。采用原生 Go 对象适配器时，包对象必须使用 artifact 的 `Fingerprint`；任意另行编译的同名 `.a` 不自动具有相同的导出指纹。`UnifiedIR` 提供对象写入器所需的锁定版本 payload。

独立导出要求准备完毕、空闲且尚未编号的本地包。与原生导出相同，它只执行一次；输出失败后也被消费，nil 输出在执行前拒绝。不会将半成品 Package 返回给源码 API 调用方。

## 迁移规则

新增边界使用现有 typed call / method binding 规则。生成器现在先记录当前包的原始方法声明与 recover 属性，再验证本包方法绑定；仅依赖绑定方法调用的 owned receiver 也必须取得实例 Context。带显式 host-stream 签名改写的 retained function，其 adapter 与改写后的原生签名逐项匹配。独立夹具验证原始方法体、实例隔离、owned receiver、恢复语义拒绝和错误签名拒绝。

## 验证

- 四组并发三包往返覆盖泛型函数/字典、跨包内联、类型别名、初始化、方法和闭包；同名类型跨不同导入路径保持本次 Universe 内的同一身份。序列化、反序列化及后续 Build 后 artifact 和旧图保持不变。
- 版本、源码提交、目标、语言、实验、阶段、包身份、格式边界、校验和、依赖替换和原生适配器身份有拒绝回归。单次导出、短写失败、nil 输出和失败后新实例均通过生命周期回归。
- 独立官方 oracle 仅在官方 `noder/linker.go` 的符号索引入口选择名称回退，保留官方前端、优化、编码及机器后端。默认、关闭内联、关闭优化三种配置的完整 Unified IR payload 与本库一致；含原生归档依赖的混合输入 payload 也一致。
- 直接消费本库 artifact 的原生 Compile 生成与官方完全一致的完整归档；使用匹配导出指纹的官方依赖对象链接后，泛型、初始化、接口、闭包和 defer 程序执行结果为 `7 18 9 10 2`。
- 既有四组 Built 图/静态数据对照、四种原生归档/链接/运行配置和各 PGO 输入格式的机器码对照继续通过。
- 全量 `go test -race -timeout 30m ./...` 通过；最终输入表复制的 compiler 竞态回归及匹配语言/CPU/实验配置往返通过。`syncgo check` 验证 474 个生成文件一致。
- Go 1.27.1 / Go 1.27.0 各 12 组完整原生归档差分通过，包括优化/调试配置、内联、大图 Phi、分离归档、cgo pragma 记录和 build ID。这里的 cgo pragma 差分不等同于完整 cgo 驱动验收。
- 冻结版本的完整 dist bootstrap 通过（428.95 秒），682 个不同包经本库编译；各阶段及最终编译器生成的完整 14072 字节测试归档与官方一致。强制重编的编译器与 toolchain3 的全部 35575009 字节一致，最终工具链程序运行结果与官方一致。该项限定 Linux/amd64、CGO_ENABLED=0、官方 Go 1.27.1 bootstrap SDK。

EarlyOptimized、Linux/arm64、完整 cgo/race 驱动、最低 bootstrap SDK 和规定的更广回归仍按原路线推进。
