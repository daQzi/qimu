---
title: 应用插件 v3 开发与接入指南（设计稿）
description: 启幕 Agent 应用插件的包合同、操作、技能、远程 API、流程及画布接入规范。
---

# 应用插件 v3 开发与接入指南

> 状态：P00–P03 已验收；P04 开放受控 HTTP Connector 和 plugin_operation 单任务，通用机制待用户验收。Pipeline、表单与任意代码运行尚未开放，真实供应商尚待选择和联调。
> 本文示例不包含真实服务或密钥。带 `example.invalid` 的地址只说明协议结构；真实模型选择和质量验证属于应用接入工作。

P00 实现离线合同校验，P01 在现有上传入口按版本分流，P02/P03 开放三个可信 Host Adapter，P04 开放 HTTP 单任务。当前 profile 接受技能、操作、基础视图、结果蓝图和受控 connectors，仍不接受 Pipeline 执行或任意代码。旧协议解析器不直接接收 v3，应用包由独立领域服务处理。

P03 开放第三个可信 Host Adapter `canvas.blueprint.instantiate`，见[P03 验收说明](../../../plans/qimu-plugin-p03-acceptance.md)。插件作者通过声明操作、视图、蓝图及技能使用该能力，不需要修改 Agent 主循环。只有新增宿主执行类别时才需要实现并注册新的可信 Adapter。

配套：[需求与架构](../../../design/qimu-plugin-platform-v3-design.md)、[实施设计](../../../plans/qimu-plugin-platform-v3-implementation.md)。现有协议插件用法见 [当前开发指南](../../../../web/src/pages/plugins/plugin-development-guide.md)。

多工作台与技能启动交互见[补充设计](../../../design/qimu-composable-workbenches-design.md)。其中 workbenches/recipes/objectTypes 是后续合同候选，本文首期示例不接受这些尚未实现的贡献；技能仍可仅提供 Markdown 方法说明。

## 1. 选择最小插件形态

| 目标 | 应提供的贡献 |
| --- | --- |
| 提供创作指南/品牌规则 | skills |
| 调用一个业务 API | operations + connectors，可附 skills |
| 新模型请求协议 | 现有 providers；需要 Agent 专门动作时再附 operations |
| 持久化多步骤业务 | operations + pipelines，可附 views |
| Agent 搭建业务画布 | 上述能力 + canvasBlueprints；复用标准节点或声明受支持 canvasNodes |
| 独立结果/参数面板 | views，不要求 canvasBlueprints |

一个包可以混合贡献。既有 `workflows` 表示 Provider 下的模型/工作流入口；v3 `pipelines` 表示业务 DAG，不能覆盖同名字段的旧含义。

混合贡献以宿主明确支持为准：首期 v3 应用不直接携带旧 providers，待协议运行时支持按 releaseId 装载后开放。当前模型协议包继续使用 v1/v2；应用通过宿主模型 Adapter 调用已有 Provider。

## 2. 包合同

### 2.1 公共字段

| 字段 | 合同 |
| --- | --- |
| apiVersion | 本设计为 `yingce.plugin/v3` |
| id | 全局稳定 kebab-case 插件 ID；归属由宿主登记，保留官方 ID 禁止冒用 |
| version | SemVer 发布版本；相同 ID/version 不得包含不同内容 |
| publisher | 作者声明；宿主将其与已登记发布者核对，不信任自报身份 |
| requires.hostApi | SemVer 宿主能力范围；是 SDK 版本，不是前端 package.json 版本 |
| permissions | 所有贡献所需权限的上界；安装后还需实际授权 |
| dependencies | 显式插件版本范围；可选依赖不可支撑必需操作 |
| contributes | 至少一种贡献；首期未知可执行贡献拒绝，不静默忽略 |

贡献内 localId 唯一，操作全名为 `pluginId.localId`。不同版本不通过拼接到操作名表达；运行请求携带 releaseId 或由宿主解析安装版本。跨插件调用必须声明依赖并进入 releaseLock。

允许目录：在当前包容器基础上增加 `skills/operations/pipelines/schemas/views/connectors/blueprints/`。首期应用包只允许已批准的文本/静态资源格式；包中存在 JS、二进制或自定义 entry 不等于宿主会执行。支付包仍按既有专用策略处理。

