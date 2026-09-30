# CPU05 不可变受理候选解析

来源：原 CPU01/CPU04/CPU05、v0.4 development D03/D05，以及当前 Goal 第 5、7、9 节。
任务来源身份及固定执行证据保存于 `.scratch/cpu-p01/runs/20260930-01/`。
本文件承接[不可变目录](cpu-p01-catalogue.md)与[执行快照](cpu-p01-snapshot.md)合同。

## 模块职责

`biz.AdmissionResolver.Resolve(ctx, request, facts)` 组合明确选择的不可变 Release、
本租户已持久 READY 的输入和显式受管事实，返回 `Snapshot` 与 `SpecHash`。
返回值是独立的候选数据；解析不写 acceptance、execution 或 input，不分配 operation、
execution、Run、TrainJob、PVC 身份，不确认当前启用指针，不发出受理回执。

Governance 继续拥有用户业务受理、幂等记录和租户+预设的当前启用指针。ModelDev
目录没有第二个默认指针，新增 Release 文件不会改变一次明确的旧选择。

## 输入与字段来源

`AdmissionResolutionRequest` 含可信 `TenantID`、原始 `Intent`、Governance 选定的
`ReleaseID/ReleaseDigest/BindingGeneration` 与固定 `AcceptedAt`。它不是公共用户 JSON，
tenant 不能由用户 body 覆盖。resolver 不检查新请求的当前授权；调用方必须先完成。

| 快照事实 | 唯一读取或输入来源 |
| --- | --- |
| Release、Preset、Pipeline ID/Version/IR、Runtime、镜像、command、参数默认值、资源、workspace、输出合同、总时限 | 固定 ID/digest 对应的规范 Release 文件 |
| InputVersion ID、版本化对象、字节数/SHA256、CSV schema/行数/维度 | 同 tenant+input ID 的 PG READY 记录及持久核验证明 |
| Namespace UID、集群/连接/experiment 引用、控制/步骤/训练/verifier 身份、ENV binding 身份及摘要 | 显式 `TenantAdmissionFacts.Environment` |
| 输入批准存储范围、发布存储范围/凭据引用、匹配的 Runtime/workspace、绝对输入文件/输出目录 | 显式 `TenantAdmissionFacts`；不能从相对 subpath 猜出绝对路径 |
| accepted binding generation 与 deadline 起点 | Governance 给定的选择代际与 AcceptedAt |

facts 必须绑定请求的同一 tenant。给定 Runtime 的 name/kind/API group/content SHA256/
target jobs 集合须与 Release 相符，workspace 合同须相同。输入对象必须位于批准
connection/bucket/prefix 下；前缀按完整路径分段判断。两个容器路径必须是干净的绝对
路径，不能重合或互相包含，不接受 traversal、反斜线、通配符或控制字符。

这些检查只验证给定事实的关联与形状。`TenantAdmissionFacts` 没有 VERIFIED 布尔值，
也不会因为 UUID、digest 或引用字符串合法就证明资源存在、身份有权、挂载可用或
环境已验收。真实来源 owner 必须证明上述 tenant 关联、配置版本、内容摘要、权限和
挂载关系；本片没有提供该生产 owner。测试中的合成事实不允许成为生产默认配置。

## 规范化与固定选择

Intent 使用 `cpup01.CanonicalIntent` 的唯一参数语法，resolver 在副本上处理。缺省
参数、显式空数组、显式默认参数和显式镜像的原 intent presence 不被改写；不同
intent hash 可以在解析默认值后得到相同 spec hash。显式参数只覆盖 Release 的
对应登记项，最终固定完整参数集合；未知、重复、类型或范围错误拒绝。显式镜像必须
与选定 Release 的登记镜像 ID 相符。`SourceExecutionID` 不用于读取旧配置作为默认。

argv 仅按 Release 的有限 source token 整项映射输入路径、输出路径、固定输入 SHA256/
字节数与规范化学习率，顺序保持不变。不执行 shell，不引入任意模板解释器。

`AcceptedAt` 必须非零、UTC 等价转换后处于支持年份内，且精确到微秒，不静默截断。
deadline 为这个固定 instant 加 Release 的总时限；越界拒绝，不读取本机当前时间或
为重试重新计时。当前 binding generation 原值进入快照；resolver 不判断它仍然当前。

最终调用共享 `Snapshot.Canonical()` 校验与规范化，再对完整字节计算小写 SHA256。
UUID 规范大小写、集合排序与参数数值处理仍由共享合同维护。返回 Snapshot 从规范
字节解码到新的零值对象，消费者修改其切片或指针不会污染读件、facts 或后续解析。

## 依赖与错误

两个窄读取 port 分别读取固定 Release 和 tenant-scoped InputVersion。现有 input
repository 直接实现输入读取；catalogue.Reader 只持有受信目录，复用严格字节/摘要/
身份/文件安全读取。目录路径不进入请求，biz 不导入文件系统 adapter 或 PG 驱动。

非法形状返回 `INVALID_ARGUMENT`；Release 不存在、内容不匹配或与用户选择不兼容返回
`NO_COMPATIBLE_RELEASE`；输入不存在保留 `INPUT_VERSION_NOT_FOUND`，非 READY 返回
`INPUT_NOT_READY`；不兼容 tenant/Runtime/workspace/输入范围/路径返回
`ENVIRONMENT_NOT_READY`。最终快照缺失或非法字段仍按共享形状校验返回
`INVALID_ARGUMENT`。依赖不可用返回有限 `PERSISTENCE_UNAVAILABLE`，不回显目录路径、
连接或原始系统错误。取消/超时保留 context 原因，所有失败均返回零值候选。
外部 RPC 状态映射不在此内部模块冻结。

## 真实验证范围与未接项

`internal/data/resolutiontest` 通过同一个 Resolve interface 使用真实目录导入/读取、
真实 PostgreSQL 的 restricted runtime role 和隔离版本化 schema。READY 在写入后
经新连接恢复；其中字节 proof 和 ENV facts 是明确的 module fixture，不代表实际 S3
核验或环境就绪。测试保留手写完整 Snapshot、argv、deadline 预期，并使用既有规范
字节独立计算 SHA256。覆盖原 intent presence、独立返回值、双租户同 ID、未 READY、
旧 Release 选择、参数与 facts 兼容、时间边界、取消及只读副作用约束。

首 stub 在 `19a3b53` 取得有效 RED；`5b0a710` 的首行为及 resolution/catalogue/input
模块通过；`b67befa` 的 tenant/READY 回归通过。`4c51554` 的完整边界组仅在真实 Release
读取后取消时失败：PG adapter 遮蔽取消为 PERSISTENCE_UNAVAILABLE；其余断言通过。
候选修复在每次依赖返回后优先检查 context，保持错误原因；修复后的固定 SHA GREEN、
最终完整 gates 和独立两轴审查尚待记录，不能用上述历史 PASS 替代。

当前没有生产 managed facts 来源，没有 Resolve RPC，也没有 Governance/BFF 调用
装配。实际接通还需：每次当前授权 → FindAccepted 查原键 → 原键未命中才读当前启用
绑定 → 受管 facts 来源与最窄 Gov-only 解析 RPC → Governance 在持久受理事务复核
generation/Release，并固定真实 command IDs。原键重放也必须先完成本次当前授权；
旧 actor 仅是审计事实。这个调用顺序尚未由本片实现，候选解析 PASS 不等于 CPU05
业务受理通过，更不等于 LIVE 或 AC02–AC04 已验收。
