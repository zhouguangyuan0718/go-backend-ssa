# M3 原生 reader 记录解码与作用域标记

本批开始迁入上游 noder/reader.go，保留其完整原生 `pkgReader`、`pkgReaderIndex`、`reader`、`readerDict` 和 `readerMethodExprInfo` 数据声明。生产方法目前包括包读取、位置基表与内联位置映射、类型/对象引用记录、标识符和 pragma 记录解码；未迁入的方法不以空实现代替。

这些都是后续类型实例化和函数体读取直接使用的原生结构。当前没有新建对外“简化 reader”模型，也没有把只读元数据入口称为完成源码到 IR。公共 API 暂保持不变；已有原生 Expr/Stmt 接口见 [IR 检查](m3-typecheck-ir.md)。

## 状态归属

- `pkgReader` 保有自己的 PkgDecoder、posBases、pkgs、typs 和 newindex 缓存，额外的 Context 指针连接已有 noder 实例。
- `reader` 保有原生 element Decoder、函数/字典/闭包/内联等字段，并绑定其创建实例。所有 base.Ctxt 访问改用既有 Context.Ctxt；GOROOT 是显式输入，不读取宿主环境。
- 包身份仍来自既有 types.Universe：同实例多次读取同一路径返回相同 Pkg，不同实例相互独立。位置基缓存属于各个 pkgReader，转换后的 XPos 注册在该实例的 Link.PosTable。
- builtin 和 unsafe 使用原生特殊路径规则；exported selector 归 LocalPkg，未导出 selector 保留来源包。配置读取方时必须按编译器协议设置 LocalPkg。
- `typInfo/objInfo` 当前只解码原生引用记录，并不调用 typIdx/objIdx 实例化类型或声明。

## 函数体所需的 ScopeMarker

同时迁入完整 `dwarfgen/marker.go` 与 `scope.go` 中的原生 xposBefore。原生 ScopeMarker 中的 parents/marks 保持实例字段；新增的轻量 Context 仅连接已有 base.LinkContext，不复制位置表、目标或函数状态。

`dwarfgen.NewScopeMarker(base)` 是内部装配入口。Push/Pop/Unpush、同位置标记压缩以及 WriteTo 均执行迁来的算法。WriteTo 按上游规则复制结果到 ir.Func 后复用 marker 缓冲，既有函数的 Parents/Marks 不受后续写入影响。无效位置通过 panic 报告，不退出宿主进程。

reader 的完整结构包含 marker 字段，但本批未接入调用它的函数体方法；后续必须在函数体装配处绑定已有 base。不能把未绑定的零值 marker 当成可用的完整 reader。

## 验证

- 真实 syntax/types2/writer 产出的泛型源码 stub，由迁来的 reader 解码本包、声明引用及 `//line` 基表；覆盖启用/禁用同步标记。
- 同实例多个 reader 的包身份和位置基缓存边界；12 个实例并发写入、读取，结束后检查保留对象。
- 内联位置的缓存身份、原始位置抑制、重复内联基表拒绝。
- 12 个实例并发构造和复用原生 ScopeMarker，检查嵌套 parent、Unpush、同位置压缩、旧函数记录保活；无效/倒退位置不能先修改 scope 记录。
- 相同 bitstream 及相同 ScopeMarker 操作与官方 noder/dwarfgen 执行结果对照。参考编译使用固定 Git blob 构建临时 overlay，不修改来源 checkout 或已安装工具链。对照覆盖本包/builtin/unsafe/外部包、GOROOT 路径、line pragma、selector、derived type 引用和内联映射。

下一步继续接 typIdx/objIdx 所依赖的原生类型、字典、方法包装器和函数体读取链路。reader 的类型实例化、完整函数 IR、walk/escape/SSA 和工具链替换仍未完成。

本批验证完成：187 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过。go1.27.0/go1.27.1 的原生 noder/dwarfgen 快照及既有 writer 字节对照全部通过；同一套规则跨两版只有 1 份 Go 产物变化，来源工作树保持不变。日志：`_work/m3-reader-metadata-tests.log`。
