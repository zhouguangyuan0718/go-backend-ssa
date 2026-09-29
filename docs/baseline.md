# 源码基线与检查证据

检查日期：2026-09-29。

## 正式设计基线

- 官方仓库：https://github.com/golang/go.git
- Tag：`go1.27.1`
- Commit：`862c888e612ac346c7c4d99c9392bdfd265f33b0`
- VERSION：`go1.27.1`；文件内时间 `2026-08-28T16:20:06Z`。
- 本地 tag 解引用与 `git ls-remote official refs/tags/go1.27.1` 返回同一 commit。
- [官方发布历史](https://go.dev/doc/devel/release#go1.27)记录该版本发布于 2026-09-01。

选择这一固定版本作为首轮移植源，不承诺动态跟随“最新”版本。

精确 pin 在 [rules.go](../internal/port/rules.go) 的 `Baseline()` 中维护。下方源码链接固定到该提交；审计报告按需生成到 `_work/`，不作为项目配置维护。

## 已核对的关键路径

所有源码链接固定到同一 commit：

- [gc/main.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/gc/main.go)：初始化顺序、完整前端阶段、类型/函数/全局对象的迭代驱动。
- [noder/irgen.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/noder/irgen.go)：types2 checker、importer、目标 Sizes、语言版本。
- [noder/unified.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/noder/unified.go)：Unified IR 与跨阶段回调。
- [types/type.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/types/type.go)、[size.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/types/size.go)、[pkg.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/types/pkg.go)：类型全局、目标布局、包表及缓存。
- [ssagen/ssa.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/ssagen/ssa.go)：buildssa 内的构造、优化、栈帧及 spill 尾部。
- [ssa/compile.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/ssa/compile.go)：early fuse/expand calls 边界及后续 pass。
- [ssa/cache.go](https://github.com/golang/go/blob/862c888e612ac346c7c4d99c9392bdfd265f33b0/src/cmd/compile/internal/ssa/cache.go)：Cache 中真实 Value/Block 的存储及 Reset。

## 本地参考环境

已有 GoALLC 工作树：`/Volumes/Disk1/00.Work/00.Code/goallc/go`，HEAD `c0e407dfd158d05217b32d4e13e008fb8c2ed792`，VERSION 为 go1.27rc2；此次读取官方 tag 的 Git 对象，没有切换该工作树。其未跟踪 `.idea/` 保留。

设计阶段宿主 PATH 的 Go 为 `go1.26.5 darwin/arm64`。M1 使用 `GOTOOLCHAIN=go1.27.1`，实际执行工具链为官方 `go1.27.1 darwin/arm64`，没有使用 GoALLC rc2 作为验证工具。

## 验证范围

设计阶段完成源码阅读、pin 核对、文档和目录创建。M1 进一步实现生产 import 闭包的保守 AST 清单及真实源码切片生成/测试；详见 [M1 记录](m1.md)。仍未实现源码到 SSA 的编译接口，也未证明整个编译器完全去全局化。
