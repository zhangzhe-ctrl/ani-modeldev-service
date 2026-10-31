# ModelDev 动态租户环境实施报告

报告时间：2026-11-01（Asia/Shanghai；原始证据保留 UTC 时间）。
执行者：Codex；源码、Git 与传输在本地，生成/构建/测试/隔离 PostgreSQL 仅在 `ssh fedora`。
目标集群已核对：`kubercloud@100.64.128.12` / `kubernetes-admin@ani-kcn-full` / `https://lb.kubesphere.local:6443` / cluster `f39bf6e2-f3ed-4191-ada3-caeebf86c220` / kube-system UID `3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24`。见 [baseline](evidence/baseline.json)、[principals](evidence/native-principals.json)。
总体状态：**BLOCKED，目标未完成**。源码和必要业务验证已交付到任务分支；旧环境尚有 3 条 OPEN 执行无法满足原关闭证明，未执行旧数据删除、Res 实际回填、新版本部署及 A/B 真实验收。上一轮Fedora SSH曾出现banner超时，本轮已恢复；EN02固定预算恢复补测已PASS，Gov原检查已确认终态。旧三执行的关闭证据阻塞仍独立存在。

## 1. 实际交付与未完成项

业务主链为：原系统按原协议排空清理 → 固定平台材料/策略/服务切换 → 同一共享进程显式 Ensure A/B → READY → 真实 CPU 一般训练 / GPU_SHARED 微调 → 原关闭及释放 ACK → 日志与 artifact 下载、隔离和轮换验收。

当前已跑通的最小纵向用例：真实 PostgreSQL、真实业务代码、CPU 进程与文件，Kubernetes/KFP/Resource 等外部边界使用明确替身；Ensure → READY → Accept → Reserve → 训练 48 optimizer steps → publish → 自然 CLOSED → 原 7 项计算资源 cleanup APPLIED → 重连日志 → 独立下载 checkpoint 并执行 `[4,2]` forward。异常 CSV 也经原失败退出与关闭；训练未启动、无发布。见 [最终主链](evidence/04/modeldev-cpu-mainflow-final-shared-budget.log)、[拒绝路径](evidence/04/modeldev-cpu-mainflow-rejected-final.log)。这不替代指定集群真实验收。

| 任务 | 状态 | Commit/证据 | 未完成影响 |
|---|---|---|---|
| 01 基线与合同 | PASS（源码） | [Result](issues/01-baseline-contracts.md)、[合同](evidence/contract-freeze.md) | 所有运行输入/IDs仍须现场取得 |
| 02 Resource Namespace/镜像 | PASS（组件、真实隔离PG） | `71b3ad7a2a88ae531e5e7b80abde2f494ff28d9d`、[证据](evidence/resource/README.md) | 实际 migration/backfill/rollout NOT_RUN |
| 03 环境注册 | PASS（组件/PG/SIGKILL及固定预算恢复） | [Result](issues/03-environment-registry.md)、[故障恢复](evidence/03-environment-process-recovery.log) | 真实 A/B READY 未验收 |
| 04 v4/facts/STS | PASS（源码及必要业务验证） | [Result](issues/04-snapshot-facts-storage.md)、[04证据](evidence/04/) | 真实 KFP/IAM/下载授权待 LIVE |
| 05 派发/关闭 | PASS（组件、真实隔离PG） | [Result](issues/05-dispatch-close.md)、[最终gate](evidence/modeldev-final-make-verify.log) | 不能因此宣称旧3执行已关闭 |
| 06 Webhook/native/RBAC | BLOCKED（handler/renderer/schema部分PASS） | [Result](issues/06-webhook-native-rbac.md)、[21对象dry-run](evidence/06-native-server-dryrun.fixed.md) | 未安装的VAP没有实际controller typeChecking/行为结论 |
| 07 Gov API | BLOCKED（相关race/PG子项PASS；必需gate实际FAIL） | [Result](issues/07-governance-api.md) | 既有VM路由权威map缺口及audit fixture命中未放行；正向HTTP组件补测已通过；仍不以子项PASS放行全仓FAIL |
| 08 平台材料工具 | BLOCKED（工具6项/四IR编译PASS） | [Result](issues/08-materials-bootstrap.md)、[工具](evidence/08/tools.log) | 真实固定镜像/四定义发布/Secrets/bundle未完成 |
| 09 旧清理与切换 | BLOCKED | [Result](issues/09-cleanup-cutover.md)、[原3执行](evidence/09/open-execution-blockers.md) | 阻断破坏性清理及新v4切换 |
| 10 真实验收交付 | BLOCKED | [Result](issues/10-live-acceptance-delivery.md)、[验收表](acceptance-results.tsv) | LIVE02–11 未完成 |

