# 内部实现

编译器部分保留上游 `src/cmd/compile/` 以下的目录、包名和文件名。当前包含 syntax 的位置/字符读取、types 的真实结构、初步 Universe、包表/驻留/查找、ir 的链接名切片；来自 `src/cmd/internal/` 的实现位于 `cmd/src/`、`cmd/objabi/` 与 `cmd/obj/`。来自 `src/internal/` 的辅助包将放入 `go/`，避免重名。

`port` 是本库维护的移植工具实现；frontend 驱动与完整 syntax/types2/noder/IR/SSA 等仍待实现。生成文件由文件头和 ledger 标记，同包手写适配使用 `adapter_*.go`。根入口负责实例组装，底层包不导入顶层 frontend。公开数据接口通过根包或公开 facade 暴露，内部对象定义不复制。
