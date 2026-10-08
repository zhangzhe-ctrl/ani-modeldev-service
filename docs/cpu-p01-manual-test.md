# CPU-P01 手动复部署与业务测试（.10～.12）

日期：2026-10-08。目标：把 ModelDev、Governance BFF 和执行证据 webhook 部署到 `ani-system`，由普通用户完成真实 CPU 训练、日志查询、产物下载与独立模型加载。训练任务使用 `ani-kfp-manual-a`；`ani-kfp-manual-b` 用于验证租户隔离，并保留后续扩展的环境基础。

这是**现有 ANI lab 的复部署操作手册**：复用集群里的 KFP、Trainer、JobSet、Runtime、Ceph、PostgreSQL、Redis、RustFS 和固定镜像，不是新集群的一键安装器。环境安装的长期入口仍应由 installer 管理。本次操作不把 RustFS、数据库或 Kubeflow 控制器迁移到 `ani-system`。

六个旧测试 namespace 删除后，旧训练工作卷和 Pod 内证据不可再查；数据库和 S3 上的输入、已发布产物、KFP 历史记录保留。历史 Execution 的 namespace UID 不修改，旧环境的验收记录不能作为新环境通过的证明。

## 1. 本次材料和运行边界

所有集群命令、Python、依赖安装和测试都在 **Fedora** 执行。不要在本机执行编译或集群操作。

在本机终端执行：

```bash
ssh fedora
```

进入 Fedora 后，设置一次变量；后面命令都在此终端执行：

```bash
set -euo pipefail
umask 077
export CPU_P01_ROOT=/home/chabking/workspace/cpu-p01-20260930-01
export CPU_P01_KIT="$CPU_P01_ROOT/manual-ani-system-20261008"
export CPU_P01_PRIVATE="$CPU_P01_KIT/.private"
function k() { python3 "$CPU_P01_KIT/manual.py" kubectl "$@"; }
function t() { python3 "$CPU_P01_KIT/test.py" "$@"; }
test -s "$CPU_P01_PRIVATE/original-resources.json"
test -s "$CPU_P01_PRIVATE/catalogue-files.json"
test -s "$CPU_P01_KIT/general-cpu.yaml"
```

保护目录 `.private/` 包含原 Secret、数据库备份、登录 token 和私有部署文件，保持目录 0700、文件 0600；不要提交到 Git，也不要在终端打印完整文件。脚本源码在仓库 `scripts/cpu-p01-manual/`；无凭据的 Pipeline IR 已保存在部署包中。

脚本子命令的副作用：

| 命令 | 做什么 |
| --- | --- |
| `manual.py prepare` | 本地生成私有 foundation、恢复 Job、写权探针文件；不写集群 |
| `manual.py pipeline` | 使用新控制 SA 的 KFP token，创建新 namespace 下的 Pipeline、Version 和 Experiment |
| `manual.py render` | 只读实际 UID、探针、RBAC 和 Runtime；生成新绑定、四种 Release、应用及 webhook 文件 |
| `k ...` | 在固定 lab 执行你明确输入的 kubectl 命令，先检查 kube-system UID |
| `manual.py forward` | 开启最长约 10 分钟的前台临时连接；Ctrl-C 关闭 |
| `test.py ...` | 普通密码登录、真实 BFF 调用，或创建明确命名的管理/验证 Job |

本次尚未替你执行新部署和业务测试，以下测试初始状态均为 **NOT_RUN**。`Ready` 只证明服务可启动，业务成功必须走到第 6 节的独立加载。

## 2. 检查保留的基础环境

```bash
k get nodes -o wide
k get namespace kubeflow kubeflow-system ani-platform
k get pods -n kubeflow
k get pods -n kubeflow-system
k get pods -n ani-platform
k get storageclass ani-cephfs ani-block
k get clustertrainingruntime ani-modeldev-cpu-uid10001-exit-v1-1eb08af8 -o yaml
k get service ani-kfp-entry -n kubeflow
k get service ani-rustfs-svc -n ani-platform
k get namespace ani-system --ignore-not-found
```

预期三节点 Ready、共享控制器正常、两个 StorageClass 存在、UID 10001 Runtime 存在。首次部署时 `ani-system` 应不存在；如果已经存在且由其他部署管理，先核对它的用途，不能覆盖。本手册使用 `ani.io/managed-by=ani-manual-20261008` 作为此部署包的归属标记。

镜像不重新编译，复用固定 digest，`imagePullPolicy: Never` 的镜像必须在所有可调度节点上存在。发生 `ErrImageNeverPull` 时先从保留的离线材料导入**缺少的具体镜像**，不要重新构建全部镜像。