引用仅限包内相对路径，拒绝目录穿越、重复 ZIP 条目、符号链接、远程 Schema 引用。读取与解压有累计上限，不能只校验压缩包大小。参考资源不能夹带模型凭证。

### 2.2 Schema 子集

首期 JSON Schema 固定采用 Draft 2020-12 的受控子集：`type/properties/required/additionalProperties/items/enum/const/minimum/maximum/minLength/maxLength/minItems/maxItems/description/$defs/$ref`。`$ref` 仅支持包内文件及 JSON Pointer；递归引用、远程引用及未支持验证关键字在安装时报错。

表单只消费兼容字段。复杂字段可以通过宿主 `resource-picker`、`mapping-editor` 等命名组件表示，但组件不改变服务端 Schema。`default` 若支持，只作为展示建议，不能替用户提供必填授权或凭证。

## 3. 完整最小示例：视频资源检查助手

以下七个文件构成最小教学样例。P02 已注册 `resource.inspect` Host Adapter，它只返回已有资源元数据；需要下载探测的媒体处理另建异步操作，避免把高成本工作藏在 inline 读取中。该 Adapter 通过统一调用 API 使用，不是独立 HTTP 路径。

```text
resource-helper/
├── manifest.json
├── skills/check-source/SKILL.md
├── operations/inspect-video.json
├── schemas/inspect-input.json
├── schemas/inspect-output.json
├── views/inspect-result.json
└── blueprints/inspect-board.json
```

### 3.1 manifest.json

```json
{
  "apiVersion": "yingce.plugin/v3",
  "id": "resource-helper",
  "name": "视频资源检查助手",
  "version": "1.0.0",
  "description": "读取已上传视频的资源信息，可展示在 Agent 或画布中。",
  "publisher": { "id": "example-publisher", "displayName": "示例作者" },
  "requires": { "hostApi": "^3.0.0" },
  "permissions": ["media.read", "canvas.read", "canvas.write"],
  "dependencies": [],
  "contributes": {
    "skills": [{
      "id": "check-source",
      "name": "check-source",
      "description": "检查用户选中的已上传视频；不执行生成或转码。",
      "entry": "skills/check-source/SKILL.md",
      "activation": "auto",
      "operations": ["resource-helper.inspect-video"]
    }],
    "operations": [{ "id": "inspect-video", "ref": "operations/inspect-video.json" }],
    "views": [{ "id": "inspect-result", "ref": "views/inspect-result.json" }],
    "canvasBlueprints": [{ "id": "inspect-board", "ref": "blueprints/inspect-board.json" }]
  }
}
```

canvas 权限仅用于可选的画布展示，读取操作自身不要求 canvasId。授权可只授予 media.read，此时 Agent 仍可读取，画布展示动作不可用。

### 3.2 skills/check-source/SKILL.md

```markdown
---
name: check-source
description: 检查已上传视频的资源信息，不生成、替换或转码视频。
---

1. 从用户当前选区或显式引用中取得真实 resourceId，不猜测 ID。
2. 查阅 resource-helper.inspect-video 的合同和可用状态。
3. 无有效视频输入时提示选择或上传视频，不擅自扩大到全部画布资源。
4. 调用操作，说明返回的资源名称和 MIME 类型。
5. 未返回时长或尺寸时明确说明元数据缺失，不推测数值。
6. 用户需要保留在画布时，调用宿主画布模板实例化动作，绑定该结果。
```

技能不直接访问网络或保存状态。修改工作方法可派生独立用户 SkillVersion；包内原始技能保持不可变。

### 3.3 operations/inspect-video.json

```json
{
  "id": "inspect-video",
  "description": "读取当前用户视频资源的已知元数据。",
  "inputSchemaRef": "schemas/inspect-input.json",
  "outputSchemaRef": "schemas/inspect-output.json",
  "requiredPermissions": ["media.read"],
  "effects": ["read"],
  "context": { "requiresCanvas": false, "requiresProject": false },
  "execution": { "kind": "host", "adapter": "resource.inspect", "mode": "inline" },
  "resultView": "inspect-result"
}
```

