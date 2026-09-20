---
title: 应用插件 v3 开发与接入指南
description: 当前可用的插件模板、打包校验、Agent 操作、远程连接、持久流程与画布接入。
---

# 应用插件 v3 开发与接入指南

本文只描述插件集成分支中已实现的接入合同。P00–P07 已验收，P08 与插件收尾 P14A 的手工验收单独记录；不代表生产已部署。完整目标架构见[设计文档](../../../design/qimu-plugin-platform-v3-design.md)，交付检查见[插件收尾验收](../../../plans/qimu-plugin-closeout-acceptance.md)。

## 1. 插件负责什么，宿主负责什么

插件是安装与版本单位，Skill 是 Agent 的方法说明，Operation 是有输入输出约束的动作。启幕负责权限、批准、任务、资源、费用和画布落盘；技能正文不能授予权限，Agent 不能代替用户批准。

| 需求 | 当前接入方式 |
| --- | --- |
| 方法指导、品牌规范、交付要求 | 纯 Markdown Skill，无需 Operation |
| 读取已上传视频元数据、保存结果快照 | 声明 Host Operation，使用 resource.inspect / resource.snapshot |
| 外部模型或业务服务 | HTTP Operation + Connector，由后台 Task 调用 |
| 多步骤、等用户输入、批次处理与局部重做 | Pipeline + Schema，复用运行、输入、批准和派生机制 |
| 结果卡片、表单、画布节点与连线 | 标准 View / CanvasBlueprint，用户批准后投影 |
| 通用业务入口、动态输入与配方组合 | Workbench + Recipe，要求 hostApi ^3.1.0，见本文 P09 接入合同 |
| 自定义模型渠道协议 | 使用原 v1/v2 Provider 体系，不混进 v3 应用 Manifest |
| 新执行类别、任意自定义 UI 或任意本地代码 | 当前不能仅靠包获得；需要单独扩展可信宿主能力 |

无需修改 Agent 主循环即可增加已有执行类别的新插件。新增宿主执行类别需要实现 Adapter，并补权限、结果合同及测试。“通用”不等于可以通过插件绕过资源归属、付费确认或执行任意代码。

## 2. 十分钟创建第一个包

工具需要项目指定的 Go 版本（见 backend/go.mod），从仓库根目录开始：

```sh
cd backend
go run ./cmd/plugin-contract -init ../../delivery-check -template resource -id delivery-check -publisher my-studio
go run ./cmd/plugin-contract -dir ../../delivery-check
go run ./cmd/plugin-contract -dir ../../delivery-check -out ../../delivery-check-1.0.0.yingce-plugin
go run ./cmd/plugin-contract -package ../../delivery-check-1.0.0.yingce-plugin
```

目标目录的父目录必须存在；模板目录及成品包必须尚不存在。可改用自己的新目录/文件名，工具不会覆盖旧文件。目录中不要混入 README、截图、node_modules、密钥或其他开发材料；此工具校验的是交付包根目录。

- `-template skill`：纯目标澄清技能，无权限和操作。
- `-template resource`：素材交付检查，包含读取、保存、画布投影三个操作及对应 Schema/View/Blueprint。仅检查已有元数据，不抽帧、不识别人脸、不调用模型。
- `-id` 和 `-publisher`：小写字母开头，可含数字和连字符，模板工具限制 1–64 字符。使用自己的稳定命名。
- `-version 1.1.0 -out <新包>`：生成新发布，不修改源 Manifest。
- `-reserved-ids id-a,id-b`：离线检查已知保留命名。服务器仍会独立检查真实命名空间与发布者归属。
- `-dir`、`-package`、`-init` 三种模式互斥；`-out` 与 `-version` 仅用于目录模式，版本覆盖必须指定输出包。

成功输出的 `valid: true` 表示静态合同通过；`runtimeVerified: false` 表示工具没有安装、授权或执行。保留的 `profile: p00-contract/1` 是冻结的合同配置标识，不代表当前运行时仍处于 P00。不要将工具输出当成供应商可达、模型效果或安全审核报告。

打包完成会使用真实安装器的有界 ZIP 读取和合同校验再验证。包摘要基于路径与文件内容，不依赖 ZIP 时间元数据。

## 3. 包结构与稳定合同

