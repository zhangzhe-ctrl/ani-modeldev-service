# 08 固定镜像、四定义与部署材料准备验证

Status: ready-for-agent
Resolution: open
Blocked by: 04, 06

## Goal

准备同一份可验证的固定软件材料、发布工具和bootstrap部署包，为09一次切换提供确定输入。
依据：[spec.md](../spec.md) §1.3、§6、§10–11、§16；本项的定义发布限准备/验证，旧环境仍服务时不破坏其材料或权限。

## Scope

- ani-modeldev-service 的四份固定pipeline编译/发布入口、catalogue、镜像构建与材料manifest。
- deploy/modeldev-webhook/render.py、deploy/modeldev-tenant-policy/render.py与06固定bootstrap/native RBAC模板。
- control/webhook部署模板、projected token挂载、受管CA/步骤配置、KFP launcher和artifact SecretRef非敏感模板。
- 执行包 deployment-inputs.json、固定四定义/IR/material摘要表和可执行发布/回读验证记录模板。
- Res/Gov仅提供已固定接口和身份输入；不在本项修改其他仓库业务或上线新运行服务。

## Implementation

1. 逐项核实镜像、bucket/connection、workspace、公开endpoint/region、metadata host/port、CA key和TLS flags输入，未知项不猜。
2. 固定业务镜像进入该tenant space authority的受管platform_project，使用digest；A/B robot可读条件明确记录。
3. driver/launcher其他authority只有平台明确匿名且实际pull证据成立才允许无ref，否则BLOCKED/由平台统一镜像。
4. 用04编译器准备四份IR和固定profile；校验root四参数、完整IR SHA、组件/driver/support image及资源合同。
5. 准备ani-system中四固定Pipeline名称和ir-<前16SHA> Version的发布工具；陌生同名/完整摘要不符必须冲突。
6. 验证发布器的精确GET与幂等冲突分支；本项不删除/替换正在服务的旧Pipeline、Version或运行catalogue。
7. 生成同一不可变材料bundle供主服务/webhook/发布证明消费；最终实际Pipeline/Version ID在09发布后回填校验。
8. 准备control投影token文件挂载、每请求重读路径和两audience配置；禁止token值入包，不能用subPath固定旧文件。
9. 准备固定launcher SecretRef及独立artifact bucket凭据文件引用；模板只有两键schema与非敏感配置，不嵌入AK/SK。
10. 渲染精确ClusterRole/native VAP/Binding/全局webhook模板，验证先策略后create/bind的安装顺序和principals输入。
11. 准备普通Pod/ModelDev Pod、正常CPU/GPU与非法roleRef/key/ns的server dry-run用例，及09/10实际安装和恢复说明。
12. 输出待切换发布清单、镜像digest/构建commit、模板摘要、剩余live输入和前置门槛；不修改旧服务启动配置或撤权。

## Acceptance

- SNP01、AD02：四IR仅固定root参数，与v4/严格Workflow解析一致；主服务和webhook使用同一材料manifest。
- IM01：generic/fine refs及SA继承材料一致；固定业务镜像单authority/platform_project，无隐含前缀放行。
- LIVE02：bootstrap模板和正反dry-run用例可直接在09窗口执行；离线渲染/测试不能记现场PASS。
- LIVE04：发布器证明固定父ani-system、完整digest回读与实际SAR合同；最终发布ID/token访问留09/10。
- LIVE09：artifact SecretRef、CA实际加载、专用bucket权限和普通tenant隔离的验证步骤完整，未实测项标NOT_RUN/BLOCKED。
- LIVE01、LIVE06：记录可知构建/部署输入及待后续取值字段；本项不要求新环境先运行才解除09依赖。

## Constraints

- 08不切换主服务/webhook、不替换全局policy、不撤销旧权限、不删除旧定义/对象；可能影响在线运行的动作仅09执行。
- 不把准备/渲染/受控发布器测试声明为四定义已在目标集群发布或LIVE已通过。
- 不按tenant复制Pipeline或profile，不将发布写权限下放runtime control/Ensure。
- 不使用HTTP/disableSSL/skipTLS绕过CA，不复制业务STS/全权AK，不将共享artifactAK声称为tenant IAM隔离。
- 不扩大Inference/installer或共享ani-kfp-trainjob；构建与必需gate在批准远端，最终imageID映射由09/10补齐。

