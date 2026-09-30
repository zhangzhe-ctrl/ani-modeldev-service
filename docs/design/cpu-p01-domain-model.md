# CPU-P01 最小领域合同

来源：[执行索引](../../.scratch/cpu-p01/spec.md)；权威任务包为
`/home/chabking/workspace/ANI-doc/02-issues/modeldev/cpu-p01` 的 CPU01、CPU04–CPU10
及 `reference/v04/source/development.md` D03–D13。2026-09-30 Goal 对执行地点、
ENV 分工和验证提交顺序的覆盖关系见执行索引。本文只定义 CPU-P01。

## 权威和关联

Governance 拥有业务授权、受理、幂等、当前启用 Release 指针和可靠 CPU 投递。
ModelDev 在持久化 inbox/execution 后 ACK，拥有执行事实及唯一 Trainer adapter。
KFP 推进 prepare → train-wait → collect → publish → close；Go coordinator 仅进行
投递、关联、观察、结果核验和关闭恢复，不按数据库阶段重演流程。

Operation 与 Execution 一对一；二者分别唯一，均在 Governance 受理时生成。
重复投递必须同时匹配 tenant、operation、execution 和完整快照摘要，否则冲突。
Execution 最多一个权威 Run、一个训练创建意图、一个 Workspace 绑定和一个主 Publication。
外部资源历史保留 TrainJob、JobSet、Job、全部 Pod 的真实 UID 及 owner 关联。
资源名不代替 UID，KFP display_name 不作为幂等键。

快照包含已经存在的环境身份和 namespace UID、固定 Release/PipelineVersion、输入
版本及实际 SHA/长度、程序镜像 digest、Runtime 名称及摘要、CPU 资源、参数和截止时间。
受理后才产生的 Run/PVC/TrainJob UID 是独立 CAS 绑定事实，不回写快照或重算 spec hash。
ModelDev 不保存第二个“当前默认 Release”指针。

## 意图规范 v1

唯一维护点是本仓 `contract/cpup01`，消费仓固定本模块 commit/Go 伪版本。
共享测试向量与源码同版本；API Proto 和本合同均由 ModelDev 维护。

幂等键作用域是 `(resource_tenant_id, actor, action, idempotency_key)`。
actor 是 Governance 验证过的 `governance:user:<id>` 或
`governance:access-key:<id>`；不得从请求 body 接受 tenant/actor。
action 固定为本片 CreateExecution。先检查已有键，再解析当前 Release；权限仍按当前
主体和资源重新检查。同键同意图返回原操作和快照，同键异意图返回 IDEMPOTENCY_CONFLICT。

`intent_hash = lowercase_hex(SHA256(canonical_intent_utf8))`，无 BOM、无尾随换行。
canonical_intent 是固定字段序 JSON：schema、name、kind、preset_id、dataset_version_id、
image_version_id、general_parameters、source_execution_id；schema 恒为
`ani.modeldev.intent.v1`。不包含幂等键、可信主体或解析默认值。

- JSON 严格拒绝未知字段、重复键（含嵌套）、null、无效 UTF-8、额外尾随值。
- name 为 1–80 Unicode 码点，不作 Unicode 归一化，拒绝控制字符及首尾空白。
- UUID 接受标准带连字符形式，统一小写；空值和零 UUID 拒绝。
- kind 仅 GENERAL_TRAINING。可选 image/source UUID 缺省时省略；显式空值拒绝。
- general_parameters 缺省时省略，显式空数组保留 `[]`，不能偷偷合并存在性。
- 参数按 ASCII name 升序，重复名拒绝。类型/值均为字符串；epochs INTEGER 固定 3，
  batch_size INTEGER 固定 64，learning_rate DECIMAL 在 (0,0.1]，只接受非指数十进制。
  小数去除尾零，整数不得带前导零或加号。类型字段分别使用 INTEGER/DECIMAL。
- JSON 使用双引号、短控制转义、其余控制字符小写 `\u00xx`；不 HTML 转义 `<>&`。
  U+2028/U+2029 使用 `\u2028`/`\u2029`。其他字符原样 UTF-8。
- 缺省参数和显式默认参数具有不同 intent hash；解析后可以具有相同配置事实。
  command/args 的顺序有意义，不排序。

## 执行和结果不变量

`execution_spec_hash` 使用独立 schema `ani.modeldev.execution-spec.v1`，对完整固定
配置的规范字节计算 SHA256；不含自身 hash、运行状态、短期凭据、签名 URL、观察时间
和后生 UID。精确字段及共享测试向量随 CPU01 后续行为切片补齐；当前尚未 CODE_READY。

compute_state、delivery_state、resource_state、close_state 相互独立。
CPU 的 resource_state 恒 NOT_APPLICABLE，不是关闭证明。训练成功后发布失败保持
compute=SUCCEEDED、delivery=FAILED；Stop 不能抹掉既有计算终态。

CreateRun 前持久 SUBMITTING；网络结果不明进入 SUBMISSION_UNCERTAIN，不回到盲重发。
Begin 只有在真实受管步骤身份、Pod/Namespace/Workflow/Run 关联均已核验后 CAS 绑定。
第二 Run 不接管，不获得训练创建许可。

Stop/deadline 先持久保存意图及单调 generation，再封闭新创建，停止 Run/TrainJob，
核对在途创建与所有历史写者。停止先到保存 operation/execution 墓碑。单次 NotFound、
deletionTimestamp 或 KFP canceled 不证明 CLOSED；无法证明时 NEEDS_REVIEW。
重启只继续原观察/投递/关闭，不延长 deadline，不重新训练。关闭与删除独立。

TrainingInput 带 execution/spec、已核验准备清单、登记程序/镜像/参数、CPU 资源、真实
Workspace UID/受限子路径、输出合同和原 deadline。TrainingResult 是 WORKSPACE_ONLY
候选，只有 CHECKPOINT；collector 检查路径、文件类型、实长和 SHA。
Publication 必须记录实际上传完成证明，并由服务读取远端实际字节核验后写入。
文件摘要、manifest 摘要、归档摘要分开，manifest 不包含自身摘要。
发布失败保留唯一有效输出，不自动清理；未发布产物不允许下载。

## 验证边界

Goal 已指定测试接缝：共享意图/快照合同、真实数据库 repository、Trainer/KFP/存储
出站边界、BFF 当前授权入口。每个行为先在 Fedora 对候选 commit 取得 RED，再实现及
GREEN。模块 fake 只覆盖外部错误分支；不能证明集群身份、资源或存储通过。
正式检查包含 pinned Proto 生成稳定性、make verify、audit/SBOM 和两轴审查。
