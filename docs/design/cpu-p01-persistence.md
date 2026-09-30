# CPU04 持久受理第一切片

来源：原 CPU04、当前 Goal §5/7/9 和本仓
[领域合同](cpu-p01-domain-model.md)、[快照规范](cpu-p01-snapshot.md)。
执行来源与固定审查基线见 `.scratch/cpu-p01/`。固定候选 `0266313` 已在 Fedora
取得有效 RED：真实 PostgreSQL、受限角色和隔离迁移 preflight PASS 后，Accept
返回 NOT_IMPLEMENTED / exit 1。首个正向持久受理在固定 `bc46d60` 取得 GREEN。
重复投递测试在固定 `e716e4e` 取得有效 RED：preflight PASS 后重复 Accept 返回
PERSISTENCE_UNAVAILABLE / exit 1。固定 `7e2a045` 的两项 repository 测试随后
GREEN / exit 0（含受限 PG preflight），gofmt 无差异。冲突/隔离候选 `7f0fb10`
已取得真实 RED / exit 1：同租户及并发异参缺少稳定冲突错误，跨租户复用 ID
错误地产生新记录。六 pool 原命令竞争、跨租户隐藏 Get、16 类非法受理以及
原正向/重投均 PASS。修复在固定 `cd393c7` 的 repository 全量、真实 PG 并发
race 和完整 make verify 均 GREEN / exit 0；该证据只覆盖该固定源码。
UUID 查询严格性修复在 `af2407b` 定向与 repository 全量 GREEN；首墓碑测试在
`90692b0` 实际 PG preflight PASS 后取得 NOT_IMPLEMENTED / exit 1 的预期 RED。
首墓碑 SQL 在固定 `cf4307a` 经 Fedora pinned sqlc 1.31.1 生成成功；固定
`c16b3a7` 的首墓碑、全部 repository 和真实 PG 并发 race 均 GREEN / exit 0。
未接业务装配，不标整个 CPU04 CODE_READY。

## 持久事实与最小边界

`biz.Admission` 保存来自可信 Governance 调用的 tenant、actor、operation、
execution、原意图/intent_hash、完整固定 Snapshot/spec_hash、accepted_at。
请求 body 不能赋予 tenant/actor 权威。Snapshot 本体无执行身份，必须与该
envelope 一起保存，不能仅凭相同 spec_hash 接管其他执行。

首个 `modeldev_executions` 行同时作为不可变执行受理和持久 inbox。无需单独
ACK 表；只有整行提交后 `Accept` 才能返回成功。`Get(tenant_id, execution_id)`
经显式租户过滤读取同一事实；没有查询当前 Release、重新解析默认值或在内存
中代替数据库的路径。受理边界使用 Accept/Get；首墓碑另外使用文末说明的
ApplyCloseIntent/GetCloseIntent，不预建其他 adapter。

`migrations/*.up.sql` 是按版本顺序应用的 schema 维护点，保留既有迁移历史。
`internal/data/execution/queries.sql` 是应用 SQL 维护点，通过 `sqlc.yaml`
生成 pgx/v5 查询到 `internal/data/execution/sqlc`。生成只在 Fedora 执行，
生成源回传审阅后提交，禁止手改生成物或在生产 Go 中拼接 SQL。

每行带 `tenant_id`；0001 的主键 `(tenant_id, execution_id)`、唯一键
`(tenant_id, operation_id)` 保留。0002 增加 operation_id、execution_id 各自的
全局唯一约束，使原 operation/execution 身份不能在另一租户再次受理。
0003 引入共享 `modeldev_execution_identities`：tenant/execution 主键，operation
和 execution 各自全局唯一，冻结 tenant/operation/execution/spec hash 关联。
已有受理从 0001 表回填，受理行和 close intent 都用包含 tenant 与完整身份的复合
外键引用它。每个写入在同一事务内保留/锁定该身份，不能跨表另占同一个 ID。
每张关系均携带 tenant_id；所有公开读取仍使用显式 tenant 过滤。
RLS 显式禁用，隔离依赖 tenant-scoped SQL 和真正受限 runtime role，不能用
superuser/BYPASSRLS 角色证明隔离。

