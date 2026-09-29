# M3 第三批：Unified IR 编码基础

已移植 Go 1.27.1 的完整 `internal/pkgbits` 生产源码，保留到 `internal/go/pkgbits`。这是 noder writer/reader 使用的原生 Unified IR 编码器和解码器，尚不是源码到 Unified IR 或 compiler IR 的入口。

## 已实现

- 原生 `PkgEncoder` / `PkgDecoder`、元素和字符串表、重定位、常量、同步标记、版本字段及 fingerprint 算法完整保留。没有另定义 artifact 格式。
- `PkgDecoder` 直接容纳原包变量 overflow；使用原 `NewPkgDecoder(pkgPath, input)` 的值返回形式。`Decoder.common` 继续指向其原生 decoder，不增加平行 Context 或全局注册表。
- 只读版本/枚举数组通过既有规则局部化。生成器仍检查不存在包变量/init、未适配的进程状态及宿主 I/O。
- 原 `fmt.Printf` 诊断改接 `PkgDecoder.TraceWriter`；nil 时丢弃。原同步标记失配的 `os.Exit(1)` 改为 panic `DecodeAbort{Status: 1}`，可在调用边界 recover。其他畸形数据和写入失败仍使用原有 panic 行为；这是内部低层 codec，不是面向不可信数据的已验证导入 API。
- 保留读取当前调用栈的 `runtime.Callers/CallersFrames` 及 frame 字段，用于 syncframes 诊断；没有放开进程配置、更改 GC、退出或文件系统访问。

新增移植规则在 [rules_pkgbits.go](../internal/port/rules_pkgbits.go)，手写对接在 [adapter_decoder.go](../internal/go/pkgbits/adapter_decoder.go)。移植工具支持原生构造函数的参数、变参和指针/值返回，并将包装函数留在原文件以保留参数类型 import。初始化前的状态依赖按函数/方法依赖闭包检查；值返回构造禁止会保留返回前实例地址的状态初始化。

## 验证范围

- 两个固定补丁版本分别编译未改写的原生 codec；迁移上游自身测试，并比较 V0–V4、无同步标记/零栈帧同步标记的完整字节、fingerprint 及双向解码结果。V0 无 Flags 字段，开启同步标记属于原生解码也失败的组合，单独比较失败行为。旧格式用于原算法兼容性测试；源码基线仍只支持固定 Go 1.27。
- 验证字符串去重、极值整数、大整数、有理数、浮点、复数、元素重定位、PeekPkgPath/PeekObj、临时解码器 scratch 复用。
- 正栈帧数量的往返、畸形输入、overflow sentinel 实例隔离；并发解码失败、独立输出、失败后继续读取及保留旧 decoder 的 race 测试。

正栈帧数量时的路径和调用位置随移植位置变化，不能声明编码逐字节相同；相应测试验证往返和诊断行为。这里只验证 codec，尚未验证 noder 导出的完整包、泛型函数体/字典及本库 artifact 与官方归档的互操作。

## 下一步

沿上游 `checkFiles → newPkgWriter/collectDecls → writePkgStub → newPkgReader/readPackage/readBodies` 接入 noder。writer 可先复用已有 syntax/types2 和 pkgbits；reader 仍需补全 compiler ir/typecheck 及其依赖，不能用空 stub 替代。上游 `freePackage` 清空 types2 图以及全局回调装配需在库边界明确去除，以保留已返回结果及实例隔离。
