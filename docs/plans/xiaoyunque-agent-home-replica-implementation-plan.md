# Qimu Agent Home 复刻实施计划

> 配套设计：[Qimu Agent Home 复刻产品与技术设计](../design/xiaoyunque-agent-home-replica-design.md)。
>
> 本文是实施拆解，不代表代码已经完成。每个阶段都必须可独立验证和回滚。

## 1. 实施原则

- 先统一合同，再替换页面；不直接复制现有画布 Agent 组件到首页。
- 第一阶段保留 `CloudAgentExecution` 和 `CreationRun`，使用统一 `AgentThread` 外层关联，避免一次性数据库迁移。
- 所有写操作沿用现有幂等、审批、预算和 owner 校验。
- 工具按 `surface`、`permissionMode`、`outputTarget` 和预算裁剪。
- Skill 是不可信数据；Skill 文本不能新增工具、权限、模型或预算。
- 插件执行必须后端 fail-closed；前端注册不能等同于执行授权。
- 每个里程碑都补单元测试、API 合同测试和最小浏览器验收。

## 2. 里程碑总览

| 里程碑 | 目标 | 主要结果 |
| --- | --- | --- |
| M0 | 合同和状态基线 | Agent surface、Thread、ToolbarSpec、迁移策略 |
| M1 | 统一 Agent 外层 | 首页和画布共享 Thread/Run/Artifact 观察模型 |
| M2 | 底部工具栏 | schema 驱动的模式、Skill、模型和输出控件 |
| M3 | 创作 Agent | Agent/视频/图片/音频三类入口完整联动 |
| M4 | 短剧 Agent | 剧本、分集、风格、本土化、画布和重制转绘 |
| M5 | 营销 Agent | 商品、角色、创意、Hook、风格和营销提交 |
| M6 | Skill 执行合同 | 输入槽、模型策略、报价和输出类型 |
| M7 | Plugin Agent Bridge | 插件 Action 后端注册、权限、审批和执行 |
| M8 | 全面验收 | 恢复、计费、安全、浏览器和回滚演练 |

## 3. M0：合同和数据基线

### 前端任务

- 新增 `AgentSurface`、`AgentOutputTarget`、`AgentHomeRequest` 类型。
- 新增 `AgentThreadSummary`、`AgentArtifact`、`AgentToolbarSpec` 类型。
- 在 `creation-types.ts` 增加 surface 和模式状态类型。
- 为 URL 查询参数定义 surface 解析和默认策略。

### 后端任务

- 新增 surface、outputTarget、modeState 的 Go 合同。
- 确定未知 surface、非法 outputTarget 和空 idempotencyKey 的错误码。
- 设计 `AgentThread` 外层记录，至少包含：用户、surface、标题、当前 run、可选 canvas、状态和 mode state。
- 确认 `CloudAgentExecution`、`CreationRun` 与 Thread 的关联字段。

### 文件范围

```text
web/src/services/api/agent.ts
web/src/pages/create/creation-types.ts
backend/internal/app/cloud_agent.go
backend/internal/app/creation.go
backend/internal/model/
backend/internal/repository/
```

### 验证

- TypeScript 类型测试。
- Go 请求校验测试。
- 同 idempotencyKey 的重复请求只产生一个运行。

## 4. M1：统一 Agent 外层

### 后端

- 新增 Thread 查询、创建、追加消息和事件接口。
- `CloudAgentRequest.CanvasID` 改为 surface 条件校验。
- 无画布 surface 不读取画布摘要，不暴露 canvas tools。
- canvas surface 继续使用现有快照、Capability 和 snapshotHash 守卫。
- 将 Cloud Agent 事件和 CreationRun 事件映射成统一 Artifact/Run 视图。

### 前端

- 新增 `agent-thread-store.ts` 或对应 Query 层。
- 首页恢复 Thread、Run、Artifact 和待审批状态。
- 画布 Agent 改为读取统一运行视图，但保留原画布同步适配器。

