# 内部实现

编译器部分保留上游 `src/cmd/compile/` 以下的目录、包名和文件名。当前包含 types 的驻留、ir 的链接名及 cmd/objabi 的路径切片。来自 `src/internal/` 的辅助包将放入 `go/`，避免重名。

`port` 是本库维护的移植工具实现；frontend 驱动与完整 syntax/types2/noder/IR/SSA 等仍待实现。生成文件由文件头和 ledger 标记，同包手写适配使用 `adapter_*.go`。根入口负责实例组装，底层包不导入顶层 frontend。公开数据接口通过根包或公开 facade 暴露，内部对象定义不复制。
