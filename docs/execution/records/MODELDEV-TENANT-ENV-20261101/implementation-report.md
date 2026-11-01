# ModelDev 动态租户环境实施报告

报告时间：2026-11-01（Asia/Shanghai；原始证据保留 UTC 时间）。
执行者：Codex；源码、Git 与传输在本地，生成/测试/隔离 PostgreSQL 在 `ssh fedora`。用户后续明确授权两个大型训练镜像在指定测试节点直接构建，不加额外 CPU、内存或并发限制；构建临时工具逐项记账，完成后清理并核实。
目标集群已核对：`kubercloud@100.64.128.12` / `kubernetes-admin@ani-kcn-full` / `https://lb.kubesphere.local:6443` / cluster `f39bf6e2-f3ed-4191-ada3-caeebf86c220` / kube-system UID `3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24`。见 [baseline](evidence/baseline.json)、[principals](evidence/native-principals.json)。
总体状态：**BLOCKED，目标未完成**。三仓源码及必要组件验证已交付；旧API维护Gov已实际部署，真实Create503、活动lease0及接纳/占额前后恒等通过。原3执行已按批准的一次性修正规则真实CLOSED、step/compute cleanup APPLIED，第三原GPU事件实际ALREADY_APPLIED且账户零差；全部四个CPU/GPU/step/webhook镜像已实际发布；新增构建工具已按清单清理并核实删除。四条原UNKNOWN也已完成原协议接收并native CLOSED/NoDispatch；7条原c99历史删除资格、外部资产/数据库清理、Res实际窗口、新v4切换及A/B真实验收仍未完成。

## 1. 实际交付与未完成项

业务主链为：原系统按原协议排空清理 → 固定平台材料/策略/服务切换 → 同一共享进程显式 Ensure A/B → READY → 真实 CPU 一般训练 / GPU_SHARED 微调 → 原关闭及释放 ACK → 日志与 artifact 下载、隔离和轮换验收。

当前已跑通的最小纵向用例：真实 PostgreSQL、真实业务代码、CPU 进程与文件，Kubernetes/KFP/Resource 等外部边界使用明确替身；Ensure → READY → Accept → Reserve → 训练 48 optimizer steps → publish → 自然 CLOSED → 原 7 项计算资源 cleanup APPLIED → 重连日志 → 独立下载 checkpoint 并执行 `[4,2]` forward。异常 CSV 也经原失败退出与关闭；训练未启动、无发布。见 [最终主链](evidence/04/modeldev-cpu-mainflow-final-shared-budget.log)、[拒绝路径](evidence/04/modeldev-cpu-mainflow-rejected-final.log)。这不替代指定集群真实验收。

| 任务 | 状态 | Commit/证据 | 未完成影响 |
|---|---|---|---|
| 01 基线与合同 | PASS（源码） | [Result](issues/01-baseline-contracts.md)、[合同](evidence/contract-freeze.md) | 所有运行输入/IDs仍须现场取得 |
| 02 Resource Namespace/镜像 | PASS（组件、真实隔离PG） | `71b3ad7a2a88ae531e5e7b80abde2f494ff28d9d`、[证据](evidence/resource/README.md) | 实际 migration/backfill/rollout NOT_RUN |
| 03 环境注册 | PASS（组件/PG/SIGKILL及固定预算恢复） | [Result](issues/03-environment-registry.md)、[故障恢复](evidence/03-environment-process-recovery.log) | 真实 A/B READY 未验收 |
| 04 v4/facts/STS | PASS（源码及必要业务验证） | [Result](issues/04-snapshot-facts-storage.md)、[04证据](evidence/04/) | 真实 KFP/IAM/下载授权待 LIVE |
| 05 派发/关闭 | PASS（组件、真实隔离PG） | [Result](issues/05-dispatch-close.md)、[最终gate](evidence/modeldev-final-make-verify.log) | 原3真实退休另见第5节 |
| 06 Webhook/native/RBAC | BLOCKED（handler/renderer/schema部分PASS） | [Result](issues/06-webhook-native-rbac.md)、[21对象dry-run](evidence/06-native-server-dryrun.fixed.md) | 未安装的VAP没有实际controller typeChecking/行为结论 |
| 07 Gov API | PASS（源码/组件/必需gate） | [Result](issues/07-governance-api.md)、[最终完整check](evidence/gov-final/vm-log-map-make-check.log) | 指定集群A/B权限、Stop与下载仍归LIVE，未执行 |
| 08 平台材料工具 | BLOCKED（工具6项/四IR编译PASS） | [Result](issues/08-materials-bootstrap.md)、[工具](evidence/08/tools.log) | 四镜像/专用artifact身份已发布；四定义/sealed bundle/安装未完成 |
| 09 旧清理与切换 | BLOCKED | [Result](issues/09-cleanup-cutover.md)、[原3执行](evidence/09/open-execution-blockers.md) | 阻断破坏性清理及新v4切换 |
| 10 真实验收交付 | BLOCKED | [Result](issues/10-live-acceptance-delivery.md)、[验收表](acceptance-results.tsv) | LIVE02–11 未完成 |