## Result

- Outcome: BLOCKED；材料工具/四profile编译与两个附加镜像本地构建已完成，真实发布/安装/I-O未完成。
- Base / final commits: 固定runtime/API源d807cbdbd5dd6664d61480acedc752e19cfca48a，tree141053367fa4be9a6fb9c764b2d7900b5282be20。689输入SHA与Fedora一致；没有用漂移的共享snapshot构建。
- Changed files / generated artifacts: 同一四profile编译、发布journal、完整Struct读回、不可变bundle、平台config和挂载renderer；step/webhook Fedora本地镜像。完整范围见`evidence/08/update-entry-complete-inventory.md`。
- Validation commands / exit codes / required gates: 6项工具真实编译/材料/恢复测试exit0，见evidence/08/tools.log；21native对象schema server dry-run通过但未安装。step/webhook固定源build exit0，缺配置真实启动exit1拒绝，见`evidence/08/build/build-receipt.json`。无真实registry push/pull/TLS/artifact结论。
- Acceptance IDs / actual values / evidence paths: step36,725,738B（35.02MiB）、webhook67,703,675B（64.57MiB），共99.59MiB本地Size，非压缩传输量。CPU/GPU training未构建，固定官方base当前构建store不在缓存；不把历史base登记或固定FROM当新runtime镜像。现有registry10.10.1.66:30003/projectani-resource-factor10-20261022、业务connection/bucket、workspace class和公开CA候选已精确只读核验，见new-public-inputs-readonly.json；新客户端加载/I-O仍NOT_RUN。
- Deployment or cleanup effects / preserved assets: 本地镜像未发布/部署；四个Pipeline/Version最终ID、sealed材料包、专用artifact身份/IAM、native/bootstrap、配置挂载未安装。没有每租户副本或新Namespace。
- Remaining blockers / affected dependencies: 最终registry digest/大小/认证pull，training固定base与build，shared workspace容量选择，四定义实际发布、bucket-only内部artifact身份及真实TLS/I-O、最终最小RBAC/native行为、旧清理完成后的v4切换与A/B验收。
- Contract conflicts / evidence / minimal correction: 指定入口只有主服务image-only目标。用户已在完整清单后批准route1，其他固定材料镜像Fedora+指定SSH维护；太慢须报告，重任务仍在Fedora且不打包传输。见update-entry-scope-approval-20261101.json。该批准未豁免任何原身份/退出/释放门禁。

## Comments

以下为按时间保留的历史记录；当前状态以本文件 Result 为准。

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮公开输入补证：evidence/08-public-inputs-readonly-20261101.json和公开CA原bytes(713bytes)；只读实际metadata Service派生新tenant候选metadata-grpc-service.kubeflow.svc:8080，原CM仅短名、旧namespace同名CM404。已取driver/launcher/Argo原digest，显式TLS flags未暴露保持UNKNOWN，不当false；未作新launcher加载/I-O。

第二轮镜像准备入口事实：evidence/08-workbench-build-only-readonly-20261101.json。线上app.js/request7字段与本地前端一致；本地后端拒额外字段且push后无条件deploy，但live后端hash未暴露不宣称同源。没有可安全使用的build-only操作，现有modeldev recipe仅主service，未涵盖webhook/step/training；WORKLOAD_IMAGE尚未固定。未POST job或直接绕开入口。需取得支持完整镜像发布的确切入口合同，实际构建与digest仍NOT_RUN。

新增本地镜像构建部分PASS：Fedora固定Git d807的689输入校验后实际构建step/webhook，缺配置拒绝启动，source/tree/binarySHA/localImageID/localOCIdigest完整见evidence/08/build/build-receipt.json；未push/rollout。CPU/GPU因指定store固定LFbase缺失BLOCKED。此产物阶段未走Workbench job，不更新测试环境；服务切换仍指定Workbench。原snapshot55差异已被guard拒绝，实际构建从固定Git普通文件context，无Fedora archive打包传输。