## 2. 源码与部署映射

| 仓库 | Base | Runtime/API source commit | API pin/生成物 | 新image digest / deployed imageID |
|---|---|---|---|---|
| ModelDev | `21bf4bd6540d462a61c91b629f26250899583223` | `d807cbdbd5dd6664d61480acedc752e19cfca48a` | buf1.60.0/sqlc1.31.1；Resource `v0.0.0-20261031180451-71b3ad7a2a88` | NOT_RUN / NOT_RUN |
| Gov | `ddd98f4c79751ba848fa862bab6878dc7bc99bb7` | `fa80f886d022177173d7e9cf81ec51e54c2df4c8` | `v0.0.0-20261031195825-d807cbdbd5dd`；正式module，禁止replace/复制pb | NOT_RUN / NOT_RUN |
| Resource | `23e25b33ca02890231cd273ef7246b0fc415476c` | `71b3ad7a2a88ae531e5e7b80abde2f494ff28d9d` | pinned proto/config/sqlc产物 | NOT_RUN / NOT_RUN |
| Gov旧API维护版本 | 同Gov base | `6a7c721ea113d51497421d7975122ec7ca8105c2` | 原API pin；只有Create维护开关与publisher drain | NOT_RUN / NOT_RUN |

分支均为 `codex/modeldev-dynamic-tenant-environment`；旧API维护分支为 `codex/modeldev-create-maintenance`。已提交版本使用 SSH 推送。没有修改 main、合并 PR 或部署上述版本。ModelDev 另有仅CL03补测提交 `e4067444498394d0c5247bc927fa5476bc198015`（定向exit0），固定预算续跑仅测试提交 `6b0b952478081034baee8ab27fd8216b26742700`（25.555s、exit0），Gov仅HTTP集成测试提交 `5cfa9065190b9585b799e428537a0019ca70e41a`（定向race exit0），以及只含本报告/Results/证据的记录提交；API pin固定上述已测试运行/API源码提交。

main 重新读取值与 spec 基线相同，相关 diff 见 [contract-freeze](evidence/contract-freeze.md)。ModelDev运行/API提交d807的845个输入与对应Fedora最终源码hash一致（后续两项仅测试提交各有独立源码hash和定向结果），见 [source identity](evidence/modeldev-tested-source-identity.json)。测试后仅增加生成输出忽略规则并排除本次生成二进制/SQL scratch，业务源码未变。