## 2. 源码与部署映射

| 仓库 | Base | Runtime/API source commit | API pin/生成物 | 新image digest / deployed imageID |
|---|---|---|---|---|
| ModelDev | `21bf4bd6540d462a61c91b629f26250899583223` | `d807cbdbd5dd6664d61480acedc752e19cfca48a` | buf1.60.0/sqlc1.31.1；Resource `v0.0.0-20261031180451-71b3ad7a2a88` | NOT_RUN / NOT_RUN |
| Gov | `ddd98f4c79751ba848fa862bab6878dc7bc99bb7` | `244780ffe6b9a7deb1c3f6f162e474e76a73a566`（原API实现fa80；后续仅UTC测试fixture） | `v0.0.0-20261031195825-d807cbdbd5dd`；正式module，禁止replace/复制pb | NOT_RUN / NOT_RUN |
| Resource | `23e25b33ca02890231cd273ef7246b0fc415476c` | `67dceef03f01215c294a55f11464e3e3a52cb783`（API实现71b3） | pinned proto/config/sqlc产物 | NOT_RUN / NOT_RUN |
| Gov旧API维护版本 | 同Gov base | `6c17f0103994620f49c8f6f10c09f414169c6b7d` | 固定原ModelDev `21bf4bd` codec，缺省暂停Create与publisher drain | Workbench `3b191f8ee306437ea5cdcbba6520399b` 已SUCCEEDED；imageID `sha256:2f66dc480e1a45a1a6c2cfc6df2275cd8496eb29a5d9ad5066f2776f02528d05`；实际503/lease0 PASS |

分支均为 `codex/modeldev-dynamic-tenant-environment`；旧API维护分支为 `codex/modeldev-create-maintenance`，独立旧执行维护工具为 `codex/modeldev-legacy-retirement`。均正常SSH推送，没有修改main或合并PR。首个维护job `eda85e66a4f24c3895b7be87a5211500` 因时区fixture失败；第二个 `8d8b448e1a8846c0ba96bb391d323a7e` 已确认verify超时失败，两者build/push/deploy均未开始。固定6c17在Fedora完整make check（含audit）实际exit0；新job `3b191f8ee306437ea5cdcbba6520399b` 复用同一提交的完整检查，run_verify=false，已实际完成build/push/deploy/ready。首次同request HTTP500已保留；核实原生同actor/request/selection幂等后，仅同ID恢复一次，返回上述唯一job，后续只查询。见[完整检查](evidence/gov-maintenance-codec-pin/delivery.json)、[原生幂等证据](evidence/09/workbench-native-idempotency-proof-20261101.json)、[终态job](evidence/09/workbench-maintenance-6c17-terminal.json)。

ModelDev测试补充e406744/6b0b952、Gov HTTP组件补充5cfa906/fixture注释f6a68ed保持原独立证据。Gov02f4全check/audit和旧维护45ed49全check/audit的PASS仅对应其各固定源码；后续244780/32c61只改相同timeutil UTC测试fixture，Europe/Berlin原192/191失败已真实复现，修后UTC/Berlin/Shanghai三个race包测试均exit0，生产diff.go SHA及API pins不变，见[时区修正证据](evidence/gov-maintenance/maintenance-timezone-commits.json)。没有把旧完整gate冒称新候选的完整gate。正式API pin仍为已测试的d807/71b3。