数据库原有 schema 和业务数据保留；这里不执行初始 migration、不恢复 dump、不重置租户。RustFS 用户、授权范围和 bucket 也保留，仍按业务服务分配身份。bucket 名包含旧 namespace 字样并不表示 Kubernetes namespace 删除会删除这个 bucket，因此本次不改 bucket 或已有对象路径。

## 3. 创建新的基础环境和实际写权探针

```bash
python3 "$CPU_P01_KIT/manual.py" prepare
k apply -f - < "$CPU_P01_PRIVATE/foundation.json"
k apply -f - < "$CPU_P01_PRIVATE/catalogue-restore.json"
k apply -f - < "$CPU_P01_PRIVATE/workspace-probes.json"
k wait -n ani-system --for=condition=complete job/modeldev-catalogue-restore --timeout=180s
k wait -n ani-kfp-manual-a --for=condition=complete job/manual-workspace-probe --timeout=180s
k wait -n ani-kfp-manual-b --for=condition=complete job/manual-workspace-probe --timeout=180s
k logs -n ani-system job/modeldev-catalogue-restore
k logs -n ani-kfp-manual-a job/manual-workspace-probe
k logs -n ani-kfp-manual-b job/manual-workspace-probe
```

预期 catalogue 恢复 32 个历史文件；两个探针均输出 `uid=10001`、`gid=10001`、`atomic_write_read=true`、`result=PASS`。这是实际 CephFS 写入、重命名、读回，不能用 PVC `Bound` 或 fsGroup 配置代替。

遇到失败：

```bash
k get pods,pvc,resourcequota -n ani-system
k get pods,pvc,resourcequota -n ani-kfp-manual-a
k get events -n ani-kfp-manual-a --sort-by=.lastTimestamp
```

不要删除失败 Job 后盲目重跑；先确认镜像、配额、挂载和退出码。修复确定的原因后再处理这个具体探针。

## 4. 创建 KFP 业务对象，绑定实际新 UID

Pipeline IR 不重新编译；它引用租户 namespace 内 `modeldev-step-owner` ConfigMap。新 namespace 下必须创建自己的 Pipeline、Version 和 Experiment，随后重新生成 Release，不能复用旧 Experiment ID 或旧 namespace UID。

建立独立 Python 环境，固定 KFP SDK 版本（首次执行）：

```bash
python3 -m venv --system-site-packages "$CPU_P01_KIT/sdk"
"$CPU_P01_KIT/sdk/bin/python" -m pip install 'kfp==2.16.0'
"$CPU_P01_KIT/sdk/bin/python" "$CPU_P01_KIT/manual.py" pipeline
python3 "$CPU_P01_KIT/manual.py" render
k apply -f - < "$CPU_P01_PRIVATE/release-seed.json"
k wait -n ani-system --for=condition=complete job/modeldev-manual-releases --timeout=180s
k logs -n ani-system job/modeldev-manual-releases
```

预期 KFP receipt 为 `CONFIRMED`，新环境 receipt 保存三个实际 namespace UID，Runtime 未漂移，RBAC 验证通过。新增 Release 的四个文件写入新 catalogue PVC，历史 Release 字节不修改。

`pipeline` 任何一步中断都保留阶段文件 `.private/pipeline-receipt.json`。先按记录的 ID 查询 KFP，核实服务器是否已创建；不要删账本重新上传。`render` 生成随机新 Release ID，首次成功后保存输出，不重复执行来改变已经导入的版本。

## 5. 启动应用，再启用 webhook

```bash
k apply -f - < "$CPU_P01_PRIVATE/application.json"
k rollout status -n ani-system deployment/ani-modeldev --timeout=180s
k rollout status -n ani-system deployment/ani-governance --timeout=180s
k rollout status -n ani-system deployment/ani-modeldev-exit-retention --timeout=180s
k get endpointslice -n ani-system -l kubernetes.io/service-name=ani-modeldev-exit-retention
k apply -f - < "$CPU_P01_PRIVATE/webhook.json"
k get mutatingwebhookconfiguration ani-modeldev-exit-retention
k get pods,services,pvc -n ani-system
```

先等 webhook 后端 Ready 且有 Endpoint，再启用 `failurePolicy: Fail` 的 admission 配置。新 webhook 证书的 SAN 指向 `ani-modeldev-exit-retention.ani-system.svc`；ModelDev/Governance 使用原有固定 mTLS 服务身份，地址已改到 `ani-system`。

在**另一个 Fedora 终端**运行连接，保持前台：

```bash
python3 /home/chabking/workspace/cpu-p01-20260930-01/manual-ani-system-20261008/manual.py forward
```

回到设置了变量和 `k`/`t` 的终端：

```bash
t login
t catalogue
t enable success
t catalogue
```

