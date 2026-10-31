# 06 动态 webhook、native 策略与精确权限

Status: ready-for-agent
Resolution: open
Blocked by: 03, 04

## Goal

使共享webhook按live投影验证ModelDev资源，并让共Namespace普通工作负载免受该远端webhook影响。
依据：[spec.md](../spec.md) §6.3、§10–11；使用03投影与04四参数/v4合同，保持原退出证据及合法镜像继承。

## Scope

- 仅 ani-modeldev-service；internal/server/modeldevwebhook/{config,kubernetes,policy,training,handler}.go。
- cmd/ani-modeldev-webhook/main.go与相关fixed catalogue/profile接线。
- deploy/modeldev-webhook/render.py、deploy/modeldev-tenant-policy/render.py及固定bootstrap/native RBAC模板。
- 原owner chain、stepidentity/trainer镜像合同、Pod UPDATE/ephemeral guard必要共享校验。
- 包内精确KFP RBAC、artifact、projection合同作为输入；实际principals由01/08证据提供，不猜测。

## Implementation

1. 删除启动bindings数组及旧数量上限，保留单个全局ani-modeldev-webhook；每次匹配精确GET live Namespace和固定投影。
2. 严格验证projection schema/规范字节/digest、tenant/cluster/原UID和受管材料；未知binding不再outside_scope。
3. GET原SA并比较UID与exact refs，解析实际owner chain和原对象UID；不读业务DB/Gov，不引入缓存。
4. 所有KFP Pod含root/backend driver，从原Workflow固定runtime-config路径严格解析四参数和pipeline_root，重复/未知字段拒绝。
5. 按固定profile/image/command/support images、资源向量、workspace、creator与退出证据验证；verifier身份命中明确拒绝新Pod。
6. matchConditions以reserved SA、保留标记/finalizer、reserved PVC、artifact Secret引用任一项OR匹配，所有可选字段先has。
7. 非法creator不能放入匹配条件而被跳过；错误Namespace同样进入验证；普通VM/Instance/default Pod不进入远端。
8. 固定三个native VAP/Binding：bootstrap写约束、受管配置保护、工作负载guard；失败关闭且Deny。
9. bootstrap策略约束所有Namespace中的control写，再验证标准name/labels、固定对象/key/roleRef/subject；先策略后create/bind授权。
10. UPDATE使用oldObject防删标记绕过；冻结Workflow模板/参数/entrypoint/SA，精确允许KFP API principal将activeDeadlineSeconds设0，以及spec不变的Argo全对象UPDATE与必要status/metadata/finalizer路径。
11. 保护Pod模板、ephemeralcontainers和artifact Secret/SA旁路；所有Namespace禁止三个reserved SA的legacy service-account-token Secret CREATE/UPDATE；普通tenant实际groups宽权限冲突必须明确BLOCKED，不自动重写其他owner授权。
12. 生成最小ClusterRole/RoleBinding：Namespace仅GET，固定resourceNames读/patch和有限bind；按contracts/authorization-inventory.md增加明确的只读RBAC授权边审查权限；镜像合法SA继承按exact refs保留。

## Acceptance

- AD01：任一保留身份/引用命中；去标签不能逃逸；unknown projection deny；普通Pod远端请求计数为零。
- AD02：四参数覆盖driver，迟到旧UID拒绝；Workflow模板、Pod UPDATE/ephemeral绕过失败；正常terminate/status通过。
- EN04、IM01：坏/未知投影拒绝；合法imagePullSecrets继承通过，额外/前缀伪装及SA UID变化拒绝。
- NS02：live Namespace异常拒绝，组件action中无Namespace create/patch；不因共享NS部署干扰无关owner。
- LIVE02、LIVE10：本项提供server dry-run正反对象和恢复步骤；实际安装/故障验证在09/10，不能用渲染成功声称现场通过。
- LIVE04、LIVE09：精确SAR矩阵和Secret/SA借用否定例可执行；真实principal、token和普通tenant权限由后续live验证。

## Constraints

- 不增加tenant级webhook item、全租户启动map、正/负缓存、DB访问或回退outside_scope。
- 不改共享ani-kfp-trainjob、不添加ani.io/kubeflow-tenant标签迁就它，不全局放宽已有策略。
- 不使用通配RBAC、任意Secret GET、impersonate、serviceaccounts/token或control自建Role/ClusterRole能力。
- 不将所有Pod送远端后handler no-op称为隔离；不冻结整个对象而阻断合法关闭。
- 只生成/验证新模板；旧环境尚运行时不得替换全局策略、撤销旧权限或切换webhook，统一交09。

