# 生成文件清单

[generated.json](generated.json) 是唯一保留的 JSON 文件，由工具自动维护，记录生成文件归属、源码来源及校验哈希，用于防止覆盖人工修改和清理过期生成文件。它不是人工配置，也不参与库的运行。

上游版本、抽取范围和 Context 改写规则集中在 [rules.go](../internal/port/rules.go)。路径与 import 映射由工具推导，package 名直接保留上游源码中的声明。

审计报告按需输出到忽略的 `_work/`，不作为生成输入或版本管理内容。完整工具设计见 [移植机制](../docs/porting.md)，当前能力见 [M1 记录](../docs/m1.md)。
