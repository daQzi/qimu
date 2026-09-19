# P02 交付与用户验收：统一操作入口与 Agent 调用

> 状态：用户已确认 P02 验收通过，提交 `0783a2e1`，已合入 `codex/codex-plugin`；当前继续 P03。本文保留 P02 交付时的能力与 schema 记录。
> 开发流程：独立 worktree 的 `codex/plugin-p02-operations` 同步 main v1.5.4 后实施，阶段完成后合并到 `codex/codex-plugin`；不合并 main、不推送。

## 1. 本阶段交付

- `operation_search/describe/invoke` 共用后端服务；HTTP、插件中心调试按钮和 Cloud Agent 调用同一权限、Schema、版本与资源归属校验。
- `resource.inspect` 同步读取当前用户已上传且 ready 的视频元数据，不创建 Run，不返回存储 URL、ObjectKey、Endpoint、Bucket 或凭据，不伪造未知尺寸/时长。
- `resource.snapshot` 将同一份元数据准备为持久结果，创建单步 PluginRun/Step/Event，必须由用户在运行卡片批准（包括 Agent 的 auto 模式）；Agent 只持有 runId 引用，不能自批。已停止或检查点过期的 Agent 不能新建运行。
- 持久写入使用 `(user_id, Idempotency-Key)` 唯一身份；相同请求返回原 Run，不同请求冲突。批准前重新读取资源并核对摘要，资源变化会使旧批准失效。
- ResultRef 只在 succeeded 后出现；当前为小 JSON 内联结果，没有画布节点。待审批和成功的 PluginRun 保护源视频不被删除；拒绝或取消会释放该运行的源资源引用，其他业务引用仍生效。
- Cloud Agent 支持 `hostSurface=agent-home` 且 canvasId 可空：仅加载用户偏好、技能和插件工具，不创建临时画布，不暴露 canvas、generate_media 或 task_get。原画布 Agent 保持原工具和恢复路径。
- 插件中心增加操作调试区和可恢复运行卡片。提供 P02 资源助手 1.2.0 验收包，不调用远程 API、模型或媒体生成。

P02 仍不包含：远程 Connector、Task Worker 插件作业、Pipeline、画布 `plugin-result` 节点、首页无画布 Agent UI。它们分别在 P03–P08 推进。

## 2. main 同步与数据库迁移

阶段开始时已把 main `308cf424`（v1.5.4）合并到阶段分支，保留其支付 FM、模型展示名归组、管理员积分成本和画布删除修复，并重新运行插件、Agent、数据库和前端回归。

当前 schema 为 **29**：

| 版本 | 主线新数据库 | 已执行 P01 v27 的验收数据库 |
| --- | --- | --- |
| 27 | `channel_credit_cost` | 保留原 `application_plugin_releases` 名称与校验和 |
| 28 | `application_plugin_releases` | 补 `channel_credit_cost`，校验和沿用主线定义 |
| 29 | `application_plugin_invocations` | 同左，创建 PluginRun/Step/Event |

迁移器只对名称和校验和精确匹配的 P01 v27 使用兼容谱系；未知 v27 继续拒绝。两条谱系均已在隔离 SQLite 和 PostgreSQL 测试，PostgreSQL 生命周期与 P02 调用也已在独立容器验证。没有迁移真实业务数据库。

部署验收前备份数据库与数据目录，用本分支构建的迁移工具执行 `migrate-schema up` 和 `verify`。旧二进制不理解 v29，不能只切回旧后端；完整回退使用匹配备份和二进制。

## 3. 验收包

[resource-helper-1.2.0.yingce-plugin](../../plugin-packages/application-examples/resource-helper-1.2.0.yingce-plugin)

该包申请 `media.read` 和 `resource.create`，包含：

- `inspect-video`：read，inline。
- `snapshot-video`：draft_write，持久运行并等待用户批准。
- `check-source` Skill：先检查，只有用户明确要求时才创建快照。

包内没有密钥、远程 URL、模型配置或任意代码。1.0/1.1 仍可用于只读检查；1.2 才有快照操作。

## 4. 页面验收

验收分支使用 `codex/codex-plugin`。启动前确认迁移状态为 29，前后端必须来自同一分支。

### A. 安装与授权

1. 管理员进入 `/admin/plugins`，上传 1.2.0 包；平台保持开放。
2. 普通用户进入 `/plugins`，选择 1.2.0，同时勾选 `media.read` 和 `resource.create` 后启用。
3. 技能列表应有 `check-source`；操作清单中 inspect-video 与 snapshot-video 显示“可调用”。取消 `resource.create` 后重新保存时，inspect 可用，snapshot 不可用或技能因必需权限不加入。

### B. 同步读取

1. 先上传一段真实视频，取得它的 Resource ID。可从素材/资源请求返回的 `resourceId` 查看；不能使用本地文件名、节点 ID 或他人 ID。
2. 在插件卡“操作调试与运行查询”输入 Resource ID，点击“读取视频信息”。
3. 结果至少包含 resourceId、kind=video、name、mimeType；数据库已有正数尺寸/时长才显示。不得出现 URL、ObjectKey、Bucket、签名参数或未知值 0。
4. 换成图片资源、未就绪视频、错误 ID或另一账号的 ID，调用必须失败且不创建运行记录。

