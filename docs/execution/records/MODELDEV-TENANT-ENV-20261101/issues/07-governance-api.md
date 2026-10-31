# 07 Gov 当前租户入口、授权、错误与预算

Status: in-progress
Resolution: open
Blocked by: 03, 04

## Goal

提供当前租户显式环境Ensure/Get的BFF入口，并确保环境错误在原Create占额前被准确返回。
依据：[spec.md](../spec.md) §2.3、§4、§7.4、§12.5、§14.2；只消费正式ModelDev API，不新增租户初始化或quota业务。

## Scope

- 仅 ani-governance；api/protos/admin/service/v1/i_modeldev.proto、新api/protos/modeldev/service/v1/environment.proto。
- app/admin/service/internal/server/modeldev_environment_http.go、instance_timeout.go、rest_server.go。
- app/admin/service/internal/service/modeldev_environment_service.go、modeldev_service.go。
- app/admin/service/internal/data/modeldev_environment_client.go、modeldev_authorization.go、modeldev_client.go及必要wiring。
- 正式Go module pin、固定API/OpenAPI生成物、权限迁移；docs/contracts/frontend-http-api.md、service-to-service-calls.md。

## Implementation

1. 新增POST /api/v1/modeldev/environment:ensure与GET /api/v1/modeldev/environment，路径与DTO严格按01合同。
2. Ensure只收≤1024字节application/json空对象加EOF，禁止query，恰一个canonical非零UUID Idempotency-Key；Get禁止body/query。
3. 复用Principal且SubjectUser、tenantID>0，重新检查当前用户/租户/角色/ALL范围、动作Permission及精确API grant。
4. 登记manage_environment/view_environment Permission/API关联与默认租户管理员授权；只读角色须明确获授，不依赖菜单。
5. ResourceTenantID取当前权威映射；丢弃浏览器x-ani-*，重建tenant/actor/独立requestID/精确authorized-method/tenant-all。
6. 通过正式module pin调用两个RPC；严格拒绝request/response/nested view/ErrorDetail/FieldViolation未知字段及非法READY。
7. Ensure RPC≤28秒、HTTP≤30秒，Get RPC≤3秒、HTTP≤5秒；精确路径更新timeout filter，服从上游剩余预算。
8. PROVISIONING映射202和Retry-After=2，其他持久state200；Busy按实际state，Get无登记404，不伪造开通成功。
9. 不缓存首次PROVISIONING、不循环sleep等READY；同key重试重新授权并返回最新持久结果，operation_id只作关联。
10. 为Prepare/Finalize新增严格环境错误白名单：Unavailable/ENVIRONMENT_NOT_READY/tenant_environment/一致correlation/单environment violation。
11. 仅HTTP metadata.environment_reason输出有限枚举；Get NotFound+38独立严格转换，不能被通用query错误吞掉。
12. 保留当前ComputeSelection、AcceptFrozen与GPU原charge同事务及可靠CREATE投递；环境接口不调用该链，环境失败在其前终止。

## Acceptance

- API01：当前tenant权限、tenant0拒绝、撤权后的同key重试拒绝、伪造metadata/任意body-query-header和未知proto拒绝。
- API01：精确两路径实际预算生效；其他ModelDev路由预算未整体放宽，无请求内sleep/轮询或首响应缓存。
- API02：所有允许environment_reason准确通过，其余kind/violations/上游message被拒；ErrorReason38保留为404。
- API02：Ensure/Get、缺环境及不READY的Prepare均无acceptance/charge/可靠CREATE副作用。
- ST02：无环境合法tenant的source-only Readiness保持；CPU不被要求Gov推导cluster，GPU错cluster不接受。
- 正式API pin、生成物、Permission/API关联、OpenAPI和文档一致；真实A/B授权、查询/Stop隔离留LIVE03/LIVE08。

## Constraints

- 不新增任意target tenant、namespace/cluster/Experiment输入，不加平台tenant0代办接口。
- 不实现完整tenant初始化、套餐/菜单/新页面、租户namespace字段或新的quota/outbox。
- 不允许业务提交隐式Ensure，不把queryContext复用误当Ensure免授权，不用硬编码管理员ID。
- 不透传任意上游错误文本/metadata，不放宽全ModelDev前缀timeout，不将unknown fields静默丢弃。
- 不提交本地replace或复制ModelDev生成代码；线上路由切换和旧Create停入操作由09统一执行。

## Result