```text
manifest.json
skills/<skill-id>/SKILL.md
operations/<operation-id>.json
schemas/<schema-id>.json
views/<view-id>.json
blueprints/<blueprint-id>.json
connectors/<connector-id>.json
pipelines/<pipeline-id>.json
```

只有 manifest.json 必需，其余按贡献使用。Manifest 使用 `yingce.plugin/v3`，包含 id、version、publisher、requires、permissions、dependencies、contributes。可用贡献键为 skills、operations、views、canvasBlueprints、connectors、pipelines。

生成模板是可复制的最小完整示例。权威 Schema 和验证器在 `backend/internal/plugins/contracts/`；完整模板在 `backend/internal/plugins/authoring/templates/`。不要从旧设计稿复制未实施的字段。

限制：ZIP ≤48 MiB、文件数 ≤256、单文件 ≤16 MiB、解压总量 ≤64 MiB；Manifest/JSON ≤512 KiB、JSON 深度 ≤32。内容必须 UTF-8，拒绝重复 JSON 字段、未识别字段、越界引用、路径穿越、符号链接和任意脚本。Schema 是受支持子集，不支持任意远程 $ref。

发布不可原地改写：相同 id/version 的不同内容会冲突，必须增加版本。新增权限须由用户重新确认；新增发布不会自动切换用户当前版本或历史 Run。依赖版本、有效授权和能力可用性由服务器复核。

publisher 是管理员受控安装下的归属标识，不是签名认证；填写“官方作者”不会得到可信执行权限。公开作者注册、签名审核、社区市场尚未开放。

## 4. 安装、启用与 Agent 使用

1. 管理员在 `/admin/plugins` 上传 .yingce-plugin；普通用户不能安装服务器包。
2. 用户在 `/plugins` 选择应用的发布版本并启用，核对请求权限。资源模板需要 media.read、resource.create、canvas.read、canvas.write。
3. 检查“我的技能”中出现插件技能；模板默认 explicit，用户在 Agent 中明确选择后使用。
4. 上传就绪视频，在画布引用它并选择技能，要求“检查这段视频的已有信息，确认后保存到画布”。
5. Agent 取得真实 resourceId，描述并调用检查操作；未知字段应说明未记录。nodeId、文件名不能代替资源 ID。
6. 保存快照、投影到画布分别走宿主批准。未批准前不写入，Agent 不自批；无画布时可以只检查和保存结果。

Agent 通过通用 operation_search / operation_describe / operation_invoke 使用插件，而不是每个插件新增工具名。已启用不意味着每项 Operation 当前都有权限；调用前描述操作，使用服务器返回的可用性和合同，不能凭技能正文推定。

宿主固定当前发布、上下文和授权。首页与画布共享会话属于 P08；插件不创建第二套聊天记录。用户输入、批准和任务状态保存在后台，不依赖 Agent 一次回复或浏览器持续打开。

## 5. Operation 与画布合同

Host 操作声明 inputSchemaRef、outputSchemaRef、requiredPermissions、effects、context、execution；模板展示确切字段。

当前三个可信 Host Adapter：

| Adapter | 权限 | 实际作用 |
| --- | --- | --- |
| resource.inspect | media.read | 读取当前用户就绪视频的已记录元数据，返回摘要 |
| resource.snapshot | media.read、resource.create | 携带上次读取的 expectedDigest，批准后保存结构化结果 |
| canvas.blueprint.instantiate | canvas.read、canvas.write | 将有效结果或受支持输入绑定为标准画布节点，批准后写入 |

画布是结果与交互入口，不是后台工作流状态机。投影前读取 canvas snapshotHash；提交 runId、resultDigest、blueprintId、snapshotHash、instanceKey。重复投影保留同一 instanceKey，只有用户明确要求另建时才换值。画布变化导致冲突时刷新快照、重新请求批准，不覆盖用户编辑、不重新执行原业务。

P07 支持 key-value、表格、实体卡片、媒体对比、输入/映射编辑等标准视图及受控蓝图动作；精确组件和节点合同以[对应样例与验收](../../../plans/qimu-plugin-p07-acceptance.md)为准。不能通过包注入 React/JS 组件或读写任意画布字段。

## 6. 外部 API：先验证协议，再验证模型效果