| 必需gate | 结果与边界 | 证据 |
|---|---|---|
| ModelDev `python3 scripts/with-test-postgres.py make verify` | PASS，exit0；固定生成检查、tidy、默认Go测试、vet/build/modverify/diff；全部在Fedora | [完整日志](evidence/modeldev-final-make-verify.log)、[生成](evidence/modeldev-final-generation.log) |
| ModelDev targeted/tagged主链与race | PASS；真实PG/业务代码，外部边界替身；不含真实GPU | [主链](evidence/04/modeldev-cpu-mainflow-final-shared-budget.log)、[关闭回归](evidence/04/modeldev-compute-parent-budget-final.log)、[共享并发](evidence/worker-coordinator-tests.log) |
| Resource `make verify` | PASS，exit0；另选2项真实PG；不含现场回填 | [verify](evidence/resource/verify.log)、[PG](evidence/resource/namespace-pg.log) |
| Gov最终代码 | 相关default/race和真实PG六项PASS；原make check exit2 FAIL（既有VM权限map）；audit网络恢复后续一次exit1 FAIL（0 affected calls、Gitleaks1原fixture）；未放行 | [原会话终态](evidence/gov-final/fedora-round2-original-session-exits.json)、[audit续跑](evidence/gov-final/environment-formal-audit-resume-once.log)、[finding](evidence/gov-final/gitleaks-final-finding-metadata.json) |
| Gov旧API维护 `make check` | FAIL：原有VM日志路由缺权威permission映射；未扩VM权限 | [结果](evidence/gov-maintenance/result.json)、[check](evidence/gov-maintenance/maintenance-make-check-final.log) |
| Gov旧API维护独立audit | FAIL：govulncheck 0 called；gitleaks命中原base未改的component-name负向fixture，第57–58行；未添加忽略 | [audit](evidence/gov-maintenance/maintenance-audit.log)、[gitleaks](evidence/gov-maintenance/maintenance-gitleaks.log) |

前两次 ModelDev gate 暴露过旧fixture/PG资源配置及一处不可达return；失败日志保留，最终完整gate确实重新执行exit0。没有把局部补测拼接称为完整 `make verify`。

## 3. 固定合同落地

接口/权限/错误/超时：显式 Ensure/Get，当前tenant/cluster，13字段环境view、严格有限reason和ErrorReason38；mTLS/委托；Ensure25s、Gov RPC28s/3s、HTTP30s/5s、外层预留5s。Get无登记404且不建行；Prepare失败不创建原acceptance/charge。Gov正式pin、相关data/service/server race和六项真实PG已通过，44个变更文件与fa80f886本地manifest一致。必需make check/audit实际FAIL；本轮新增真实JWT/Redis/Casbin/PG授权→生产HTTP/service/client→外部ModelDev mTLS peer集成PASS：Ensure202/200、Get200/404、Prepare有限503、同key撤manage后403无新增上游RPC、每请求acceptance/charge0；22cause是转换器单测，真实HTTP实跑1cause。见 [HTTP证据](evidence/gov-final/environment-http-integration-source-and-coverage.json)。现场A/B仍未执行。

namespace规则与Res UID：唯一 `tenant-<canonical非零UUID>`；原network两标签和原UID；新接纳要求Active且非deleting；不存在即错。没有新Res Namespace RPC，没有Namespace Create/Patch/adopt。Network原UID事务、nullable migration和精确条件回填工具已实现；现场列尚未创建。原read/Stop/close仍保留冻结原UID规则及Terminating容忍，不重建旧Owner。

环境唯一性/状态/恢复：`(tenant_id,cluster_id)`唯一行，PG专用连接advisory lock；显式Ensure序列和receipts读回；seal→canonical projection→readback→CAS READY。KFP POST未知不擅自重发；有候选冲突、预算耗尽或危险RBAC审查时有限失败。真实PG与SIGKILL后原SA UID/Experiment唯一POST恢复见03证据。

v4快照/四root参数：只接受v4；DispatchOwner、三个SA UID、ManagedImageAccess与投影均参与hash和严格比较。execution_id/spec_hash/environment_binding_id/namespace_uid四参数覆盖driver与step。旧静态bindings/facts/租户枚举已移除或reserved，不增加v3兼容reader。

固定镜像/平台项目：单受管registry/platform project，general仅TrainingImage.PullSecretRef，fine显式managed refs，受控允许SA继承；实际固定digest资产尚未现场确认/发布，不能声明真实pull成功。

KFP artifact、CA和凭据隔离：平台固定文件，KFP projected token每次请求重读且audience为pipelines.kubeflow.org；Kube callback token独立。专用bucket/两key Secret/共享artifact平台身份及有界RBAC审查已接线，真实IAM、TLS加载、I/O与隔离尚未验收。不会把业务STS与KFP内部artifact凭据混为同一授权。

