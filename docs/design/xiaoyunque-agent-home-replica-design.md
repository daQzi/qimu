# Qimu Agent Home 复刻产品与技术设计

> 状态：提案，尚未实施。
>
> 目标：在 qimu 现有画布 Agent、创作任务、Skills、Plugins、报价与画布持久化能力之上，实现与小云雀首页等价的 Agent 工作模式、底部工具栏和业务流程。本文描述 qimu 自己的产品合同，不复制外部站点的品牌、素材、文案或私有实现。

## 1. 设计结论

小云雀的三个入口不是三个独立聊天页面，而是同一个 Agent 平台的三类业务工作流：

```text
统一 Agent Platform
  ├── Creative Agent     通用创作、直接媒体生成
  ├── Short Drama Agent  剧本、分集、分镜、重制转绘
  ├── Marketing Agent    商品、角色、创意、Hook、风格、营销视频
  └── Canvas Agent       节点、连线、画布媒体和分镜表
```

四类工作流共享 Thread、Run、Skill、Tool、Model、Quote、Approval、Artifact 和 Task，但每个 surface 有不同的输入槽、工具集、参数和输出目标。

qimu 当前是两个运行系统：

```text
首页 CreatePage      -> 直接生成任务 / 前端创作控制器
画布 Cloud Agent     -> 后端 CloudAgentExecution / 工具循环 / SSE
```

本方案要求先统一外层 Agent 合同，再逐步复用两个已有执行内核；第一阶段不强行合并数据库表，以降低迁移和恢复风险。

## 2. 现状基线

### 2.1 已有能力

| 能力 | qimu 当前入口 | 复用策略 |
| --- | --- | --- |
| 首页创作输入 | `web/src/pages/create/index.tsx` | 改造成 Agent Home Shell 的 creative surface |
| 图片、视频、文本生成 | `web/src/services/api/generation-task.ts`、`web/src/lib/canvas/canvas-project-generation.ts` | 保留任务创建、媒体资源化和恢复逻辑 |
| 创作状态、报价、审批 | `backend/internal/app/creation.go`、`model.CreationRun`、`model.CreationSubmission` | 作为通用报价和提交内核 |
| 画布 Agent | `web/src/components/canvas/canvas-cloud-agent-panel.tsx`、`backend/internal/app/cloud_agent_runtime.go` | 作为 canvas surface 执行内核 |
| 画布工具 | `backend/internal/app/cloud_agent_tools.go` | 按 surface 分组和裁剪 |
| Skills | `web/src/services/skill-runtime.ts`、`backend/internal/skills` | 从文本参考扩展为可执行声明 |
| Plugins | `web/src/lib/plugins/plugin-registry.ts`、`web/src/lib/plugins/plugin-types.ts` | 增加后端 Agent Action Bridge |
| 画布输出 | `canvas_apply_ops`、`CreationCanvas` | 作为统一输出目标之一 |

### 2.2 当前缺口

- 首页不能使用与画布相同的持久化 Agent Run。
- `CloudAgentRequest.CanvasID` 当前作为画布前置条件，首页无法成为无画布 Agent。
- 底部 Composer 是固定控件，不是由模式、Skill 和模型能力生成的 schema。
- Skill 目前主要传递 `SKILL.md` 和 reference，不能声明输入槽、默认模型、参数覆盖和计费方式。
- qimu 没有商品实体、营销角色实体、营销模板状态和角色生成流程。
- 短剧剧本、集数、本土化、重制转绘尚未形成统一的首页工作流合同。
- 前端插件的 `agentActions` 尚未接入后端 Cloud Agent 工具循环。

## 3. 产品范围

### 3.1 路由

建议使用 qimu 自己的路由命名，并保留 surface 查询参数用于刷新恢复和深链：

| 路由 | Surface | 说明 |
| --- | --- | --- |
| `/` | `creative` | 默认首页创作 Agent |
| `/create` | `creative` | 兼容现有创作入口 |
| `/short-drama` | `short_drama` | 短剧 Agent |
| `/marketing` | `marketing` | 营销 Agent |
| `/canvas/:id` | `canvas` | 已有画布和画布 Agent |

