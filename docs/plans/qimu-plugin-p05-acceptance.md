# P05：持久流程与用户输入

状态：用户已确认 P05 验收通过。P04 已验收；本阶段从含 P00–P04 的集成提交 `98fc867a` 创建独立 worktree，分支 `codex/plugin-p05-pipelines`。开始前 fetch origin 并同步 main `cb68476e`；提交前发现本地 main 追加 v1.5.5（`b9d60f98`），通过 `7bc6489d` 合入并保留其媒体/画布更新。此时 origin/main 仍为 `cb68476e`，本地 main 已包含远端全部提交，未覆盖其他任务的新提交。完成后只合入 `codex/codex-plugin`，不合入 main、不自动推送、不自动部署、不操作业务数据库。

## 1. 本次能力

插件可声明顺序流程：资源检查 → 等待用户选择 → 远程处理 → 结果。后台 Worker 独立推进流程，关闭页面、Agent 结束本轮不会丢失进度；重新进入插件中心可从“我的插件运行”找回。输入草稿和正式提交分开，旧页面/重复提交不能覆盖新内容。

每个副作用步骤创建原有 PluginRun；该子 Run 的用户审批、报价、连接修订、Task、取消、恢复、账务和资源导入均复用 P04。没有父级“批准全部”，提交表单不代表付费或发送数据授权。父流程保存原 Agent 身份，后续收费子任务继续受其累计预算约束。

本阶段不包含并行、foreach/when、批次预算、派生重做、跨包步骤、嵌套 Pipeline、自定义输入 view、画布 plugin-input 节点或实际视频模型。它们不被默默忽略；未支持的执行合同明确拒绝。

## 2. 作者接入合同

源样例：`backend/internal/plugins/contracts/testdata/pipeline-helper-p05/`。
安装包：`plugin-packages/application-examples/pipeline-helper-1.0.0.yingce-plugin`。

Manifest 新增：

```json
{"contributes":{"pipelines":[{"id":"process","ref":"pipelines/process.json"}]}}
```

入口 Operation 仍通过统一调用入口发现、描述和调用：

```json
{"execution":{"kind":"pipeline","pipeline":"process","mode":"task"}}
```

入口和 Pipeline 的 inputSchemaRef/outputSchemaRef 必须相同。入口权限/效果至少覆盖子步骤合集，另含 draft_write；画布/项目要求不能弱于子操作。步骤引用同一不可变包内的普通 host/http Operation，实际调用时仍再次检查用户、功能门禁、版本和授权。

步骤为最多 64 步的显式顺序链，第一步无依赖，后续每步依赖紧邻前步，可额外声明更早依赖以读取其结果。支持：

- operation：operation、dependsOn、inputs。
- wait_input：formSchemaRef、dependsOn；Schema 来自固定包。
- 参数绑定：`{"literal": ...}`、`{"from":"input#/resourceId"}`、`{"from":"steps/choose#/prompt"}`。引用前一步以外结果时必须额外声明 dependsOn；缺失路径暂停，不填猜测值。
- 最终 outputs 使用同一绑定规则。单次输入/最终结果最多 64KB，累计步骤结果 256KB，Schema 合计 24KB。媒体传资源引用。

简单 object 字段生成普通表单控件；嵌套/引用等复杂 Schema 使用 JSON 编辑与格式说明，由同一后端校验。没有插件脚本执行，也不会用 Schema default 自动代替用户选择。

## 3. API 与 Agent

| 入口 | 语义 |
| --- | --- |
| POST /api/plugin-invocations | 原入口启动 Pipeline；固定发布、幂等键和权限 |
| GET /api/plugin-runs?offset=0 | 当前账号根运行历史，每页 30 条 |
| GET /api/plugin-runs/:id | 权威快照，含 pipeline cursor/steps/inputs/schemas/outputs/childRunId/stepRuns |
| PUT /api/plugin-runs/:id/inputs/:inputId | revision、mode=draft/submit、value；submit 带 Idempotency-Key |
| GET /api/plugin-runs/:id/events | SSE event=plugin-run，id=sequence，Last-Event-ID 优先于 after |
| POST /api/plugin-runs/:id/cancel | 父流程停止调度并收束原子任务 |
| POST /api/plugin-runs/:id/resume | 暂停父流程仅支持 retry_safe 重新检查条件 |

SSE 只通知状态与 revision，不包含输入正文或凭据；客户端按事件刷新快照。连接最多 25 秒，重连按序号重放；超前游标返回冲突。历史输入/结果仍按账号隔离，Nginx 仅对该精确事件路径关闭缓冲。前端保留定时快照刷新作为断线恢复。

Agent 仍用 operation_search/describe/invoke，新增 plugin_input_submit，与表单共用 UpdateInput。模型必须先读请求 Schema，仅提交用户明确提供的字段；缺少字段要询问。只读模式不提供此工具，过期/停止的 Agent 检查点不能提交。收费审批不向模型开放。

步骤与远程 Task 的关系通过父 Run → stepRuns（parentStepKey）→ 子 Run.taskId 持久追溯；只读步骤输出、已提交用户输入单独保存。Agent 恢复优先关联父流程。

## 4. 调度与数据

schema 34 追加 `application_plugin_pipelines`：

- PluginRun 新增 pipelineId、parentRunId、parentStepKey。
- PluginPipelineExecution 保存游标、当前子运行、累计输出及 Run 租约。
- PluginInputRequest 保存输入请求 ID/Schema/revision、草稿、提交值与幂等摘要。

