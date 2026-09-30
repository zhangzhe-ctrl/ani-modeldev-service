# CPU07 持久提交预约

Phase A 已在固定提交 `b8b1bc3` 取得真实 PostgreSQL 行为 RED：现有 Admission
持久化成功后，预约 stub 返回 `PERSISTENCE_UNAVAILABLE`，未发出期望的唯一许可。
SQL 0007 及独立 sqlc 输出已固定；当前 repository 实现候选尚待固定提交后的
Fedora GREEN，不包含发送循环或生产接线。

首个行为限定为：先经现有 execution repository 真正持久受理 Admission；
两个独立连接竞争同一预约，恰好一次返回 SendPermit；返回时第三个连接必须
已能读取完整 SUBMITTING；全部写连接重建后，重放返回原预约且不再发许可。
预检、迁移、缺表或编译错误不算行为 RED。

## 内部冻结计划

`PipelineDispatchRequest` 包含原 Admission 和显式 owner 配置引用。
引用用名称及 SHA-256 版本标识，另带确切 PipelineRoot，不含短期凭据。
内部计划通过现有 `Admission.CanonicalPayloads` 验证和规范化 Admission，
从中导出 tenant/execution/operation、原 spec hash、完整 environment 身份、
Experiment、managed SA、Pipeline/Version、display name 和原 deadline。
提交端不能传另一份 SA/Experiment/Pipeline 字段覆盖 Admission。

独立计划 canonical/hash 覆盖这些派生字段及 owner 引用、版本和 PipelineRoot。
它不是新公共 snapshot，不改变既有 canonical bytes 或 SpecHash；原 SpecHash
并不覆盖 PipelineRoot。配置引用和 hash 的形状合法，也不证明 owner 配置存在、
不可变或获授权。当前 owner 资料仅为模块 fixture，真实绑定仍未交接。

PG 实现候选在共享 execution identity 锁中与完整已存 Admission 比较，
检查 close fence 和数据库当前时间下的原 deadline，持久写入 SUBMITTING 后
成功 COMMIT 才返回 SendPermit。提交结果不明也不发许可。Get 和 Reserve
重放均不能从持久行重建发送许可；既有预约不能被当前 owner 默认值改写。
计划、Admission 和其 hash 保留各自身份；读回时重新核对 canonical bytes、
hash 和原 Admission 派生字段。完整原件或计划不一致返回 ADMISSION_CONFLICT；
缺失返回 NOT_FOUND；首次预约因 close 或 deadline 拒绝返回 PIPELINE_DISPATCH_BLOCKED。
已存在的同件预约在 close 或 deadline 后仍返回原事实，但不返回许可。

## 当前不成立的能力

Phase A 未实现 HTTP 调用、调用结果写回、Run 句柄、租约、恢复或权威 Run CAS。
现有 KFP CreateRun 仍从客户端配置读取 root，故此候选不把预约接到该客户端。
后续独立行为必须证明：close 先到拒绝新许可；预约先到后关闭不能漏掉在途
创建；HTTP 不明只能保留 UNCERTAIN，不能因租约到期或查询未命中盲重发；
迟到句柄按原 attempt/计划匹配保存而不丢失，Confirmed 不被迟到不明降级。

SQL 0007 只建设本切片的不可变预约关系，不预建 handles、lease 或 reconcile
schema。双外键要求完整租户身份及实际已持久 Admission，close-only tombstone
不能单独授权提交。新增表显式 tenant 隔离并禁用 RLS；SQL 用独立 submission
query 源和 sqlc 生成。ENV、真实 KFP、生产身份和 LIVE 仍未验证。
