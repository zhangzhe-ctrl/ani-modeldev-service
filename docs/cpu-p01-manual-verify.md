# CPU-P01 训练验证记录（新 CSV 全链路）

更新日期：2026-10-10。面向验收与运维人员。

对应文档：[手动部署与当前状态](cpu-p01-manual-deploy.md)、[手动复部署与业务测试](cpu-p01-manual-test.md)。

本文记录 2026-10-09 至 2026-10-10 在 `ani-system` 用**全新合成的 CSV** 跑通 CPU-P01 正向业务链与反例矩阵的实际结果，并如实标注尚未执行的部分。所有结论以集群实测为准，不包含登录凭据与签名 URL。

## 1. 完成度总览

| 阶段 | 命令 | 结果 |
| --- | --- | --- |
| 租户对齐核对 | BFF 查询 | ✅ presets=4 / input-versions=5 / executions=23 |
| 启用成功预设 | `t enable success` | ✅ Release 导入 + 绑定启用（generation=1） |
| 登记新输入 | `admin modeldev-import-csv` | ✅ 新 CSV → READY（1024×16） |
| 提交执行（幂等） | `t run bprime-02 success` | ✅ 受理 + 重放一致 |
| 等待完成 | `t wait bprime-02 SUCCEEDED` | ✅ SUCCEEDED / PUBLISHED / CLOSED |
| 独立加载验证 | `t verify bprime-02` | ✅ **L4_PASS** |
| 第二次成功训练 | `t run/wait/verify bprime-03` | ✅ SUCCEEDED / PUBLISHED / CLOSED + **L4_PASS** |
| 反例（隔离/幂等/未授权） | `t negatives bprime-03` | ✅ **PASS**（7 项断言全绿） |
| 正式清理 | `t cleanup-plan/apply` | ⬜ 未执行 |

> 结论：**真实训练、四文件发布、普通 BFF 下载、独立 CPU 加载的正向链已完整跑通并复演一次；手册 §7 反例矩阵（幂等改意图、跨租户隔离、未授权、无 token）亦全部通过**。仅剩手册的正式清理尚未执行。

## 2. 关键事实

| 项 | 值 |
| --- | --- |
| 执行 ID | `f8aa5902-7c77-4b59-8b3a-3dcd1df8ff8d` |
| Operation ID | `b7fc379c-d0ce-4bb6-8536-955f4e54b561` |
| 解析 Release | `fabc4827-165c-4946-aeef-e5a16997dee8`（digest `53c73c02…a61`） |
| 输入版本 | `1cb6b8d9-a108-4194-b4f2-b56723584d35`（1024 行 / 16 维，READY） |
| 输入对象 | `ani-cpu-p01-input-d606e28a` / `inputs/fixed/data-bprime-20261009/data.csv` |
| 对象版本 | `99f575a3-9107-46e3-a6c5-ead786e10df8`（size 157724，sha256 `d1e660bd…398f`） |
| KFP Workflow | `general-cpu-wkfq4`（Succeeded） |
| 训练参数 | epochs=3 / batch_size=64 / learning_rate=0.01 |

第二次训练（复演，`bprime-03`）：

| 项 | 值 |
| --- | --- |
| 执行 ID | `7af805a8-5370-4dff-92b1-24a8568d16ac` |
| Operation ID | `7c59e676-9881-4538-a245-4215d54b2508` |
| 解析 Release | `fabc4827-165c-4946-aeef-e5a16997dee8`（同一 Release） |
| 输入版本 | `1cb6b8d9-a108-4194-b4f2-b56723584d35`（同一 READY 输入） |
| TrainJob | `md-7af805a8-5370-4dff-92b1-24a8568d16ac`（jobset `Complete`） |
| 训练参数 | epochs=3 / batch_size=64 / learning_rate=0.01 |

## 3. 执行结果

### 3.1 状态机

```
compute_state   = SUCCEEDED
delivery_state  = PUBLISHED
close_state     = CLOSED
```

`t wait` 最终输出 `{"result":"PASS","expected_compute":"SUCCEEDED"}`。

### 3.2 独立加载（`t verify`）

独立 verifier Job **不挂载原训练 PVC、不持有 S3 key、不自动挂载 SA token**，经 BFF 授权下载并核验四个产物：

| 文件 | 字节 | sha256 |
| --- | --- | --- |
| `model_config.json` | 116 | `1286ff35…e0f9` |
| `model.pt` | 4903 | `730424bb…67d7` |
| `summary.json` | 357 | `d67f7413…e035` |
| `metrics.jsonl` | 7236 | `78b53233…1c36` |

