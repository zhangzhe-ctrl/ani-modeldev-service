# CPU-P01 Governance command delivery

来源：CPU-P01 CPU02/CPU04/CPU05，v04 D05 受理后可靠投递与 A11 入口边界；本轮Goal规定本机编辑、Fedora真实数据库/网络模块检查，目标集群另验。

## 当前切片

`ApplyCloseIntent` 接收已受理的 Governance USER_STOP，持久关闭墓碑后才返回 ACK。
停止可先于 Admission 到达，source generation 与 ModelDev owner close generation 分开。
`CloseReceipt.Replayed` 来自同一事务的原命令分支；读取 `CloseRecord` 不包含投递回执。
新连接/重启重放保持原 actor、时间与 owner fence，失败或结果不明的 commit 不返回成功回执。
`CLOSING` 只是封闭新创建的关闭意图，不代表 KFP/训练资源或历史写者已经停止。

## 当前身份边界

受理后的后台投递使用当次获准的 Governance 工作负载。冻结 actor 是审计关联，不保存、重放原用户 bearer/Cookie/delegation。
新的用户查询/停止仍需 Governance BFF 按当前权限检查；本入口不是另一套普通用户 BFF。

服务器要求 TLS 1.3、私有 CA 验证链及唯一精确 Governance DNS SAN；不接受 CN、通配或混合身份。
每个 RPC 重新验证实际 TLS peer 的证书当前有效期，长连接不能延长过期身份。
这是本分支现有 mTLS 工作负载信任合同，不声称已使用 IAM 在线 Grant/撤权。
只有 Command 的精确 unary 方法及实际装配后的 `ResolveAdmission` 可进入；后者复用
相同认证边界，只返回未持久受理的候选。当前 AcceptExecution 明确 Unimplemented，
Query/Step 尚未装配，stream与其他方法不获得此入口权限。

单值 `x-ani-tenant-id`、`x-ani-actor`、`x-ani-request-id` 来自上述认证工作负载的声明。
对关闭投递，它们关联已冻结命令；对候选解析，Governance 仍须在调用前检查当前用户授权。
UUID metadata 必须规范非零；actor 必须是非零 uint32 的规范 `governance:user:<id>` 或 `governance:access-key:<id>`。
入站 scope 与 Close body 的租户/actor 一致；缺可信 scope、重复头、非法字段/enum/time、unknown protobuf字段不能写库。
RPC只返回有限安全错误与已定义ErrorDetail，不回显SQL、连接、证书或凭据。

## 显式装配

`Bootstrap.command` 缺省时仍是未就绪的原 runtime shell，没有业务Command注册。
显式配置该块时必须提供数据库URL文件、CA文件、server证书/私钥文件与Governance精确DNS名；普通YAML只保存引用。
cmd读取有界文件、建立实际PG连接并注册mTLS listener，缺材料/连接失败不能回退到明文业务入口。
装配不迁移数据库、不创建角色、不改共享环境；部署需另提供受限runtime角色、版本化schema和真实应用所有权。
证书/连接材料在进程启动时读取；当前轮换通过受控重启，不声称热轮换或在线撤权。
持久Command可用仍不使全链ready：训练、观察、发布未接齐时 `/readyz` 保持503。
可选 `command.admission_resolution` 在同一listener与PG池上装配候选解析，省略时不注册
该RPC；显式材料失败拒绝启动。字段与文件边界见[候选解析](cpu-p01-admission-resolution.md#启动配置)。

## 验证边界

真实关闭receipt的4b6e96b RED→b4a0010 GREEN、6deb310全execution及race/真实Commit故障验证均在Fedora独占受限PG。
真实TLS socket的9da9080 RED→ce0283a首GREEN，781b86d收紧actor后8项负向GREEN；证书是测试CA，不是目标集群身份。
配置9eb241b与实际生产装配dcfe7c0 RED→c5e9bf3 GREEN；监听端口启动失败遗留数据库连接的983a40c RED→674f0f7 GREEN，覆盖真实连接释放。
夹带凭据字段的c9bd53e RED→674f0f7修复；最终格式版本d8d4218的完整证书/metadata/body/方法隔离/持久失败套件及race通过。
这些检查使用真实TLS socket、独占PG schema和测试CA；最终组合门禁单独记录，模块通过不替代全仓或LIVE结果。
本阶段无真实Governance投递worker、Accept状态/revision、KFP/Trainer、目标集群mTLS、L1–L4或AC16/17整链验收结论。
