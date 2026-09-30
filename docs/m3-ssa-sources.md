# M3：原生 SSA 源码诊断与 HTML 构造

本批迁入 buildssa 所需的源码诊断依赖：ssagen 的 InitEnv、DumpInline、readFuncLines、dumpSourcesColumn，以及 ssa 的 NewHTMLWriter、newDotWriter、passIdxByName。源码选择、扫描、排序、HTML 构造和阶段范围解析全部来自固定上游源码。

## 原生路径

InitEnv 保留 GOSSAFUNC 中函数名、尾随 `+` 与 CFG 阶段的解析；DumpInline 保留函数名匹配和原始 IR 函数身份。源码列先显示目标函数，再按文件名和行号排列内联函数。readFuncLines 保留环境展开后的打开路径、原始显示文件名、包含两端的行号范围和原生扫描/关闭行为，包括上游忽略 Scanner.Err 与 Close 错误的行为。

NewHTMLWriter 保留路径规范化、打开标志/权限、相对路径的工作目录解析、原生 HTML 起始内容和关闭输出。CFG 阶段选择支持全部阶段、区间、并集及下划线替代空格，非法区间和 dot 查找失败保留原生结果与诊断。

## 实例输入

ssagen.Context 的 DumpFunction / DumpDirectory 提供 GOSSAFUNC / GOSSADIR。OpenSource 返回 io.ReadCloser，ExpandSourcePath 显式提供源码路径的环境展开。SSA Context 的 OpenHTML 返回 io.WriteCloser，WorkingDirectory 和 LookPath 提供报告路径及 dot 查找；已有 Command 提供实际 SVG 渲染时的进程工厂。库不隐式读取宿主文件、环境或可执行文件路径，未配置实际需要的服务会被拒绝。

公开 ssa.HTMLWriter、ssa.FuncLines 和 ssa.NewHTMLWriter，复用原生数据表示。DumpInline 在追加前检查 IR 函数属于当前实例。

## 生成器约束

HostCallRule 新增严格审核的 Open、OpenFile、Getwd、ExpandEnv、MkdirAll 与 LookPath 绑定。文件返回值仅缩窄为所需读/写关闭接口，参数与错误结果保留原生签名；FileMode 别名按其真实类型核对。OpenFile 的三种原生写入标志仅在绑定调用的 flags 参数内作为只读常量通过审计。

StringArguments 支持多个明确的常量键，要求每个列出的键实际使用。动态键、别名调用、未审核的键、重复/空键、两种键配置混用、额外文件方法、不支持的标志及接口/参数放宽均有拒绝测试。MkdirAll 绑定已通过生成器的实际执行测试，供后续完整 buildssa 使用；本批迁入的源码诊断尚不调用它。

## 验证

本地测试使用真实原生方法，验证 HTML 内容和关闭、读写顺序、权限/标志、相对与绝对路径、CFG 阶段选择、源文件范围、错误传播、内联归属及 8 个并发实例。生成器测试实际编译并运行迁移后的代码，检查流身份、关闭、参数传递及拒绝条件。独立 consumer 从公共包创建报告并写入内存流。

上游差分分别比较 HTML 构造/CFG 选择和源码选择/排序，使用 Go 1.27.1 与 Go 1.27.0 的原生实现；两个版本分别验证 386、amd64、mips、ppc64：HTML/CFG 与源码列共 16 份快照全部一致，包含缺失内联源文件与缺失目标源文件的报告。

全量 `go test -race -count=1 ./...` 已通过，包含独立 consumer；`syncgo check` 验证 406 个生成文件一致。

这一步补齐 buildssa 的诊断依赖。IR→SSA 状态构造、phi/intrinsics、完整 GC 栈图与调试位置、指令输出、统一驱动及替换/自举尚未完成。