登录使用现有真实租户 A、B 普通用户以及一个无 ModelDev 授权用户的密码文件，不伪造 JWT。token 保存为私有文件。`enable` 先经 Governance 管理 CLI 导入新 Release，再读取当前 `binding_generation`，按 CAS 启用。

预期 success preset 的 `active_release_id` 指向新 Release、`new_submissions_enabled=true`，READY 输入为 1024 行、16 维特征。这里只保留一个租户 A 的实际 ModelDev dispatch binding；租户 B 用于权限隔离测试，不能据此宣称 B 的训练已接通。多租户同时训练需要正式部署每租户绑定的服务实例，或扩展当前单绑定 dispatcher。

管理操作创建具名 Job、临时 Secret 和阶段账本；若返回不确定结果，核对 Job 退出码、响应和业务状态，不能改 Job 名或 generation 反复重试。token 过期可再次 `t login`。连接到时后重新运行前台 `forward`。

## 6. 两次真实成功训练、日志和独立加载

```bash
t run success-01 success
t wait success-01 SUCCEEDED
t logs success-01
t inspect success-01
t verify success-01

t run success-02 success
t wait success-02 SUCCEEDED
t logs success-02
t inspect success-02
t verify success-02
```

每次 `run` 使用固定保存的请求和 idempotency key 提交两遍，必须返回同一个 Execution、Operation，第二遍 `replayed=true`。再次运行同名 `run` 仍使用原键；要发起独立执行必须换测试名。

通过条件：

1. 两个执行 ID 不同；最终 `compute_state=SUCCEEDED`、`delivery_state=PUBLISHED`、`close_state=CLOSED`。
2. 日志包含实际 `ani.metric.v1` / `train.loss`，48 次 optimizer update；数据为 1024 行、16 维，3 epoch，batch size 64。
3. `inspect` 的 Run、Workflow、TrainJob/JobSet/Job、workspace PVC 与本次 Execution 的 namespace UID 一致；不要只看 BFF HTTP 202。
4. 每个独立 verifier Job **不挂载原训练 PVC，不持有 S3 key，不自动挂载 SA token**，经用户 BFF 下载授权取得 `model.pt`、`model_config.json`、`metrics.jsonl`、`summary.json`；核对四个文件的字节数和 SHA256。
5. verifier 严格加载真实 PyTorch 权重，610 个参数，CPU 前向输出 `[4,2]` 且有限；最终输出 `L4_PASS`，Job 容器退出码 0。

验证日志保存在部署包根目录 `manual-verify-success-01.log` / `manual-verify-success-02.log`，不包含签名下载 URL。失败时检查：

```bash
k get trainjobs,jobsets,jobs,pods,pvc -n ani-kfp-manual-a
k get events -n ani-kfp-manual-a --sort-by=.lastTimestamp
k logs -n ani-system deployment/ani-modeldev --tail=100
k logs -n ani-system deployment/ani-governance --tail=100
```

服务日志可能包含业务细节，分享前脱敏。不要将 `NEEDS_REVIEW` 手动改成 `CLOSED`。

## 7. 幂等冲突、参数校验和租户隔离

```bash
t negatives success-01
```

预期同键不同 name 返回 409；非法 epochs 和 GPU kind 返回 400；租户 B 查询 A 的 Execution 和产物授权均为 404；无授权用户查询为 403；未登录请求为 401。

必须确认这些拒绝没有生成额外 Execution/TrainJob。用原始两次 `receipt` 的 ID 对照列表和资源，不把“请求被拒绝”直接等同于“没有副作用”：

```bash
k get trainjobs -n ani-kfp-manual-a
k get trainjobs -n ani-kfp-manual-b
```

## 8. 实际训练失败、运行中停止和原截止时间

逐个执行，避免训练 CPU 配额相互竞争。

```bash
t enable fail
t run failure-01 fail
t wait failure-01 CLOSED
t logs failure-01
t inspect failure-01

t enable stop
t run stop-01 stop
t inspect stop-01

t enable deadline
t run deadline-01 deadline
t wait deadline-01 CLOSED
t inspect deadline-01
```

fail：真实训练程序使用失败配方，检查原训练容器非零退出、没有 PUBLISHED 产物、关闭完成，inspect 的 `compute_outcome` 和关闭证据与实际失败一致。`wait ... CLOSED` 只检查关闭完成，不替代失败原因核验。

stop：慢训练镜像实际输出第 3 步及以后、尚未到第 48 步时，客户端立即发送 Stop 并重放 Stop；必须同 Operation、同 intent generation，最终 CLOSED、Stop intent 保留。客户端输出只代表观察到该状态；还需 `inspect` 验证 `close_reason=USER_STOP`，KFP 原 Run 取消、相关训练和步骤 Pod 没有运行中的写者，停止之后不发布完整模型。