### 验证

- 首页创建无画布 Run。
- 首页 Run 继续追加消息。
- 画布 Run 继续读写画布。
- 浏览器断线后通过事件游标恢复。

## 5. M2：底部工具栏替换

### 组件拆分

新增：

```text
web/src/components/home-agent/agent-toolbar.tsx
web/src/components/home-agent/agent-toolbar-spec.ts
web/src/components/home-agent/agent-mode-picker.tsx
web/src/components/home-agent/agent-input-slots.tsx
web/src/components/home-agent/agent-model-preferences.tsx
web/src/components/home-agent/agent-output-toggle.tsx
```

### 行为

- 由 surface 和模式返回 ToolbarSpec。
- `visibleWhen` 和 `disabledWhen` 只控制界面，不替代服务端校验。
- 参数变化更新结构化 `modeState`。
- 模型变化重新计算能力、价格和可用参数。
- 画布开关只改变 outputTarget，不改变历史 Artifact。
- 输入槽绑定真实资产 ID，不把临时 URL 直接写进会话。

### 验证

- 每个 surface 的工具栏快照测试。
- 直接视频模式禁用不兼容参数。
- Skill 绑定后工具栏能动态增加输入槽。
- 移除 Skill 后完整恢复默认状态。

## 6. M3：创作 Agent

### 前端

- `CreatePage` 改为使用统一 Thread。
- 保留现有图片、视频、文本生成任务恢复逻辑。
- 增加 Agent/视频/图片/音频 mode picker。
- 增加模型、比例、分辨率、时长、种子、报价显示。
- 增加画布输出开关。
- 将推荐案例点击转成 prompt 草稿，不直接提交。

### 后端

- 增加 creative surface 工具集：规划、提示词优化、模型目录、媒体任务。
- 复用 `CreationSubmission` 生成报价。
- 画布开启时使用现有 `CreateRunCanvas` / `CommitCreationCanvas`。
- 画布关闭时只创建普通 Artifact 和媒体任务。

### 文件范围

```text
web/src/pages/create/index.tsx
web/src/pages/create/creation-workspace.tsx
web/src/pages/create/creation-runtime.ts
web/src/services/api/generation-task.ts
backend/internal/app/cloud_agent_runtime.go
backend/internal/app/creation.go
```

## 7. M4：短剧 Agent

### 数据和 API

新增：

```text
ShortDramaProject
ShortDramaScript
ShortDramaEpisode
ShortDramaRemakeTask
```

API：

```text
POST /api/short-drama/scripts/upload
POST /api/short-drama/scripts/paste
POST /api/short-drama/scripts/generate
PATCH /api/short-drama/scripts/:id
POST /api/short-drama/episodes/generate
POST /api/short-drama/storyboard/generate
POST /api/short-drama/remake
POST /api/short-drama/canvas
```

### 前端文件

```text
web/src/pages/short-drama/index.tsx
web/src/components/short-drama/script-input-panel.tsx
web/src/components/short-drama/script-generation-panel.tsx
web/src/components/short-drama/remake-upload-panel.tsx
web/src/components/short-drama/short-drama-toolbar.tsx
web/src/services/api/short-drama-agent.ts
```

### 验证

- 10 万字限制和字符计数。
- 上传/粘贴状态互斥且可切换。
- 集数、比例、风格和本土化进入请求快照。
- 跳过剧本可以直接创建画布。
- 重制转绘不会误走普通视频生成接口。

## 8. M5：营销 Agent

### 数据模型

新增：

```text
MarketingProduct
MarketingCharacter
MarketingCreativeTemplate
MarketingHookTemplate
MarketingVisualStyle
MarketingAgentState
```

商品和角色都必须带用户归属、可见范围、资源引用和版本信息。

### API