## Result

- Outcome: COMPONENT_PASS + SERVER_SCHEMA_PASS；Resolution保持open。动态Webhook组件与21对象server schema dry-run已通过，真实安装、controller typeChecking和正反业务行为尚未完成，不构成AD01/AD02或LIVE02全PASS。
- Base / final commits: ModelDev base 21bf4bd6540d462a61c91b629f26250899583223；最终源码提交 d807cbdbd5dd6664d61480acedc752e19cfca48a，对应 evidence/modeldev-tested-source-identity.json。最终仓库门禁通过；不声明新webhook镜像已部署。
- Changed files / generated artifacts: internal/server/modeldevwebhook/及相关测试；cmd/ani-modeldev-webhook/main.go；deploy/modeldev-webhook/；deploy/modeldev-tenant-policy/。配置schema ani.modeldev.webhook.v2删除旧bindings；一个全局OR registration、三固定native VAP/Binding、精确RBAC、共享bundle及独立platform普通文件copy均已实现。
- Validation commands / exit codes / required gates: Fedora Webhook Go整包与cmd编译exit0（evidence/06-webhook-component-tests.log）；两renderer各3项Python测试exit0（evidence/06-renderer-component-tests.log），CEL最小修正后native3项再次exit0（evidence/06-native-server-dryrun.python-tests.log）。重核完整指定集群身份后逐21对象 kubectl apply --dry-run=server 全exit0，无stderr错误/警告，原对象UID/RV/完整摘要前后一致（evidence/06-native-server-dryrun.fixed.json、evidence/06-native-server-dryrun.fixed.log）。最终 make verify PASS（evidence/modeldev-final-make-verify.log）；生产HTTPS webhook参与的真实PG/48步CPU/关闭清理替身纵向通过（evidence/04/modeldev-cpu-mainflow-final-shared-budget.log）。
- Acceptance IDs / actual values / evidence paths: AD01/EN04/NS02/IM01组件PASS：同一Handler迟到B可见、逐请求投影、坏/未知binding、原SA UID变化及额外refs拒绝，普通default Pod零依赖读；AD02组件PASS：driver和step均校验四root及原pipeline_root，训练owner图/资源/镜像/workspace/retention正反例，见 evidence/06-webhook-component-tests.log。固定三VAP与有限RBAC渲染子项PASS见 evidence/06-renderer-component-tests.log。server schema子项21/21 PASS见 evidence/06-native-server-dryrun.fixed.md；三VAP均仅返回status={}，不是安装后的typeChecking证据。LIVE02/04/09/10完整现场验收仍NOT_RUN。
- Deployment or cleanup effects / preserved assets: 仅执行无持久效果的server dry-run；没有安装/替换策略、撤权、Namespace写或Inference改动。实现了八文件+manifest及独立platform原SHA的init copy，尚未用于本次新部署；不把代码功能描述成实际安装。
- Remaining blockers / affected dependencies: 03/04代码与最终装配门禁已具备。本次实际principals、nodes及CA key证据已采集（evidence/native-principals.json），schema dry-run已通过；09被三条OPEN旧执行证据阻断（evidence/09/open-execution-blockers.md），因此不能安装全局策略/新webhook。安装后的observedGeneration、typeChecking无expressionWarnings、matchConditions正反、合法terminate/status、真实原groups权限和webhook不可达共存均NOT_RUN。native_probe不得以一般RBAC拒绝冒充admission拒绝。
- Contract conflicts / evidence / minimal correction: 与03相同原IRbytes/Struct事实修正由原bundle文件SHA加GetVersion完整结构等值实现；06未改变身份或权限方案。部署事实：03 LoadConfig(config.go)使用readRegularFile/O_NOFOLLOW，ConfigMap投影是symlink，因此webhook binary提供有限copy-platform init路径先核原SHA再原子0400普通文件，保留runtime严格检查；不是放宽文件读取或改digest。实际server dry-run首轮19/21通过，失败事实为CEL保留变量namespace及Workflow伪PodSpec异构map字面量类型；保留首轮原错evidence/06-native-server-dryrun.{json,log,md}。最小实现修正仅变量rename及四map值显式dyn，全部规则/权限/作用域不变；原19个其他对象完全相同，重跑21/21通过。此为实现语法修正，不是spec权限/身份方案变更。
- Evidence provenance: 上述PASS仅引用本次evidence/实际输出；附件verification及其他历史记录只作输入，不作为本次验收。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。
