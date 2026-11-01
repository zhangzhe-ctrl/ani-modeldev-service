# 09 原环境关闭清理、Res 回填与一次切换

Status: ready-for-agent
Resolution: open
Blocked by: 02, 05, 06, 07, 08

## Goal

按原关闭/释放协议排空并精准清理旧ModelDev，再完成共享Namespace UID维护和仅v4新系统切换。
依据：[spec.md](../spec.md) §1、§13、§16；本项是唯一旧环境清理/运行切换任务，保留共Namespace其他owner资产。

## Scope

- 三仓本次已完成提交、旧ModelDev/Gov原聚合和指定测试集群；写前核对五项集群身份。
- ModelDev原Stop/close/cleanup/GPU release和一次性manifest维护工具；Gov精确Create维护入口与publisher生命周期。
- 02的Res nullable UID migration/精确回填命令及新Res配置；既有Network/VM/Instance记录保留。
- 08准备的四定义/镜像/bundle、06固定bootstrap/native policy、主服务与webhook部署；旧配置入口的移除。
- 只操作精确manifest原ID/UID/对象版本；记录清理摘要、FK/行数、quota恒等式与部署映射。

## Implementation

1. 核对指定入口/context/API/cluster_id/kube-system UID；不一致停止写。生成非敏感旧资源清单和摘要，维护工具先只输出计划。
2. 精确停止Gov ModelDev新Create与Create publisher新claim，等待在途lease；原Stop/close/cleanup/release继续，逐原operation对账。
3. 固定原tenant/operation/execution/spec/dispatch/Run/Workflow/namespace及子UID、存储引用、charge/outbox/ACK，区分独占与外部引用资产。
4. 原尚未发送/未知/已发送分别走原fence与Stop/close；先停止原创建者，持久step退出，按原UID/RV处理自有finalizer。
5. 先保存日志，再按原TrainJob→JobSet→Job→Pod→PodGroup→合格临时PVC→Workflow→TrainingRuntime顺序cleanup，保留writer检查和3次上限。
6. 符合原资格后seal原GPU scope+outbox并取得Gov匹配ACK；scope-without-outbox仅按ADR0002一次性journal原事件结清，不SQL清零。
7. 核验每charge全额释放和账户恒等式、原ACK落盘；有未决创建/写者/账目即保留对应证明并BLOCKED，不继续破坏其依据。
8. 停止旧MD worker和Gov旧项重放，确认无active lease/迟到Create；再删manifest内独占旧PVC/配置/SARB/KFP/S3版本，保留共享及外部资产。
9. 按spec九表FK顺序删除精确旧聚合；Gov在实际FK受控事务删全释放原项，保留账户/租户/角色/套餐/政策与其他owner。
10. Res小窗口停新Network身份接纳并等在途→新增UID列→精确回填验零NULL→上线拒NULL/不自动建NS新Res→恢复写，禁止旧Res再写NULL。
11. 核验旧可恢复执行/可投递command/绑定配置为零；按08工具发布四定义并回读ID/digest，先native策略验证再授create/bind及部署新服务。
12. 记录commit→image digest→imageID；装同一bundle并显式Ensure A再B；未满足前置保持BLOCKED，交10继续完整LIVE验收。

## Acceptance

- CL01、CL02、CL03：原关闭、writer/UID保护及GPU事件/ACK完整；不以Get404或物理GPU空闲代替原资格。
- RS01：回填前后精确计数、既有UID不覆盖、零NULL；运行入口拒NULL，旧Res不会回填后继续写空。
- LIVE01、LIVE02：目标身份和部署输入有证据，native模板无编译/type-check warning，实际正反dry-run满足固定合同。
- LIVE03：同一新MD/webhook部署可先A后B显式开通；完整并发/不重启证据由10补齐，未达READY如实阻塞。
- LIVE11：manifest旧资源、九表旧行和Gov可投递项为零；原charge全退，A/B原VM/Sandbox/无关PVC UID与健康保持。
- 切换记录含精确manifest摘要/删除数量/保留资产/发布IDs/最终镜像映射；LIVE04–LIVE10不在本项伪记PASS。

## Constraints

- 不删标准或旧Namespace本身，不DeleteCollection/整桶/全库TRUNCATE CASCADE，不以清空其他owner资源达到计数目标。
- 不跳过原release ACK，不SQL改occupied_units，不加ACC ACK前置，不重发不确定Run或更换原operation。
- 不将现有ApplyExecutionCleanup成功当PVC/S3/DB已全清；KFP DeleteRun2xx/Get404不证明Workflow删除。
- 未排空旧执行前不撤销旧运行所需权限、替换破坏性全局策略或上线仅v4读取其旧数据。
- 不保留legacy worker/reader/双写/旧binding导入；事实冲突留证，不暗自放宽身份或授权。

## Result

