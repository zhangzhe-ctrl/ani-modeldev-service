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

- Outcome: BLOCKED。相关default/race和真实PG子项PASS；必需make check/audit实际FAIL；真实A/B仍由09阻断。本轮真实HTTP正向/撤权补测已PASS，仍不将组件通过升级为现场A/B或全仓质量gate通过。
- Base / final commits: Gov base `ddd98f4c79751ba848fa862bab6878dc7bc99bb7`；运行/API源码 `fa80f886d022177173d7e9cf81ec51e54c2df4c8`与仅测试提交 `5cfa9065190b9585b799e428537a0019ca70e41a`均已SSH推送；44项变更文件Fedora/local SHA一致（evidence/gov-final/fedora-round2-source-identity.json）。旧API维护专用 `6a7c721ea113d51497421d7975122ec7ca8105c2`单独保留，不代替final v4结果。
- Changed files / generated artifacts: 官方API/OpenAPI、当前tenant显式Ensure/Get、精确权限/metadata/预算/Error38有限转换、Permission迁移/权威map及Create/publisher维护gate。完整已提交清单/manifest见evidence/gov-final/local-changed-files.txt、local-source-sha256.json；无replace、复制pb、compat或VM/Inference权限修改。
- Validation commands / exit codes / required gates: 正式ModelDev module `v0.0.0-20261031195825-d807cbdbd5dd`及Resource `v0.0.0-20261031180451-71b3ad7a2a88` go get exit0；真实隔离PG六项exit0/0.294s，owner/runtime分离且自有容器已移除，原完整log已取回environment-formal-pg-tests.log。原default/race session44762终态exit0，data15.993s/service1.453s/server1.941s；不是重启重跑，见fedora-round2-original-session-exits.json及environment-formal-default-tests.log。此前permcode生成/check和测试exit0仍保留。
- Final required checks: 原make check46554终态exit2，真实失败为基线TestVMCatalogUsesRuntimePathVariables：活跃GET /api/v1/virtual-machines/{vm_id}/logs缺权威capability mapping；未扩大VM权限。原audit91298终态exit1为vuln.go.dev网络read timeout，尚未到Gitleaks；网络恢复后只续一次原audit，exit1，govulncheck调用受影响0（另有1导入包/10required模块漏洞观察），Gitleaks1项。精确redacted诊断确认app/admin/service/internal/data/platform_component_log_scope_test.go:57–58、generic-api-key，base/final blob相同，为原负向fixture；未加allowlist、未标audit通过。证据environment-formal-make-check.log、environment-formal-audit-resume-once.log及receipt、gitleaks-final-finding-metadata.json；均为evidence/gov-final/下本次实际输出。原RESULT_UNKNOWN仅历史，已由终态证据取代。
- Acceptance IDs / actual values / evidence paths: 现有API组件验证当前独立manage/view、精确API/ALL范围、tenant0/API-key拒绝、真实PG撤权后AuthorizeEnvironment重验；权限迁移实际为manager/template各2 grant、viewer0及重入仍viewer0，没有证明既有OFF grant重入不复活。mTLS metadata重建、28/3秒RPC及30/5秒HTTP父预算、严格body/query/header/未知proto、reason白名单及无登记38未吞均有默认组证据；新真实JWT/Redis/Casbin/PG、生产HTTP/service/client、外部mTLS ModelDev peer集成已实跑PASS：Ensure202→200/Get200/无登记404、Prepare503仅指定cause、同JWT同key撤manage后403且全部ModelDev RPC计数不增、view独立，每请求acceptance/charge0。22cause仅converter单测，HTTP实跑1cause；证据evidence/gov-final/environment-http-integration-{final.log,final-receipt.json,source-and-coverage.json}。原CPU无GPU账、原GPU单次可靠受理及并发原key一次占额由真实PG六项验证。NS01 Gov不拥有Namespace resolver，不复制Res/MD vectors；只消费正式v4 codec并拒绝foreignName/未知view。现场A/B/Stop/下载授权仍NOT_RUN。
- Deployment or cleanup effects / preserved assets: 无线上DB/集群或服务更新；临时PG为独占任务资源且清理。维护开关只阻Create/claim，原Stop/close/release保留；未把final decoder部署用于旧v3排空。未修改VM/Inference权限或关闭/退额协议。
- Remaining blockers / affected dependencies: Fedora入口本轮恢复，原会话均权威终态，无相关Gov进程残留；不存在继续凭SSH超时称RESULT_UNKNOWN的理由。质量gate实际FAIL待明确处理边界，旧三执行仍缺原关闭证明；未推进v4 rollout/真实A-B验收。
- Contract conflicts / evidence / minimal correction: 本任务不为既有VM gate失败扩权限，不为fixture扫描命中加忽略。新事实是网络恢复允许取回原结果并仅续一次未完成audit；这些已完成，未重复默认组或make check。旧单独maintenance结果未替代final结果。必要HTTP补测复用现行真实PG/JWT/Redis/Casbin设施，只有外部ModelDev mTLS及既有Resource选择传输替身；定向race exit0/1.960s，单文件SHA f084a4d2e3a124e8e08c862704a2fba858e2bf5f3151c8f55f35c160b93534e5。两次夹具失败保留，未改生产或重跑全量。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。
