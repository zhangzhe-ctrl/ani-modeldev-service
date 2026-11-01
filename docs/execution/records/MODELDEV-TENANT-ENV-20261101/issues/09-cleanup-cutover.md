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

- Outcome: BLOCKED；没有执行真实旧资源/数据库删除、Res live窗口或新v4切换。
- Base / final commits: 独立旧base维护工具ae42fb0（f7有限诊断后补精确原Service传输）；DB两阶段工具e682a29；Res窗口67dceef；旧API Gov32c61。均正常SSH推送，正式运行API pin不变。
- Changed files / generated artifacts: `cmd/ani-modeldev-legacy-retirement`、原资格的有限维护扩展；`scripts/legacy-modeldev-data-cleanup`；Res `scripts/tenant-namespace-window`；完整脱敏原清单/批准/原事件/部署维护job收据。外部逐UID/对象版本清理CLI仍在实现，不把原ApplyExecutionCleanup当完整清理。
- Validation commands / exit codes / required gates: 旧维护初始9c2受影响真实PG/定向/vet/build PASS；完整旧base make verify实际FAIL保留（固定依赖API/原fixtures/materials panic），没有以局部PASS覆盖。f7有限诊断cmd/runtimeproof定向/vet/build exit0；见`evidence/09/legacy-retirement/final-stage-source-identity.json`。DB工具Fedora真实PG/race、vet/build/modverify于UTC00:18:07–00:18:30全exit0，12源码本地/远端一致，见`evidence/09/legacy-data-cleanup/delivery.json`。Res4项真实PG及完整verify exit0，见res-uid-window-final.json。所有live apply NOT_RUN。
- Acceptance IDs / actual values / evidence paths: 固定原清单50MD/10input/47CLOSED+3OPEN、54Gov acceptance/23原charges，12旧outbox已ACK，账户恒等式仅原只读状态。精确3条例外已由用户批准，见retirement-amendment-approval-20261101.json。最终真实只读plan UTC00:43:07第三条Ready=true，原owner/archive/当前writer/PVC/task与原Govreceipt/account资格通过；其余两条仍CURRENT_ORIGINAL_TRAINING_EXIT，整体Ready=false。原宿主DNS缺记录的传输已仅在维护cmd补齐，原URL/CA/SNI/token保持，必要定向/vet/build exit0；见legacy-retirement/final-route-source-identity.json和final-route-live-readonly-plan.json。不放宽原guard。f216实际普通GET503根因尚未证实；四UNKNOWN原canonical形状未新取得，retry_blocked不会因Stop自动清除；见original-owner-close-qualification-20261101.json。它们不在3条批准中。
- Deployment or cleanup effects / preserved assets: 原Gov job eda85e66实际verify FAIL，未build/deploy；最小UTC fixture32c61新job8d8b448e最后成功查询UTC00:43仍RUNNING verify，run_verify=true；后续网络失败不重复POST。实际新Gov image/503/lease drain NOT_RUN。旧MD worker未停、旧UID对象/行/S3/KFP删除0、Res迁移/回填0。保留4VM/3VMI/34非MD PVC/A/B无关Pod原基线；清理后健康比较NOT_RUN。
- Remaining blockers / affected dependencies: 真实停入/关闭/退出/writer/PVC/cleanup/原GPU ACK、f216及4UNKNOWN原owner资格、外部固定对象/版本manifest与清理、正式Res窗口、材料/策略/挂载/新v4切换及A/B全部仍未完成。仅同一原canonical在先durable fence ACK后由原队列重投可能合法，不能替换ID/重发不确定Run；Gov与MD都称v3但具体training_image合同不同，必须核原字节。
- Contract conflicts / evidence / minimal correction: 只限精确3条的日志不可恢复/原UID archive与精确denial例外；不伪造原事实、不新v4兼容、不改释放事件。用户另已批准route1：主服务镜像经Workbench，固定附加镜像/材料Fedora与指定SSH维护，太慢报告；见`evidence/08/update-entry-scope-approval-20261101.json`。原身份、资格和门禁不变。

## Comments

以下为按时间保留的历史记录；当前状态以本文件 Result 为准。

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮当前状态复核：2026-10-31T20:23:19Z至20:23:22Z，完整identity一致；三原execution仍OPEN且close reason/原Pod与containerID、115bytes日志hash均未变；三个原Workflow仍NotFound。无新可推进原关闭证据。证据evidence/09/open-current-recheck-20261031T202319Z.json；未重跑全量inventory/OpenSearch，未进行持久写。

第二轮新原证据：官方v1 GetRun detail精确返回3原Run的6份manifest，原WorkflowUID/name/Namespace匹配，35/35/25节点终态，但无直接PodUID或原日志archive URI。第三条原CloseAuthority已存在、4原Pod退出已核；原live UID/RV读取、image_contract Error资格及完整writer/PVC/模板证明仍不满足，单纯authority重放no-op。详evidence/09/kfp-v1-original-run-detail-20261031T204206Z.json及third-archive-close-assessment.md。未做CAS/重建/清理；与此前‘未建立归档来源’不同，当前具体断点是原关闭不支持该archive证据及额外资格缺口。

用户最新明确答复：无其他原日志/Workflow归档，并批准报告§7限定一次性退休修正，见[evidence/09/retirement-amendment-approval-20261101.json](../evidence/09/retirement-amendment-approval-20261101.json)。上述日志/归档不满足原协议的内容保留为历史发现；现由独立旧base维护工具如实记录两条不可恢复日志与原UID archive，未解除其他writer/exit/fence/CAS/cleanup/原释放资格。工具定向验证已运行，现场只读plan和apply仍NOT_RUN。

当前现场具体阻塞是指定SSH入口连接关闭/单文件传输超时，及本地SOCKS对指定更新入口仍RemoteDisconnected；原job只查询、不重复POST。证据见[SSH](../evidence/09/entry-connectivity-20261101.json)、[SOCKS](../evidence/09/workbench-local-socks-connectivity-20261101.json)。原GPU事件独立维护决策见[原事件评估](../evidence/09/original-gpu-event-assessment-20261101.json)，尚无实际新ACK。部署后[维护门禁探针](../evidence/09/gov-maintenance-live-acceptance.py)只准备未执行，不作为PASS。

原更新任务恢复查询确定verify FAIL（UTC22:04:07），未进build/push/deploy，精确失败为timeutil日期fixture受host DST时区影响；见evidence/09/workbench-maintenance-verify-failure-20261101.json。修复仍保留原run_verify=true和原失败记录，不重复原job。