verifier 严格加载真实 PyTorch 权重并做 CPU 前向，输出：

```json
{"event":"L4_PASS","execution_id":"f8aa5902-...","shape":[4,2],"device":"cpu","epochs":3,"optimizer_steps":48,"artifact_count":4}
```

`48` 次 optimizer update 与合同（3 epoch × 64 batch × 1024 样本）一致。

### 3.3 发布产物（对象存储）

执行完成后，产物已发布到对象存储 `ani-cpu-p01-artifact-d606e28a`，前缀为租户与该执行：

```
executions/46936e6b-3a3b-4868-8d88-6833665abb70/f8aa5902-7c77-4b59-8b3a-3dcd1df8ff8d/
├── model.pt              4903
├── model_config.json      116
├── metrics.jsonl         7236
├── summary.json           357
└── output-manifest.json  1228   ← 发布清单/收据
```

| 对象 | 说明 |
| --- | --- |
| `model.pt` / `model_config.json` | 模型权重与结构 |
| `metrics.jsonl` / `summary.json` | 训练指标与汇总 |
| `output-manifest.json` | 发布清单：登记四文件的引用与输入摘要，是「四文件已发布」的凭证 |

> `PUBLISHED` 的物证即上述对象存在；`t verify` 通过 BFF 下载授权取得前四个文件并逐一核对字节与 SHA256（见 §3.2）。

### 3.4 第二次训练（复演）

以**新执行名** `bprime-03` 在同一预设、同一 READY 输入上再次提交，验证结果可复演、非偶然：

- 提交 `POST /executions` 返回 `202`，且**同键重放**返回同一 `execution_id`/`operation_id` 且 `replayed=true`（幂等 PASS）；
- 状态机推进：`SUBMISSION_CONFIRMED → TRAINING → SUCCEEDED`，随后 `delivery_state=PUBLISHED`、`close_state=CLOSING → CLOSED`；
- `t wait` 输出 `{"result":"PASS","expected_compute":"SUCCEEDED"}`；
- 独立 verifier 再次下载四文件并严格加载：

```json
{"event":"L4_PASS","execution_id":"7af805a8-...","shape":[4,2],"device":"cpu","epochs":3,"optimizer_steps":48,"artifact_count":4}
```

`48` 次 optimizer update 与合同一致（3 epoch × 64 batch × 1024 样本），与首次训练完全相同，说明链路稳定可复现。

### 3.5 反例矩阵（`t negatives`）

在第二次执行 `bprime-03` 上执行反例矩阵，覆盖幂等、参数校验、租户隔离、越权与未认证五类拒绝路径，**7 项断言全部符合预期**：

| 断言 | 期望 | 实测 | 语义 |
| --- | --- | --- | --- |
| `same-key-changed-name` | 409 | `IDEMPOTENCY_CONFLICT` | 同幂等键但改名 → 拒绝（幂等键绑定原意图） |
| `wrong-epochs` | 400 | `INVALID_MODELDEV_CREATE` | 非法训练参数（epochs=4 越界）→ 拒绝 |
| `gpu-kind` | 400 | `INVALID_MODELDEV_CREATE` | CPU 预设提交 GPU 类型 → 拒绝 |
| `b-execution` | 404 | `RESOURCE_NOT_FOUND` | **跨租户隔离**：租户 B 看不到租户 A 的执行 |
| `b-artifact` | 404 | `RESOURCE_NOT_FOUND` | **跨租户隔离**：租户 B 取不到租户 A 的产物内容 |
| `ungranted-execution` | 403 | `FORBIDDEN` | **同租户越权**：无 ModelDev 授权用户被拒 |
| `no-token` | 401 | `401` | 未携带令牌 → 未认证 |

> 该矩阵证明：执行与产物按租户强隔离（跨租户返回 404 而非泄露），权限按用户模型判定（无授权返回 403），幂等与参数边界按合同拒绝。至此手册 §7 反例矩阵在真实环境通过。

## 4. 验证流程与验收标准

本节说明本次实际执行的步骤、每步的入口与通过判据，便于复现与核对。

### 4.1 流程总览

```
用户登录(BFF) → 启用预设(enable) → 登记输入(import-csv)
     → 提交执行(run，幂等) → 观察至关闭(wait) → 独立加载验证(verify)
```

