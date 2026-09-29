# M3 原生符号与内联位置

IR 的 Name/Func 依赖 compiler Sym 到 obj.LSym 的绑定；节点格式化和闭包命名也依赖 Link 的内联调用链。本批移植这些真实上游方法，保留已有完整 Link/LSym/InlTree 数据定义。

## 公共入口

```go
layout, _ := target.NewLayout("linux", "amd64")
u, _ := types.NewUniverse(layout)
link := &gcbridge.Link{}
u.BindLink(link)
name := u.NewPkg("example/p", "p").Lookup("F")
name.SetFunc(true)
symbol := name.Linksym() // 同一 Link 中 ABIInternal 的 example/p.F
// symbol == link.LookupABI("example/p.F", gcbridge.ABIInternal)
```

`Sym.Linksym/LinksymABI`、`Link.Lookup/LookupInit/LookupABI/LookupABIInit` 保留原生签名。不同 ABI 使用上游已有的两张 Link map，一次性初始化仍由 Link 的原生锁和 symOnce 控制。linkname、go:/type: 保留前缀及空白符号采用原生 base.PkgLinksym/Linkname 算法。

Universe 必须显式 BindLink，重复绑定同一个 Link 无操作，绑定到另一个 Link 会 panic。独立编译需要独立 Universe 和 Link；绑定操作由调用者串行执行。该操作绑定符号命名空间，不迁移或合并 Universe 和 Link 各自已有的位置表。XPos 仍只能通过它原来的 PosTable 解码。

共享的输入保存在 `base.LinkContext`：原先的全局 Ctxt、Flag 和 ReservedImports 改为实例字段。`Universe.LinkContext()` 返回该已绑定输入对象，供后续编译器驱动共享；取得后不能把 Ctxt 改绑到其他 Link。ReservedImports 默认 map 每个实例独立。这里没有新符号模型或进程全局注册表。

`gcbridge` 公开原生 ABI/ABISet 和 ABI0/ABIInternal 常量。`InlTree.Add/AllParents/Parent/InlinedFunction/InlinedFuncName/InlinedFuncPkg/CallPos` 以及 `Link.AllPos/InnermostPos/OutermostPos` 直接复用原生算法。base.FmtPos 使用实例 Link 和原生 Flag.C/Flag.L，不读取宿主全局。

## 规则与验证

- 符号适配使用上一批 ContextFrom，通过 Sym.Pkg 所属 Universe 取得已绑定 Link。保留原生 Sym、Pkg 和 LSym 身份。
- 同一包内允许按文件选择只将依赖状态的函数改为实例方法，保留其他已评审文件的原有规则。因此 `CompareSyms/IsExported` 仍是无状态函数，Sym 的实例方法签名也保持不变。
- 32 个 goroutine 对同一 Link/ABI 查找，验证唯一对象和初始化仅执行一次；ABI/普通符号 map 共用与分离、先查找再初始化、属性更新。
- 16 个独立编译实例生成相同名称，验证对象隔离；函数/数据 ABI、linkname、保留前缀、空白符号、绑定失败和旧对象归属。
- 两层内联调用链、最内/最外位置、完整 AllPos 顺序、实例路径显示选项。
- 两套固定上游源码比较所选 obj 方法的完整签名和函数体，保留原生对象声明对照和已有前端/编码差分；独立 consumer module 通过公共 API 验证符号与内联位置。

完整 IR 节点构造、reader/typecheck 和 SSA 仍待接入；本批不能作为原生编译器替换或工具链自举完成的证据。