`mode` 为 inline/task，由宿主 Adapter 校验，插件不能把远程收费操作声明为 inline 读取。所有 Operation 默认输出名称为 `result`，其内容受 outputSchemaRef 约束；Pipeline 可提供命名输出。

### 3.4 schemas/inspect-input.json

```json
{
  "type": "object",
  "properties": {
    "resourceId": { "type": "string", "minLength": 1, "maxLength": 80 }
  },
  "required": ["resourceId"],
  "additionalProperties": false
}
```

### 3.5 schemas/inspect-output.json

```json
{
  "type": "object",
  "properties": {
    "resourceId": { "type": "string", "minLength": 1 },
    "kind": { "const": "video" },
    "name": { "type": "string" },
    "mimeType": { "type": "string" },
    "durationMs": { "type": "number", "minimum": 0 },
    "width": { "type": "integer", "minimum": 1 },
    "height": { "type": "integer", "minimum": 1 }
  },
  "required": ["resourceId", "kind", "name", "mimeType"],
  "additionalProperties": false
}
```

资源 kind 与归属由宿主实查；JSON Schema 本身不能证明资源属于用户。缺失的可选元数据省略，不以 0 填充未知时长。

### 3.6 views/inspect-result.json

```json
{
  "id": "inspect-result",
  "component": "key-value/v1",
  "schemaRef": "schemas/inspect-output.json",
  "fields": [
    { "path": "/name", "label": "名称" },
    { "path": "/mimeType", "label": "格式" },
    { "path": "/durationMs", "label": "时长（毫秒）" }
  ]
}
```

### 3.7 blueprints/inspect-board.json

```json
{
  "id": "inspect-board",
  "nodes": [{
    "key": "source-info",
    "nodeType": "plugin-result",
    "title": "视频资源信息",
    "position": { "x": 0, "y": 0 },
    "binding": "result",
    "view": "inspect-result"
  }],
  "connections": []
}
```

`plugin-result` 已在 P03 注册为标准节点。`key` 是模板内标识，宿主将其解析为真实 nodeId。实例化动作需要当前画布 snapshotHash、实际授权和唯一调用键；读取 inspect-video 成功本身不等于自动获得 canvas.write。节点不接受任意结果 JSON 或媒体地址，只保存成功结果引用。

本例 inline 结果没有 runId。`resource.snapshot` 在用户批准后以单步 PluginRun 固化结果并生成 ResultRef；P03 的 1.3.0 样例要求把 inline 返回的 digest 作为 expectedDigest 回传。画布操作引用已成功的 runId 与 ResultRef.digest，不使用虚构 runId，也不重新执行收费服务。

### 3.5 P03 当前可执行的画布保存合同

操作声明 `execution={kind:"host", adapter:"canvas.blueprint.instantiate", mode:"inline"}`、`effects=["draft_write"]`、`requiredPermissions=["canvas.read","canvas.write"]`，且 `context.requiresCanvas=true`。调用示例：

```json
{
  "operation": "resource-helper.place-result",
  "releaseId": "从 describe 获取的真实发布 ID",
  "context": { "hostSurface": "canvas", "canvasId": "当前用户的真实画布 ID" },
  "input": {
    "runId": "已成功快照的运行 ID",
    "resultDigest": "ResultRef 返回的 64 位摘要",
    "blueprintId": "inspect-board",
    "snapshotHash": "canvas_get_state 返回的最新 64 位摘要",
    "instanceKey": "default",
    "x": 80,
    "y": 80
  }
}
```

HTTP 使用 `POST /api/plugin-invocations` 并带幂等键；按钮可以从 `GET /api/plugin-canvases/:id/snapshot` 获取同一画布摘要。结果只允许来自相同用户和同一发布，跨发布/跨插件结果绑定尚未开放。所有蓝图节点必须为 plugin-result，connections 必须为空；每个节点的视图 Schema 必须接受该成功结果。

批准后返回 `canvasId/sourceRunId/blueprintId/projectionId/bindings`。同一 instanceKey 重试返回同一绑定；目标标题、位置、尺寸或结果绑定已变化、节点被删除时返回 projection_conflict。先取消过期审批，再读取新 snapshotHash 重试；只有用户明确要另建时才使用新 instanceKey。冲突不改写成功业务结果，不重新检查或生成媒体。