测试在独占 schema 中顺序应用全部版本化 up 迁移；sqlc 配置同样绑定 0001 至
0003，不能只让测试或生成器看到其中一部分。如果已有数据违反新增全局约束，
迁移必须失败并保留现场；禁止删除或改写不可变受理事实来强行迁移。

`biz.Admission.CanonicalPayloads` 委托公共 `cpup01.AdmissionEnvelope` 的唯一
合同实现，并将合同拒绝映射到稳定领域错误，避免消费者重复维护一致性规则。
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
用 INVALID_ARGUMENT、NOT_FOUND、ADMISSION_CONFLICT、PERSISTENCE_UNAVAILABLE
表示本切片错误。唯一冲突后当前 tenant/execution 不存在或完整事实不一致，
返回同一 ADMISSION_CONFLICT，不能越租户读取赢家或返回其身份。其他数据库
错误仍为 PERSISTENCE_UNAVAILABLE，不伪装成业务冲突。数据库读取
重新规范化并核对字节/摘要，不能把损坏的持久事实当作有效受理。
首墓碑实现范围见下文；资源历史、输入/目录导入和 publication 尚未实现。

严格重复回放在共享身份锁下进行。INSERT 使用 ON CONFLICT DO NOTHING，
未插入时按原 tenant/execution 读取原行。比较 tenant、operation、execution、actor、
accepted_at、两个摘要及完整 canonical intent/snapshot 字节，全部相同才返回
原受理事实。唯一竞争后使用新查询语句读取赢家，不盲目重建、更新
原记录或退回内存回执。数据库异常或不一致仍返回失败，不把它们当作重复成功。
actor 保存可信上下文字符串，例如 `governance:user:42`，不要求主体 ID 为 UUID。

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
用迁移角色应用版本化 schema，只给 runtime 各业务关系 INSERT/SELECT 和
identity anchor 的 close_generation 列 UPDATE（用于行锁与 owner 代际更新），
不给 tenant/operation/execution/spec 身份列 UPDATE。直接 SQL 仅在
这种数据库准备、角色核查、权限和本测试精确 schema 清理中使用。

依赖/权限/迁移失败带 `CPU04_DB_PREFLIGHT` 和 `behavior NOT_RUN`，不能算作
产品 RED。只有 preflight 成功后，合法受理被 NOT_IMPLEMENTED 拒绝，才是本
行为的预期 RED。测试不跳过缺失数据库，也不输出原始连接异常。

允许的测试实例仅为任务独占 Fedora PostgreSQL 17.11 容器；它有资源上限，
使用受限 loopback 通道，不能指向共享业务数据库。当前已提供的实例及镜像
身份以本轮 baseline 为准，测试不内置凭据、端口或生产默认值。

`TestAcceptDuplicateAfterReconnectReturnsOriginalAdmission` 在首次 Accept 后
关闭 pool，经新 pool 重投同一命令，核对回放响应与 Get 均保持完整原受理。
此测试已取得上述 RED/GREEN。

下一候选通过真实 repository 接口覆盖完整事实冲突、跨租户复用全局身份、
六独立 pool 同命令竞争、异参唯一赢家、跨租户隐藏读取及非法受理不占 inbox。
固定 `7f0fb10` 的失败与 `cd393c7` 的全量 GREEN 分别保留；实现采用稳定冲突
错误和 0002 全局约束。现有校验直接通过的用例记作回归
PASS，不制造失败。测试只通过 Accept/Get 断言业务事实，不直接查业务表。

