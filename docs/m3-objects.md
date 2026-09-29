# M3 第四批：noder 与原生后端共享的数据声明

已按原文移植 noder 接入前所需的 `obj.Link`、`LinkArch`、`Prog`、`Addr`、内联树、DWARF 回调/修补表、目标架构及相关依赖声明。所有结构完整保留私有字段、接口和函数签名；没有删字段、改成 any 或以简化 Link 替代。`ir.Op/PragmaFlag` 及其枚举、`base.CmdFlags/DebugFlags` 也直接来自固定上游。

这些是共享数据模型，不代表原生汇编、对象输出、DWARF 算法、命令行解析或 compiler IR 节点已经可用。原有 LSym/Type/Sym 的身份保持不变。CmdFlags 的 flag.Value 是被动接口字段，移植审计仅允许该类型声明，仍拒绝进程 FlagSet/Parse 等调用。

## 位置与消费

- 规则：[rules_objects.go](../internal/port/rules_objects.go)，依赖按顺序加入现有 Migration；没有新增配置文件。
- 编译器声明保留 `internal/base`、`internal/ir`，辅助声明保留 `internal/cmd/obj`、`dwarf`、`goobj`、`sys` 等来源结构。
- [gcbridge/object.go](../gcbridge/object.go) 提供 Link/Prog/Arch 和回调签名所需的公共别名。普通外部 Go module 可持有和传递同一批原生对象，不需要导入 internal。
- `Link.PosTable` 是该实例的位置表，和已公开的 position.Table 同类型。Link 本身包含锁，不能按值复制；可变对象、表和回调仍需调用方按实例拥有。
- CmdFlags/DebugFlags 只移植数据声明，不读取环境、不解析命令行，也不自动安装上游驱动的默认值。

## 证据

两套固定源码的生成/升级测试直接逐一比较所选原生类型的完整 AST 类型体；只允许导入限定名重定位，覆盖私有字段和嵌套回调签名。独立 consumer module 验证 Link→位置表→Prog→LSym 的身份、原生 Assemble 回调的参数和 ProgAlloc 返回身份，以及另一个实例不会改写旧图。全量 race 和既有 frontend/codec 差分继续覆盖。

下一步把 noder 的位置转换接到显式 Link.PosTable，把 writer 的 checker 身份缓存接到现有 types2.Context，并为依赖这些输入的初始化提供实例入口。原生 writer/reader 的完整接入、compiler IR、SSA 及替换工具链自举仍未实现。
