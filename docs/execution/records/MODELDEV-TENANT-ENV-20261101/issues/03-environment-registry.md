# 03 环境登记、Ensure/Get 与有限开通

Status: ready-for-agent
Resolution: open
Blocked by: 01, 02

## Goal

实现 ModelDev 持久环境登记和显式有限开通，使同一受信tenant+cluster总是恢复同一环境。
依据：[spec.md](../spec.md) §3–5、§6.1、§10；采用01固定API和02动态镜像合同，不引入后台开通器。

## Scope

- 仅 ani-modeldev-service；internal/biz/tenant_environment.go 与必要领域port。
- internal/data/tenantenvironment/：migration、queries、repository、专用连接锁、typed receipts、kubernetes.go、kfp.go。
- internal/service/tenant_environment.go、internal/server/governance_command.go、gRPC注册和精确verified methods。
- cmd/ani-modeldev-service/{runtime,runtime_operations,model_connection,app}.go、internal/conf/v1/{conf.proto,validate.go}。
- 只实现spec固定SA/RB/CM/专用artifact Secret及Res镜像引用适配；平台bootstrap模板由06/08负责。

## Implementation

1. 建立唯一表 modeldev_tenant_execution_environments；environment_id就是BindingID，unique(tenant_id,cluster_id)，四state与有限phase固定。
2. 短事务INSERT ON CONFLICT+GET先登记，丢弃竞争失败方随机ID；外部操作前按固定算法取得专用连接advisory lock并重新读行。
3. 推进预算25秒、单上游5秒；不跨外部调用持SQL事务；解锁用独立≤2秒context，状态不可信则丢弃连接。
4. 仅信Governance认证方法与当前tenant范围；Ensure仅operation_id，Get空请求；严格拒绝protobuf未知字段和非法枚举。
5. 按NAMESPACE→RESOURCES→EXPERIMENT→VERIFY→PUBLISH→COMPLETE推进；先读标准Namespace并固定原UID。
6. 固定对象先持久意图再GET，仅NotFound可Create；任何响应后读回精确owner/spec/原UID，未知结果不换名。
7. 调用Res获取单受管authority镜像引用；按固定SA规则配置refs；在首次写入/轮换专用artifact两键Secret前完成contracts/authorization-inventory.md的只读授权边审查；支持原UID值轮换。
8. Experiment固定md-env-<environment_id>与规范描述；持久原请求、处理完整分页，未知POST仅在唯一性已证实条件下有限同请求重试。
9. 平台control token每个有界请求重新读投影文件；未有binding时使用私有平台身份路径，不伪造空Run绑定。
10. 核对原对象、Experiment、ani-system四定义、精确bootstrap材料；不授Ensure任意SAR/impersonate或创建测试Run。
11. 先固定binding规范字节，创建immutable投影并读回，再CAS READY；不含投影自身UID/RV、Secret值/摘要或可变状态。
12. Get只返回持久观察且无行404；Busy返回真实已登记state；预算/失败保留receipts供显式Ensure恢复，不隐式重新绑定。

## Acceptance

- EN01：真实PostgreSQL并发不同operation仅一行/同一environment_id；锁忙不改写READY/BLOCKED/NEEDS_REVIEW。
- EN02：各Create响应丢失及进程中断后精确恢复原对象；foreign-owner/换UID拒绝；多次有界调用能继续receipts。
- EN03：完整分页零/一/多匹配、未知POST及未证实唯一性分支均符合spec，每次Ensure最多一次Experiment POST。
- EN04：seal→投影→readback→CAS顺序、摘要无自引用/未知字段拒绝；Get无登记404且不插入。
- NS02、API01：错误Namespace零子资源写；精确trusted方法/unknown fields/预算生效；token逐请求重读有受控测试；普通主体危险授权边、审查不完整均阻塞Secret写入。
- API02：Ensure/Get不创建Execution、acceptance或charge；真实A/B开通和artifact I/O留09/10，不伪称组件证据为LIVE通过。

## Constraints

- 不实现环境worker、lease/heartbeat、新队列、Delete/Rebind/UpdateProfile接口或自动补偿删除。
- 不CREATE/PATCH/Adopt Namespace，不替换已固定Namespace/SA UID、Experiment、scope或provision_spec。
- 不复制业务STS/管理员存储凭据作KFP artifact身份，不在DB/投影/日志/回包保存秘密或其摘要。
- 不上传Pipeline/Version、不创建租户Profile、不发用户token，不将平台固定cluster开放为请求字段。
- 不触碰旧环境在线部署或旧v3数据；迁移和切换由09按原关闭/释放顺序执行。

## Result