- Outcome: BLOCKED；原3执行已真实完整退休，原GPU事件ACK与账户恒等式通过；其余原命令退休、完整历史删除、Res窗口与v4切换仍未完成。
- Base / final commits: Gov旧API维护6c17f010、独立原命令辅助ca01cabf；旧MD原3退休651f5ea1、历史c99有限资格/DB工具1464b131；Res窗口67dceef0。均SSH正常推送，运行API pin d807/71b3不变。
- Changed files / generated artifacts: 固定原UID/字节/plan摘要的独立CLI、精确DB与外部清理工具、Res窗口与完整脱敏收据；无SQL CLOSED/ACK、无v3运行时兼容、无新RPC。
- Validation commands / exit codes / required gates: Gov6c17及ca01完整make check/audit exit0；原命令生产sender/真实PG与旧MDfence/noSend真实PG通过。旧MD651原受影响测试/vet/build通过，但完整旧base make verify实际FAIL保留，不能声称通过。1464有限c99工具/DB消费真实PG/race/vet/build/modverify通过，Res4项真实PG及完整verify exit0。
- Acceptance IDs / actual values / evidence paths: 原1cce950b、4a1a92f7、eca4d47b均真实CLOSED，step/compute cleanup APPLIED；两日志如实TRAINING_LOGS_UNRECOVERABLE/LogsReady=false，第三只用固定原UID archive及精确denial资格。原事件54c1fb78…实际mTLS ALREADY_APPLIED并持久CAS，24570原charge全退，账户occupied/all-owner前后0；见evidence/09/legacy-retirement/original-stop-three-retirements-complete.json、original-stop-final-durable-journals.json。
- Deployment or cleanup effects / preserved assets: Workbench唯一job3b191f8ee306437ea5cdcbba6520399b实际SUCCEEDED，源6c17，Govimage sha256:2f66dc480e1a45a1a6c2cfc6df2275cd8496eb29a5d9ad5066f2776f02528d05、PodUID7e3c6bd5-4d76-4b44-92bf-6e3b89d48a6f；真实Create503/lease0/54acceptance与23operation和charges前后相同。四UNKNOWN经真实tenant-a/admin正常Stop均202、原owner fence ACK/CLOSING、原MD admission/runtime0，四原命令均一次deliver成功、DispatchACKED，原worker实际closed_at非空/USER_STOP/NoDispatch。原worker经UID/RV/replicas/image精确test patch scale0，原PodUID已消失；fresh54/54关闭且Govlease0，见old-worker-stopped-proof-20261101T024313Z.json。见evidence/09/four-original-normal-http-stop-resume-after-202-result.json与gov-original-command-retirement/gov-aux-first-post-stop-readonly-plan.json。其他owner资产基线保留。
- Remaining blockers / affected dependencies: 四条原命令现已同原ID/actor/acceptedAt/字节接收并native关闭；新增4行后必须fresh54MD manifest，不能沿用50行计划。7条c99完整历史资格工具已通过实读，实际删除资格/外部固定对象版本清理、DB受控删除、Res live冻结/迁移/回填/释放、新v4部署与A/B仍未完成。
- Contract conflicts / evidence / minimal correction: 原3只采用用户明确批准的限定日志/归档/精确denial修正，所有其他UID/writer/PVC/fence/CAS/cleanup/原事件ACK保持。维护CLI原短DNS缺失仅使用同原ServiceUID/VIP和原CA/SNI/cert，不改集群DNS或身份；首次未知结果只读对账后同原事件恢复。

## Comments

以下为按时间保留的历史记录；当前状态以本文件 Result 为准。

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮当前状态复核：2026-10-31T20:23:19Z至20:23:22Z，完整identity一致；三原execution仍OPEN且close reason/原Pod与containerID、115bytes日志hash均未变；三个原Workflow仍NotFound。无新可推进原关闭证据。证据evidence/09/open-current-recheck-20261031T202319Z.json；未重跑全量inventory/OpenSearch，未进行持久写。

第二轮新原证据：官方v1 GetRun detail精确返回3原Run的6份manifest，原WorkflowUID/name/Namespace匹配，35/35/25节点终态，但无直接PodUID或原日志archive URI。第三条原CloseAuthority已存在、4原Pod退出已核；原live UID/RV读取、image_contract Error资格及完整writer/PVC/模板证明仍不满足，单纯authority重放no-op。详evidence/09/kfp-v1-original-run-detail-20261031T204206Z.json及third-archive-close-assessment.md。未做CAS/重建/清理；与此前‘未建立归档来源’不同，当前具体断点是原关闭不支持该archive证据及额外资格缺口。

用户最新明确答复：无其他原日志/Workflow归档，并批准报告§7限定一次性退休修正，见[evidence/09/retirement-amendment-approval-20261101.json](../evidence/09/retirement-amendment-approval-20261101.json)。上述日志/归档不满足原协议的内容保留为历史发现；现由独立旧base维护工具如实记录两条不可恢复日志与原UID archive，未解除其他writer/exit/fence/CAS/cleanup/原释放资格。工具定向验证已运行，现场只读plan和apply仍NOT_RUN。

当前现场具体阻塞是指定SSH入口连接关闭/单文件传输超时，及本地SOCKS对指定更新入口仍RemoteDisconnected；原job只查询、不重复POST。证据见[SSH](../evidence/09/entry-connectivity-20261101.json)、[SOCKS](../evidence/09/workbench-local-socks-connectivity-20261101.json)。原GPU事件独立维护决策见[原事件评估](../evidence/09/original-gpu-event-assessment-20261101.json)，尚无实际新ACK。部署后[维护门禁探针](../evidence/09/gov-maintenance-live-acceptance.py)只准备未执行，不作为PASS。

原更新任务恢复查询确定verify FAIL（UTC22:04:07），未进build/push/deploy，精确失败为timeutil日期fixture受host DST时区影响；见evidence/09/workbench-maintenance-verify-failure-20261101.json。修复仍保留原run_verify=true和原失败记录，不重复原job。
