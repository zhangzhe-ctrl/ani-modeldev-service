# CPU04 持久受理第一切片

来源：原 CPU04、当前 Goal §5/7/9 和本仓
[领域合同](cpu-p01-domain-model.md)、[快照规范](cpu-p01-snapshot.md)。
执行来源与固定审查基线见 `.scratch/cpu-p01/`。固定候选 `0266313` 已在 Fedora
取得有效 RED：真实 PostgreSQL、受限角色和隔离迁移 preflight PASS 后，Accept
返回 NOT_IMPLEMENTED / exit 1。当前为首个 Accept/Get 实现候选，尚待固定提交
GREEN；未接业务装配，不标整个 CPU04 CODE_READY。

## 持久事实与最小边界

`biz.Admission` 保存来自可信 Governance 调用的 tenant、actor、operation、
execution、原意图/intent_hash、完整固定 Snapshot/spec_hash、accepted_at。
请求 body 不能赋予 tenant/actor 权威。Snapshot 本体无执行身份，必须与该
envelope 一起保存，不能仅凭相同 spec_hash 接管其他执行。

首个 `modeldev_executions` 行同时作为不可变执行受理和持久 inbox。无需单独
ACK 表；只有整行提交后 `Accept` 才能返回成功。`Get(tenant_id, execution_id)`
经显式租户过滤读取同一事实；没有查询当前 Release、重新解析默认值或在内存
中代替数据库的路径。本阶段只有 Accept/Get 两个 port 方法，不预建其他 adapter。

`migrations/0001_execution.up.sql` 是 schema 维护点。
`internal/data/execution/queries.sql` 是应用 SQL 维护点，通过 `sqlc.yaml`
生成 pgx/v5 查询到 `internal/data/execution/sqlc`。生成只在 Fedora 执行，
生成源回传审阅后提交，禁止手改生成物或在生产 Go 中拼接 SQL。

每行带 `tenant_id`；主键 `(tenant_id, execution_id)`，唯一键
`(tenant_id, operation_id)`，使租户内 operation/execution 一对一。
当前只建一张表，没有跨表引用；后续表必须带 tenant_id 并使用保租户复合外键。
RLS 显式禁用，隔离依赖 tenant-scoped SQL 和真正受限 runtime role，不能用
superuser/BYPASSRLS 角色证明隔离。

intent 和 snapshot 保存规范 UTF-8 字节及各自摘要，避免 JSONB 重排或数字
转换改变合同字节。实现阶段必须重新规范化/计算两个摘要并比对来件，核对
intent 的 preset/input/显式 image 等选择与 Snapshot，以及原截止时间与
accepted_at 的关系；不能直接信任摘要字符串。原意图的缺省与显式值保持
存在性，不回填已解析默认值。`accepted_at` 使用 PostgreSQL timestamptz，规范化
为 UTC，要求非零、年份 1–9999、微秒精度且早于原 deadline；亚微秒值拒绝，
不默默截断或四舍五入。Snapshot deadline 仍以规范字节保存完整纳秒精度。
验证不比较当前时钟，历史投递不能因此改写原 deadline。拒绝行为须在后续真实
数据库负向切片验收，首个正向向量本身不证明全部拒绝规则。

业务错误必须稳定，不暴露 SQL、DSN、另一租户记录或原始驱动异常。当前实现
用 INVALID_ARGUMENT、NOT_FOUND、PERSISTENCE_UNAVAILABLE 表示本切片错误；
冲突投递的稳定拒绝和重放语义由下一条真实 RED/GREEN 切片补齐。数据库读取
重新规范化并核对字节/摘要，不能把损坏的持久事实当作有效受理。
停止墓碑、关闭意图、资源历史、输入/目录导入和 publication 尚未实现。

## 第一条真实 PostgreSQL 测试

`TestAcceptPersistsCompleteAdmissionAcrossNewConnections` 通过真实 repository
接口测试一个合法可信受理：Accept 返回后关闭原 pool，用全新 pool/repository
读取同 tenant/execution，核对完整身份、operation、两个摘要、accepted_at 和
规范意图/快照内容。测试不直接 SELECT 业务表断言实现，不以进程内 map 替代持久化。
共享 Snapshot 来自公共 `contract/cpup01/conformance.SnapshotV1()`，仍是合成
合同 fixture，不是已启用 Release/ENV 身份。

Fedora 执行者从受保护文件载入 `CPU_P01_TEST_DATABASE_URL` 和
`CPU_P01_TEST_DATABASE_ADMIN_URL`，不得打印/回传其值。测试先验证两角色指向
同一测试实例/库，runtime 为 NOSUPERUSER/NOBYPASSRLS，创建本次独占 schema，
用迁移角色应用版本化 schema，只给 runtime INSERT/SELECT。直接 SQL 仅在
这种数据库准备、角色核查、权限和本测试精确 schema 清理中使用。

依赖/权限/迁移失败带 `CPU04_DB_PREFLIGHT` 和 `behavior NOT_RUN`，不能算作
产品 RED。只有 preflight 成功后，合法受理被 NOT_IMPLEMENTED 拒绝，才是本
行为的预期 RED。测试不跳过缺失数据库，也不输出原始连接异常。

允许的测试实例仅为任务独占 Fedora PostgreSQL 17.11 容器；它有资源上限，
使用受限 loopback 通道，不能指向共享业务数据库。当前已提供的实例及镜像
身份以本轮 baseline 为准，测试不内置凭据、端口或生产默认值。

重复投递、并发、同键异参、跨租户负向、停止墓碑和恢复门闩是后续独立
RED/GREEN 切片；本条正向持久化不能代替这些证据。真实 PG 模块结果亦不能
证明目标集群、BFF、KFP、Trainer、S3 或 L1–L4 验收通过。新 pool 读取证明数据库
已提交的持久事实；进程中途终止与重启恢复属于 CPU10 后续验证，不在这里冒称完成。