main 重新读取值与 spec 基线相同，相关 diff 见 [contract-freeze](evidence/contract-freeze.md)。ModelDev运行/API提交d807的845个输入与对应Fedora最终源码hash一致（后续两项仅测试提交各有独立源码hash和定向结果），见 [source identity](evidence/modeldev-tested-source-identity.json)。测试后仅增加生成输出忽略规则并排除本次生成二进制/SQL scratch，业务源码未变。

| 必需gate | 结果与边界 | 证据 |
|---|---|---|
| ModelDev `python3 scripts/with-test-postgres.py make verify` | PASS，exit0；固定生成检查、tidy、默认Go测试、vet/build/modverify/diff；全部在Fedora | [完整日志](evidence/modeldev-final-make-verify.log)、[生成](evidence/modeldev-final-generation.log) |
| ModelDev targeted/tagged主链与race | PASS；真实PG/业务代码，外部边界替身；不含真实GPU | [主链](evidence/04/modeldev-cpu-mainflow-final-shared-budget.log)、[关闭回归](evidence/04/modeldev-compute-parent-budget-final.log)、[共享并发](evidence/worker-coordinator-tests.log) |
| Resource `make verify` | PASS，exit0；另选2项真实PG；不含现场回填 | [verify](evidence/resource/verify.log)、[PG](evidence/resource/namespace-pg.log) |
| Gov最终代码 | PASS，最终 `make check` exit0，包含同一最终四文件hash快照完整audit；另相关default/race、真实PG六项与实际HTTP组件PASS。实际调用漏洞0，未调用package1/module10观察保留，Gitleaks无泄漏 | [完整check/audit](evidence/gov-final/vm-log-map-make-check.log)、[退出码/hash](evidence/gov-final/vm-log-map-make-check-receipt.json)、[最终proof](evidence/gov-final/vm-log-map-final-proof.json) |
| Gov旧API维护45ed49 `make check` | PASS：UTC21:26:55–21:32:47，含完整audit；原FAIL保留为历史 | [最终结果](evidence/gov-maintenance/maintenance-default-pause-final-result.json)、[check](evidence/gov-maintenance/maintenance-default-pause-make-check.log) |
| Gov旧API维护45ed49 audit | PASS：实际调用漏洞0；未调用导入包1/module10保留；Gitleaks无泄漏，未添加忽略 | [回执](evidence/gov-maintenance/maintenance-default-pause-make-check-receipt.json)、[范围](evidence/gov-maintenance/maintenance-default-pause-scope-proof.json) |

前两次 ModelDev gate 暴露过旧fixture/PG资源配置及一处不可达return；失败日志保留，最终完整gate确实重新执行exit0。没有把局部补测拼接称为完整 `make verify`。

Resource切换屏障补充提交67dcee已正常SSH推送；4项真实PG定向窗口/并发测试及最终make verify exit0，8458本地/Fedora文件一致。覆盖旧ON CONFLICT/UPDATE拒绝、未知writer来源拒绝、旧backend生存拒绝、guard替换回滚及原回填；现场freeze/migration/backfill/release全部NOT_RUN，模板SHA不能作为liveplan，见[最终回执](evidence/09/res-uid-window-final.json)。

## 3. 固定合同落地

接口/权限/错误/超时：显式 Ensure/Get，当前tenant/cluster，13字段环境view、严格有限reason和ErrorReason38；mTLS/委托；Ensure25s、Gov RPC28s/3s、HTTP30s/5s、外层预留5s。Get无登记404且不建行；Prepare失败不创建原acceptance/charge。Gov正式pin、相关data/service/server race和六项真实PG已通过，44个变更文件与fa80f886本地manifest一致。原失败经固定base SQL映射同步、准确fixture分组说明后，最终完整make check（含audit）已exit0；真实JWT/Redis/Casbin/PG授权→生产HTTP/service/client→外部ModelDev mTLS peer集成PASS：Ensure202/200、Get200/404、Prepare有限503、同key撤manage后403无新增上游RPC、每请求acceptance/charge0；22cause是转换器单测，真实HTTP实跑1cause。见 [HTTP证据](evidence/gov-final/environment-http-integration-source-and-coverage.json)。现场A/B仍未执行。

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