动态facts/STS/base scope/readiness：READY及原Namespace/SA UID、平台catalogue共同提供事实；input `inputs/<tenant>`，publication `executions/<tenant>/<execution>/`，model `<tenant>`，内部artifact `artifacts/<tenant>/<execution>`。Readiness保持source-only。实际跨租户STS和平台身份边界归LIVE。

派发/关闭/已seal outbox：保留原Reserve、commit未知禁发、SendPermit和原关闭/释放权威。两级keyset、16tenant×4行、数据库sweep、坏行推进；两loop共享全局4槽/同tenant1，无队列/lease。单SQL3s，dispatch/close30s，sealed outbox10s；合法已seal原事件不依赖当前READY/Namespace仍可原Gov ACK，半ACK隔离；未seal原资格不放松。

webhook/native策略及共Namespace：动态全局handler，只对保留SA/PVC/Secret/标记命中；普通Pod不调用远端MD webhook。三VAP和最小角色renderer已实现。实际K8s1.35.8仅做21对象server dry-run，未持久安装，返回status{}不足证明controller typeChecking或正反行为。原UID/writer/同名替换/三次cleanup上限回归PASS，未用这些组件证明现场VM共存已验收。

## 4. 部署输入与四个定义

[deployment-inputs.json](deployment-inputs.json) 保留所有已知事实和未取得的null；null是明确待取得输入，不是可随意选择的默认值。文件摘要纳入最终 `evidence-manifest.sha256`。

| Pipeline（固定父namespace ani-system） | Pipeline/Version ID | IR/material hash | 实际发布 |
|---|---|---|---|
| modeldev-mlp-cpu | NOT_RUN | 固定profile编译测试PASS；最终平台材料未定故无最终hash | NOT_RUN |
| modeldev-mlp-gpu | NOT_RUN | 同上 | NOT_RUN |
| modeldev-sft-lora-cpu | NOT_RUN | 同上 | NOT_RUN |
| modeldev-sft-lora-gpu | NOT_RUN | 同上 | NOT_RUN |

6项工具测试覆盖四profile编译、不可变bundle、严格读回、失响应恢复与未知结果不重发，见 [08/tools](evidence/08/tools.log)。该临时测试IR不能充当真实发布IR。

本轮只读取到KFP公开CA 713bytes（已保存）、metadata Service namespace-qualified候选metadata-grpc-service.kubeflow.svc:8080和实际KFP入口。来源/原UID/证书SHA见 [public inputs](evidence/08-public-inputs-readonly-20261101.json)。旧CM仅短名metadata-grpc-service，不能跨namespace照搬；旧namespace的同名metadata CM404。新launcher加载与MLMD/artifact I/O仍NOT_RUN，driver/launcher无显式flags证据保持UNKNOWN，未将未发现当false。

真实principal已核对：ani-system:modeldev-control / ani-modeldev-webhook，kubeflow:ml-pipeline / argo / ml-pipeline-persistenceagent，kubeflow-system:trainer / jobset，kube-system:job-controller，maintenance kubernetes-admin，node1/2/3。精确全名、SA原UID、实际controller配置来源见 [principals](evidence/native-principals.json)。Node principal是Node/RBAC与节点身份核验，实际新请求审计NOT_RUN。

三VAP名称固定；最终render manifest SHA256 `c0c0b90750be1817aa84c70636b77843d2296df018260b7233e87843903adbbe`。未安装，因此新policy UID、实际typeChecking、SAR及共存故障证据均NOT_RUN。

仍未取得输入：最终platform catalogue及hash、受管镜像registry/project/digests/匿名支持镜像清单、workspace配置、callback endpoint、ModelDev callback/S3公开CA实际文件及加载事实（KFP公开CA已取得）；四Pipeline/Version最终ID与IR/material hash；KFP connection_ref/实际token认证轮换/Experiment唯一约束/driver与launcher显式TLS flags/Argo defaults；业务input/publication/model连接与bucket；专用artifact bucket/平台identity文件引用、bucket-only IAM/TLS/I/O/RBAC隔离证明；新policy UID/digest、实际请求principal审核；Res应用回填计数及三仓新image digest/imageID。08/09/10均未声称完成这些输入。

