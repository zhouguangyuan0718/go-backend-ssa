# syncgo

固定版本源码审计与移植命令，使用 Go 1.27.1。版本与规则直接定义在 [rules.go](../../internal/port/rules.go)，支持 `-root` 指定项目根目录。更新上游版本时修改 Go 定义；工具仍检查 tag/commit/VERSION 一致及 Go 1.27 范围。

```sh
go run ./cmd/syncgo inspect -repo /path/to/go -out _work/inspection.json -summary _work/inspection-summary.json
go run ./cmd/syncgo generate -repo /path/to/go
go run ./cmd/syncgo check -repo /path/to/go
```

inspect 不指定 -out 时输出完整 JSON 到 stdout；报告较大，通常写到忽略的 `_work/`。generate/check 不接受 -out/-summary。普通源码仓库无需干净，不会切换 HEAD 或修改工作树。

generate 遇到被编辑的生成文件或同名未登记文件会拒绝覆盖；应先审阅改动，将需要保留的修复移到规则/adapter，再恢复 ledger 对应版本。不要通过删除 ledger 绕过所有权检查。异常终止遗留的 `.cache/syncgo.lock` 只可在确认没有生成进程后清理。

实现范围见 [M1 记录](../../docs/m1.md)。
