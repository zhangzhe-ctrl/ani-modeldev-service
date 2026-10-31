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

- Outcome: BLOCKED。
- Base / final commits: 三仓base见deployment-inputs；Res `71b3ad7a2a88ae531e5e7b80abde2f494ff28d9d`；旧API维护Gov `6a7c721ea113d51497421d7975122ec7ca8105c2`已SSH推送但未部署。
- Changed files / generated artifacts: 只读旧聚合/FK/触发器清单及摘要、Gov原charge/receipt/account对账、原Pod/KFP/Workflow精确诊断、OpenSearch有限字段恢复探测、Res迁移前2行精确Namespace核验、A/B共存资产基线。
- Validation commands / exit codes / required gates: 所有集群调用仅ssh kubercloud@100.64.128.12，五项完整identity一致；DB强制default_transaction_read_only。21个bootstrap/native对象server dry-run全部exit0且before/after原对象不变；无持久策略安装。SQL查询/原对象读取命令和结果在evidence/09及06证据。
- Acceptance IDs / actual values / evidence paths: 旧MD 50执行/10输入、47CLOSED/3OPEN、50confirmed dispatch/44authority；12原GPU outbox均ACK+DeliveredAt；Gov54acceptances（50ACKED、4UNKNOWN且retry_blocked INVALID_COMMAND）、23原MD charges全额released、2账户恒等式均成立。Res原Namespace表2行、namespace_uid列尚不存在，A/B标准名/UID/归属/Active均正确；RS01实际migration/backfill未执行。证据 `evidence/09/old-inventory.json`+SHA、old-gov-inventory.json、open-execution-blockers.md、res-namespace-pre-migration.json、evidence/preserved-assets.json、native-principals.json。
- Deployment or cleanup effects / preserved assets: 未停止原worker、未删旧对象/行/S3/KFP、未迁移或回填Res、未切服务。保留基线4VM/3VMI/34非MD PVC、A/B无关Pod/Sandbox原UID与原健康。
- Remaining blockers / affected dependencies: 原执行1cce950b-d6e9-453e-aa11-fffd8846224b及4a1a92f7-4d94-439d-a8d7-1833f38c7dbc原Pod日志实际只有115字节kubelet取容器日志失败体，原node日志/容器也不可用；第三eca4d47b-5911-4f0f-8b45-89c1be775925的原Workflow不可读。三原KFP Run FAILED，但三Workflow已404，26保留step Pod不替代完整原writer/退出证据。OpenSearch有两日索引但无原UID或完整containerID精确字段；未按名称伪造关联。依§9/§13.3无法原协议排空，因此新v4切换及LIVE03–11受阻。
- Contract conflicts / evidence / minimal correction: 不接受HTTP200 kubelet错误为日志、不用404/FAILED补造退出、不SQL置CLOSED或退款。最小保持方案是从已有外部归档恢复真正原UID日志/Workflow证明，经有界维护导入/原repository CAS继续关闭；当前未取得这种归档、未实现未经证实的数据导入。已向用户询问是否有另外的原UID归档位置，继续不受阻代码和门禁。Gov正式v4 decoder不能承担旧v3排空，另有旧base+仅maintenance开关提交保持旧API；未部署，不把make check基线FAIL当上线就绪。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮当前状态复核：2026-10-31T20:23:19Z至20:23:22Z，完整identity一致；三原execution仍OPEN且close reason/原Pod与containerID、115bytes日志hash均未变；三个原Workflow仍NotFound。无新可推进原关闭证据。证据evidence/09/open-current-recheck-20261031T202319Z.json；未重跑全量inventory/OpenSearch，未进行持久写。

第二轮新原证据：官方v1 GetRun detail精确返回3原Run的6份manifest，原WorkflowUID/name/Namespace匹配，35/35/25节点终态，但无直接PodUID或原日志archive URI。第三条原CloseAuthority已存在、4原Pod退出已核；原live UID/RV读取、image_contract Error资格及完整writer/PVC/模板证明仍不满足，单纯authority重放no-op。详evidence/09/kfp-v1-original-run-detail-20261031T204206Z.json及third-archive-close-assessment.md。未做CAS/重建/清理；与此前‘未建立归档来源’不同，当前具体断点是原关闭不支持该archive证据及额外资格缺口。
