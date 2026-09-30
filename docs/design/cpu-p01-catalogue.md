# CPU04 不可变 Release 目录

来源：原 `CPU04` 第 3、6 项、`CPU01`、v0.4 development D03/D05/D13、operations O08，
以及当前 Goal 第 5、9、10 节。任务来源身份见 `.scratch/cpu-p01/`；本文件承接
[持久化方案](cpu-p01-persistence.md) 的下一目录切片，不复制原任务包。

本切片提供按固定身份读取和不可覆盖的文件导入适配器。当前绑定、启用开关与
generation 的权威仍在 Governance；目录适配器不会启用 Release，也不会生成
InputVersion、ENV 身份或完整 ExecutionSnapshot。T02 的受管授权入口和事实
核验、T03 实际输入导入及 ResolveAdmission 由各自切片实现。

## 文件合同

`contract/cpup01.ReleaseDocument` 是 `ani.modeldev.release.v1`。每个 Release
在受管目录内对应 `<release_id>.json`，ID 为非零标准小写 UUID。新内容使用
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
故意用同 ID 同时换文件与期望摘要。下述 ImportRelease 负责受管写入路径上的
原子安装及同 ID 不覆盖；受管权限、实际环境/镜像/Pipeline 核验和启用审计，
仍必须由后续 T02 独立保证，不能冒称已实现。

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
固定 `f16bcdeb375b5703044b7610b9e3d58381b445aa` 的完整 cpup01、protobuf 与
目录首测试均 GREEN / exit 0，既有 Intent/Snapshot 向量未改变。verify-source
的 Buf/Proto/sqlc/向量生成字节稳定，但其最终全仓 gofmt 检查因并行模块的
`internal/data/objectstore/input_verifier_test.go` 未格式化而 exit 1；不能把该
完整 gate 记 PASS，也没有改动其他 owner 文件。

同 SHA 的 Fedora Python hashlib 独立计算模块 Release 字面量为 2362 字节，
SHA256 `388c7687614c92a2b50a14c8ee29df04f4d8934cf1dbf88d7b31258efc9f3dae`。
它是测试规范向量，不是真实可用 Release 或发布证据。

固定 `9aaf6249c7fc66fa7a851c1b0c53247406493d1a` 的精确摘要向量、37 类严格
JSON/内容、同 ID 改字节、文内身份不符、软硬链接/FIFO/目录/空或超大文件、
非法选择、缺失及取消全部回归 PASS / exit 0（0.048s）。这些行为的既有实现
已经正确，没有制造新 RED。该测试 gofmt 回传后固定于 `1989863`。

规范 JSON 已机械迁移到公共 conformance 的 `release-v1.json` 单一维护点，
精确字节和上述摘要不变；只提供复制的测试数据访问器，保留 reader 测试独立
手写的期望 Go 对象。没有新生产目录或实际来源证明。迁移后固定组合
`241cec394a86ad3967bd3477d3eea9c677268f9b` 的完整合同/protobuf、目录全部测试
及 verify-source 生成稳定性/全仓格式检查均 PASS / exit 0。整个 CPU04 仍 IN_PROGRESS。

## 不可变文件导入适配器

依据 D03/O08，`catalogue.ImportRelease(ctx, trustedDirectory, raw, expectedDigest)`
是 T02 将来复用的窄存储接口，返回 `ReleaseID, Digest, Created`。它不接收目录
状态、不更新 Governance 当前指针，也不授予“已验收/可启用”资格。受认证受管
T02 调用者须先完成真实 ENV、镜像、Runtime、PipelineVersion 及其证据核验；
本 data adapter 尚未接产品命令或外部入口，不能由纯语法通过替代这些事实。

写入承诺：复用严格 ParseRelease 并核对实际摘要，在受信 no-follow
目录中创建本次私有临时文件，完整写入与 fsync 后以 Linux
`renameat2(RENAME_NOREPLACE)` 原子安装到 `<ID>.json`，再 fsync 目录后返回。
不支持该原子操作的文件系统须失败，不退回覆盖式 rename。相同 ID+摘要的
已存在文件必须通过真实 ReadRelease 校验才可幂等回放；不同内容冲突，旧文件
不得被覆盖。只清理本次精确临时名字，不扫描或删旧版本。

取消或在写入后遇到同步错误可能留下原 ID 的已安装文件；调用者重试同 ID 与
摘要完成对账，不换 ID、不给失败冒充成功，也不删除唯一已有版本。
目录应由受管配置指定，仅授权发布身份可写；应用 reader 可只读挂载。数据库、
schema 和当前启用记录均不在本切片。

经 root 确认的首个测试只要求：成功导入回执后，独立真实 ReadRelease 能按
同 ID/digest 读取完整原配置。首 test/stub 固定 `88fa723925872662f893fcc76fff219bc3339111`
已在 Fedora 取得有效 RED：目录与 reader 预检 PASS 后 Import 返回
`RELEASE_IMPORT_NOT_IMPLEMENTED`，exit 1（0.004s）。首次原子安装实现候选
复用仓库已锁定 x/sys v0.47.0，实际使用 RENAME_NOREPLACE 和文件/目录 fsync；
格式与 x/sys direct 分类回传后，固定 `341ca00f685d865140cbb4386d4d3dbdec7b98d9`
的首次 Import、目录全模块及完整合同/protobuf GREEN / test.exit 0。该版本的
verify-source 生成内容稳定，最后全仓格式 gate 因并行输入模块未格式化而失败，
不冒称全 gate PASS。该阶段的同件 replay 尚未实现，下一独立测试要求 Created=false、
原 ID/digest 回执及完整旧文件保持。固定 `1d1d66acc646f64ffac0eee39e4600832ff702a1`
已在 Fedora 得到有效 replay RED：首次真实 Import 成功后，原件重投错误返回
RELEASE_CONFLICT，exit 1（0.008s）。候选修复在原目录文件描述符上复用 reader
的完整字节/身份/安全读取校验，再同步目录并返回 Created=false；不凭文件名、
stat 或旧进程内结果回放，也不因目录路径切换而验证另一目录。格式回传后
固定 `d2c89b6f4da7059e53067c73d8178cdd3af9cad1` 的首次导入、严格同件回放、
完整目录与合同/protobuf 测试均 GREEN / exit 0（catalogue 0.046s）；没有重复
运行其他 owner 正在变更的全仓 gate。

固定 `daec8bd040b813e6b775e64b707e7b9cf19002a2` 的回归覆盖同 ID 异件拒绝、
六并发同件仅一次 Created、四并发两种内容唯一
赢家/同件回放、取消和非法候选无安装、不安全或损坏 existing 不得回放/覆盖、
symlink 根目录拒绝。并发通过同时释放的 channel 与逐项等待结果完成，不用
sleep 推测顺序。完整目录及合同/protobuf 均 PASS / exit 0（catalogue 0.197s），
两项并发行为各 `-race -count=20` 均 PASS / exit 0（1.750s）。已有行为正确，
没有制造 RED；唯一新增测试文件的 gofmt 已回传，格式固定版本复验待完成。
真实文件测试不证明断电恢复，更不证明 T02 的真实环境校验或授权入口已经交付。
