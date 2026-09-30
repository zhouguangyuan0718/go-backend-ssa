# M3 原生整包收尾

`gc.Context.FinishPackage` 保留固定上游 `Main` 在声明固定点之后、归档写出之前的连续原生区间：runtime 写屏障递归检查、全局 map 保活重定位、计时阶段、`dumpdata` 和符号编号。

`gc.Context.CompilePackage(symABIs, profile)` 将 `PrepareFrontend`、`CompileDeclarations` 和 `FinishPackage` 串起来。输入是已加载并完成覆盖率修正的本地 IR；归档配置和输出流仍由调用方提供。该入口不是源码加载或命令行入口，也不代替独立后端所需的 Built/EarlyOptimized 阶段接口。

## 边界与失败状态

收尾要求前端和声明阶段已经成功完成，只能运行一次。全局 map 的 `R_KEEP` 在所有函数编译后建立，随后参与符号编号。禁用 map outlining 时保留原生条件分支。runtime 写屏障检查使用前端阶段收集并在编译时补全的调用图。

生成器为第三个 Main 连续区间复用已验证的函数复制和结构锚点；不另写阶段算法。原生 `Link.NumberSyms` 调用通过已有的方法绑定规则，显式连接同一 writer 的 builtin 表初始化和成功标志提交。原生编号函数体保持不变。

发生数据提供器 panic、编号失败或 runtime 诊断后，该编译实例不能继续收尾或配置归档头。编号的成功标志只在完整算法返回后设置；不能把开始编号当成成功。调用者仍持有已成功输出的归档，下一次编译使用独立状态。

## 验证

- 8 个并发实例验证真实全局 map 初始化包装、收尾前无 R_KEEP、收尾后恰有指向机器码的保活重定位，以及完整归档和旧输出保活。
- 未执行/未完成前端或声明阶段、重复入口、禁用 outlining、runtime 禁写屏障诊断、嵌入数据提供器 panic 和编号失败均有定向回归。
- Go 1.27.1 / Go 1.27.0 共 28 组归档逐字节差分通过，包含强制和禁用 map outlining、四 worker、内联、ABI、DWARF、分离归档与官方 archive/goobj 回读；原生参考直接从固定 Main 另行抽取收尾区间。
- 全量 `go test -race -timeout 30m ./...` 通过；468 个生成文件的一致性复查通过。

## 后续

仍需源码到包的统一 Build、命令行/输出诊断服务、真实链接运行、第二目标和替换编译器自举验收。当前接口完成的是已加载 IR 的原生编译与编号流程；Main 在归档之后执行的 `CheckLargeStacks`、`CheckFuncStack`、优化日志和最终诊断仍需由完整驱动接入。