- Outcome: COMPONENT_IMPLEMENTED / VALIDATION_BLOCKED；Resolution: open。EN02固定25秒预算耗尽后续跑的直接补测因Fedora SSH失联NOT_RUN，不能宣称本项全部验收完成。显式 Ensure/Get、真实持久化、typed receipts恢复及最终仓库门禁均已有本次证据；不表示指定集群A/B开通、artifact I/O或新服务上线完成。
- Base / final commits: ModelDev base 21bf4bd6540d462a61c91b629f26250899583223；最终已测试源码提交 d807cbdbd5dd6664d61480acedc752e19cfca48a，对应 evidence/modeldev-tested-source-identity.json（matched=true、different_files=[]）。Res依赖正式pin 71b3ad7a2a88ae531e5e7b80abde2f494ff28d9d；推送/部署镜像映射由主任务统一记录，本项不声明已部署。
- Changed files / generated artifacts: internal/biz/tenant_environment.go；internal/data/tenantenvironment/；contract/tenantenvironment/binding.go；internal/service/tenant_environment.go及测试；migrations/0002_tenant_execution_environments.up.sql。gRPC/proto、精确方法认证与runtime装配已纳入主任务最终门禁。
- Validation commands / exit codes / required gates: Fedora定向 go test ./internal/data/tenantenvironment ./contract/tenantenvironment ./internal/service -run 'TenantEnvironment|Environment' -count=1 -v，exit0（evidence/03-environment-component-tests.log）；真实PG子进程SIGKILL恢复定向测试exit0（evidence/03-environment-process-recovery.log）。最终ModelDev make verify PASS，含生成一致性、全仓Go测试、vet/build/mod verify/diff check（evidence/modeldev-final-make-verify.log）。真实PG与生产业务纵向CPU外部边界替身也通过（evidence/04/modeldev-cpu-mainflow-final-shared-budget.log），不作为真实KFP/Kubernetes/S3验收。
- Acceptance IDs / actual values / evidence paths: EN01组件PASS：12并发不同operation仅1行/同ID、专用锁忙/释放、stale CAS、原UID和sealed bytes数据库拒改；EN02组件PASS：SA/RB/CM/Secret Create响应丢失恢复原对象，子进程在SA创建后及Experiment POST后SIGKILL，新进程恢复同environment/Namespace UID及原外部身份，Experiment总POST=1；EN03组件PASS：完整分页、重复/foreign/未知POST/continue循环与最多一次POST；EN04组件PASS：生产Ensure+真实PG完成seal→immutable投影→回读→CAS READY、Get只读及坏投影拒绝；NS02/API01/API02组件PASS：错误NS零子写、精确方法/unknown fields、授权边审查先于Secret、token每请求重读、原Secret UID值轮换。对应实际输出为 evidence/03-environment-component-tests.log 与 evidence/03-environment-process-recovery.log；全仓集成为 evidence/modeldev-final-make-verify.log。
- Deployment or cleanup effects / preserved assets: 本项没有持久集群写、旧执行清理或Inference改动。测试仅在Kubernetes/KFP/Res外部边界使用替身，环境用例、状态机和PG持久化均为生产代码；不得将替身READY写成现场READY。
- Remaining blockers / affected dependencies: 源码组件无未完成项；09的三条旧执行仍OPEN，证据 evidence/09/open-execution-blockers.md，因此未部署新平台、未显式开通A/B，真实artifact I/O、token轮换和A/B真实业务均NOT_RUN。现场bootstrap原UID/摘要、四定义原ID与材料及隔离授权由09/10完成，不能以本项resolved越过排空前置。
- Contract conflicts / evidence / minimal correction: spec§6.2与§5.1 VERIFY要求原IR完整SHA256；固定KFP源码backend/api/v2beta1/pipeline.proto:185–186以google.protobuf.Struct返回pipeline_spec，不保留编译YAML/JSON原字节；server/api_converter.go:470–484调用YamlStringToPipelineSpecStruct；server/api_util.go:152–201在有第二platform文档时返回{pipeline_spec:doc1,platform_spec:doc2}。采用主任务确认的最小修正：Config.ir_sources固定四个同bundle文件引用，先对原始文件bytes验证原IRSHA256，再与GetVersion的完整结构等值比较；仍用原IRSHA的ir-前16版本名，不改hash语义、不增加API或权限。源码均固定85a38e0719870e5b2f97a74f5c966d79bcc7c960，链接：https://github.com/kubeflow/pipelines/blob/85a38e0719870e5b2f97a74f5c966d79bcc7c960/backend/src/apiserver/server/api_util.go#L152 。
- Evidence provenance: 上述PASS仅引用本次evidence/实际输出；附件verification及其他历史记录只作输入，不作为本次验收。
- EN02补充覆盖：新增 internal/data/tenantenvironment/budget_recovery_test.go 验证真实25秒总预算耗尽后，从PG持久receipts显式续跑同环境/原UID；该后续测试尚未执行（NOT_RUN），首次Fedora传输无返回，独立入口诊断为SSH banner exchange超时。新测试尚未远端格式化或核对SHA，不纳入d807已通过门禁范围；当前本地SHA与未执行命令/exit=null见 evidence/03-environment-budget-recovery-record.json。既有Create丢响应及SIGKILL恢复PASS不能替代本新增用例。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。
