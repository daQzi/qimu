# P07：声明式交互与画布模板

状态：P00–P06 已由用户验收。本阶段在包含全部已验收代码的独立 worktree 开发，完成后只合入 `codex/codex-plugin`，等待用户验收，不进入 P08。未接入 Hypit、外部 Codex 或真实收费供应商。

## 1. 本阶段范围

- 新增 `plugin-input` 节点。节点只保存 Run/inputRequestId/固定发布/视图/投影来源，表单数据仍在既有 PluginInputRequest；节点和运行卡片共享草稿、Schema 校验、revision CAS 与幂等提交。
- 开放 `table/v1`、`entity-cards/v1`、`mapping-editor/v1`、`media-compare/v1`，保留 `key-value/v1`。组件不执行脚本，不加载插件给定的 HTML、模块或任意媒体 URL。
- 蓝图支持相对位置、同一来源的多个节点、模板内流程连线、输入/成功结果绑定、命名动作和来源追溯。节点/连线合计最多 20 项，超限拒绝；单次投影与绑定记录在一个数据库事务中提交。
- Agent 读取 Run 后可用 `canvasActions` 请求输入投影。用户点击“定位下一个待填写节点”；结果节点可“选中结果，继续与 Agent 对话”，只填入对话草稿，不自动发送。
- 发生画布快照、节点或连线冲突时保留后台状态，允许重新读取或另建节点；不触发模型重跑。删除节点不取消 Run。已有画布历史/撤销只影响展示，不能撤销后台输入、任务或费用。

本阶段没有新数据库表或迁移，继续使用 schema 35 的运行、输入请求、投影记录与原画布版本历史。没有新增模型进程或队列；新增开销主要是表单/结果渲染和既有 Run 状态读取。

## 2. 开发合同

### 2.1 视图目录

| component | 配置与用途 | 写入语义 |
| --- | --- | --- |
| `key-value/v1` | fields 的 path 为结果根对象 JSON Pointer，label 为展示名；作为输入视图时按原 Schema 渲染简单字段 | 仅在待输入表单显式提交 |
| `table/v1` | 数据根为数组，或用 collectionPath 指向数组；fields 相对每行；每页 20 项 | 只读 |
| `entity-cards/v1` | 同 table 的数据寻址，按实体卡片展示，每页 20 项 | 只读 |
| `mapping-editor/v1` | 输入对象的单层 collectionPath 指向对象数组；fields 指向每行的单层 string/number/integer/boolean 字段，支持 enum、添加/删除行和每页 10 行 | 保存草稿不推进，提交才使用原输入 API |
| `media-compare/v1` | fields 指向资源 ID 字符串或 `{resourceId}`；使用宿主资源鉴权 API 加载图片/视频/音频 | 只读，不生成媒体 |

mapping-editor 最多编辑 256 行，若 Schema maxItems 更低则遵守更低限制。复杂 `$ref`、嵌套字段或不支持的数据形状可切换 JSON 编辑，最终仍由后端 Schema 验证。非法 JSON 不会被当成空对象覆盖。媒体资源不是就绪状态、无权访问或加载失败时明确显示错误；不使用插件提供的 URL 兜底。媒体对比是并列播放/查看，本阶段不提供视频帧级同步、深度提取或自动替换。

输入步骤可声明 `view`，必须引用包内视图，且 schemaRef 与 formSchemaRef 一致。仅 key-value/mapping-editor 可作为输入视图；table/cards/compare 不可伪装成输入组件。

### 2.2 蓝图与执行动作

```json
{
  "id": "mapping-input",
  "nodes": [{
    "key": "mapping", "nodeType": "plugin-input", "title": "填写替换映射",
    "position": {"x": 0, "y": 0}, "binding": "input", "view": "mapping",
    "actions": ["editor.focus", "input.submit"]
  }],
  "connections": []
}
```

一个模板绑定一个待输入请求，或一个成功结果；不混用多个 Run、多个输入请求、未来结果占位或跨发布结果。完整流程通过多次小模板投影组成，所有业务运行仍使用同一个父 Run。结果模板可提供多个视图节点，并用 `{from: "模板key", to: "模板key"}` 声明关系。

`connections` 只连接本模板节点，禁止缺失端点、自连和重复边；合计预算包括连线。宿主写入 relation=`plugin-flow`，表示展示关系，不提供生成输入能力。插件输入/结果节点不能从普通新增菜单或 Agent `canvas_apply_ops` 直接伪造绑定，也不允许直接修改绑定元数据。

命名动作分为：

- `editor.focus`：在线画布定位/选择，后台 Pipeline 不得调用为 Host Adapter。
- `result.continue`：选中结果并打开当前画布 Agent，填入基于 Run 的对话草稿；用户自行发送。未开启画布、节点已删除或尚未同步时显示提示。
- `input.submit`：复用 `PUT /api/plugin-runs/:runId/inputs/:inputRequestId`，不代表任何模型或费用批准。

动作声明描述组件能力，不授予权限。真正后台写入依旧通过 operation 的 requiredPermissions/effects 和既有审批。现有 `canvas.blueprint.instantiate` 是后台可执行 Adapter；纯浏览器动作不能填入它的 execution.adapter。

### 2.3 投影请求与恢复