旧入口 `/create` 继续可用；首页模式切换不能丢失当前 Thread、草稿、上传资源或未完成 Run。

### 3.2 统一页面结构

```text
AgentHomeShell
  ├── WorkspaceNavigation
  ├── AgentModeHeader
  ├── SurfaceContextPanel
  ├── AgentComposer
  │   ├── PromptEditor
  │   ├── ReferenceRail
  │   └── AgentToolbar
  ├── RecommendedSkills
  ├── FeatureHighlights
  ├── InspirationGallery
  └── AgentConversation / ArtifactTimeline
```

页面必须支持三种状态：

1. 空状态：模式入口、快捷案例、亮点功能。
2. 草稿状态：输入、素材、Skill、参数和报价准备。
3. 运行状态：消息、工具进度、审批、Artifact、任务和恢复。

## 4. 统一 Agent 合同

### 4.1 Surface

```ts
type AgentSurface = "creative" | "short_drama" | "marketing" | "canvas";
type AgentOutputTarget = "text" | "media" | "script" | "storyboard" | "artifact" | "canvas";
```

### 4.2 请求

```ts
type AgentHomeRequest = {
    surface: AgentSurface;
    threadId?: string;
    prompt: string;
    canvasId?: string;
    outputTarget: AgentOutputTarget;
    skillBindings?: AgentSkillBinding[];
    inputReferences?: AgentInputReference[];
    modelPolicy?: AgentModelPolicy;
    modeState?: Record<string, unknown>;
    budget?: { maxCredits?: number; maxGenerationTasks?: number; maxVideoSeconds?: number };
    profileRevision?: string;
    idempotencyKey: string;
};
```

约束：

- 首页无画布时 `canvasId` 可为空。
- `canvas` surface 必须有画布。
- 媒体提交必须经过现有报价、审批和预算 admission。
- 请求不能携带浏览器 API Key、上游 URL、客户端伪造价格或权限字段。
- 模式状态和输入引用必须保存快照，运行中不能被后续编辑静默覆盖。

### 4.3 Run / Artifact / Event

统一外层概念，不要求第一阶段立即合并已有表：

```text
AgentThread
  ├── AgentRun (one immutable turn)
  │   ├── AgentMessage
  │   ├── ToolCall / ToolResult
  │   ├── Approval
  │   ├── Quote
  │   ├── Artifact
  │   └── GenerationTask
  └── CanvasBinding (optional)
```

事件类型：

```text
run_queued
assistant_delta
assistant_message
reasoning_message
tool_started
tool_completed
tool_failed
quote_created
approval_requested
approval_decided
artifact_created
canvas_updated
generation_task_created
run_completed
run_failed
run_cancelled
```

运行状态：

```text
idle -> drafting -> validating -> waiting_user_input
     -> quoting -> waiting_approval -> queued -> running
     -> artifact_ready -> canvas_committing -> completed
```

任何状态都必须支持明确的 `failed`、`cancelled` 和可恢复的 `reconnecting` 展示。

## 5. 创作 Agent 需求

### 5.1 Agent 模式

底部工具栏：

```text
添加参考素材 | @引用 | Agent 模式 | Skill | 文件
智能匹配 / 创作偏好 | 画布 | 发送
```

行为：

- 允许自然语言描述目标。
- Agent 可提问，但问题必须受当前 surface 和 Skill 限制。
- Agent 可使用 Skill、模型目录和已有素材。
- Agent 可先输出方案，再报价，再等待确认。
- 画布开启时，产物可以创建节点或分镜表。
- 画布关闭时，产物仅保存在 Thread、Artifact 或任务中心。

### 5.2 直接媒体模式

创作模式下可切换：

```text
Agent 模式
视频生成
图片生成
音频生成
```

直接视频控件：

- 模型
- 参考生成 / 视频编辑 / 视频延长 / 首尾帧
- 画面比例
- 分辨率
- 时长
- 随机种子
- 参考视频、图片、音频
- 价格

直接图片控件：

- 模型
- 画面比例
- 2K / 4K
- 数量
- 参考图
- 质量
- 价格

直接音频控件：

- 模型
- 音色或参考音频
- 时长/文本
- 价格

底部工具栏必须依据当前操作和模型能力动态禁用不适用参数，而不是提交后才报错。