环境更新入口只允许 `http://192.168.25.40:9998/`。本轮只读核对线上app.js与本地ani-deploy前端SHA一致、API请求7字段且无build_only/skip_deploy；本地后端合同拒额外字段、push后无条件deploy和ready，run_verify=false仅跳质量。线上未提供后端源码hash，因此不能声称本地后端就是已核实live版本，也未发现可安全使用的只build/push入口。现有ModelDev recipe仅主service，未覆盖webhook/step/training。精确边界见 [Workbench receipt](evidence/08-workbench-build-only-readonly-20261101.json)。未POST job、未构建/部署，没有以手工rollout绕过指定入口。

## 5. 旧数据和资源清理

旧聚合只读清单（非可执行删除manifest，资格未完成）：[old-inventory](evidence/09/old-inventory.json)，SHA256 `aa5420bd1c2ad02f373509d625a99080b4c6721050de1bcdb3496ad1d15a44d2`；原查询强制read-only事务；保留精确ID/UID/冻结scope，已去除重复snapshot和原日志内容。不是数据库备份。

停止Create流入/在途lease处置：NOT_RUN。旧API维护提交具备Create503与publisher停止claim/drain，定向及真实PG lease测试PASS；旧环境当前未部署开关。已有Gov4条UNKNOWN均retry_blocked INVALID_COMMAND、无active lease；这不等于维护窗口已停所有创建者。

原执行关闭/资源writer证据：50 executions / 10 inputs，47 CLOSED / 3 OPEN，50 CONFIRMED dispatch，44 authority。3条原执行见 [阻塞证明](evidence/09/open-execution-blockers.md)：

| 原execution | 已确认 | 缺少、不能替代的证明 |
|---|---|---|
| 1cce950b-d6e9-453e-aa11-fffd8846224b | 原Pod UID49c82c2b-f819-4a03-b7b2-1789999cdd2e，Failed/143；原Run FAILED | 原最终training logs只115字节kubelet错误；原Workflow已404 |
| 4a1a92f7-4d94-439d-a8d7-1833f38c7dbc | 原Pod UIDd8fb153a-5763-4ae7-8d44-63e3f996a830，Failed/143；原Run FAILED | 同类原最终日志缺失；原Workflow已404 |
| eca4d47b-5911-4f0f-8b45-89c1be775925 | 原Run FAILED、CloseAuthority已在；v1原UID归档已取回、4原Pod已退出 | live原Workflow404；原协议强制live UID/RV，两个image_contract Error节点不满足原no-process资格；authority重放no-op |

26个保留step Pod仍关联3个原Workflow UID。GET404不证明完整原writer图已终止。本轮找到官方v1 RunDetail持久manifest：三条原Run全部200、6份manifest原UID/name/Namespace匹配，35/35/25节点终态；没有直接PodUID或原日志archive URI。第三条4个原Pod终态已精确核验，但旧live UID/RV与image_contract资格guard仍不满足，且完整当前writer/PVC/模板证明未完成。详见 [原v1归档](evidence/09/kfp-v1-original-run-detail-20261031T204206Z.json)、[第三条原关闭评估](evidence/09/third-archive-close-assessment.md)。原节点精确日志路径/容器读取失败；现有control权限GET正常。有限OpenSearch字段探测无精确PodUID/完整containerID字段，没有按名称伪造关联；也没有证明所有外部归档都不存在。用户已被询问是否另有原UID归档位置，尚未取得。

原Gov charge/event/outbox/ACK：54 acceptances（50 ACKED、4 UNKNOWN retryblocked），23笔原ModelDev charge均全额released；12原GPU outbox均ACK+DeliveredAt。两个账户 `occupied_units=0=sum(all owner unreleased charges)=0`。这是原状态只读对账，本轮没有新退款/SQL清零。见 [Gov对账](evidence/09/old-gov-inventory.json)。

各表删除计数：所有ModelDev九表及Gov旧项 **0（NOT_RUN）**；未执行删除SQL、truncate或级联。旧资源K8s/KFP/S3精确删除均 **0（NOT_RUN）**；未删任何Namespace、finalizer或原日志证据。47 CLOSED不等于其整组资源/S3/共享引用已可无条件删除。