全程经 **Governance BFF** 的 `/admin/v1/modeldev/*` 用户接口发起，管理动作（导入 Release / 登记 CSV / 启用绑定）经 Governance admin CLI 以 mTLS 调 ModelDev；普通用户不直连 ModelDev、不参与对象存储授权。

### 4.2 各步骤与通过判据

| 步骤 | 入口 | 通过判据 |
| --- | --- | --- |
| 登录 | `POST /api/v1/auth/password/login` | 返回 access_token，且其权限声明含 `modeldev:*` |
| 启用预设 | `enable success`（admin `modeldev-import-release` + `modeldev-enable`） | Release 导入成功；绑定 `generation=1`、`new_submissions_enabled=true` |
| 登记输入 | admin `modeldev-import-csv` | 状态 `READY`，`row_count=1024`、`feature_count=16` |
| 提交执行 | `POST /executions`（`202`） | 返回 execution/operation；**同键重放**返回同一对 ID 且 `replayed=true` |
| 观察完成 | `GET /executions/{id}` 轮询 | `compute_state=SUCCEEDED`、`delivery_state=PUBLISHED`、`close_state=CLOSED` |
| 独立加载 | verifier Job（`verify`） | 四文件字节/SHA 校验一致；严格加载权重、CPU 前向输出有限 → `L4_PASS` |

### 4.3 训练合同比对（实测 vs 约定）

| 合同项 | 约定 | 实测 |
| --- | --- | --- |
| 类型 | `GENERAL_TRAINING`，CPU 单节点 | 一致 |
| 输入 | CSV，`x0..x15,label`，1024 行 × 16 维 | 一致（`1cb6b8d9`） |
| 网络 | MLP `16 → 32 → 2` | verifier 前向输出 `[4,2]` |
| 训练 | Adam，3 epoch，batch 64，48 次 update | `epochs=3`，`optimizer_steps=48` |
| 产物 | `model.pt`/`model_config.json`/`metrics.jsonl`/`summary.json` | 四文件齐全且校验通过 |

### 4.4 覆盖与未覆盖