画布 metadata.pluginResult 仅含 runId、digest、releaseId、viewId、projectionId、bindingKey。显示组件按用户权限重新读取 Run 并校验 digest，包内字段标签和数据只作为文本渲染。当前 progress 展示来自真实运行状态；`progress/v1` 表单/流程组件仍是后续目标。

## 4. 接入一个真实远程 API

P04 当前可执行合同以 `backend/internal/plugins/contracts/testdata/remote-helper-p04/` 为准，安装包与完整页面验收见 [P04 验收说明](../../../plans/qimu-plugin-p04-acceptance.md)。Manifest 新增 `contributes.connectors`，operation 使用 `execution.kind=http/mode=task`、connector、action。必须声明 connection.use 与 external_write/generation；资源输入另需 media.read，媒体输出另需 resource.create。

当前映射使用 `input.<字段>` 与 `resource.<字段>`，后者在 `resources` 中声明 image/video/audio，宿主验证归属后准备临时地址。结果 `outputs` 把目标字段映射到响应 JSON Pointer；`artifact={urlPath,field,kind}` 声明一份需导入的媒体，宿主用资源对象替换临时地址。原始 URL 和密钥不能出现在公开结果。

Idempotency 的 header 模式必须提供 header 与 retentionSeconds（60–86400），恢复保留同一键和同一正文；lookup 为 GET `/requests/{submissionKey}` 之类的受控查询。没有这两种能力时提交不明进入 paused。Cancellation 的 request 模式声明受控请求，仍需通过 statusPath/statusMap 读取取消确认；收到 HTTP 成功回执不代表已取消。

用户连接经后端加密存储并固定修订，插件包不含真实 Key。管理员服务费以每次成功结果固定微积分配置，批准时预留；用户 Key 的供应商费用单独展示，不由该账务估算或退款。结果导入失败只恢复原任务查询/导入；普通 Task 重试不能重新发起生成。

### 4.1 接入前记录服务合同

作者必须确认同步/异步协议、认证、输入上传方式、请求大小、状态值、幂等/查询语义、取消、结果有效期、计价和错误类型。支持视频生成不等于支持深度、姿态或遮罩输入。

复用已有生成 Provider 时优先绑定逻辑模型及能力限制，不另存一份模型 Key。非生成服务使用 Connector。用户配置的连接是独立实例，包中只包含声明。

### 4.2 异步 HTTP 合同片段

下面是设计语法片段，不是现成深度模型供应商配置。真实插件需要在 manifest 登记 connector/operation，并提供对应输入输出 Schema。

```json
{
  "id": "depth-api",
  "transport": "http",
  "baseUrl": "https://depth.example.invalid",
  "auth": { "type": "bearer" },
  "actions": {
    "estimate": {
      "submit": {
        "method": "POST",
        "path": "/jobs",
        "body": {
          "video_url": { "from": "resource.sourceVideo" }
        }
      },
      "jobIdPath": "/id",
      "poll": { "method": "GET", "path": "/jobs/{jobId}" },
      "statusPath": "/status",
      "statusMap": {
        "queued": "pending",
        "running": "pending",
        "completed": "succeeded",
        "failed": "failed"
      },
      "resources": { "sourceVideo": "video" },
      "outputs": { "summary": "/output/summary" },
      "artifact": { "urlPath": "/output/depth_url", "field": "depthVideo", "kind": "video" },
      "idempotency": { "mode": "unsupported" },
      "cancellation": { "mode": "unsupported" }
    }
  }
}
```

此例明确不支持幂等和取消，提交网络超时进入 unknown 待核实；不能拿它测试“自动安全重试”。实际服务支持幂等时才声明具体 Header/字段及查询端点，并做联调证明。

`from` 只引用 input 中的字段或 resources 声明的用户资源；路径参数按 URL segment 编码；禁止任意模板表达式或 JS。响应路径使用 JSON Pointer，状态值未知时暂停核实。HTTP transport 使用已有出站防护，鉴权 API 不跟随重定向，结果下载每次重定向重新检查。媒体资源以宿主签发的短期地址发送，不把鉴权 URL 放入模型上下文。

Operation 执行绑定示例：

```json
{
  "kind": "http",
  "connector": "depth-api",
  "action": "estimate",
  "mode": "task"
}
```