- Outcome: PARTIAL_COMPONENT_PASS；final default/race、必需make check和audit均RESULT_UNKNOWN；整体真实集成BLOCKED，未部署，Resolution保持open。
- Base / final commits: Gov base `ddd98f4c79751ba848fa862bab6878dc7bc99bb7`；最终提交`fa80f886d022177173d7e9cf81ec51e54c2df4c8`（`codex/modeldev-dynamic-tenant-environment`），本地工作树clean；允许审查，不标门禁通过/部署就绪。另旧API维护专用 `6a7c721ea113d51497421d7975122ec7ca8105c2` 已SSH推送，仅作09前置维护，不作为最终v4验收。
- Changed files / generated artifacts: 44项本地源码/生成物/正式依赖变更，完整清单和本地SHA见`evidence/gov-final/local-changed-files.txt`与`local-source-sha256.json`。新增当前tenant显式Ensure/Get、精确权限/metadata/预算/严格Error38与有限cause转换；官方API/OpenAPI生成物、独立Permission迁移/权威映射；一个维护gate接原Create和publisher，不阻Stop/close/release。v4边界fixture按真实codec同步固定Namespace、原runtime名称、tenant input/publish scope、owner root及dataset digest，业务validator和负例未放宽。
- Validation commands / exit codes / required gates: 正式`go get github.com/zhangzhe-ctrl/ani-modeldev-service@d807cbdbd5dd6664d61480acedc752e19cfca48a` exit0，ModelDev pin为`v0.0.0-20261031195825-d807cbdbd5dd`，同时Resource正式升级`v0.0.0-20261031180451-71b3ad7a2a88`；go.mod/sum已取回，无replace/go.work。Fedora真实独占PG六项exit0（0.294s），owner/runtime角色分离，容器已移除；已取回输出见`formal-module-pin-captured.log`、`formal-pg-six-captured.log`和`run-pg-reproduction.sh`。此前权限map生成`--check`及`go test ./pkg/permcode` exit0见`evidence/gov-environment-permcode-tests.log`。官方make api此前已完成，但本轮最终树身份未对比，原远端生成日志未取回，不借此声明final gate完成。
- Final required checks: Fedora默认ModelDev data/service/server race测试session44762、make check session46554、独立audit session91298均已发起，最终退出码未返回，严格RESULT_UNKNOWN。最后只观察到check的quota/layout部分通过；audit只观察到Go/scanner/DB版本信息。tail session60970、ps session61100同样挂起；完整命令/路径/最后输出见`evidence/gov-final/pending-fedora-sessions.json`。维护版基线VM映射FAIL/Gitleaks误判FAIL单独保留于`evidence/gov-maintenance/`，**不得替代本次final Gov结果**。
- Acceptance IDs / actual values / evidence paths: API01仅已验证真实PG当前独立manage/view授权、撤权后重验、精确API与ALL范围、tenant0拒绝，以及独立权限迁移只授tenant-manager/template、重入不复活撤权；相关流程acceptance/charge计数0。API02的默认mTLS/error/public HTTP测试已发起但结果未知，不标PASS。原CPU无GPU账、原GPU单次可靠受理/原账及并发同key一次占额已由本次真实PG六项验证。NS01 Gov不拥有Namespace resolver、不GET/创建K8 Namespace，仅消费正式cpup01/codec；不复制Res/ModelDev vectors来造Gov向量PASS。真实A/B授权、HTTP状态、Stop隔离和新环境业务链仍NOT_RUN/BLOCKED，留09/LIVE03/LIVE08。
- Deployment or cleanup effects / preserved assets: 未更新服务、未部署三仓、未写线上DB/集群。临时PG仅本任务自有资源且已清理。未改Inference/VM权限；没有compat、旧codec、额外quota/outbox。维护开关`ANI_MODELDEV_CREATE_PAUSED=true`仅阻Create与新claim；空/false恢复，其余拼写拒启动；切换需重启。停止时最多20s且服从上游deadline等待当前请求/claim与原worker活跃租约，未排空报错。
- Remaining blockers / affected dependencies: Fedora入口握手阻断，现有会话最终结果无法读取；root要求只做最后一次恢复探测，2026-10-31T20:12:40.481941Z至20:12:50.503803Z仍exit255，原输出`Connection timed out during banner exchange`及目标172.16.101.31:22，见`fedora-final-recovery-probe.json`。本地ssh -G确认controlmaster=false/controlpersist=no/无controlpath，无旧master可复用。未换主机或本地跑测试，未重复重跑。旧3原执行未安全关闭的09阻塞仍独立存在。
- Contract conflicts / evidence / minimal correction: final代码与固定合同无已知事实冲突。此前基线Network测试25/实际38的仅测试描述符修正保留；VM活跃路由权威映射缺口及基线Gitleaks组件名误判未扩权限/allowlist处理。当前新增事实冲突是执行入口不可用，最小恢复动作为原Fedora入口恢复后先读取原会话/log，再仅补未完成必要门禁；不能把旧失败或局部通过伪装final验收。未核对的本地/远端final源码SHA明确NOT_COMPARED，仅保存实际本地manifest。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。
