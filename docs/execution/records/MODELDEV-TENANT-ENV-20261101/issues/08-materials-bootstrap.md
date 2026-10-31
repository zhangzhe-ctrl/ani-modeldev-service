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

- Outcome: BLOCKED（组件子项PASS：工具、真实四IR编译、不可变材料与渲染子项）；Resolution保持open。现场新镜像构建/运行映射、四定义实际发布ID、最终材料安装及切换未完成，不能记LIVE PASS。
- Base / final commits: ModelDev base 21bf4bd6540d462a61c91b629f26250899583223；最终源码提交 d807cbdbd5dd6664d61480acedc752e19cfca48a及 matched=true 的本次源码核对见 evidence/modeldev-tested-source-identity.json；不将附件历史镜像/发布记录当作本次最终映射。
- Changed files / generated artifacts: deploy/modeldev-environment/{bundle.py,publisher.py,platform_config.py,render.py,verify_target.py,test_materials.py,README.md}。固定四profile原IR，发布journal先意图后POST且未知结果不重发；同一canonical manifest/raw SHA供service和webhook；八材料普通文件copy与独立platform copy；Kubernetes/KFP独立投影audiences及artifact两键Secret文件引用，未带tenant map或秘密值。
- Validation commands / exit codes / required gates: Fedora /home/chabking/tenantenv-20261101/modeldev-snapshot，固定KFP/kfp-kubernetes/kfp-pipeline-spec2.16.0，PYTHONPATH=deploy/modeldev-environment .../kfp-resource-venv/bin/python -m unittest discover -s deploy/modeldev-environment -p test_materials.py -v，6项、exit0、4.535s（evidence/08/tools.log）。最终ModelDev make verify PASS（evidence/modeldev-final-make-verify.log）；该Go门禁不代替上述Python编译测试或真实发布。
- Acceptance IDs / actual values / evidence paths: SNP01/AD02的四IR root编译与共用bundle子项PASS、IM01单authority/platform_project与明确匿名证据工具拒绝分支PASS，实际输出 evidence/08/tools.log。完整集群身份只读采集见 evidence/08-identity-receipt.json；控制器/CA key等实际输入见 evidence/native-principals.json。06后续schema dry-run21/21 PASS见 evidence/06-native-server-dryrun.fixed.json；它不是本项真实安装或LIVE02全PASS。LIVE01/02/04/06/09中部署imageID、发布ID/完整GET、artifact/CA/授权和普通tenant隔离等完整现场项均未完成。
- Deployment or cleanup effects / preserved assets: 本项无持久集群写、正式新镜像部署、四定义现场发布、旧定义删除、撤权或数据清理；publisher测试仅替代外部发布传输。真实四IR编译结果属于测试产物，不冒充ani-system已发布的Pipeline/Version。
- Remaining blockers / affected dependencies: 06最终模板已交付并通过server schema dry-run。09的旧执行排空仍BLOCKED（evidence/09/open-execution-blockers.md），所以实际发布四定义、以正式ID生成最终canonical materials、seal同一bundle、真实平台输入加载、安装和服务/webhook切换均NOT_RUN。此前身份收据不作为未来写授权；09任何写前须重核完整身份与实际输入，不猜未知值。
- Contract conflicts / evidence / minimal correction: 固定 KFP SHA `85a38e0719870e5b2f97a74f5c966d79bcc7c960` 的 `backend/api/v2beta1/pipeline.proto:185–186` 把 GetVersion.pipeline_spec 定义为 protobuf Struct；`backend/src/apiserver/server/api_converter.go:470–484` 通过 `api_util.go:152–201` 将多doc原YAML投影成 `{pipeline_spec:doc1,platform_spec:doc2}`，无法从 GET 恢复原 bytes。受影响 spec 的“原 IR SHA 与精确 GET”验证。主 agent 已确认最小修正：同一 immutable bundle 保留四份原 IR，以原 bytes SHA 对 Config.IRSHA，再完整结构比较官方 GET Struct；不重定义 IR SHA、不绕过 GET、不加兼容或扩大身份。发布器和03 IRSources reader均用该语义，官方源链接见README。
- Evidence provenance: 上述PASS仅引用本次evidence/实际输出；附件verification及其他历史记录只作输入，不作为本次验收。

## Comments

- 初始任务，尚无实施或验收完成声明；后续按时间追加事实与证据。
- 达到本项验收后再更新 Resolution 和 Result；后续现场证据不得提前标记通过。

第二轮公开输入补证：evidence/08-public-inputs-readonly-20261101.json和公开CA原bytes(713bytes)；只读实际metadata Service派生新tenant候选metadata-grpc-service.kubeflow.svc:8080，原CM仅短名、旧namespace同名CM404。已取driver/launcher/Argo原digest，显式TLS flags未暴露保持UNKNOWN，不当false；未作新launcher加载/I-O。

第二轮镜像准备入口事实：evidence/08-workbench-build-only-readonly-20261101.json。线上app.js/request7字段与本地前端一致；本地后端拒额外字段且push后无条件deploy，但live后端hash未暴露不宣称同源。没有可安全使用的build-only操作，现有modeldev recipe仅主service，未涵盖webhook/step/training；WORKLOAD_IMAGE尚未固定。未POST job或直接绕开入口。需取得支持完整镜像发布的确切入口合同，实际构建与digest仍NOT_RUN。