不同作者共享的 HTTP transport 负责提交、轮询、超时、导入等机制；特殊协议可以由远程包装服务归一化。需要新宿主 Adapter 时提交 SDK 扩展，不在 Agent 主循环加入供应商名称分支。

### 4.3 资源与凭证

- 参数传 resourceId；宿主验证资源就绪、归属及允许发送范围。
- 用户连接记录引用 credentialRef；密钥不进入 manifest、Skill、URL、事件或结果。
- 上游临时结果先导入启幕，再向 Agent 发布 Resource 引用；导入失败保留 upstream_completed 状态，重试导入而非重做模型。
- 使用服务端已实现的资源配额与引用保护；资源下载有大小、MIME 和目标地址检查。
- 需要私网服务时通过现有精确允许主机配置接入，不接受插件请求全私网放行。

## 5. 流程开发合同

### 5.1 步骤语法

Pipeline 文件声明 `id/inputSchemaRef/outputSchemaRef/steps/outputs`。首期步骤类型为 `operation`、`wait_input`；并发、条件与批量是步骤属性。生成、翻译、合成都是 Operation，不另建各自执行引擎。

| 属性 | 语义 |
| --- | --- |
| key | 流程内稳定步骤名 |
| dependsOn | 显式依赖；安装时检查 DAG，无环 |
| operation | 操作全名，来自自身包或显式依赖 |
| inputs | 字段映射，每项为 `{literal: ...}` 或 `{from: ...}` |
| when | 受控 `exists/equals` 判断，不执行脚本 |
| foreach | 上游数组引用、稳定 itemKey 字段、并发上限 |
| formSchemaRef/view | wait_input 的表单合同及可选视图 |

`from` 格式为 `input#/field`、`steps/<key>#/result/field` 或批量步骤内 `item#/field`。不支持任意对象遍历、代码执行、隐式字符串插值。必须显式声明使用的上游依赖；不存在字段视为错误，不静默传空值。

### 5.2 流程片段

```json
{
  "steps": [
    {
      "key": "analysis",
      "type": "operation",
      "operation": "video-localization.analyze",
      "dependsOn": [],
      "inputs": { "sourceResourceId": { "from": "input#/sourceResourceId" } }
    },
    {
      "key": "replacements",
      "type": "wait_input",
      "dependsOn": ["analysis"],
      "formSchemaRef": "schemas/replacements.json",
      "view": "replacement-editor"
    },
    {
      "key": "generation",
      "type": "operation",
      "operation": "video-localization.generate-shot",
      "dependsOn": ["analysis", "replacements"],
      "foreach": {
        "from": "steps/analysis#/result/shots",
        "itemKey": "/shotId",
        "maxConcurrency": 2
      },
      "inputs": {
        "shot": { "from": "item#" },
        "replacements": { "from": "steps/replacements#/result" }
      }
    }
  ]
}
```

此片段需要真实 analyze/generate-shot 操作与表单 Schema，不能单独作为可安装包。并发值 2 只是例子，最终取插件请求、用户配额、模型和宿主限制的最小值。

批量项必须有稳定 itemKey；重复 key 拒绝。后续聚合结果按输入顺序发布，不按任务完成顺序重排镜头。首期条件跳过产生 skipped，依赖跳过值必须有明确缺省映射，否则阻塞并返回数据依赖错误。

### 5.3 用户输入与派生运行

用户操作节点时传 runId、inputRequestId、runRevision、inputRevision 和 values。后端验证 Schema、资源归属和当前等待步骤，原子保存并推进。网络重复提交返回首次结果；同键不同内容冲突。

修改已提交方案使用 derive：生成新 runId，保存 parentRunId，只复用经摘要验证未受影响的输出。不会在已成功的旧运行中覆盖历史数据。重生成收费镜头仍需符合现有授权范围。

## 6. 画布与界面接入

### 6.1 读取与修改

先读取真实画布和选区、获取 snapshotHash，再通过宿主命令操作。首期沿用后端已开放的 add/update/connect 和结构化分镜/批量表动作，不开放任意 metadata、删除或媒体 URL。

插件 View 按钮绑定操作全名或宿主输入动作。输出应用到已有节点需要校验版本；发生冲突可新建结果节点。布局、分组、视口聚焦等能力以 SDK catalog 为准，不能仅凭前端类型判断后端可执行。