已补齐现有配置来源：受管registry `10.10.1.66:30003` / project `ani-resource-factor10-20261022`；存储connection `modeldev-newenv-rustfs-20261022` 及原input/output/model物理bucket；workspace class `ani-cephfs`；callback候选 `ani-modeldev.ani-system.svc:9001` 与实际证书SAN；ModelDev/S3公开CA SHA与已保存文件相同。见[精确public-source读回](evidence/08/new-public-inputs-readonly.json)。旧input `inputs/fixed` 不作为新tenant input scope。旧MLP workspace4GiB、SFT64GiB，新共享配置最终size尚未选择；候选endpoint/CA不等于新系统实际加载或RPC/I-O成功。

仍未完成最终bundle/hash、其余镜像发布与实际训练pull证明、四Pipeline/Version ID、KFP实际token授权轮换/唯一约束/driver-launcher TLS flags、专用artifact bucket/共享平台身份和bucket-only IAM/TLS/I-O/隔离、native实际安装/行为、Res窗口、新A/B完整验收。null与NOT_RUN均保留，不用旧A资产替代B资格。
环境更新入口为 `http://192.168.25.40:9998/`。已核入口只有主服务image-only更新，缺少独立webhook/step/training目标、bootstrap/挂载配置和材料发布操作。用户要求先列全范围及大小；[完整清单](evidence/08/update-entry-complete-inventory.md)已列两个已构建附加镜像共99.59MiB（本地Size，非压缩传输量）、未构建训练/主服务镜像、支持镜像和全部非镜像动作。用户随后明确批准route1：主服务镜像经入口，其余固定材料/镜像Fedora和指定SSH维护，太慢报告；见[批准](evidence/08/update-entry-scope-approval-20261101.json)。没有把先前“列出来”误当批准。实际代理配置为192.168.2.197:1080，本次proxychains SSH主机22返回Connection refused、经同SOCKS的HTTP查询ConnectionError；原7897失败只是历史诊断，不能冒充用户实际代理。
d807/tree141053367fa4be9a6fb9c764b2d7900b5282be20的689固定输入已逐文件核对，从普通文件context在Fedora构建两个本地镜像：step ID `102a17fcca37414302ee9703e310bae554905349d8f3b6556ac323ea1408a2a9`，webhook ID `c1eb9355492835c7cb4897e01fb5b7c400fad91235e0910024c2ded76725e252`，缺配置真实启动均exit1拒绝；全部binarySHA/localOCI digest/命令/耗时见[构建收据](evidence/08/build/build-receipt.json)。上述为早期LOCAL产物历史；当前四镜像实际发布以固定registry digest清单为准。CPU/GPU现已完成指定测试节点直接构建并发布；固定LF base原有29层完整缓存，未重拉9GB。CPU digest `sha256:31227c3af9d3d0afe5cffddf34d6a3e0a922e29ea7e153892c8b0a0d1ed7f1d5`、GPU digest `sha256:aa1f6ae2159eead52d0d2343d53d785021aa4572dc9978e1af221eb30519eb6e`，均位于受管project `ani-resource-factor10-20261022`；Harbor实际size分别9,369,651,248B/9,369,651,247B。689固定源及step binarySHA一致，CPU/GPU并行实际6.110s/6.437s，无CPU/内存/并发额外限额，registry manifestSHA读回匹配，缺配置启动均exit1拒绝。见[实际构建](evidence/08/image-publication/node-receipt.json)、[Harbor读回](evidence/08/image-publication/node-registry-readback-and-stopped-units.json)。这不构成真实训练验收。step/webhook亦已发布并按registry digest读回，分别为sha256:edc52340b24ef678608c3119c7eb05a95cd9f60505324f59f83dd47ef1f6a600和sha256:98715f788bceeae293c64cc10617190b6cc28c0468824f24e59dac02f3795ec4；新增构建工具和私有目录已清理，逐项SHA/absence证明见[逐项台账](evidence/08/image-publication/added-tools-cleanup-ledger.json)。