## 6. 短剧 Agent 需求

### 6.1 启动入口

```text
上传剧本 | 剧本创作 | 自由画布 | 重制转绘
```

### 6.2 上传/粘贴剧本

约束：

- 支持 `txt`、`docx`、`pdf`。
- 最大 100000 字。
- 粘贴弹窗显示字符计数。
- 完成按钮在无内容时禁用。
- 显示版权确认提示。
- 允许跳过剧本进入画布。

输出 Artifact：

```text
script_summary
character_list
relationship_graph
episode_outline
storyboard_plan
```

### 6.3 剧本创作

字段：

- 输入目标或上传参考剧本
- 风格库
- 画面比例：默认、9:16、16:9、21:9、3:4、4:3、1:1
- 集数：5、10、30、60、80、100、自定义
- 国内剧本 / 出海剧本
- 字符计费和生成预估

### 6.4 重制转绘

独立任务 `short_drama_remake`：

- MP4/MOV
- 5～300 秒
- 单文件 <= 500MB
- 最多 50 个文件
- 角色、场景、语言、画风可变
- 必须确认版权或授权

## 7. 营销 Agent 需求

### 7.1 商品

```ts
type MarketingProduct = {
    id: string;
    name: string;
    imageAssetIds: string[];
    sellingPoints?: string;
    description?: string;
    visibility: "public" | "private";
};
```

约束：最多 10 张商品图，名称必填，描述最多 300 字。

### 7.2 角色

真人角色：

- 名称
- 手机扫码认证
- 二维码 10 分钟有效
- 公开/私有
- 公开风险提示

非真人角色：

- 最多 9 张角色图
- 主图
- 角色描述
- 风格
- 参考图
- AI 帮写
- 音色描述
- 音频上传
- 角色生成任务

### 7.3 营销状态

```ts
type MarketingModeState = {
    productIds: string[];
    characterIds: string[];
    creativeId?: string;
    hookId?: string;
    styleId?: string;
    canvasEnabled: boolean;
};
```

创意分类：剧情广告、抖音爆款、跨境电商、达人口播、开箱试用、品牌 TVC、AI 炫酷视觉。

Hook 分类：画面吸睛、爆款脚本、口播留人、抓耳音效、花式特效。

风格分类：自然生活、东方美学、高级质感、年轻潮流、科技未来。

模板选择后只保存结构化 ID，不能只把名称追加到 prompt。

## 8. 底部工具栏设计

底部工具栏必须 schema 驱动：

```ts
type ToolbarControl = {
    id: string;
    type: "asset_slot" | "mention" | "skill" | "mode_picker" | "model_picker" | "preference" | "toggle" | "price" | "submit";
    visibleWhen?: Condition;
    disabledWhen?: Condition;
    payloadPath?: string;
    validation?: ValidationRule[];
};
```

示例：营销模式：

```json
{
  "surface": "marketing",
  "controls": [
    { "id": "products", "type": "asset_slot", "payloadPath": "modeState.productIds" },
    { "id": "characters", "type": "asset_slot", "payloadPath": "modeState.characterIds" },
    { "id": "creative", "type": "skill", "payloadPath": "modeState.creativeId" },
    { "id": "hook", "type": "skill", "payloadPath": "modeState.hookId" },
    { "id": "style", "type": "skill", "payloadPath": "modeState.styleId" },
    { "id": "canvas", "type": "toggle", "payloadPath": "modeState.canvasEnabled" },
    { "id": "submit", "type": "submit" }
  ]
}
```

## 9. Skills 和 Plugins

### 9.1 Skill 合同

qimu 当前 Skill 主要是 `SKILL.md` 和参考文件；目标合同增加：

```ts
type ExecutableSkillContract = {
    skillId: string;
    inputSlots: Array<{ id: string; kind: string; required: boolean; maxCount?: number }>;
    defaultPrompt?: string;
    supportedSurfaces: AgentSurface[];
    supportedModes?: string[];
    modelPolicy?: Record<string, unknown>;
    optionsSchema?: Record<string, unknown>;
    billingPolicy?: Record<string, unknown>;
    outputTypes: AgentOutputTarget[];
    toolBindings: string[];
};
```