### 6.2 标准组件

拟提供 `form/v1`、`table/v1`、`entity-cards/v1`、`mapping-editor/v1`、`media-compare/v1`、`progress/v1`、`key-value/v1`。仅支持宿主已登记的组件及数据 Schema。

没有画布时结果可在 Agent 卡片或应用面板展示；纯技能和素材查询不得要求创建临时画布。用户关页面后后台流程继续；纯浏览器编辑功能必须标记 executionEnvironment=browser，并禁止被离线 Pipeline 当作已完成。

## 7. 内部接口与实施状态

以下均为登录态内部 API，不是对外开放平台。P02 实现操作检索/描述/调用、Run 查询、批准与取消；P04 实现连接管理及受控 resume。输入、SSE、派生和分页大结果仍是后续目标。不覆盖现有 `/api/plugins`、`/api/agent` 或 `/api/creation-runs`。

P03 新增 `/api/plugin-canvases/:id/snapshot` 和 Run 的 `viewId` 查询参数；投影继续复用已有调用与审批接口，不新增一套执行器。下面表格同时包含未来接口，只有上述已实现路径可实际调用。

| 方法与路径 | 用途 |
| --- | --- |
| GET `/api/plugin-operations?q=&cursor=` | 检索当前用户可发现的操作摘要及可用性 |
| GET `/api/plugin-operations/:pluginId/:operationId?releaseId=` | 读取完整合同 |
| POST `/api/plugin-invocations` | 校验并调用；必要时返回等待审批的运行 |
| GET `/api/plugin-runs/:id` | 获取状态、步骤摘要、待输入/审批、结果及投影状态 |
| POST `/api/plugin-runs/:id/inputs/:inputRequestId` | 提交结构化输入 |
| POST `/api/plugin-runs/:id/approvals/:approvalId` | 用户批准或拒绝；Agent 不可自行批准 |
| POST `/api/plugin-runs/:id/resume` | 继续暂停运行；不能绕过缺输入或待审批 |
| POST `/api/plugin-runs/:id/cancel` | 持久化取消意图并取消子任务 |
| POST `/api/plugin-runs/:id/derive` | 修改输入后派生运行 |
| GET `/api/plugin-runs/:id/events?after=` | SSE 重放 |
| GET `/api/plugin-runs/:id/results/:resultId?cursor=` | 分页读取结果 |
| GET/PUT `/api/plugin-connections` | 当前用户连接列表/保存新修订（P04 已实现） |
| PATCH `/api/plugin-connections/:id` | 更新连接配置；服务端控制 secret 写入及脱敏返回 |

创建调用体示例：

```json
{
  "operation": "resource-helper.inspect-video",
  "releaseId": "<从目录取得的真实发布ID>",
  "input": { "resourceId": "<真实资源ID>" },
  "context": { "canvasId": "<可选真实画布ID>" }
}
```

示例尖括号是说明占位符，实际调用必须替换。HTTP 写请求使用 `Idempotency-Key`；Agent 侧由宿主按 agentRunId/toolCallId 生成稳定键，不要求模型生成。读取操作返回 `kind=inline/result`；持久调用返回 `kind=run/runId/status`。异步接收可用 HTTP 202 + `{code:0,data,msg}`，业务失败按现有 AppError 返回对应 HTTP 状态及 `reason`。

同一个调用键只接受同一规范输入、操作版本和作用域。身份从登录上下文读取；context 只是目标声明，必须查实际归属。客户端不得提交授权状态、价格、credentialRef 或有效权限列表。

SSE 使用单 run 单调 sequence 作为事件 ID；支持 Last-Event-ID/after。连接断开不取消运行。事件过期返回需刷新快照的明确原因，不能假装从头完整重放。事件类型至少包括 run.created、step.started、step.completed、input.requested、approval.requested、result.ready、projection.conflict、run.completed、run.failed、run.cancelled。

### 7.1 可诊断错误