专用artifact bucket `ani-md-tenantenv-kfp-20261101` 已创建并启用Versioning；源Secret `ani-system/modeldev-tenantenv-artifact-source-20261101` 原UID `bc62d45a-d83c-4ca8-b5fa-ca07c6cadb47`，精确两键Opaque。独立平台共享账户实际policy读回匹配，只限该bucket，业务input/output/model三bucket GET及admin API均403；原凭据不输出。见[存储/IAM实证](evidence/08/artifact-storage-complete-account-result.json)。首次账户display name超32字符的400保留，核实账户不存在后仅缩短名称，未改policy/凭据/身份。launcher挂载、真实I/O、普通tenant隔离和轮换仍NOT_RUN。

## 5. 旧数据和资源清理

原始只读清单：[old-inventory](evidence/09/old-inventory.json)，SHA256 `aa5420bd1c2ad02f373509d625a99080b4c6721050de1bcdb3496ad1d15a44d2`；50原MD聚合/10input、54Gov接纳/23原charges。它是原始证据，后续原协议状态改变后必须重新采集可执行删除manifest，不沿用旧rowSHA。

停止Create流入/在途lease：Gov维护提交6c17已由指定Workbench job实际部署，源HEAD与受测三文件hash完全匹配，dirty项仅审定构建recipe。新Pod UID `7e3c6bd5-4d76-4b44-92bf-6e3b89d48a6f`、imageID `sha256:2f66dc480e1a45a1a6c2cfc6df2275cd8496eb29a5d9ad5066f2776f02528d05`。实际tenant登录200/Create503 MODELDEV_CREATE_MAINTENANCE，active_create_leases=0，54接纳/23操作/fence与charge SHA前后相等。见[部署终态](evidence/09/workbench-maintenance-6c17-terminal.json)、[源核对](evidence/09/workbench-maintenance-built-source-verification.json)、[真实维护门禁](evidence/09/gov-maintenance-probe-076fbe97-25d6-493a-bf63-adf55a980129.json)。

原3执行已真实退休：`1cce950b-d6e9-453e-aa11-fffd8846224b`、`4a1a92f7-4d94-439d-a8d7-1833f38c7dbc`、`eca4d47b-5911-4f0f-8b45-89c1be775925`。固定651工具真实plan及root审核/fresh503门禁后，原CAS/fence关闭、step完成、compute APPLIED各1次已持久读回；CPU revision22/22，GPU revision13。两条训练结果保持原UNKNOWN+实际143，原gen1STEP_FAILED停止epoch与current gen2USER_STOP都保留，LogsReady=false/captured0，日志如实不可恢复。第三按原UID归档与完整图资格执行。见[真实完成](evidence/09/legacy-retirement/original-stop-three-retirements-complete.json)、[实际续跑输出](evidence/09/legacy-retirement/original-stop-final-maintenance-resume.jsonl)、[durable读回](evidence/09/legacy-retirement/original-stop-final-durable-journals.json)。

第三原GPU event `54c1fb78-da48-4586-990e-333ed8090aef`、receipt `23600748-d80d-5ceb-be35-664a995bb7db`、RESOURCE_RELEASED payload保持不变；实际mTLS ACK=ALREADY_APPLIED，独立退休journal CAS已写。Gov前后证据SHA同为 `2c97a0d72b9755a128f31cdbe6da6a13b83af2ddcd1ff836c9726d5b6684d1aa`，原24570 charge已全退、两GPU账户occupied/all-owner均0→0；原MD anchor/scope仍NULL，没有新seal/outbox。原宿主短DNS未解析，只在维护私有config把原Service UID绑定原VIP/端口，保留CA/client证书/ServerName及RPC；见[传输证据](evidence/09/legacy-retirement/original-stop-gov-private-transport-binding.json)。

四条原UNKNOWN已由正常业务Stop入口实收202并原owner fence ACK/CLOSING，同原命令均一次投递完成，原worker实际CLOSED/USER_STOP/NoDispatch：aux分支 `codex/modeldev-original-command-retirement` / ca01cab已完成真实原四PG、生产mTLS sender边界与完整make check/audit；旧服务分支保持6c17。工具只准先原Stop durable fence ACK，再同原Create envelope/lease/sender/receipt；正常owner CLOSING保持真实，最终CLOSED由原worker产生。四条Gov DispatchACKED/retryfalse，MD closed_at非空、gen1/rev3、NoDispatch=true；pipeline dispatch/confirmedRun/authority均0，训练/workspace/handle/publication无；step/compute/outbox均NULL如实，未伪APPLIED。见[原worker关闭实证](evidence/09/gov-original-command-retirement/original-four-post-delivery-native-20261101T024109Z.json)。现有54个MD聚合须重新采完整manifest，不沿用50聚合删除计划。

