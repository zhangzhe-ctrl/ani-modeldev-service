# CPU07 持久提交预约

当前是 Phase A 的首个 TDD 候选：业务值和真实 PostgreSQL 接缝测试已写，
submission repository 明确返回 `PERSISTENCE_UNAVAILABLE`。尚未取得该候选的
行为 RED，也没有持久提交 schema、SQL 实现、发送循环或生产接线。

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

后续 PG 实现必须在共享 execution identity 锁中与完整已存 Admission 比较，
检查 close fence 和原 deadline，持久写入 SUBMITTING 后成功 COMMIT 才返回
SendPermit。Get 和 Reserve 重放均不能从持久行重建发送许可；既有预约不能
被当前 owner 默认值改写。计划、Admission 和其 hash 必须保留各自身份。

## 当前不成立的能力

Phase A 未实现 HTTP 调用、调用结果写回、Run 句柄、租约、恢复或权威 Run CAS。
现有 KFP CreateRun 仍从客户端配置读取 root，故此候选不把预约接到该客户端。
后续独立行为必须证明：close 先到拒绝新许可；预约先到后关闭不能漏掉在途
创建；HTTP 不明只能保留 UNCERTAIN，不能因租约到期或查询未命中盲重发；
迟到句柄按原 attempt/计划匹配保存而不丢失，Confirmed 不被迟到不明降级。

SQL 0007 留待行为 RED 后实现，只建设真实首切片需要的关系，不预建 handles、
lease 或 reconcile schema。所有新增表显式 tenant 隔离并禁用 RLS；SQL 用独立
submission query 源和 sqlc 生成。ENV、真实 KFP、生产身份和 LIVE 仍未验证。
