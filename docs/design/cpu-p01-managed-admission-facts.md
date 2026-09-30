# CPU05 受管解析事实文件

来源：CPU01/CPU02/CPU04/CPU05、v0.4 development D03/D05 与 operations O08，以及
当前 Goal 第 3、5、7、9 节。承接[候选解析](cpu-p01-admission-resolution.md)。

## 首片边界

本片入口为 `AdmissionResolver.ResolveManaged(ctx, request)`：从显式绑定的
facts reader 读取可信 tenant 和明确 Release ID/digest 对应的事实，再交给既有
`Resolve(ctx, request, facts)` 组合核。普通请求不能提供 facts 或配置文件路径。
原显式事实入口保留为内部组合能力，不因此成为网络入口。

实际文件 owner 属于 `internal/data/admissionfacts`；`FileSource{Path, SHA256}` 是
有限的受信配置输入。它加载固定字节，按 tenant＋Release ID/digest 查找，返回独立值。
这个 key 不含 Governance binding generation，不选择最新、当前或默认 Release。
此片不改 Proto、server、cmd、conf 或 Governance，不注册 RPC，不装配生产启动配置。

## 私有文件与摘要

私有 schema 为 `ani.modeldev.managed-admission-facts.v1`。明确字段为
resource_tenant_id、release_id、release_digest、environment、input_scope、
publication_scope、runtime、workspace、input_file_path、output_directory_path，
以及 environment_evidence/application_evidence 两个 `{reference, sha256}`。
嵌套事实复用当前 cpup01 typed 值；文件 envelope 不是共享公共 API。

外部 SHA 固定文件的实际 UTF-8 字节，不写入文件自身。加载候选限定 1–64 个来源、
每文件最多 64 KiB、JSON 嵌套最多 16 层。打开时不跟随最后一级符号链接，不阻塞于
FIFO；从同一文件描述符核对普通文件、单链接、长度和读取前后的变更时间。这里只
限制受信配置给定的文件，不接受用户文件路径，也不声称排除了所有父目录符号链接。

解码候选拒绝未知/重复字段、字段大小写别名、null、尾随值、非法 UTF-8/替换字符、
摘要错配及重复 tenant/Release key。file owner 核对文档版本、固定索引身份和 evidence
引用形状；实际 Runtime/workspace/输入范围等兼容及完整快照 shape 继续由既有 resolver
和 cpup01 校验，避免另造一套业务规则。语法加载成功不等于其事实已通过业务组合。
全部文件成功后才返回只读索引，读取时复制唯一可变的 Runtime target-jobs 切片。
这些拒绝行为尚需后续真实负向 RED/回归，不能以首正向测试代表全部边界已验证。

四类摘要保留各自定义：

- `Environment.BindingDigest` 来自已有 ENV 绑定身份/配置的实际交接及其摘要规范。
- `FileSource.SHA256` 固定本私有文件的实际字节，不替代 ENV 绑定摘要。
- `ReleaseDigest` 固定原不可变 Release，由现有 catalogue reader 独立核验。
- `PipelineOwnerConfiguration.RevisionSHA256` 属于后续 KFP owner 配置；PipelineRoot
  不进入此文件或 Snapshot，也不能借前三者的 hash 代填。

两项 evidence 只定位各自材料的字节身份。reader 不探测环境，也不因 reference、digest
形状或存在字段而确认 Namespace/Runtime 存在、身份有权、存储和挂载可用。真实生产
文件必须由 ENV/CPU02 证据消费和受管启用流程形成；本片 fixture 不得成为生产默认。
缺真实材料仍不装配，整体 `/readyz` 保持 503。

## 首个有意义的 RED

`TestResolveManagedAdmissionUsesPinnedFactsWithDurableInput` 复用既有真实 catalogue
及 restricted-role PostgreSQL fixture：实际导入 Release，持久 READY，再用新连接恢复。
测试运行时才写实际 JSON 文件并计算外部字节 SHA；本机不运行 fixture 生成程序。
这些前置全部通过后，才调用新产品入口。预期初次失败为
`managed admission facts loading not implemented`，不是数据库、文件准备或格式错误。

首测试按手写固定 Snapshot 得到独立期望规范摘要，绝不调用被测 ResolveManaged 构造
expected。验证完整事实、ENV digest 独立性、原 intent 不变、返回事实/候选无可变别名、
重复解析一致、持久 READY 不变且未创建 execution。fixture 中 ENV 和字节核验记录均为
明确模块样本；本测试不证明真实 ENV、对象存储或目标集群业务通过。

固定 `6d336f2e556e4ddc450ad347f370de5a6bc4baa0` 已在 Fedora 新 checkout 运行首测试：
PG 和 managed-facts 两项 preflight PASS，唯一产品失败为上述 Load stub，0.083s、exit 1。
证据目录为本轮 `managed-facts-red-6d336f2e556e4ddc450ad347f370de5a6bc4baa0/`。
随后 gofmt -d 返回格式差异 exit 1，中断了 runner 的归档末尾；原测试日志/退出码完整，
独立收尾脚本在不重跑测试、不修改源树的条件下补齐记录和 manifest。此格式结果不是
产品 RED，也不是格式 PASS。

当前已按该 RED 写最小 Load/read/ResolveManaged 实现候选，等待 root 固定新 SHA 和
Fedora GREEN/最终格式回传。严格负向边界、生产配置与 RPC/Governance 消费均 NOT_RUN。
