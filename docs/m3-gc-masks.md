# M3 原生 GC 位图和符号写入

本批迁入完整 `bitvec/bv.go` 和 `typebits/typebits.go`，以及 reflectdata 的 GCSym、WriteGCSymbols、dgcsym、普通/按需 GC mask 写入、fillptrmask 和 ZeroAddr。位图算法、GC 扫描规则、符号命名及排序来自固定的 Go 编译器源码。

## 原生 API 与状态归属

公共 `bitvec.BitVec/Bulk` 和 New/NewBulk 直接使用原生结构与构造函数；既有 `objw.BitVec` 仍是同一个类型。BitVec 的 Get/Set/Unset/Eq 保持原签名，只有原生 Fatalf 边界改为 panic；没有为位图添加编译上下文字段。

公共 `typebits.Set/SetNoCheck` 保持上游函数签名。目标指针宽度从输入 Type 已有的 Universe 取得，每次调用只取出目标字宽，不引入全局变量或另一套可变编译状态。SetNoCheck 只跳过最初的类型对齐检查，仍保留指针槽的对齐检查。

reflectdata 的 gcsymmu/gcsymset 与 ZeroSize 进入已有 Context。GCSym 登记待写入类型并返回原生 LSym；WriteGCSymbols 按原生类型顺序写入位图、标志和对齐信息，利用 OnList 避免重复写入。ZeroAddr 保留原生最大 extent 累积和 `go:map.zero` IR 地址引用。

GCSym 的原生后端并发用法得以保留：先完成参与类型的布局，禁止同时由其他阶段修改类型图，再并发登记，等待所有调用结束后串行 WriteGCSymbols。其他可变阶段仍按既有实例契约串行。不同实例的登记集合、Link 数据列表和 ZeroSize 互不共享。

## GC 语义

迁入的算法保留了 interface 只扫描 data 槽、不扫描 type/itab 槽，slice/string 只扫描数据指针，以及 NotInHeap 指针排除、数组和结构体递归、零长数组等规则。GC mask 按目标 uintptr 大小补齐，并作为原生 RODATA、DUPOK、LOCAL、content-addressable 符号写入。

按需 mask 阈值采用原生 `abi.MaxPtrmaskBytes`。超过阈值且调用者允许时，dgcsym 可写出供运行时填充的 NOPTRBSS 指针单元。需要区分上游两条路径：GCSym 的登记本身不写数据；WriteGCSymbols 始终写完整普通位图，按需单元由描述符生成路径单独写入。本批原样保留该行为，不会把两条路径合并。完整描述符/ITab 生成仍未接入公开编译流程。

ZeroAddr 只生成地址引用并累计 ZeroSize；最终零区符号声明仍由后续编译驱动接入。本批不宣称这些局部数据已构成可链接对象文件。

## 验证

- 32/64 位类型扫描覆盖指针、接口、slice、NotInHeap、数组和偏移；检查 Set 与 SetNoCheck 的原生对齐边界。
- 原生 BitVec/Bulk 的完整声明和无需环境改写的方法逐一与固定源码比较；差分覆盖零长和跨 32 位字边界的算术集合操作、计数、Next、尾位掩码、复制及批量分配。
- 精确检查 GC 位图字节、ptrdata、目标对齐、普通/按需阈值与 NOPTRBSS 单元，以及重复写入和旧实例保活。
- 64 个并发调用登记已完成布局的类型（包含按需 mask），race 验证符号身份与集合完整，随后统一生成普通位图。
- 外部 consumer 通过公共 bitvec/typebits/reflectdata 操作同一原生位图、生成 GC 符号和 ZeroAddr。
- 两版固定源码、四种目标的原生快照对照覆盖以上生成数据；符号属性比较完整原始位值，不依赖当前尚未迁入的 Attribute.String 格式化方法。

下一步继续原生 hash/equality 函数和闭包生成，再接静态名称数据、完整类型描述符与 ITab，并推进 reader。完整源码到 SSA 与替换上游编译器仍未完成。

本批验证完成：205 份产物通过 `syncgo check`；带来源仓库的全量 `go test -race -v ./...` 通过，port 包耗时 216.906 秒。两版源码四种目标的完整 reflectdata/GC 快照一致，同一套规则升级仍仅 1 份 Go 产物变化，来源工作树未改变。日志：`_work/m3-gc-masks-tests.log`。
