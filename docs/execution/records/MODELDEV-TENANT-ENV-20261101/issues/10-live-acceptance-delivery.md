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

- Outcome: BLOCKED；19组件行PASS，AD01/02及LIVE01–11仍BLOCKED；没有真实A/B成功声明。
- Base / final commits: 三仓与独立维护分支映射见implementation-report.md/deployment-inputs.json；固定API源d807/71b3和Gov正式pin未变。
- Changed files / generated artifacts: 验收表/部署输入/八节报告、原身份与保留资产清单、B IAM与GPU政策准备、精确旧资格和更新范围收据。
- Validation commands / exit codes / required gates: 只复用已完成必要组件/真实隔离PG/gate证据；未因文档更新重跑全量。旧维护完整gate实际FAIL保留。Live新服务启动、Ensure、训练、Stop、下载/隔离/轮换/共存故障测试NOT_RUN。
- Acceptance IDs / actual values / evidence paths: A Gov2/Resource64c5c35a-f554-432c-b4bc-7b4f36f08420；B Gov40/code e2e-full-20261029/Resource207328fc-f08d-4dc4-b403-16ec86396ce2，原标准NS UID均已核。B现有role26具备原模型/训练API权限，尚未安装新environment manage/view权限；B无既有MD input，B合法model归属UNKNOWN，GPU_SHARED政策缺失，不借用A资产或直接扩权。见evidence/10/b-principal-preparation.json。
- Deployment or cleanup effects / preserved assets: 新A/B账号、quota配置及Ensure尚未创建/执行；其他owner资产未改。没有用同名旧ModelDev Pod或旧A训练当新共享进程验收。
- Remaining blockers / affected dependencies: 09原退休/精确清理/Res窗口和08实际材料/策略/挂载/服务切换，B合法输入/基模/必要测试政策，再跑全部LIVE01–11。当前网络间歇失败如实留证，不缩减目标。
- Contract conflicts / evidence / minimal correction: 更新route1已明确批准；仅额外发布/维护路径改变，唯一个集群入口、完整身份核验、无新RPC/无v3兼容/原Reserve-fence-release和Inference边界保持。

## Comments

以下为按时间保留的历史记录；当前状态以本文件 Result 为准。

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮补齐EN02真实25秒预算续跑与Gov真实JWT/Redis/Casbin/PG→HTTP→mTLS peer正向/撤权组件集成，均exit0；19项组件行PASS不代替AD/native或11项LIVE。Gov原default/race exit0、make check exit2、audit恢复后续一次exit1已取得，失联/未知状态已解除。新发现官方v1原UID归档，但第三原live/denial/writer资格仍不满足，两原训练日志仍缺；详新的09归档评估，不再泛称所有Workflow归档不存在。

当前续跑状态：用户已批准报告§7限定一次性退休修正（两日志不可恢复如实记载、原UID archive明确来源）；原先待裁定问题已答复，不再要求补不存在的归档。现场仍须原exit/writer/PVC/fence/CAS/cleanup/原事件ACK，未通过。旧API维护45ed49完整make check/audit PASS且原Workbench job eda85e66a4f24c3895b7be87a5211500已提交；目前指定SSH连接关闭与本地SOCKS GET连接关闭，最终job/503未确认，无A/B真实PASS。当前证据见evidence/09/retirement-amendment-approval-20261101.json、entry-connectivity-20261101.json、workbench-local-socks-connectivity-20261101.json。
