# 10 指定集群 A/B 验收、旧代码清理与交付

Status: ready-for-agent
Resolution: open
Blocked by: 09

## Goal

完成指定环境全部必要合同与A/B真实矩阵，核对清理和共存边界，形成可追溯的最终实施交付。
依据：[spec.md](../spec.md) §13.5、§15、§17；只有全部必要证据满足才将相关任务resolved，未测不等于通过。

## Scope

- 三仓本次相关代码/配置/测试/正式文档与09新部署；只在既定测试集群使用合法A/B身份。
- ModelDev CONTEXT.md、正式环境/API/部署合同、ADR0001过时静态描述；ADR0002原release权威保留。
- 包内 acceptance-results.tsv、deployment-inputs.json、implementation-report.md、证据目录及各issue Result。
- 09的清理manifest、原charge/release ACK、保留共存资源、四定义/IR/bundle和最终部署imageID记录。
- 按仓库要求在批准远端运行定向集成与make verify；本地仅轻量编辑，不扩其他集群/账户/Inference。

## Implementation

1. 复核01–09实际Result及所有NS/EN/API/SNP/ST/DP/CL/IM/RS/AD组件行，缺证据补定向验证而非沿用设计声明。
2. LIVE01/02：重核五项身份、完成部署输入与旧manifest，保存策略状态/无CEL warning/正反server dry-run和最小RBAC证据。
3. LIVE03：同一进程Ensure A再B、A并发重放及Get；记录服务Pod UID/启动时间不变、各一READY与Experiment。
4. LIVE04：真实投影control token读取A/B Experiment及ani-system父Pipeline/Version；正确SAR与错误audience/未授权目标否定例。
5. LIVE05：保持同一进程观察实际token轮换，完成其他测试后再次LIVE04；只记录非敏感代际/期限，调整合法TTL必须恢复。
6. LIVE06：A小型GENERAL_TRAINING CPU成功，B小型FINETUNING GPU_SHARED成功，完整记录原UID链、I/O/logs/publish/Model及B GPU释放ACK。
7. LIVE07/08：B另一次小CPU运行中合法Stop且A查询可用；A访问B详情/日志/Stop/artifact授权及至少一次反向访问均拒绝无副作用。
8. LIVE09：实际受管镜像pull、STS跨scope否定、普通tenant原groups对Secret/挂载/reserved SA/exec/TokenRequest的边界验证。
9. LIVE10：短维护窗口受控webhook不可达，普通VM/Instance代表Pod与MD Pod server dry-run；立即恢复并记录探针，不创建多余VM。
10. LIVE11：复核原清单/旧表/Gov commands/旧配置引用为零及全额退额；共存VM/Sandbox/无关PVC UID和健康保持。
11. 删除旧静态binding/facts/STS数组/webhook上限/按tenant全局策略脚本/旧allowlist/v3 fixture等残留，更新文档并完成必需仓库gate。
12. 填每行PASS/FAIL/BLOCKED/NOT_RUN和证据路径、实际值；记录最终commit/API pin/image映射/材料摘要/保留资产，再更新真实完成任务Result。

## Acceptance

- NS01、NS02、EN01–EN04、API01–API02、SNP01、ST01–ST02全部有有效证据，不以live成功替代关键负向合同。
- DP01–DP03、CL01–CL03、IM01、RS01、AD01–AD02全部通过，GPU_WHOLE仅列实际完成的定向回归。
- LIVE01–LIVE05证明目标正确、策略/权限、同进程A/B开通与实际token轮换；无敏感token或Secret值输出。
- LIVE06–LIVE09证明CPU训练/GPU_SHARED微调、Stop、跨租户拒绝、镜像/存储/Secret边界及原release链。
- LIVE10–LIVE11证明webhook故障隔离、即时恢复、旧资源清零/账目及共Namespace其他owner健康。
- 最终三仓提交、正式API pin、生成物、部署映射、全部输入、任务Result和实施报告一致；所有必要行满足后才声明完成。

## Constraints

- 没有合法GPU_SHARED preset/容量或B基模则具体BLOCKED，不能换CPU宣称全链通过；不声称未实测GPU_WHOLE。
- 不删除真实Namespace/SA做UID负例，不实际创建额外VM；UID重建用fake API/事务fixture。
- 不手造token、靠重启冒充轮换或长时间循环sleep；不以直接伪造tenant header替代实际Gov授权。
- 不把旧报告UNKNOWN升级成通过，不把NOT_RUN/BLOCKED记PASS，不用单次旧A CPU代替新A/B矩阵。
- 不保留无用旧兼容入口，不扩共享owner策略/ACC/Model协议；必要外部阻塞写精确对象/调用/原因和剩余项。

## Result

