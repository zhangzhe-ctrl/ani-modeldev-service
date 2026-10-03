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
reader 本身不拥有 Proto、server、cmd、conf 或 Governance。后续受信 RPC 与可选启动
装配见[候选解析启动配置](cpu-p01-admission-resolution.md#启动配置)，沿用此唯一文件 reader。

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
部署材料必须提供直接 regular readonly/subPath 文件；常规挂载中最后一级仍是符号
链接的路径不受支持，不能用测试普通文件成功宣称该挂载方式已经可用。

解码候选拒绝未知/重复字段、字段大小写别名、null、尾随值、非法 UTF-8/替换字符、
摘要错配及重复 tenant/Release key。file owner 的合同还要求全部必要字段存在且符合
typed 文件形状：不能缺失 environment/runtime/workspace/存储 scope/路径，也不能
丢掉它们的必要嵌套身份、摘要和映射字段。私有 reader 从实际文件的 typed struct tags
检查字段 presence；输入 credential reference 可显式为空，但不能省略字段。
实际 Runtime/workspace/输入范围等业务兼容及完整快照 shape 继续由既有 resolver
和 cpup01 校验；presence 检查不另造业务规则，也不生成假 Snapshot 进行校验。
全部文件成功后才返回只读索引，读取时复制唯一可变的 Runtime target-jobs 切片。
这些拒绝行为已由下述实际文件负向测试覆盖，不能推导出生产环境能力已验证。

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

固定 `9ac465173372815ae9ad6ae53d03ffdf7773efb6` 在 Fedora 取得首 GREEN（0.110s）及
resolution/catalogue/input 完整回归（1.361s/0.204s/3.996s），均 exit0、无 SKIP。
admissionfacts 由真实集成测试实际调用，其包没有独立测试文件。测试后的两个文件
纯格式差异已由 root 审阅固定为 `d8a073ff05618011102c2e2eba482c7290f2399e`，
已随后述固定组合 SHA 一并复验。

`managed_facts_boundaries_test.go` 全部使用真实文件，变体在测试运行时按新
字节重新 pin。固定 `fcaf8efc1d2fbb68fa3590906aba2e4cfe84c9bd` 实际 RED 为 19 个必要
字段缺失被 Load 接受；另外 7 个 schema/key/evidence 缺字段场景已拒绝。
schema/key/evidence
已有拒绝、严格 JSON/UTF、真实字节改变、空白等价、重复文件 key、文件类型/64 KiB/
64 来源上限、tenant/Release 选择、取消及独立副本是回归，不能伪称全部都修复自 RED。
文件限额用 65 个各自唯一 key 的实际文件，避免被重复 key 拒绝误代来源数校验。
FIFO 等失败有测试侧时间上限，所有拒绝必须返回 nil reader 和有限错误，不回显路径。

最小 presence 修复与测试格式回传固定为 `484fa0c864a921f695bb5b8b24370facbd10debf`。
组合源码 `9c02b724ef427349b7f7d74f314de8c0e5391922` 在 Fedora 的六项边界测试全部
PASS（0.552s）；完整 resolution/catalogue/input 回归分别 2.056s/0.207s/4.343s，
均 exit0、无 SKIP，格式无差异。原 19 个 RED 修复与其余回归结果分别保留。
这是该文件模块的历史验证；后续 RPC/启动装配证据由执行卡分别记录。实际环境材料与
Governance 业务消费仍未接通，这些文件测试不证明真实 ENV 可用。