参考 `backend/internal/plugins/contracts/testdata/remote-helper-p04/`。该样例只验证同步回显、异步任务与图片导入，不具备真实视频处理能力。

当前 HTTP Connector 支持固定 JSON POST 提交、GET 轮询/按提交键查询、POST/DELETE 取消；鉴权为 none、bearer 或自定义鉴权头。请求映射支持 literal、input 字段、已授权 resource 字段，响应使用 JSON Pointer；路径变量仅限受控 jobId / submissionKey。

用户在插件连接面板配置地址与凭据，凭据由宿主加密存储。禁止写入包、技能、提示词或公开结果。初始 URL、拨号及下载重定向均校验出站范围。默认禁止私网，隔离开发环境确需本地演示时按 P04 文档精确放行，不使用宽泛白名单。

单次请求最多 8 个输入资源、1 个媒体产物；请求/响应 JSON ≤256 KiB，发布结果 ≤64 KiB，下载受资源策略及 64 MiB 上限约束。音视频导入依赖 ffprobe。较长短剧整集、多产物或大体积深度视频需要先评估拆分、压缩或宿主扩展，不能仅凭“API 可调用”承诺整集处理。

上游必须明确幂等支持、保存窗口、任务查询、结果有效期、取消语义和费用。提交不明时暂停核实；导入失败只恢复导入，不能重新生成。平台服务费与供应商使用用户 Key 的费用分别说明。OAuth、Webhook、任意 multipart、流式 API、多媒体产物批量导入均未开放；必要时由独立服务适配成当前合同。

一键出海、深度估计、人物动作/运镜还原的真实模型效果仍需单独接入验收，Hypit 不属于当前依赖。

## 7. 持久流程、用户输入和批次

从 `pipeline-helper-p05`、`batch-helper-p06` 和 `canvas-helper-p07` 示例逐步增加贡献：

- Pipeline 持久执行本包操作，支持 wait_input；草稿保存与正式提交是不同动作，提交带 revision 和幂等键。
- P06 支持受控 DAG、when/foreach、稳定 itemKey、有序结果、批次精确批准、并发与预算限制。
- 派生会创建新 Run，显式复用有效的成功结果；用户更改输入不能重写历史运行。
- P07 把待输入和结果展示到画布；删除节点不等于取消后台 Run，操作仍须经过运行权限校验。
- 持久执行不代表无限重试；提交未知、权限变化、连接异常和预算不足必须保留真实状态。

每个流程最多 64 步、最多 256 项、并发上限 8；实际可用并发还受用户、连接与平台限制。工作台与配方是独立贡献，不是 Pipeline 字段。P09 开放 workbenches / recipes；objectTypes 尚未开放。

## 8. 当前 HTTP 接入面

以下路径带 `/api` 前缀，使用启幕登录身份及对象归属校验；它们是当前产品 API，不是已开放的第三方机器身份 API。成功响应使用 `{code,data,msg}`，失败还可能包含 reason；HTTP 200 也必须检查 code。

| 方法与路径 | 用途 |
| --- | --- |
| POST /api/plugins | 管理员上传二进制包或 multipart 的 file 字段 |
| GET /api/plugins/applications | 用户可见应用、发布与当前授权状态 |
| PUT /api/plugins/applications/:id/activation | releaseId、enabled、grantedPermissions、revision |
| PUT /api/admin/plugins/applications/:id | 管理员 availability / uninstall / revoke，带最新 revision |
| GET /api/plugin-operations | q、cursor、hostSurface、canvasId、projectId；返回 operations/nextCursor |
| GET /api/plugin-operations/:pluginId/:operationId | releaseId 和上下文，查询实际合同与可用性 |
| POST /api/plugin-invocations | operation、releaseId、input、可选 context；副作用携带 Idempotency-Key |
| GET /api/plugin-runs | 用户运行历史，精确筛选字段见 handler/plugin_pipeline.go |
| GET /api/plugin-runs/:id | 运行、结果、批准、输入；可选 viewId |
| POST /api/plugin-runs/:id/approvals/:approvalId | decision、revision，必须由用户决定 |
| PUT /api/plugin-runs/:id/inputs/:inputId | mode、value、revision，提交携带幂等键 |
| GET /api/plugin-runs/:id/events | SSE，Last-Event-ID 或 after 为递增事件序号 |
| POST /api/plugin-runs/:id/cancel | 停止请求，外部任务取消以供应商确认状态为准 |
| POST /api/plugin-runs/:id/resume | revision、action、按需 providerJobId |
| POST /api/plugin-runs/:id/derive | 创建派生运行，字段见 handler/plugin_pipeline.go |
| GET /api/plugin-runs/:id/batch-quote | 当前批次报价与授权范围 |
| POST /api/plugin-runs/:id/batch-approval | 批准当前报价，不授权未知后续批次 |
| GET /api/plugin-connections | 当前用户连接及状态，不返回明文凭据 |
| PUT /api/plugin-connections | 保存连接与不可变修订；不是 PATCH /:id |
| GET /api/plugin-canvases/:id/snapshot | 投影前取得实际画布快照 |
| GET /api/plugin-batch-diagnostics | 当前用户范围内的批次诊断 |