并发、同键异参、跨租户负向已在上述固定 repository 范围验证；停止墓碑和
恢复门闩是后续独立 RED/GREEN 切片。真实 PG 模块结果不能
证明目标集群、BFF、KFP、Trainer、S3 或 L1–L4 验收通过。新 pool 读取证明数据库
已提交的持久事实；进程中途终止与重启恢复属于 CPU10 后续验证，不在这里冒称完成。

## 首个 USER_STOP 墓碑合同与测试候选

`CloseIntent.SourceGeneration` 是 Governance 来件的去重序号，对应内部 RPC 的
来源 intent generation；`CloseRecord.Generation` 是 ModelDev 独占分配、在同一
execution 内跨所有关闭来源共享的持久围栏代际。两者不能混用：首条来源序号
41 的关闭意图产生 owner fence 1，不把 41 直接写成 owner 代际。各新关闭意图
由 ModelDev 事务至多递增一次，原来件重放不递增。USER_STOP、受管步骤和
deadline 将共用这一权威；本切片只做来自可信 Governance 的首个 USER_STOP。

该分工落实原 CPU10 的持久 stop/单调 generation 要求，没有改变业务范围。
当前 actor 授权仍在受信入口核对，保存的 RequestedActor 只作审计事实。
领域层只增加本行为实际需要的 ApplyCloseIntent/GetCloseIntent 两个方法；不预建
Ensure/Create、worker 或跨系统关闭 adapter。

`TestUserStopBeforeAdmissionPersistsClosingTombstoneAcrossNewConnections` 使用
真实受限 PG：尚未有 Admission，先提交含 tenant/operation/execution/spec hash、
SourceGeneration、reason、requested_at/actor 的关闭意图；关闭原 pool 后从新
repository 读取同一墓碑，要求原事实、owner generation 1 和 CLOSING。查询
Admission 仍为 NOT_FOUND，不创造伪快照，不将停止受理冒充 CLOSED。

首墓碑 RED 已保存。当前实现候选在 0003 的共享身份锁下增加 owner 代际，并与
USER_STOP receipt 同事务提交；两个序号用 numeric(20,0) 精确覆盖 uint64，失败
事务连同计数一起回滚。来源固定为 GOVERNANCE，已验证首条关闭意图与新连接
读取；当前重放候选另见下文，不预先接入 Step/deadline。

共享 anchor 已随本切片建立，因此原 Accept 全部行为必须回归；matching late
Admission 的关闭状态传播及真实创建许可消费仍需要后续真实并发 RED/GREEN。
此首墓碑切片绝不宣称迟到 Accept 或真实资源创建已经闭锁，也不证明
KFP/TrainJob 已终止或无活跃写者。重复关闭的成功回执/来源代际冲突同样留待
下一独立行为，不能用首次持久化证据代替。

下一条 `TestUserStopDuplicateAfterReconnectReturnsOriginalFenceAndFacts` 只增加
完整同件 USER_STOP 重放行为：新连接重投 source generation 41，回执与读取必须
保持完整原事实及 owner generation 1，不能再次分配代际或产生 Admission。
该测试在固定 `f8c88ca` 取得真实 PG RED / exit 1：preflight PASS 后重放返回
PERSISTENCE_UNAVAILABLE。候选实现先持有共享身份锁，按 tenant/execution 和
GOVERNANCE 来源序号查原 receipt；完整身份、spec、原因和审计事实一致才回放，
跳过 owner 代际递增。固定 `70da54b` 格式无差异，全部 repository 和真实 PG
受理并发 race 均 GREEN / exit 0。

下一组 close 不变量候选通过公开 repository 接口覆盖：同来源序号的审计/身份
冲突及失败不消费 owner 代际，跨租户读取隐藏和 UUID 拒绝，六 pool 同件竞争、
异参唯一赢家，以及后续来源受理后旧来源重放不覆盖最新 owner 围栏。
这些场景等待固定候选验证；已有实现正确的场景如实记回归 PASS，不制造 RED。
迟到 Admission 携带关闭事实和创建许可消费仍是后续独立行为。