Res UID回填前后数量：迁移前原namespace映射2行，列尚不存在；2行名称/UID/标签/Active可精确核对。实际新增列、NULL计数、回填、零NULL、服务切换均NOT_RUN；不把“2条eligible”当“回填2条”。[只读预检](evidence/09/res-namespace-pre-migration.json)。

保留其他owner资产：基线4 VM（3 Running/1原Stopped）、3 VMI Running、34非MD PVC Bound、5个A/B无关Pod；Sandbox代表原Pod UID `889b0eb5-4980-49c7-8eec-f0b694df0b15` Running/Ready。精确UID和原健康见 [资产清单](evidence/preserved-assets.json)。本轮未修改它们；切换后的再次健康比较NOT_RUN。

清单外/不确定项：原Workflow归档已找到；真实完整训练日志归档、全部原writer/PVC/模板证明及旧S3版本/外部引用精确删除资格仍未完成。归档资格不能冒充旧live读取，没有扩大清理范围。

## 6. 验收结果

逐项结果见 [acceptance-results.tsv](acceptance-results.tsv)。PASS均限定证据层级；附件 `verification/` 是此前调查输入，不能充当本轮新实现LIVE验收。19个组件行PASS；AD01/02及LIVE01–11共13行BLOCKED，内部未执行动作明确NOT_RUN，未缩减目标。

A/B标准namespace原身份已核对：A Gov2 / Resource64c5c35a-f554-432c-b4bc-7b4f36f08420 / NS UIDa8d8a472-8a8a-43cf-abf0-0e805bbe67b0；B Gov40 / Resource207328fc-f08d-4dc4-b403-16ec86396ce2 / NS UID8698b0b5-f4c8-4770-85e5-10f25ad7b347。二者均Active、无deletion、network标签正确。

A/B Ensure时间、同一新服务Pod UID/启动时间、新environment/SA/Experiment原身份：NOT_RUN。原基线ModelDev Pod UID `592bdd86-ab5b-4416-8a2c-07c21ee69f8d` 与旧imageID仅作基线，不能冒充新进程或新环境。

CPU一般训练、B GPU_SHARED微调、运行中Stop：LIVE NOT_RUN；只存在上述明确外部边界替身CPU闭环。真实GPU_WHOLE没有测试，未作其可用声明。

跨tenant业务否定例、token实际轮换前后调用、artifact真实I/O/TLS/授权、webhook故障共存及恢复：LIVE NOT_RUN。21对象schema dry-run未安装policy，不是这些测试。

原释放ACK/账户恒等式：只读原状态通过如第5节；新A/B执行的原释放链和账户对账NOT_RUN。未把collector/HTTP200、环境READY组件测试或CPU替身成功当真实产品验收。

## 7. 风险和边界

- **BLOCKED**：spec §9/§13.3.3–12所需三原执行关闭证据未取得。保留原worker/权限/数据/对象；不得安装破坏性新策略或仅v4系统读取旧v3聚合。09导致清理/Res回填/发布/rollout/LIVE02–11全部未完成。
- **FAIL**：旧API维护Gov版本的make check存在原有VM capability映射缺口，audit命中baseline fixture，如第2节；没有为过gate扩VM权限或加忽略。
- **已解除的执行阻塞**：Fedora本轮恢复，原Gov会话终态为0/2/1，未因观察超时重跑；EN02新增固定25秒预算耗尽续跑测试本轮唯一一次exit0，7条持久receipt续跑同原UID到sealed READY，见 [记录](evidence/03-environment-budget-recovery-record.json) 和 [实际输出](evidence/03-environment-budget-recovery.log)。原草稿NOT_RUN只保留为历史，不能作为当前状态。
- **FAIL**：最终Gov make check仍为既有VM路由authority mapping缺口；audit续跑govulncheck调用受影响0、另有1导入包/10required模块漏洞观察，Gitleaks命中同一未改base负向fixture（blob相同）。没有把它们写PASS，也未加allowlist或扩VM权限。
- **NOT_RUN**：所有新镜像构建与imageID、真实材料/四定义/专用artifact identity、native安装与实际typeChecking/行为、停止旧Create及lease维护、原协议关闭补全、旧K8s/KFP/S3/DB删除、Res真实回填、新A/B训练/Stop/隔离/token轮换/故障恢复/新释放验收。没有权限问题阻止已授权动作；阻止切换的是冻结合同的实际前置证据缺失。
- 共享KFP内部artifact平台身份的实际隔离边界尚未现场验收；Secret挂载/TokenRequest/exec及普通tenant RBAC必须实际否定例通过后才能宣称成立。
- 未实测GPU_WHOLE；真实GPU_SHARED也尚未执行。依赖/权限/Pod ready或模拟向量不证明真实训练。

