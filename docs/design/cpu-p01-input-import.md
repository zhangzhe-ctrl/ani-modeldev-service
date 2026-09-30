# CPU-P01 受管 CSV 输入导入

范围来自 CPU04 及 v0.4 D07/O07；只处理固定 CSV，不实现浏览器上传、
multipart、任意 endpoint、连接管理或第二套输入管理平台。

## 本次持久切片

管理员导入有稳定 request_id 和 input_version_id。可信入口负责当前管理员、
目标租户及源范围授权；它们不是从普通请求 body 接受的权限。首版选择受管
存储的显式非 null VersionID，不支持仅声明 immutable_copy 的可变源。

先按 `(tenant_id, request_id)` 持久固定 input_version_id、批准的 connection /
bucket / prefix、对象 key / VersionID / 声明大小 / 预期 SHA256 和原审计信息。
同租户 input_version_id 也唯一。保存成功后状态为 VALIDATING，尚不可作为
READY 输入受理训练。相同固定申请重放返回原记录；替换任一固定内容或占用
已有 input_version_id 均冲突，不覆盖旧输入。request_id 不变时不能换对象重试。

实际 S3 GET 与 CSV 检查在事务外进行，仅读记录固定的版本。必须同时完成
实际版本、实长、SHA256、1024 行/16 特征/二分类格式检查后，内部用例才能
以相同固定申请 CAS 保存 READY 及校验事实；失败保留申请与有限错误状态。
此后正常读取仍检查当前授权，旧 actor 只供审计。没有公开 state=READY 参数，
没有直接改 SQL 的导入脚本。进程中断恢复同一固定申请，不解析新“最新版本”。

## 数据库与测试接缝

ModelDev 维护版本化 SQL/sqlc。输入记录使用 tenant_id 的显式查询和复合唯一
约束，禁用 RLS；后续输入引用必须保留租户维度。首个测试仅证明 VALIDATING
申请提交后可从新连接读取，不证明对象存在、READY、管理员权限或业务装配。
使用已有 Fedora 专属 PostgreSQL 的受限 runtime、独立随机 schema；复用
execution 的真实数据库准备方法，准备失败与行为 RED 分开报告。

当前 VerifyCSV 固定 `3910d33` 的 SDK 边界测试 PASS。输入持久化、READY CAS、
受管入口和 List/Get 授权正在逐个实施；本文合同不是已交付状态。
