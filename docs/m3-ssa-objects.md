# M3：SSA 使用的汇编编号

完整 SSA Config/opcode 表直接引用原生汇编器的寄存器号和指令号。本批迁入这些依赖，避免用本项目另造的枚举代替上游编号。

`internal/cmd/obj` 新增原生 `AXXX`/`A_ARCHSPECIFIC`、各架构 ABase 和 RBase 常量组；下列目录按上游名称保留 SSA 实际引用的完整常量组及 iota、别名关系：

- arm、arm64、mips、s390x、wasm 的 `a.out.go`。
- loong64 的 `cpu.go`、`inst.go`。
- ppc64 的 `a.out.go`、`asm9_gtables.go`。
- riscv 的 `cpu.go`。
- x86 的 `a.out.go`、`aenum.go`。

共 9 个架构包、12 个新增生成文件；两个已有 obj 文件仅增加常量声明，不改变原有函数算法。迁移规则仍为普通 Go 声明列表，不新增配置文件或后端抽象。未迁入汇编器注册 init、DWARF 寄存器全局 map 或汇编编码实现。

## 验证

`TestPinnedSSAObjects` 从被验证 Git 版本的 `ssa/opGen.go` 中读取 opcode/register 初始化器，自动提取实际引用的所有外部常量，为原版和移植版生成同一份逐项数值对照程序。原版使用该 Git 版本的 obj 及各架构包覆盖层；移植版必须能导入并引用每个相同名字。新增上游引用会自动进入检查范围，缺失迁移不能靠缩小固定测试列表绕过。

- go1.27.1、go1.27.0 的全部引用对照通过，38.27s，日志 `_work/m3-ssa-objects-source.log`。这是所有架构编号的比较，不是跨架构机器码执行测试。
- `syncgo check` 验证 302 个生成文件，工具版本 syncgo-m3-v26。
- `GOTOOLCHAIN=go1.27.1 go test -race -timeout 30m ./...` 通过，internal/port 67.084s，日志 `_work/m3-ssa-objects-unit.log`。

```sh
GO_SSA_SOURCE_REPO=/path/to/go GOTOOLCHAIN=go1.27.1 \
  go test ./internal/port -run '^TestPinnedSSAObjects$' -count=1 -timeout 10m -v
```

本批仍不提供 SSA Config/Func 构造或汇编输出。下一步需要迁入 opcode/register 数据与原生图结构，并解决 Config 修改 opcodeTable、Op 的无接收实例查询方法、Cache 生命周期之间的关系。原生重写函数和最终编译器回接仍在后续工作范围内。