事实冲突及最小修正：

1. 原source gate只允许baseline migration，与spec新增表/投影冲突。只显式加入0002/0003，并接入相同sqlc/PG输入；不加兼容reader。见contract-freeze。
2. K8s1.35.8首次server dry-run拒绝CEL保留字namespace及异质对象推断；renderer改变量名standardNamespace并显式dyn，固定权限/规则不变；21对象再次dry-run通过。没有用status{}宣称安装typecheck通过。
3. 完整关闭主链实测原Kube限流5/10在第154次调用抛would-exceed-deadline，错误被父GET归为身份冲突。保留30秒预算，三client共享有限20/40 limiter，依赖读取失败保持uncertain；修复后真实业务代码CPU闭环245调用完成，原UID/writer/重试上限不变。
4. 固定KFP2.16的GetVersion.pipeline_spec为protobuf Struct，原多doc YAML bytes不能从GET恢复。按08记录的官方源码，保留不可变bundle内原IR bytes并核对IRSHA，再与官方GET完整结构比较；不重定义IR SHA或跳过读回。
5. 指定Workbench只证实既有build→deploy job合同，没有可安全使用的build-only能力，且现有recipe不含全部所需镜像。影响08镜像准备与09映射；最小处理是取得该入口覆盖目标镜像的确切受支持发布合同/配置后再构建，不试调用未知变体或绕过入口。
6. live原Workflow缺失与严格旧退休前置冲突。本轮已找到真正原UID KFP v1归档，但旧关闭协议强制live UID/RV，且第三条image_contract Error节点超出原无进程资格；现成authority重放为no-op。最小可能修正是固定旧base、单execution、先只读dry-run的一次性归档维护合同，先完整核定原plan/owner/PVC/模板/退出/fence，再明确裁定归档来源和精确denial资格；不能伪造live对象、兼容进新runtime或直接SQL CLOSED。两个原训练日志仍未恢复，因此新维护合同也不自动解除全部清理阻塞；当前未实施资格扩展。

## 8. 交付文件

正式合同/ADR：[contracts](contracts/)、[spec](spec.md)、ModelDev `docs/modeldev-tenant-environments.md` 及 `docs/adr/0001-modeldev-webhook.md`。ADR0002原释放权威未改；Inference未改。

部署/维护工具：ModelDev `deploy/modeldev-environment/{publisher,bundle,render,verify_target,platform_config}.py`、`deploy/modeldev-tenant-policy/`、`deploy/modeldev-webhook/`；Resource `scripts/tenant-namespace-backfill`；Gov旧API维护开关和有界publisher drain。原只读清单脚本在 `evidence/09/`。没有尚未证实可恢复数据的通用日志导入器。

完整脱敏证据目录：[evidence](evidence/)，文件摘要见 `evidence-manifest.sha256`；原附件 `SHA256SUMS` 只验证未改输入，Results/表/报告修改后不再用它宣称整个工作包原样。`verification/` 明确为附件历史输入。

任务Result：[issues/01–10](issues/)，逐项回填；[验收表](acceptance-results.tsv)、[部署输入](deployment-inputs.json)。本报告遵循原8节模板；总体BLOCKED不表示目标完成。