`GET /api/plugin-runs/:id` 返回 `pipeline.inputs[].view` 和匹配当前输入/结果的 `canvasActions`。每项包含 operation、releaseId、blueprintId，输入项额外含 inputRequestId。

输入投影调用上述 operation，传入 runId、inputRequestId、blueprintId、真实 snapshotHash、instanceKey；不要传 resultDigest。成功结果投影使用 resultDigest，不传 inputRequestId。两种来源互斥，后端同时验证当前用户、固定发布、请求归属、视图 Schema 与待输入状态。审批时重新读取，输入已提交或运行停止则拒绝迟到投影。

实例身份包含用户、画布、来源 Run、发布、模板、instanceKey；输入模板额外包含 inputRequestId。同实例可幂等重放；用户移动/改名/删除节点或删除流程连线后，旧实例不会覆盖修改。需要另建时使用新 instanceKey。确认原投影仍待审批时应先取消旧审批，再用当前快照重试。此操作只保存展示，不应重新调用 process 或远程模型。

## 3. 可安装样例

目录：`backend/internal/plugins/contracts/testdata/canvas-helper-p07`。从本阶段代码的 backend 目录运行：

```sh
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/canvas-helper-p07 -out /tmp/qimu-canvas-helper-p07.yingce-plugin
```

输出已存在时命令会拒绝覆盖，请换一个新的输出文件名。用管理员插件管理页上传，用户启用 `canvas-helper` 1.0.0，授予画布读写权限，将其技能加入“我的技能”。沿用项目配置的 Go/Bun 工具链，不需要安装远程 API 连接。

样例仅让用户填写映射、选择已有媒体，并展示它们；不声称已完成拉片、角色提取或视频生成。已有媒体的 resourceId 可由 Agent 读取所选画布媒体获得。填写资源 ID 只用于展示，不会复制媒体文件或建立新的模型任务；仍遵守原资源可用性与删除规则。

## 4. 用户验收步骤

1. **安装与原有功能**：确认插件上传/启用/技能可见，P03 原结果保存、P05 表单、P06 批次运行仍可使用。普通文本、图片、视频、分镜节点的创建、改名、拖动、连线不变。
2. **Agent 引导输入**：在已有画布上传两份媒体，引用 canvas-helper 的技能，请求“演示画布交互，用输入节点填写映射，再展示已有媒体对比，不调用生成模型”。Agent 应只启动一次 process，再根据 canvasActions 请求 mapping-input 投影。审批前没有节点；审批后同步出现输入节点。
3. **填写与焦点**：在节点添加 source/target 映射，保存草稿后刷新，再提交。测试中文输入、Tab、Enter、滚轮、选中文本和删除行；编辑控件时不应拖动画布、触发删除节点或缩放。检查大表分页、JSON 切换、明暗主题和窄屏。
4. **下一步与结果**：同一 Run 进入 media 输入；请 Agent 创建对应输入节点，或在带 canvasId 的运行卡片点击创建。填写两份已有媒体的资源 ID。完成后保存 results 模板，应生成表格、实体卡片、媒体对比三个节点和两条关系线。定位输入与结果继续对话应选中正确节点，不自动发送消息或启动模型。
5. **冲突、删除、刷新**：保存投影前移动一个节点，旧审批应报告冲突；取消旧审批后重新读取画布，或另建实例。删除输入节点，后台输入仍在运行卡片中可填写；删除结果节点，不改变结果 Run/摘要；删除模板连线后，同实例重放不能无提示修复连线。跨标签页重复提交旧 revision 应报告冲突，不重复推进；同幂等键重试同一次提交成功。
6. **无画布与权限**：插件中心展开“调试持久操作：process”，以 `{}` 启动，直接在运行卡片填写两步，仍可完成。切换账号不能看到原账号 Run/草稿；停用插件阻止新的写入但保留历史读取。原流程若有收费步骤，其审批不会被投影或表单提交代替。

可分开验收：先走插件中心流程验证输入和结果，再验证 Agent 编排与画布操作。若 Agent 没自动选对操作，保留工具调用记录和 Run ID，不能把人工操作成功等同于 Agent 编排成功。

## 5. 验证记录与回退

自动化包含真实应用服务下的 SQLite/PostgreSQL 输入→投影→审批→提交→结果流程、越权/错误绑定/旧输入审批、删除后的恢复、幂等、连线冲突以及投影记录提交故障的事务回滚；Agent 并行待审批输入投影另有验证。前端使用真实组件测试与项目生产构建，不能替代浏览器拖拽、焦点和主题验收。

本次验证记录：插件/画布领域全量测试通过；应用与 HTTP 层插件/流程专项通过；P03/P07 SQLite/PostgreSQL race 专项通过；前端插件及相邻画布回归共 148 项通过；`bun run build` 通过（保留既有大 chunk 提示）；OpenAPI YAML 解析通过。没有启动开发服务器，没有进行真实浏览器视觉或交互验收。

本阶段不自动启动开发服务、部署或迁移业务数据库。暂时禁用新模板投影即可退回运行卡片继续输入，不丢失后台 Run；已有节点元数据保持可追溯。插件包使用旧宿主不支持的组件时会校验失败，需先升级宿主。P07 验收通过后再进入 P08 统一会话。