deadline：Release 的原始 execution timeout 为 60 秒；应在原截止时间后关闭，inspect 的 `close_reason=DEADLINE`。60 秒是触发截止意图的时间，不保证资源在第 60 秒已经清理完。BFF 的 compute_state 可能保留最后一次计算观察，关闭原因以 inspect 和原资源退出证据为准，不能强改成 FAILED/CANCELED 或启动替代 Run 来宣称恢复。

```bash
k get trainjobs,jobsets,jobs,pods -n ani-kfp-manual-a
```

success 使用独立 preset，前面的 fail/stop/deadline 不会切换它，无须再次执行 success 管理操作。

## 9. 关闭后重启、正式清理与再次下载

先确认以上执行均 CLOSED，随后重启应用并验证历史执行/已发布数据持久保存：

```bash
k rollout restart -n ani-system deployment/ani-modeldev
k rollout restart -n ani-system deployment/ani-governance
k rollout status -n ani-system deployment/ani-modeldev --timeout=180s
k rollout status -n ani-system deployment/ani-governance --timeout=180s
t login
t wait success-01 SUCCEEDED
t inspect success-02
t cleanup-plan success-02
```

阅读 plan 的 targets、exact UID 和 retained 字段：只允许已关闭执行的计算资源；workspace PVC、发布产物和持久证据应保留。确认后再执行：

```bash
t cleanup-apply success-02
t inspect success-02
cp "$CPU_P01_PRIVATE/success-02.receipt.json" "$CPU_P01_PRIVATE/success-02-after-cleanup.receipt.json"
t verify success-02-after-cleanup
```

最后的验证别名复制同一个已清理 Execution 的 receipt，创建一个新的 verifier Job。

预期 cleanup `phase=APPLIED` 或 `RECONCILED`，targets 的原 UID 均不存在、workspace PVC 保留；新 verifier 仍能下载和加载同一发布模型。出现 `NEEDS_REVIEW` 或不确定删除时停止 Apply，先 inspect 原结果。不要删除 workspace PVC 或 S3 发布目录。

## 10. 覆盖范围、扩展及 installer 收敛

本手册覆盖部署到 `ani-system`、真实训练与交付、独立加载、基础错误/权限拒绝、训练失败、运行中 Stop、deadline、关闭后重启和正式计算资源清理。KFP 创建响应丢失、晚创建 Run 与 Stop 竞态、S3 发布故障、运行中 owner 崩溃等历史故障注入矩阵，需要另行恢复受控故障代理和对应固定配置；本包不部署旧故障代理，也不据此声明这些矩阵在新环境已经重测。

后续 installer 应交付：每套环境的 namespace/SA 实际身份、Runtime 合同、卷写权探针、RBAC/网络策略/CA，以及 KFP/对象存储连接信息。ModelDev/Governance 部署交付新环境 binding、Pipeline/Release/InputVersion 和真实业务验收。模板可以放在 installer 与应用仓库；带 UID 的环境 receipt 和凭据按环境分别保存，不能用一套现场文件代替全部环境。

installer 的原 site 配置包含 `ani-kfp-probe-a/b`。本次删除后，它的旧 `ENV_READY` 是历史记录；下次 installer 安装或验证前，先把 site 的租户配置改成计划中的正式 namespace，否则可能重新创建旧 probes。不要直接照抄旧 environment-handoff 的 namespace UID。本手册生成的 receipt 只记录已经检查的事实，不冒充 installer 全量 EAC 验收。

本次还发现一个实际 teardown 问题：`ani-kfp-workspace` 准入策略对 PVC CREATE/UPDATE 都要求参数 ConfigMap，namespace 删除先清除该 ConfigMap 后，`Deny` 会阻断 PVC 保护控制器的 finalizer 更新。本次在确认终态 namespace 无 Pod、容器、挂载和 VolumeAttachment 后，仅移除该 namespace 的 `ani.io/kubeflow-tenant` 选择标签，让控制器正常回收；未修改全局策略或 CSI finalizer。installer 后续需要同时定义环境创建和删除顺序，避免再次出现这一生命周期死锁。

## 11. 清理结果及恢复入口

此次六 namespace 的删除结果见部署包中的 `retirement-result.json`；无凭据记录同时保存在本地 `.scratch/cpu-p01-manual-20261008/`。受保护恢复材料在 `.private/`：两个数据库 dump、原资源 JSON、catalogue tar/文件快照。数据库和 S3 未删除；dump 用于灾难恢复，不能在当前数据库上重复恢复。

本手册的测试步骤应记录为 `PASS` / `FAIL` / `NOT_RUN`，并附本次 Execution ID、Job 退出码及验证日志。旧环境通过、离线检查通过和新环境实际通过是三种不同证据。