- **本次覆盖**：单租户（tenant-a）的完整正向链（授权受理 → 冻结 → KFP 训练 → 四文件发布 → BFF 授权下载 → 脱离训练卷的独立 CPU 加载），正向链**复演一次**（`bprime-03`）；反例矩阵全部 7 项（幂等改意图、非法参数、跨租户隔离、未授权、无 token）。
- **本次未覆盖**：训练失败（fail）与 Stop/deadline 三种非成功路径在真实环境的重演、正式清理。详见 [§6 待办](#6-尚未执行--待办)。

### 4.5 与手册的对应

本流程对应 [手动复部署与测试手册](cpu-p01-manual-test.md) §5（`login → catalogue → enable`）与 §6（`run → wait → logs → inspect → verify`）。本次以**全新合成的 CSV** 执行，替代手册中复用既有 READY 输入的做法，其余步骤与判据与手册一致。

## 5. 走通本流程所做的四处修复

这些修复位于部署包的测试脚手架/集群配置中，非 modeldev 业务代码。

### 5.1 `test.py` 的 admin 镜像与配置源

现象一：`manage()` 用在线 `ani-governance` server 镜像，该镜像入口为 `/app/bin/server`，**无 `/app/bin/admin`** → Pod `StartError`。

修复：`manage()` 改用含 admin 的镜像 `172.16.101.10:5000/ani-governance-cpu-p01:live-20261005-05`（`imagePullPolicy: IfNotPresent`）。该镜像 admin 支持 `modeldev-import-release / modeldev-import-csv / modeldev-enable`。

现象二：admin 报 `modeldev management requires a valid tenant user access token and session`。根因是 admin Job 挂载单数 Secret `governance-config`（旧库 `ani_cpu_p01_governance` + HS256），而在线 governance 用复数 Secret `governance-configs`（新库 `ani_governance` + RS256）。

修复：manner Job 的 `config` 卷改用 `governance-configs`，initContainer 改为复制整个配置目录（而非仅 `bootstrap.json`）。

### 5.2 `release-imports.json` 与已部署 facts 对齐

现象：`t enable success` 的 admin 返回 503 `query unavailable`（modeldev 侧 `ImportRelease` 查询失败）。

根因：`release-imports.json` 是重新 render 生成的，其 release_id 与**已部署的** `md-fixed-config` facts / `modeldev-manual-releases` catalogue 不一致。

修复：以已部署 catalogue 的 canonical Release 重建 `release-imports.json`，四个 mode 的 digest 与 facts 逐一核对一致：

| mode | release_id | 已部署 facts digest（一致） |
| --- | --- | --- |
| success | `fabc4827-165c-4946-aeef-e5a16997dee8` | `53c73c02…` |
| fail | `cf61932f-59e1-42ed-86eb-7912a9373a5a` | `7f066d72…` |
| stop | `3db624c8-e83a-40d4-af72-e5102d27d09a` | `1767ce78…` |
| deadline | `cf914145-4d68-430d-bdda-49343e97b9e2` | `a43b1f0c…` |

### 5.3 exit-retention webhook 超时（**必须固化，否则复现会卡**）

现象：首次执行 `b79fb009` 训练计算 `SUCCEEDED`，但 `close` 阶段 Pod 被拒：

```
failed calling webhook "step-exit-retention.modeldev.ani.io":
Post "https://ani-modeldev-exit-retention.ani-system.svc:443/mutate?timeout=2s": context deadline exceeded
```

工作流整体 `Error`，执行卡在 `delivery_state=PENDING / close_state=OPEN`。

根因：`MutatingWebhookConfiguration ani-modeldev-exit-retention` 的 `timeoutSeconds: 2` 对"每次冷建 TLS 且握手串行化"的 Python 后端（`/app/retention.py`）太紧；早前的执行只因抖动才未命中。

修复（保持 `failurePolicy: Fail`）：

```bash
kubectl patch mutatingwebhookconfiguration ani-modeldev-exit-retention --type=json \
  -p '[{"op":"replace","path":"/webhooks/0/timeoutSeconds","value":10}]'
```

> ⚠️ **固化提醒**：`manual.py render` 从 seed 重建 webhook 时**不覆盖 `timeoutSeconds`**，任何重新 render/deploy 都会把 2s 打回。需把本 patch 记入部署流程（改 seed 或加 patch 步骤），否则下次复现会重新卡在 close。
>
> 该 webhook 作用域极窄（仅 `ani-kfp-manual-a` 内由 argo 创建、带 KFP v2 标签的步骤 Pod），改超时不影响控制面。

### 5.4 `test.py negatives` 的 `UnboundLocalError`

现象：执行 `t negatives` 立即报 `STOPPED: UnboundLocalError`。

根因：`negatives()` 内有一句函数级 `import urllib.error`，使 `urllib` 在整个函数作用域被当作**局部名**；而同函数上一行的 `urllib.request.Request(...)` 在赋值前引用它，触发 `UnboundLocalError`。

修复：把 `urllib.error` 提到模块级 `import`，删除函数内的局部 import。修复后 `t negatives` 通过（见 §3.5）。

## 6. 尚未执行 / 待办

| # | 项 | 前置条件 |
| --- | --- | --- |
| 1 | 非成功路径复演（fail / stop / deadline） | 无，可直接用对应预设 `t run/wait` |
| 2 | 正式清理 `t cleanup-plan/apply` | 需先选定目标执行 |
| 3 | `modeldev-mtls` 客户端证书换发 | **2026-10-12 到期**，到期后 governance→modeldev mTLS 失效 |
| 4 | webhook `timeoutSeconds` 固化 | 见 5.3 |

## 7. 复用命令（本次实际使用）

在 ani-01 部署包目录 `manual-ani-system-20261008/` 下：

```bash
# 1) 长生命周期 BFF 转发（前台或后台 setsid）
sudo -n kubectl -n ani-system port-forward --address 127.0.0.1 service/ani-governance 19778:7788

# 2) 登录三类身份（tenant-a admin / tenant-b / 同租户未授权用户）
#    凭据材料放在 .private/live-governance-20261005/（bootstrap-settings.json + *.password）
python3 test.py login

# 3) 启用 + 登记新输入（.private/bprime-import.request.json 指向新对象版本）
python3 test.py enable success
python3 -c "import test; test.manage('modeldev-import-csv','manual-import-csv-bprime', <request>)"

# 4) 预置 bprime-0x.request.json（dataset_version_id=新 input_version_id）后训练与验收
python3 test.py run    bprime-03 success
python3 test.py wait   bprime-03 SUCCEEDED
python3 test.py verify bprime-03

# 5) 反例矩阵（幂等改意图 / 非法参数 / 跨租户 / 越权 / 无 token）
python3 test.py negatives bprime-03
```

## 8. 相关文件

- 部署入口：`scripts/cpu-p01-manual/manual.py`
- 业务测试：`scripts/cpu-p01-manual/test.py`
- 验收实现：`scripts/cpu-p01-manual/acceptance.py`
