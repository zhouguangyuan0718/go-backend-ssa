# M3 原生 PGO 输入与带 PGO 的自编译

`compiler.Request.PGOProfile` 接受显式名称和字节，支持未压缩 pprof、gzip pprof 及 Go 预处理 profile。名称只用于诊断；库不自行读取文件或环境。nil 表示关闭 PGO，空 profile 保留原生无权重处理。输入被复制，归档返回值与输入存储独立。

## 原生算法与实例边界

迁入固定上游 `internal/profile` 的 protobuf/gzip 解析、消息解码和 profile 校验，`cmd/internal/pgo` 的完整 pprof 转调用边算法，以及 `pgoir.New` 的格式探测、读取和 IR 图归属流程。解码表、错误身份和解析消息归属于实例；原生边排序、调用位置和权重语义保持不变。公开 `pgo.FromPProf(io.Reader)` 可单独预处理输入。

`pgoir.Context.OpenProfile` 显式提供输入流，原生读取流程负责关闭。打开/读取失败保留错误原因；未配置 provider 或返回 nil 流会报错。所有非空 IR 节点必须属于相同编译实例，包含前端惰性查找得到的函数。

PGO 在源码加载、SSA 配置及覆盖率修正之后读取，再传给包前端和声明固定点。隔离上游驱动接受 `-pgoprofile`，将输入文件读成字节交给库，并在可选成功调用记录中保存 profile 名称。

## 初始化顺序回归

默认编译器 profile 会触发此前普通构建没有触发的导入函数体查找。延迟创建 walk 状态会使 `internal/runtime/math` 先以空包名进入缓存，再与原生 walk 包初始化使用的 `math` 名冲突。

walk 现在可先通过 `NewPackageState` 执行完整原生包变量初始化，再创建本地包并初始化 builtin 类型；随后通过 `BindFrontend` 绑定同一图的 noder/escape。这样也保留 builtin `error.Error` 与本地导出方法所需的符号身份。单独回归同时编译普通包和 `internal/runtime/math` 本身，覆盖实现 error 接口和动态切片分配。

## 验证

- 四种模式（关闭 PGO、raw、gzip、预处理）完整归档与官方 Go 1.27.1 逐字节相同；官方 preprofile 的输出与固定测试输入一致。
- 对实际机器码反汇编：关闭 PGO 时 Caller 包含 Hot 调用，三种热点 profile 模式均已消除该调用；三种 profile 表示生成相同归档。
- 解码/序列化往返、12 个并发 decoder、16 个并发预处理实例、错误身份隔离、失败后复用、输入关闭及外来 IR 拒绝均有回归。
- 隔离替换版 cmd/compile 强制重建含 runtime 的 59 包程序和全部 374 个 Linux/amd64、CGO_ENABLED=0 标准包。
- 编译器自编译使用上游默认 PGO profile，成功调用记录证明主包收到该 profile；生成的 Linux/amd64 编译器可执行，并生成与宿主第一阶段完全相同的 2386 字节测试归档。
- 最终全量 `go test -race -timeout 30m ./...`、474 个生成文件一致性检查及 Go 1.27.1／1.27.0 共 28 组整包归档差分全部通过；完整 PGO 隔离集成复跑通过（44.24 秒）。

这些结果覆盖一次带 PGO 的真实编译器自编译。完整 dist bootstrap、多阶段稳定性、更多上游回归、cgo/race 驱动、第二目标和独立 Built/EarlyOptimized API 仍待完成。