另7条旧snapshot的原c99完整codec通过、21bf首guard为training image。该事实影响历史删除资格；正在以独立exact-c99只读有限资格工具核对7个原PK/字节/完整aggregate rowSHA与原close/cleanup/ACK，删除工具只消费root审核的精确证明。新v4与服务运行时不加兼容或放宽guard。其余43/50 MD、47/54 Gov及原四UNKNOWN的21bf完整字节资格通过。

各表历史删除、KFP/S3/其他固定UID清理、旧bootstrap撤销、Res freeze/migration/backfill/release、新v4切换：NOT_RUN。原3的原协议计算清理已实际APPLIED，原四为真实NoDispatch关闭；旧worker现已精确scale0、原PodUID消失，fresh54/54关闭且Gov活动lease0，见[停机实证](evidence/09/old-worker-stopped-proof-20261101T024313Z.json)。其余删除范围仍需完整原资格与新manifest。保留标准和旧Namespace本身。原保留资产4VM/3VMI/34非MD PVC及无关Pod的最终健康比对待后续完整清理完成。

## 6. 验收结果

逐项结果见 [acceptance-results.tsv](acceptance-results.tsv)。PASS均限定证据层级；附件 `verification/` 是此前调查输入，不能充当本轮新实现LIVE验收。19个组件行PASS；AD01/02及LIVE01–11共13行BLOCKED，内部未执行动作明确NOT_RUN，未缩减目标。

A/B标准namespace原身份已核对：A Gov2 / Resource64c5c35a-f554-432c-b4bc-7b4f36f08420 / NS UIDa8d8a472-8a8a-43cf-abf0-0e805bbe67b0；B Gov40 / Resource207328fc-f08d-4dc4-b403-16ec86396ce2 / NS UID8698b0b5-f4c8-4770-85e5-10f25ad7b347。二者均Active、无deletion、network标签正确。

B另经正常CRUD建立专用plan10，完整复制原plan2的12模块和原physical.count=1，仅增加gpu.shared_memory_mib=49140，且仅tenant40引用；原plan2完整前后相同，不写占额账户/charges，见[政策实证](evidence/10/b-gpu-private-plan-apply-result.json)。B专用验收账号user50 `md-tenantenv-20261101` 已经正常platform API创建，沿用既有tenant40/role26并实际登录HTTP200，无新角色/授权，见[收据](evidence/10/b-test-principal-create-result.json)。新environment manage/view权限仍待正式迁移。A/B Ensure时间、同一新服务Pod UID/启动时间、新environment/SA/Experiment原身份：NOT_RUN。原基线ModelDev Pod UID `592bdd86-ab5b-4416-8a2c-07c21ee69f8d` 与旧imageID仅作基线，不能冒充新进程或新环境。

CPU一般训练、B GPU_SHARED微调、运行中Stop：LIVE NOT_RUN；只存在上述明确外部边界替身CPU闭环。真实GPU_WHOLE没有测试，未作其可用声明。

跨tenant业务否定例、token实际轮换前后调用、artifact真实I/O/TLS/授权、webhook故障共存及恢复：LIVE NOT_RUN。21对象schema dry-run未安装policy，不是这些测试。

原释放ACK/账户恒等式：只读原状态通过如第5节；新A/B执行的原释放链和账户对账NOT_RUN。未把collector/HTTP200、环境READY组件测试或CPU替身成功当真实产品验收。

## 7. 风险和边界

