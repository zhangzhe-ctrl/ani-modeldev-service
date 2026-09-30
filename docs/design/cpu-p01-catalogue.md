# CPU04 不可变 Release 文件读取

来源：原 `CPU04` 第 3、6 项、`CPU01`、v0.4 development D03/D05/D13、operations O08，
以及当前 Goal 第 5、9、10 节。任务来源身份见 `.scratch/cpu-p01/`；本文件承接
[持久化方案](cpu-p01-persistence.md) 的下一目录切片，不复制原任务包。

本切片只读显式指定的 Release 文件。当前绑定、启用开关与 generation 的权威
仍在 Governance；读取目录不会启用 Release，也不会生成 InputVersion、ENV
身份或完整 ExecutionSnapshot。T02 校验导入与 CAS、T03 实际输入导入和
ResolveAdmission 后续各自实现。

## 文件合同

`contract/cpup01.ReleaseDocument` 是 `ani.modeldev.release.v1`。每个 Release
在受管只读目录内对应 `<release_id>.json`，ID 为非零标准小写 UUID。新内容使用
新 Release ID，旧文件保留；目录没有 `latest`、默认 ID、READY 或 VERIFIED 字段。

规范 JSON 的根字段按顺序为：`schema_version, release_id, preset_id, kind,
delivery_mode, pipeline_id, pipeline_version_id, pipeline_ir_sha256, runtime,
program, resources, workspace, output_contract, execution_timeout_seconds`。

- Release/preset/Pipeline/PipelineVersion/登记镜像均有固定非零 UUID；Pipeline IR
  和 Runtime 有 64 位小写 SHA256。kind 固定 GENERAL_TRAINING，delivery_mode
  固定 SAVE_ARTIFACTS。缺失、UNKNOWN、NOT_READY、UNSET 均拒绝。
- `runtime` 复用既有 `RuntimeRef`，固定 Runtime 名、kind/API group、内容摘要和
  target job 名称集合。`resources`、`workspace`、`output_contract` 分别复用
  `CPUResources`、`WorkspaceContract`、`OutputContract`，不再定义第二套快照字段。
- `program` 顺序为 `image_version_id, image_digest, command, args_template,
  parameter_contract_version, default_parameters`。只有一个已登记 CPU MLP 镜像，
  使用完整 registry/repository@sha256 引用；command 固定为
  `[/opt/venv/bin/python, -I, /opt/cpu03/train_mlp.py]`。
- 每个 args template 元素恰有 `literal` 或 `source` 一项。基础参数顺序固定为
  `--data / INPUT_PATH`、`--output / OUTPUT_PATH`、`--expected-input-sha256 /
  INPUT_SHA256`、`--expected-input-bytes / INPUT_BYTES`、`--learning-rate /
  LEARNING_RATE`。这里斜线表示相邻 argv 项，不是字符串替换表达式。可额外固定
  `--recipe` 与已登记的 `success`、`fail` 或 `slow-stop` 字面量；测试配方的目录
  不能因此向普通租户启用。source 只整项映射可信准备结果或固定参数，不做 shell
  插值，不接受自由脚本、任意 token 或浏览器传入的 command/args。
- 参数合同版本为 `ani.cpu.mlp.parameters.v1`，其唯一语法是现有 CPU-P01 Intent
  白名单：epochs INTEGER=3、batch_size INTEGER=64、learning_rate DECIMAL (0,0.1]。
  `default_parameters` 必须完整含这三项，使用现有 `Parameter` 类型；本文件没有
  第二套可编辑的参数范围引擎。
- `execution_timeout_seconds` 为正 uint32，单位秒，是此 Release 固定的总时限。
  后续解析用原 accepted_at 计算 deadline 并检查时间范围；重放不重新计时。

`WorkspaceContract.StorageClass` 只是固定期望名称。后续 Resolve 必须与实际 ENV
绑定匹配并消费其核验证据；目录 reader 不证明 StorageClass 存在、可用或有权限。
Release 不包含租户、Namespace UID、ServiceAccount、S3 credential、当前绑定
generation、输入对象事实或本次 deadline。真实 ENV 或 PipelineVersion 来源未交付
时，不得用合法 UUID 替代事实进入受理或启用。

## 摘要与严格读取

`release_digest = lowercase_hex(SHA256(实际规范文件字节))`。文件不自含 digest，
不把 intent/spec/镜像/IR 摘要混作 Release digest。规范字节使用 UTF-8，无 BOM、
空白或尾随换行；字符串及整数遵守既有 Snapshot 规范，int64 写十进制字符串，
uint32 写 JSON 数字。嵌套复用类型字段顺序沿用 Snapshot；参数按 name、Runtime
target jobs 按名称、required_files 按 relative_path 排序。argv 顺序不改变。

`ParseRelease(raw)` 是公共纯合同入口，返回完整文档与实际摘要。输入上限 64 KiB；
严格拒绝未知字段、重复字段、null、非规范字段顺序/大小写/数值、非法 Unicode、
多份 JSON 或尾随字节。不先宽松接受再静默规范化已发布文件；校验后规范重编码
必须逐字等于输入。

`catalogue.ReadRelease(ctx, trustedDirectory, releaseID, expectedDigest)` 是本次
测试的公开接口。它只打开明确 leaf 文件，拒绝 symlink、非普通文件和多硬链接，
有界读取并检查读取期间文件身份/尺寸/修改时间，核对文内 ID 与来件 ID、实际
摘要与期望摘要；不会扫描目录、选最新文件或使用进程内默认指针。目录由受管
装配提供，不能来自普通用户请求。文件 I/O 与摘要不进入 Governance DB 事务。

只读 reader 能保证旧 ID+旧摘要不会读成改过的内容；它不能跨进程发现管理员
故意用同 ID 同时换文件与期望摘要。禁止同 ID 改字节、原子导入、受管权限、实际
环境/镜像/Pipeline 核验和启用审计，必须由后续 T02 独立保证，不能冒称已实现。

## 首条测试与当前状态

经 root 确认的首个 seam 是真实文件 reader。单个测试在 Fedora 的独占临时目录
写入两个明确为合成的模块 Release 文件，按旧 ID+摘要读取完整旧配置，再读新
版本并重新读取旧版本；调用者修改先前返回值不能改变旧文件或后续结果。测试不
导入 Snapshot fixture、不制造 READY 输入、不连接 ENV，也不模拟文件系统。

首 test/stub 候选固定为 `c5911e13efd69f495d2e7833e81cbedbb80cb55f`，在 Fedora
独占新 worktree 已取得有效行为 RED：真实文件准备成功后，旧版本读取返回
`RELEASE_NOT_IMPLEMENTED`，go test exit 1；没有编译、依赖或准备失败。
证据为本轮 `cpu04-catalogue-red-c5911e13efd69f495d2e7833e81cbedbb80cb55f`。

读取实现候选已加入严格字节解析、显式 ID/digest 核对与有界文件读取。经 root
授权，Runtime/CPU resources/Workspace/Output 的既有局部校验机械提取为共享
私有函数，保持 Snapshot 的原字段错误；参数 grammar 从 Intent 机械提取为
共享函数，保留 Intent presence 规则，不构造伪 Snapshot 或伪 Dataset ID。
尚未取得新固定 SHA 的 GREEN，不是 CODE_READY。后续严格解析、同 ID
改字节、缺失文件、路径和取消等负向分小步加入，不能由首条正向测试推导已覆盖。