选择 Skill 后应触发：输入槽、默认指令、模型限制、参数限制、价格策略和输出类型联动。

### 9.2 Plugin Agent Bridge

```text
Plugin Manifest
  -> Agent Action Schema
  -> 后端工具注册
  -> 权限校验
  -> 预算/审批 admission
  -> 执行
  -> Artifact / Canvas Patch
```

插件不能直接绕过 qimu 的任务队列、资源服务、权限和计费。前端注册表只负责发现和 UI；真正执行必须在后端完成。

## 10. API 与数据设计

建议增加统一入口，不破坏现有 `/agent` 和 `/creation-runs`：

```text
GET  /api/agent/home/capabilities
POST /api/agent/home/threads
GET  /api/agent/home/threads/:id
POST /api/agent/home/threads/:id/messages
GET  /api/agent/home/runs/:id/events
POST /api/agent/home/runs/:id/cancel
POST /api/agent/home/runs/:id/approvals/:approvalId/decision
```

模式专属 API：

```text
GET/POST/PATCH /api/marketing/products
GET/POST/PATCH /api/marketing/characters
GET /api/marketing/templates
POST /api/short-drama/scripts/parse
POST /api/short-drama/scripts/generate
POST /api/short-drama/remake
```

统一请求字段：

```text
surface
threadId
prompt
canvasId?
outputTarget
modeState
inputReferences
skillBindings
modelPolicy
budget
profileRevision
idempotencyKey
```

第一阶段可以让 `AgentThread` 关联现有 `CloudAgentExecution` 或 `CreationRun`，不立即合并表结构。

## 11. qimu 文件改造范围

前端：

- `web/src/router.tsx`
- `web/src/pages/create/index.tsx`
- `web/src/pages/create/creation-workspace.tsx`
- `web/src/pages/create/creation-types.ts`
- `web/src/pages/create/creation-references.ts`
- `web/src/pages/create/creation-runtime.ts`
- `web/src/services/api/agent.ts`
- `web/src/services/api/skills.ts`
- 新增 `web/src/services/api/marketing-agent.ts`
- 新增 `web/src/services/api/short-drama-agent.ts`
- 新增 `web/src/components/home-agent/agent-toolbar.tsx`
- 新增 `web/src/components/home-agent/agent-mode-shell.tsx`
- 新增 `web/src/components/home-agent/asset-slot-picker.tsx`
- 新增 `web/src/components/home-agent/model-preference-popover.tsx`

后端：

- `backend/internal/app/cloud_agent.go`
- `backend/internal/app/cloud_agent_runtime.go`
- `backend/internal/app/cloud_agent_tools.go`
- `backend/internal/app/cloud_agent_media.go`
- `backend/internal/app/creation.go`
- `backend/internal/handler/agent.go`
- `backend/internal/handler/creation.go`
- `backend/internal/model/models_agent_profile.go`
- 新增 `backend/internal/app/marketing_agent.go`
- 新增 `backend/internal/app/short_drama_agent.go`
- 新增 `backend/internal/app/agent_skill_contract.go`
- 新增 `backend/internal/app/agent_artifact.go`
- 新增 `backend/internal/handler/marketing.go`
- 新增 `backend/internal/handler/short_drama.go`
- 新增商品、角色、营销模板模型和 repository
- 新增数据库迁移和权限测试

## 12. 验收标准

必须通过：

1. 首页和画布可以使用同一个 Thread 继续对话。
2. Agent、视频、图片、音频模式切换不会丢失草稿和引用。
3. Skill 选择后能改变输入槽、提示词、模型和计费策略。
4. Skill 移除后恢复默认工具栏。
5. 短剧支持上传、粘贴、剧本生成、集数、风格、本土化。
6. 短剧可以跳过剧本直接进入画布。
7. 营销可以创建商品和角色，并在当前 Agent 中选择。
8. 营销创意、Hook、风格使用结构化 ID 保存。
9. 关闭画布时不创建画布节点，开启画布时能落地 Artifact。
10. 页面刷新、SSE 断线和重复提交可恢复且不重复扣费。
11. 插件工具缺少权限时由后端拒绝。
12. 前端不能伪造模型、价格、技能权限、审批或产物状态。