- Outcome: BLOCKED；Resolution保持open。ModelDev/Resource源码及门禁、真实PG的CPU外部边界替身纵向已完成；最终Gov默认race/make check/audit结果因Fedora SSH失联未取回，EN02固定25秒续跑补测未运行；09存在三条OPEN旧执行，严格清理前置未满足，未部署新系统，不能完成真实A/B LIVE验收或宣称整个方案完成。
- Base / final commits: ModelDev base 21bf4bd6540d462a61c91b629f26250899583223；最终已测试源码提交 d807cbdbd5dd6664d61480acedc752e19cfca48a，evidence/modeldev-tested-source-identity.json记录matched=true、different_files=[]。三仓正式提交/API pin/推送与部署映射由主任务最终交付统一列示；本Result不伪造未发生的imageID或发布ID。
- Changed files / generated artifacts: CONTEXT.md、configs/examples/managed-runtime.yaml、docs/modeldev-tenant-environments.md、docs/modeldev-webhook.md、docs/runtime-verification.md、docs/adr/0001-modeldev-webhook.md及相关当前facts/Prepare-Finalize与历史设计标识。移除当前示例中的binding_file/binding_sha256/dispatch_batch_size和STS租户数组说明，补必需catalogue/Resource接线；ADR0002原释放权威未改。精确范围 evidence/10-source-doc-residue.md；本项Result不改acceptance表或主报告。
- Validation commands / exit codes / required gates: 最终Fedora ModelDev make verify PASS，含生成一致性、全仓Go测试、vet/build/mod verify/diff check（evidence/modeldev-final-make-verify.log）。真实PG+生产业务+仅外部KFP/Kubernetes/S3边界替身纵向PASS：48步实际CPU MLP、文件发布核验、CLOSED持久化、原6个step及compute清理、日志重连查询和跨tenant拒绝；坏CSV不训练/不发布并关闭（evidence/04/modeldev-cpu-mainflow-final-shared-budget.log）。KFP/cleanup定向整包exit0（evidence/10-kfp-cleanup-regression.log）。示例严格YAML→protobuf→Bootstrap.Validate及旧binding_file拒绝PASS（evidence/10-example-config-check.log）。这些均非指定集群A/B训练验收。
- Acceptance IDs / actual values / evidence paths: 本次EN组件与SIGKILL恢复证据 evidence/03-environment-component-tests.log、evidence/03-environment-process-recovery.log；EN02预算恢复新增草稿NOT_RUN见evidence/03-environment-budget-recovery-record.json；AD/IM组件证据 evidence/06-webhook-component-tests.log；四IR/bundle工具证据 evidence/08/tools.log；LIVE01目标身份只读子项 evidence/native-principals.json，LIVE02仅server schema子项21/21 evidence/06-native-server-dryrun.fixed.json。LIVE01/02完整部署/安装/观测typeChecking与正反矩阵未完成，LIVE03–LIVE10真实同进程A/B开通、token轮换、CPU/GPU_SHARED微调、Stop、隔离与webhook故障共存均NOT_RUN；LIVE11旧资源/数据清零BLOCKED。不得将局部PASS升级为整行LIVE PASS。
- Deployment or cleanup effects / preserved assets: 没有完成旧聚合/资源清零、新MD/webhook/三VAP持久安装、四定义正式发布或新A/B实际训练。server dry-run未改变任何原对象（evidence/06-native-server-dryrun.fixed.json）；保留资产清单 evidence/preserved-assets.json，切换后的UID/健康复核仍NOT_RUN。没有删除Namespace、批量移除finalizer、SQL清零占额或修改Inference。
- Remaining blockers / affected dependencies: evidence/09/old-inventory.json为50执行/47CLOSED/3OPEN；1cce950b-d6e9-453e-aa11-fffd8846224b与4a1a92f7-4d94-439d-a8d7-1833f38c7dbc为TRAINING_LOGS_UNAVAILABLE（原Pod存在且原control日志GET200，但实际115字节kubelet错误体，无有效日志）；eca4d47b-5911-4f0f-8b45-89c1be775925为KFP_CREATE_UNRESOLVED，原Workflow404。三原KFP Run FAILED不等于完整退出证明。精确证据 evidence/09/open-execution-blockers.md、evidence/09/open-readonly-protocol-evidence.json；原charge/命令清单 evidence/09/old-gov-inventory.json。原UID和原完整containerID的有界归档恢复字段探测分别BLOCKED，见 evidence/09/opensearch-original-uid-result.json、evidence/09/opensearch-original-container-result.json。未恢复真实原日志/Workflow证据前不得跳过spec§9、§13.3原关闭/日志/清理/release资格；因此09清理、部署和本项LIVE均受阻。
- Contract conflicts / evidence / minimal correction: 原IR byte SHA与KFP Struct返回的事实差异采用03/08已记录的最小修正；CEL保留字/异构map仅做06语法修正，未放宽规则。旧执行的原日志/Workflow证据缺失是现场阻断，未伪造空日志、退出/CLOSED或退款资格，未改原身份方案。可能恢复仅限取得真实原证据后按原身份与CAS完成有限维护；当前未建立可用恢复来源，保持BLOCKED。附件verification目录和其他历史PASS仅为原始输入，不充当本次实际验收；本次每个PASS只引用evidence/中的本次原始输出。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。