- **BLOCKED**：原3退休、原GPU ACK与原四UNKNOWN退休均已完成；7条原codec历史删除资格、完整外部/DB删除、Res窗口仍未完成，阻断新v4切换和A/B验收。
- **已解除的维护候选门禁**：原6a7c721的失败原样保留；旧API候选45ed49已移入相同最小map/fixture修正，原MD/Res pins及go.mod/go.sum未变，完整make check/audit PASS。仅该旧API候选已通过指定Workbench提交，原job不得因查询网络失败重复提交。
- **已解除的执行阻塞**：Fedora本轮恢复，原Gov会话终态为0/2/1，未因观察超时重跑；EN02新增固定25秒预算耗尽续跑测试本轮唯一一次exit0，7条持久receipt续跑同原UID到sealed READY，见 [记录](evidence/03-environment-budget-recovery-record.json) 和 [实际输出](evidence/03-environment-budget-recovery.log)。原草稿NOT_RUN只保留为历史，不能作为当前状态。
- **已解除的最终Gov门禁**：最终commit02f4的完整make check于UTC21:07:47–21:13:38 exit0，含同hash快照audit；实际调用漏洞0、未调用导入包1/required模块10观察保留，Gitleaks无泄漏。原FAIL日志和网络失败仍保留为历史。没有新增角色/套餐授予、改SQL/VM业务逻辑或加扫描忽略。
- **NOT_RUN**：最终v4主服务镜像/部署、最终材料/四定义、launcher真实artifact I/O与权限轮换、native安装与实际typeChecking/行为、完整旧KFP/S3/DB删除、Res真实回填、新A/B训练/Stop/隔离/token轮换/故障恢复/新释放验收。全部四镜像发布、构建工具清理、维护503与原3退休/ACK已有真实证据，不能由此提前升级上述未完成项。
- 共享KFP内部artifact平台身份的实际隔离边界尚未现场验收；Secret挂载/TokenRequest/exec及普通tenant RBAC必须实际否定例通过后才能宣称成立。
- 未实测GPU_WHOLE；真实GPU_SHARED也尚未执行。依赖/权限/Pod ready或模拟向量不证明真实训练。

事实冲突及最小修正：

1. 原source gate只允许baseline migration，与spec新增表/投影冲突。只显式加入0002/0003，并接入相同sqlc/PG输入；不加兼容reader。见contract-freeze。
2. K8s1.35.8首次server dry-run拒绝CEL保留字namespace及异质对象推断；renderer改变量名standardNamespace并显式dyn，固定权限/规则不变；21对象再次dry-run通过。没有用status{}宣称安装typecheck通过。
3. 完整关闭主链实测原Kube限流5/10在第154次调用抛would-exceed-deadline，错误被父GET归为身份冲突。保留30秒预算，三client共享有限20/40 limiter，依赖读取失败保持uncertain；修复后真实业务代码CPU闭环245调用完成，原UID/writer/重试上限不变。
4. 固定KFP2.16的GetVersion.pipeline_spec为protobuf Struct，原多doc YAML bytes不能从GET恢复。按08记录的官方源码，保留不可变bundle内原IR bytes并核对IRSHA，再与官方GET完整结构比较；不重定义IR SHA或跳过读回。
5. 指定Workbench只证实既有build→deploy job合同，没有可安全使用的build-only能力，且现有recipe不含全部所需镜像。影响08镜像准备与09映射；已列全镜像实际大小与全部非镜像动作，用户随后已明确批准route1且要求慢则报告；本地附加构建不冒充发布，不试未知变体或绕过入口。
6. live原Workflow缺失与严格旧退休前置冲突。本轮已找到真正原UID KFP v1归档，但旧关闭协议强制live UID/RV，且第三条image_contract Error节点超出原无进程资格；现成authority重放为no-op。最小可能修正是固定旧base、单execution、先只读dry-run的一次性归档维护合同，先完整核定原plan/owner/PVC/模板/退出/fence，再明确裁定归档来源和精确denial资格；不能伪造live对象、兼容进新runtime或直接SQL CLOSED。两个原训练日志仍未恢复，因此新维护合同也不自动解除全部清理阻塞；该历史缺口已取得用户限定批准；独立维护工具现场资格、原关闭/cleanup和原事件ACK现均已通过，详第5节。

7. Gov原基线GET VM logs已在SQL登记为vm:read，但capability TSV漏1行、覆盖计数过时；按同base/current OpenAPI完整28路与两原SQL的逐项证据同步map和测试计数，保留全部断言。原非秘密负向fixture只加准确分组注释、12值/顺序/断言不变。影响07必需check/audit，最小修正没有新增权限或scanner allowlist；最终完整gate已真实PASS，见 [28路来源](evidence/gov-final/vm-log-map-existing-28-contracts.json)、[fixture同值证明](evidence/gov-final/gitleaks-fixture-grouping-source.json)。