```text
GET/POST/PATCH/DELETE /api/marketing/products
GET/POST/PATCH/DELETE /api/marketing/characters
POST /api/marketing/characters/:id/generate
GET /api/marketing/templates
POST /api/marketing/agent/runs
POST /api/marketing/agent/runs/:id/messages
POST /api/marketing/agent/runs/:id/submit
```

### 前端文件

```text
web/src/pages/marketing/index.tsx
web/src/components/marketing/product-picker.tsx
web/src/components/marketing/product-editor.tsx
web/src/components/marketing/character-picker.tsx
web/src/components/marketing/character-editor.tsx
web/src/components/marketing/marketing-template-picker.tsx
web/src/components/marketing/marketing-toolbar.tsx
web/src/services/api/marketing-agent.ts
```

### 验证

- 商品图最多 10 张。
- 非真人角色图最多 9 张。
- 真人角色必须经过扫码流程，不允许前端伪造通过状态。
- 创意、Hook、风格使用 ID 保存。
- 营销 Agent 只能读取当前用户有权访问的商品和角色。

## 9. M6：Skill 执行合同

### 后端

- 扩展 Skill 元数据和版本快照。
- 增加 `inputSlots`、`toolBindings`、`modelPolicy`、`billingPolicy`。
- 创建 Skill 绑定校验器。
- Skill 变更后拒绝混用旧版本。
- Skill 内容不能扩展权限和工具。

### 前端

- Skill 选择后读取 contract，不直接猜控件。
- 根据 input slot 渲染上传、资产选择、文本或角色槽。
- 显示 Skill 版本和缺失输入。
- Skill 移除时清理其专属输入，但保留用户普通 prompt。

### 验证

- Skill 缺少必需视频时不能提交。
- Skill 指定视频模型时不能选择图片模型。
- Skill 计费策略和报价一致。

## 10. M7：Plugin Agent Bridge

### 后端

- 将插件 `agentActions` 转为受控工具 schema。
- 工具名命名空间：`plugin.<pluginId>.<actionId>`。
- 每次执行校验插件平台状态、用户状态、版本、权限和 surface。
- 写操作统一生成 approval preview。
- 运行快照保存 plugin ID、版本和包摘要。

### 前端

- 插件能力出现在 toolbar/Skill/工具状态中。
- 插件禁用后不影响历史 Artifact，但禁止新调用。
- 插件失败显示可解释错误，不静默降级。

### 安全测试

- 未注册插件拒绝。
- 缺少权限拒绝。
- Skill/插件文本不能改变工具集合。
- 插件不能直接访问密钥、Cookie、数据库或任意网络。

## 11. M8：验证和发布

### 自动化

```text
cd qimu/web && bun run build
cd qimu/backend && go test ./...
```

新增专项测试：

- Agent surface 请求合同
- Thread 幂等和追加
- ToolbarSpec 条件渲染
- Skill 输入槽和版本冻结
- 商品/角色归属
- 报价刷新和审批
- 画布开关输出分支
- SSE 断线恢复
- 插件 Action 权限

### 浏览器验收

1. 创作/短剧/营销路由刷新后保持当前 surface。
2. Agent/直接生成切换不丢草稿。
3. Skill 选择改变工具栏和输入槽。
4. 短剧粘贴弹窗字符计数正确。
5. 营销商品和角色资产可选择和移除。
6. 画布开关控制输出目标。
7. 报价、审批、任务和 Artifact 在刷新后恢复。
8. 浏览器断开不会停止后端任务。

## 12. 回滚策略

- 通过 feature flag 分别启用 creative、short_drama、marketing surface。
- 保留 `/create` 旧入口作为 fallback。
- 新 Agent Thread 只读历史不改写既有 `CloudAgentExecution` 和 `CreationRun`。
- 新商品/角色数据独立表，删除功能和清理用户数据分离。
- 插件 Bridge 可以单独关闭，不影响内置 Skill 和画布 Agent。