| reason | 处理 |
| --- | --- |
| plugin_disabled / plugin_revoked | 查看启用或管理员策略，不自动切换插件 |
| plugin_version_conflict | 获取安装版本；运行中不能静默升级 |
| plugin_dependency_missing | 说明缺失依赖，不自动安装 |
| operation_input_invalid | 修正 Schema 指出的字段 |
| operation_unavailable | 展示具体能力限制 |
| connection_unconfigured | 提示配置指定连接，不询问模型上下文中的密钥 |
| scope_forbidden | 检查资源归属与范围，不能换默认用户重试 |
| approval_required / quote_changed | 打开对应授权卡片 |
| upstream_submission_unknown | 查询上游或人工核实，禁止直接重发 |
| upstream_output_invalid | 保存诊断，不能发布成功结果 |
| projection_conflict | 保留成功结果，重新选择目标 |
| run_revision_conflict | 刷新当前输入与进度，保留用户草稿 |

## 8. 深度视频插件的输出合同

深度预览和控制数据分别输出，建议 Schema 身份 `media.video-depth/v1`。字段至少包含：

| 字段 | 含义 |
| --- | --- |
| sourceResourceId/sourceDigest | 对应原始视频和内容摘要 |
| previewResourceId | 可选灰度/伪彩预览，仅供观看 |
| controlResourceId/format | 真正供模型使用的数据及格式 |
| width/height/frameCount | 空间与帧数量 |
| timeBase/frameMapping | 对齐原视频的时间基准；变帧率时逐帧时间戳，不只给 FPS |
| depthType/units | relative 或 metric；相对深度不能假称米 |
| nearConvention/normalization/invalidValue | 近远方向、归一化方法、无效值编码 |
| model/provider/version | 来源与可复现范围 |

预览视频压缩、颜色映射或 8 bit 量化可能损失控制信息，不能将其默认等同于原始深度。下游必须声明接受的控制 Schema 和格式，必要转换是单独有损/无损操作。姿态、遮罩、光流、相机轨迹各有独立 Schema，不伪装成 depth。

## 9. 作者开发与发布流程

1. 查宿主操作及视图目录，选择已支持能力与最低 hostApi 范围。
2. 编写 Manifest、输入输出 Schema、操作与技能；需要时加入流程和视图。
3. 当前可在 backend 运行 `go run ./cmd/plugin-contract -dir <已展开插件目录>` 校验；加 `-out <新文件.yingce-plugin>` 打包，加 `-version <正式版本>` 可仅覆盖产物版本。生成器拒绝覆盖已有文件。离线工具不安装、不执行、不校验真实账号所有权；服务端安装执行额外的归属/依赖/技能存储校验。
4. 管理员在开发实例导入包；配置用户连接与权限。使用 Mock 验证协议机制，再用真实服务完成验收。
5. 通过 Agent 和界面两种入口执行同一操作，检查状态、结果与权限一致。
6. 验证重启、重复提交、取消、版本升级、用户切换和缺资源。
7. 生成 `.yingce-plugin` ZIP，manifest.json 位于根目录；版本与内容摘要固定。平台 SDK 实施后提供可重复打包和校验入口。
8. 在受控目录发布；更新增加版本，禁止同版本替换内容。发布者认证与公开社区分发另行实施。

诊断包记录 plugin/release/run/step/task/trace ID、合同摘要、阶段和机器可读原因；默认不含凭证、签名 URL 或原始敏感业务正文。

## 10. 接入验收清单

- 包内所有路径、ID、依赖及 Schema 引用有效；未经支持的运行时明确拒绝。
- 插件停用、权限不足和错误归属都无法执行；Skill 指令不能改变这些结果。
- 工具 Schema 对 Agent、表单和后端一致；未知参数拒绝。
- 模型选择与报价由宿主实查，用户未授权时没有收费副作用。
- HTTP 超时、未知状态、临时结果地址过期分别处理；不会把上游完成等同于资源已保存。
- 无画布工具可独立运行；需要画布的操作明确检查 scope。
- 刷新和重启可恢复；已知作业重试不重复提交，提交不明时按上游能力核实或暂停；取消状态如实反映上游能力。
- 成功结果可追踪输入/版本；局部修改产生派生结果；升级不改变在途流程。
- 示例插件不引用 qimu 私有 Store/service/数据库；新增供应商不修改 Agent 主循环。
- Mock 测试通过只代表协议机制通过，真实媒体质量和服务费用另有验收记录。