以下为原先提出、现已由用户“无归档”及“批准上述限定的一次性修正”答复收敛的历史裁定范围：两条TRAINING_LOGS_UNAVAILABLE原执行的可信日志归档位置及原Pod UID/完整containerID关联证明；第三条只限原execution、固定原归档SHA的一次性维护合同，明确标注来源的终态归档结合当前完整writer实读，并仅对精确image_contract服务端拒绝+零重试+完整父子/模板证明判定创建前拒绝。原fence、repository CAS、日志保存、cleanup顺序和释放ACK继续保留。本轮已收到明确批准；只允许下列限定例外，仍需真实完整writer/exit/fence/cleanup/原退额证据，不新增runtime兼容。

一次性退休最小修正规则（**用户已明确批准；原3实际资格/cleanup/ACK已通过**，见[批准记录](evidence/09/retirement-amendment-approval-20261101.json)）：用户已确认没有其他原日志/Workflow归档位置，不再重复扩大搜索。只限原execution `1cce950b-d6e9-453e-aa11-fffd8846224b`、`4a1a92f7-4d94-439d-a8d7-1833f38c7dbc`、`eca4d47b-5911-4f0f-8b45-89c1be775925` 和清单内原身份。前两条将真实记录 `TRAINING_LOGS_UNRECOVERABLE`，保留原Pod UID/完整containerID、实际退出、115bytes错误体摘要和数据不可恢复事实；绝不编造日志或写成captured。第三条只接受已核对原UID/固定SHA的官方KFP归档，来源明确标为archive，不伪装live GET；精确image_contract拒绝只在零重试、完整原模板/父子与无进程创建证明成立时纳入。一次性工具必须先输出固定摘要dry-run，再持原identity锁与原generation/revision/fence，补齐完整当前writer/PVC/退出证明，保持原cleanup顺序、UID precondition及GPU原事件/ACK/账户恒等式，才可按明确记录的退休例外推进。不得直接SQL无证据置CLOSED/清账户，不在新runtime添加v3兼容或扩大权限；任一其他资格缺失仍BLOCKED。所需裁定是spec§9与§13.3.4–5中原日志/退出来源的这一次性例外，其他合同不变。

第三条旧GPU原ModelDev的scope/outbox均为空，但Gov已有精确原事件`54c1fb78-da48-4586-990e-333ed8090aef`及receipt，canonical payload SHA已复核。普通Seal会创建不同事件且将DEADLINE理由变为ABORTED_CLEANED，因此维护工具必须保留原RESOURCE_RELEASED事件，在关闭/清理资格齐备后通过既有mTLS接口重投，实收匹配ACK并核对原charges/account恒等式；不伪造新aggregate的scope/outbox。现场已通过原mTLS重投、实收ALREADY_APPLIED并持久化CAS，原charge全退且账户前后零差，见[原事件评估](evidence/09/original-gpu-event-assessment-20261101.json)。

## 8. 交付文件

正式合同/ADR：[contracts](contracts/)、[spec](spec.md)、ModelDev `docs/modeldev-tenant-environments.md` 及 `docs/adr/0001-modeldev-webhook.md`。ADR0002原释放权威未改；Inference未改。

部署/维护工具：ModelDev `deploy/modeldev-environment/{publisher,bundle,render,verify_target,platform_config}.py`、`deploy/modeldev-tenant-policy/`、`deploy/modeldev-webhook/`；Resource `scripts/tenant-namespace-backfill`/`scripts/tenant-namespace-window`；Gov旧API维护开关和有界publisher drain；独立旧base `cmd/ani-modeldev-legacy-retirement` 与 `scripts/legacy-modeldev-data-cleanup`。原只读清单脚本在 `evidence/09/`。没有尚未证实可恢复数据的通用日志导入器。

完整脱敏证据目录：[evidence](evidence/)，文件摘要见 `evidence-manifest.sha256`；原附件 `SHA256SUMS` 只验证未改输入，Results/表/报告修改后不再用它宣称整个工作包原样。`verification/` 明确为附件历史输入。

任务Result：[issues/01–10](issues/)，逐项回填；[验收表](acceptance-results.tsv)、[部署输入](deployment-inputs.json)。本报告遵循原8节模板；总体BLOCKED不表示目标完成。