### C. 持久快照、审批与恢复

1. 对同一视频点击“保存元数据快照”，得到 waiting_approval 运行卡片；此时只有 Preview，没有成功 ResultRef。
2. 刷新插件页面，把卡片中的 runId 填入“插件运行 ID”查询，仍能恢复待确认状态。
3. 点击“确认保存快照”，状态变为 succeeded，卡片显示保存结果，查询接口返回 ResultRef；重复点击或网络重试不产生第二个 Run。
4. 新建另一次快照，在批准前修改/替换该资源的有效元数据；批准应提示资源已变化，需要发起新运行。
5. 对另一运行点击拒绝或取消，状态为 cancelled，之后不能再次批准；该运行不再阻止源资源删除（仍需满足其他业务的删除保护）。
6. 用户 B 用用户 A 的 runId 查询必须返回无权访问/不存在，不能看到 Preview。

### D. Agent 路径

1. 在已有画布 Agent 中选中 `check-source`，明确提供真实 Resource ID，要求“检查视频信息”。Agent 应先读取操作合同，再调用 inspect；不会创建新画布节点或媒体任务。
2. 再要求“保存元数据快照”。Agent 返回 PluginRun 引用，画布侧显示运行卡片；只能由用户点“确认保存快照”，auto 模式也不能自动批准。聊天中的 Agent 不应生成自身的画布审批卡来替代 PluginRun 审批。待确认期间仍可读取视频信息，但不能发起另一项持久写入。
3. 刷新会话后运行卡仍可按 runId 查询；批准后新一轮可用 `plugin_run_get/result_read` 获取结果。
4. 原有画布读取、节点写入审批、媒体生成审批和技能按需读取路径保持可用。

无画布 Agent 后端已自动验证，但 P02 不新增首页入口。可通过登录态 API 创建 `hostSurface=agent-home` 且无 canvasId 的 Agent Run；前端入口按实施手册留在 P08。不要用自动创建空画布代替此验收。

## 5. 自动验证

- 最终完整回归：`go test ./internal/app -count=1` 通过（458.476 秒）；`go test ./internal/database ./internal/repository ./internal/plugins/... ./internal/handler -count=1` 通过，其中迁移与插件调用使用独立 PostgreSQL 测试库。插件域与 repository 的 race 检查通过。
- P02 app 测试：资源隔离、Schema、授权、幂等、审批、取消、资源变化、事件序号、删除保护、Agent 合同固定、无画布创建/恢复及真实 Worker 工具表面。
- P02 plugins 测试：SQLite/PostgreSQL inline 与持久调用、同键防重、用户批准。
- 数据库测试：SQLite/PostgreSQL 上的 main v27 和 P01 v27 两条谱系都升级到 v29 并保留 P01 数据。
- 真实 Gin 路由：检索、describe、inline invoke、跨用户拒绝，以及 P01 生命周期回归。
- 前端：生产 build（含 TypeScript）通过；插件合同/权限与 main 模型、画布等 8 个文件共 141 项测试通过。构建仍提示部分产物超过 500 kB。

未调用真实模型、远程模型服务或收费媒体生成。Agent Worker 测试使用本地 mock 文本上游，只检查实际请求工具集合和持久状态。尚未进行浏览器页面验收；上述页面流程由用户验证。实际使用 Agent 仍遵循现有模型计费。

后端复验（`backend`）：

```sh
go test ./internal/app -run '^TestP02' -count=1
go test ./internal/handler -run '^TestP01ApplicationPluginHTTP$' -count=1
go test ./internal/plugins -run '^TestP02' -count=1
go test ./internal/database -run '^TestP02MigrationAcceptsMainAndP01Version27Lineages$' -count=1
```

PostgreSQL 只在 `CANVAS_TEST_POSTGRES_DSN` 指向独立测试库时执行，测试自行创建和删除 schema。

首次完整回归发现项目封面测试缺少新增引用表，以及内存 SQLite 删除测试与后台清理争用连接；已补齐测试建表并串行化该测试夹具的连接，随后相关测试与完整应用套件均通过。未放宽生产删除校验。

前端复验（`web`）：

```sh
bun run build
```

## 6. 阶段边界

操作执行没有按插件 ID写进 Agent 主循环；Agent 只使用通用工具，实际 Adapter 在 app 组合根注册。`internal/plugins` 不依赖 app/service。插件工具结果有大小和 Schema 上限，Skill/Manifest 不能修改用户授权、实际效果或审批状态。

用户已回复 **“P02 验收通过，继续 P03”**。后续结果卡片与画布绑定见 [P03 验收说明](qimu-plugin-p03-acceptance.md)；仍按阶段合入 `codex/codex-plugin`，不合并 main、不推送。
