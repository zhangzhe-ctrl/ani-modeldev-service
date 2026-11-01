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

- Outcome: BLOCKED；四镜像已实际发布并按digest读回，新增构建工具已清理；最终四Pipeline/Version、sealed材料、策略安装和真实I/O未完成。
- Base / final commits: 固定runtime/API源d807cbdbd5dd6664d61480acedc752e19cfca48a，tree141053367fa4be9a6fb9c764b2d7900b5282be20；689构建输入逐SHA一致。
- Changed files / generated artifacts: 四profile编译/发布journal/完整Struct读回/不可变bundle/配置挂载工具已实现。CPU/GPU/step/webhook四镜像实际发布清单见evidence/08/image-publication/result.json。
- Validation commands / exit codes / required gates: 必要6项工具测试exit0；21对象schema server dry-run通过但尚未持久安装。四镜像缺配置启动实际exit1；发布采用原Harbor CA验证TLS，manifest及Harbor digest读回一致；没有把它作为真实训练验收。
- Acceptance IDs / actual values / evidence paths: CPU9369651248B/digest31227c3a…，GPU9369651247B/digestaa1f6ae2…，step16911953B/digestedc52340…，webhook27604938B/digest98715f78…；完整引用与原任务未丢失证明见上述result/node receipts。两个大镜像复用原29层完整cache，实际并行构建6.110/6.437秒，无附加CPU/内存/并发限额。
- Deployment or cleanup effects / preserved assets: 新增BuildKit/私有state/socket/auth/临时unit与333493615B节点目录及262742886B本地中转已清理、SHA/absence核实，原BuildKit/containerd/cache/tasks保留。清单evidence/08/image-publication/added-tools-cleanup-ledger.json；活跃旧退休CLI暂留，任务收尾后删除。专用bucket已建且VersioningEnabled，source Secret UIDbc62d45a-d83c-4ca8-b5fa-ca07c6cadb47；共享内部artifact账号仅此bucket，其他三个业务bucket及admin API实际403，见artifact-storage-complete-account-result.json；正向launcher I/O仍NOT_RUN。
- Remaining blockers / affected dependencies: 两training镜像原Resource PLATFORM登记、四定义实际发布/真实ID、sealed bundle、native/RBAC实际安装与行为、最终主服务切换及A/B真实训练/隔离/轮换。runtime control按合同只读平台定义，不授publisher写权限。
- Contract conflicts / evidence / minimal correction: 用户已批准route1并进一步明确允许两个大镜像直接在指定测试节点构建、撤销额外构建限额；主服务仍走指定Workbench。新增工具完整记账并清理。所有原身份/退出/释放门禁保持。

## Comments

以下为按时间保留的历史记录；当前状态以本文件 Result 为准。

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮公开输入补证：evidence/08-public-inputs-readonly-20261101.json和公开CA原bytes(713bytes)；只读实际metadata Service派生新tenant候选metadata-grpc-service.kubeflow.svc:8080，原CM仅短名、旧namespace同名CM404。已取driver/launcher/Argo原digest，显式TLS flags未暴露保持UNKNOWN，不当false；未作新launcher加载/I-O。

第二轮镜像准备入口事实：evidence/08-workbench-build-only-readonly-20261101.json。线上app.js/request7字段与本地前端一致；本地后端拒额外字段且push后无条件deploy，但live后端hash未暴露不宣称同源。没有可安全使用的build-only操作，现有modeldev recipe仅主service，未涵盖webhook/step/training；WORKLOAD_IMAGE尚未固定。未POST job或直接绕开入口。需取得支持完整镜像发布的确切入口合同，实际构建与digest仍NOT_RUN。

新增本地镜像构建部分PASS：Fedora固定Git d807的689输入校验后实际构建step/webhook，缺配置拒绝启动，source/tree/binarySHA/localImageID/localOCIdigest完整见evidence/08/build/build-receipt.json；未push/rollout。CPU/GPU因指定store固定LFbase缺失BLOCKED。此产物阶段未走Workbench job，不更新测试环境；服务切换仍指定Workbench。原snapshot55差异已被guard拒绝，实际构建从固定Git普通文件context，无Fedora archive打包传输。