批准、输入、派生和恢复正文以 `web/src/services/api/plugin-operations.ts` 及后端 handler 为准；若文件调整，直接查找对应路由定义。结果读取使用 Run 接口，不存在设计稿中的 /results/:resultId 分页接口。SSE 断线重连保留事件游标并刷新 Run 快照，不能把 runId 当游标。

管理员价格使用 GET /api/admin/plugin-operation-prices/:releaseId/:operationId 与 PUT /api/admin/plugin-operation-prices。孤立包清理必须先 POST /api/admin/plugins/applications/orphans/preview 再由管理员决定 prune，不能用它替代卸载。

目前没有单独发布 JS SDK 或能力发现服务；已有操作搜索、描述、模板和真实服务合同就是接入入口。对外 API Key、OAuth 应用、Webhook、外部系统主动调度启幕另行设计。

## 9. 升级、停用、恢复和诊断

先在隔离环境验证新包，再让管理员上传，最后选择测试用户切换。平台安装不自动给所有用户授权；当前没有完整的公开市场或用户灰度规则引擎。

升级前记录插件 ID、发布版本、packageDigest、授权和依赖；相同版本内容不同必须重新发版本。问题版本可停止平台可用性或撤回指定发布；uninstall 为逻辑卸载，不删除历史运行/引用资产。撤回不可作为可随意恢复的临时开关。

停用阻止新操作，不等于撤销已经发给供应商的任务。逐个核对在途 Run/Task、批准、平台预留费与供应商账单；保留 Worker、数据库、插件存储与加密密钥以处理原任务。全局 CANVAS_APPLICATION_PLUGINS_ENABLED=false 可阻断安装/启用及新操作准入，但不是“远程任务已取消”的证明。

出现冲突先读取最新 revision/快照，不用更换幂等键盲试。提交未知只使用服务允许的 attach_job、retry_safe、retry_poll 或 retry_import；不支持的恢复动作会被拒绝。供应商幂等窗口过期必须人工核实。

排查时收集 pluginId、releaseId、包摘要、runId、taskId、当前状态、reason、revision 和脱敏日志，结合运行卡、任务中心和批次诊断。不要导出凭据、签名 URL、原始请求正文；当前未提供插件专属“一键诊断 ZIP”。

生产迁移与二进制回退按[收尾验收与运维清单](../../../plans/qimu-plugin-closeout-acceptance.md)演练。保留数据库、资源、插件归档和 .settings-key 的一致备份；旧 Worker 不认识新任务时不能直接混用或降级。

## 10. 接入完成标准

开发者应独立完成：新 ID 建包 → 离线校验 → 管理员安装 → 用户启用授权 → Skill 可见 → Operation 描述/调用 → 需要时批准 → 真实结果读取 → 按需画布投影 → 升级不篡改历史 → 停用后拒绝新调用且历史可读。

本次提供自动化真实宿主验证；“新作者未参与平台实现、只读文档即可接入”的人工体验仍需用户或另一位开发者验收。公开社区和真实视频供应商质量不包含在该结论中。

## 11. 工作台与配方

P09 在 `codex-workstudio` 中增加 hostApi 3.1 的 Workbench/Recipe 贡献，复用原安装、授权、执行和画布机制。具体字段、Schema、冲突规则、API 与版本快照见[工作台接入文档](workbench-v3-integration.md)，三个示例与验收步骤见 [P09 验收清单](../../../plans/qimu-workstudio-p09-acceptance.md)。
