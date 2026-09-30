# CPU07 持久提交预约

Phase A 已在固定提交 `b8b1bc3` 取得真实 PostgreSQL 行为 RED：现有 Admission
持久化成功后，预约 stub 返回 `PERSISTENCE_UNAVAILABLE`，未发出期望的唯一许可。
SQL 0007 及独立 sqlc 输出已固定；repository 在 `e18e6d7` 的同一行为取得
Fedora GREEN，`2a7ec3e` 全套预约与限定 race 回归通过。不包含发送循环或生产接线。

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

PG 实现在共享 execution identity 锁中与完整已存 Admission 比较，
检查 close fence 和数据库当前时间下的原 deadline，持久写入 SUBMITTING 后
成功 COMMIT 才返回 SendPermit。提交结果不明也不发许可。Get 和 Reserve
重放均不能从持久行重建发送许可；既有预约不能被当前 owner 默认值改写。
计划、Admission 和其 hash 保留各自身份；读回时重新核对 canonical bytes、
hash 和原 Admission 派生字段。完整原件或计划不一致返回 ADMISSION_CONFLICT；
缺失返回 NOT_FOUND；首次预约因 close 或 deadline 拒绝返回 PIPELINE_DISPATCH_BLOCKED。
已存在的同件预约在 close 或 deadline 后仍返回原事实，但不返回许可。

回归包括 close-first、close-only tombstone 与迟到 Admission、完整原件和 owner
配置变更拒绝、租户边界及 UUID 别名。deadline 测试通过 PostgreSQL 锁等待和
数据库时钟观测，证明等待 identity 锁期间到期后拒绝首次预约；轮询间隔不作为
排序依据。这些证据只覆盖持久预约，不证明后续 KFP HTTP 调用已获准或已完成。

## 原提交结果不明

原预约的不明结果在 `70b4957` 取得真实 PG 行为 RED；SQL 0009 和独立 sqlc
输出已固定。`MarkSubmissionUncertain` 在 `e8550d9` 取得真实 PG GREEN，
`eacb2e3` 完整 submission 回归与限定 race 通过。
它仅接受原 tenant/execution/attempt/plan hash；合法观察时间规范到 UTC、精确到
微秒且不得早于 reserved_at。首次成功提交的 SUBMISSION_UNCERTAIN/UncertainAt
不被重放刷新；关闭或原 deadline 不丢弃迟到不明观察。Get/Reserve 保留原冻结计划
且不再发许可，原 Admission 和 close 事实不变。这个方法不证明实际发送过网络
请求，也不决定关闭完成。

## 已确认创建响应的句柄

SQL 0010 增加原预约下不可变的 confirmed Run 观察，完整合同与证据见
[持久化文档的 CPU07 confirmed Run 章节](cpu-p01-persistence.md#cpu07-confirmed-run-观察的持久化)。
`RecordSubmissionConfirmed` 在共享 identity 锁下匹配原 attempt/plan，保存
Run 句柄并在同一事务进入内部 SUBMISSION_CONFIRMED，成功 commit 后才返回
回执。关闭/deadline 后原句柄仍保留，已有 UncertainAt 不丢失，迟到 Uncertain
不降级。相同 Run 不刷新首次时间，不同 Run 全部保留并返回 ConflictingRuns，
不能据此接管旧 Run 或获得训练许可。

固定 `3ede847` 的首个真实 PG 行为 RED 后，`ae2c657` 取得同一行为 GREEN；
`5ad95e4` 完整 submission/execution 回归及限定 confirmed 并发 race PASS。
Get 用同一只读 Repeatable Read 快照读取状态与全部句柄，Reserve 重放仍无
SendPermit。固定 `de3278d` 的真实 PG 延迟约束在 confirmed 自身 COMMIT 注入
精确故障，已验证空回执、无部分事实、解除后原观察可提交及重放无新许可；
完整 submission/execution/commandtest 回归 PASS，格式无差异。这些测试只
证明合成内部观察的持久化，不证明实际 KFP 响应或 Pod 身份，也不覆盖网络丢失
COMMIT 响应或服务进程中止。

## 当前不成立的能力

当前持久化切片未实现 HTTP 调用、租约、发送恢复、严格关联查询或权威 Run CAS。
现有 KFP CreateRun 仍从客户端配置读取 root，故此切片不把预约接到该客户端。
后续实际发送必须消费已持久的 frozen plan；预约先到后关闭仍须对账在途创建。
HTTP 不明不能因租约到期或查询未命中盲重发，保存句柄不证明关闭完成。

SQL 0007/0009 保留原预约和首次不明观察，0010 单独保存确认句柄；没有 lease
或 reconcile schema。原双外键要求完整租户身份及实际已持久 Admission，新增
完整 tenant/execution/attempt/plan FK 将句柄绑定原预约。close-only tombstone
不能单独授权提交。表显式 tenant 隔离并禁用 RLS；SQL 用独立 submission query
源和 sqlc 生成。ENV、真实 KFP、生产身份和 LIVE 仍未验证。
