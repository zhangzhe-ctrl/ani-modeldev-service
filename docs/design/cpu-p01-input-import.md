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

当前 VerifyCSV 包括字段长度检查的 SDK 边界测试、输入固定申请/重放/新连接读取，
已在 `3ebe0a0` 完整 Fedora `make verify` 中 PASS。输入并发和作用域回归继续。

`RecordVerifiedCSV` 是受信内部持久边界：锁定原输入后比较完整申请，
匹配校验过的同一对象版本、实长、SHA 与固定 CSV schema/行列数才从 VALIDATING
变为 READY。原申请字段永不改写；保存校验时间、schema 和行列数，并以原固定
对象记录恢复匹配的实字节证明。校验时间按 PostgreSQL 微秒精度规范化。
同一已 READY 输入的等价校验返回最早保存的证明，不刷新时间；不同申请或
证明冲突/拒绝，不能将 READY 降回校验中。首例真实 PG RED `8d1709a` → GREEN
`1e39cc0`；错误证明/原件匹配、首次证明重放、微秒 UTC 及双连接并发在
`e18b220` 整个输入模块和并发 race 中通过。

`InputImporter` 组合真实仓储与现有 `CSVVerifier`：先提交固定申请，再在事务外
读取其原版本实际字节，最后持久保存匹配证明。`d7510cb` RED → `ab65dc6` GREEN
使用真实 PG 和 HTTPS S3 SDK fixture，handler 通过另一连接确认申请先已提交，
新连接再读回 READY。坏实字节/版本/CSV 被拒绝，重放和冲突不会读取替换对象。
验证失败目前保留 VALIDATING 原申请并返回有限错误；失败状态/重试分类持久化
仍待实施，没有将这一阶段记为整个 T03 完成。

批准前缀和 key 必须位于其相对根内：`..` 与前导 `../` 不属于可用输入范围。
`5b7b2fb` 的真实 PG 与 SDK 边界负向得到 RED，最小路径校验修复 `4276d2d` 的
完整 input/execution/objectstore 模块检查通过；不能仅靠
`path.Clean(value) == value` 排除前导父路径。

管理员入口、当前授权、真实 S3 装配和 List/Get 授权尚未实现；本文合同不是
已交付状态，仓储的合成校验证明不代表 S3 或端到端业务通过。