每次调度只做一个有界、无网络的数据库转换。目录事务串行化授权、撤权和取消；Run revision CAS 与 owner/expiry fence 保护提交。游标和子任务准入同事务提交；外部网络仍由原 Task Worker 执行，不新增重作业队列。每 2 秒扫描最多 100 个可推进流程，按最近检查时间轮转；等待用户输入和暂停不占任务槽。

输入同键同值可重放，同键不同值/旧 revision 冲突。关闭或升级插件阻止新步骤，已提交远程任务仍可查询/取消/导入/结算；不会把在途运行静默换到新包。远程提交未知时父流程保留原子运行，不能重建生成。子运行失败/拒绝会收束父流程。

## 5. 手动验收

1. 在测试环境备份数据库、插件包与 .settings-key，按现有部署流程执行迁移并更新前后端到集成分支。确认 schema 34；不要与旧 Worker 混跑。此次交付没有替你执行部署或业务库迁移。
2. 管理员导入 `pipeline-helper-1.0.0.yingce-plugin`。用户在插件中心启用“持久流程示例”，授予 media.read 与 connection.use。
3. 复用 P04 演示服务。宿主机运行时在 backend 目录执行 `go run ./cmd/plugin-remote-demo -listen 127.0.0.1:18081`。后端精确配置 `CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=127.0.0.1`，连接地址为 `http://127.0.0.1:18081`、公开测试凭据 `demo-key`。容器部署应使用容器可达的独立测试地址，不把容器 localhost 当宿主机。该服务仅 JSON 回显/固定图片，不进行视频分析。
4. 在画布上传一段视频，取得真实资源 ID；插件中心“调试持久操作 process”输入 `{"resourceId":"实际视频资源ID"}` 后创建运行。也可让画布 Agent 引用 workflow 技能，先 describe 再启动同一操作。
5. 运行应完成 inspect 后停在 choose。填 prompt，点“保存草稿”，刷新或关闭页面重开，从“我的插件运行”找回，草稿应保留且远程任务尚未创建。
6. 两个页面打开同一输入。A 保存草稿后，B 用旧 revision 提交应报冲突；刷新 B 读取最新状态。缺失必填字段不能推进。
7. 提交完整 prompt，父运行显示等待确认，并嵌入 remote 子运行的费用/连接信息。未批准前上游不得收到调用。重复提交不得生成第二个子任务。
8. 在子卡片确认执行，关闭页面再进入历史，最终应看到演示结果。测试环境重启后端也应继续。需要验证收费时，由管理员设置 echo 平台费、准备测试积分，确认只预留/结算一次；BYOK 供应商收费单独显示。
9. 新开流程，待输入时停用插件，提交应拒绝；历史仍可读、可取消。重新启用同一版本后可继续；切换版本不得默默使用新版本。
10. 待输入/待审批时停止流程，不应产生新远程任务；已执行子任务的停止语义延续 P04。故障/未知提交需在原子任务卡片处理，不能靠恢复父流程重新生成。
11. 聊天输入：让 Agent 查询运行、询问 prompt，只提供明确值后提交。应与表单进入相同流程，并且远程操作仍需你点击批准；自然语言理解质量需实际 Agent 页面验收。
12. 回归 P03 结果保存到画布、P04 单任务、主线远程媒体访问、画布连线/字号/工作条切换，确认没有旧功能回退。

## 6. 自动验证与限制

已完成 SQLite/PostgreSQL + Redis 生命周期、输入幂等/旧版本、并发调度、失效租约、撤权/取消、未知提交不重发、真实路由后台 Worker、SSE Last-Event-ID 重放与 v33→v34 迁移保留测试。Agent 缺字段、过期检查点、重复提交、结束轮次后的原预算约束与父流程恢复引用均通过。

实际验证：

- `go test ./internal/app -count=1`：完整 app 通过，269.619 秒。后续增加的 Agent 输入/预算边界和精度修正分别补专项验证。
- `go test ./internal/database ./internal/handler ./internal/plugins/... ./internal/repository ./internal/protocol ./internal/canvas/... -count=1`：全部通过，使用隔离 PostgreSQL/Redis 验证对应分支。
- `go test -race ./internal/app ./internal/handler -run 'TestP05|TestP04|TestP03|TestP02' -count=1`：通过，app 424.675 秒、handler 85.229 秒。
- 最终 P05 单独 race 复核通过，app 228.895 秒、handler 41.653 秒；字段引用额外覆盖 null、JSON Pointer 转义、缺失字段和共享合同的安全整数范围，超范围数字拒绝而不静默舍入。
- 前端 13 个文件 127 项测试通过（插件合同/展示、主线连线/字号/工作条、媒体任务与素材持久化）；主线同步后 `bun run build` 通过。
- 相关 Go vet、OpenAPI YAML 解析及 P05 路径检查、`git diff --check` 通过。

测试服务为本阶段独立容器与临时数据库/HTTP fixture，没有迁移业务数据。阶段结束移除测试容器及其测试卷；保留测试日志和工作区。主线同步时只暂存两份文档，备份 stash `e7b3739f` 已应用并保留，不应重复应用。

没有启动业务开发服务器、执行浏览器交互或调用真实收费模型；页面及自然语言对话效果由本节人工验收。现有前端大 chunk 警告不影响构建，但未在本阶段治理。

## 7. 回退与阶段关口

关闭新流程入口后，保留当前版本 Worker 处理已提交流程/子任务，或由用户明确取消。不可通过关闭开关抹除账务或外部任务。历史结果/输入继续读取，不能直接删除表、包、资源或 .settings-key。需要整体版本回退时先停止新写并备份，确认在途任务已处置，再恢复同版本数据库/包/密钥与代码；不能只回退二进制后宣称流程已停止。

P05 合入集成分支后暂停；用户确认“P05 验收通过，继续 P06”才开始并行/批次预算。
