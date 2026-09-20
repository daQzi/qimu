---
title: 工作台与配方接入
description: hostApi 3.1 的通用工作台、动态表单、配方冲突和 Agent 固定上下文合同。
---

# P09 工作台与配方接入合同

这是 `codex-workstudio` 中的已实现合同，手工验收见 [P09 清单](../../../plans/qimu-workstudio-p09-acceptance.md)。旧插件交付 PR 不自动包含本阶段。仍使用 v3 包，新增能力要求 `requires.hostApi: "^3.1.0"`。

## 包声明

在 Manifest 的 `contributes` 中增加（每类最多 16 项）：

```json
{
  "workbenches": [{ "id": "compose", "ref": "workbenches/compose.json" }],
  "recipes": [{ "id": "social", "ref": "recipes/social.json" }]
}
```

`workbenches/compose.json` 示例（`assist` 须在同包声明为 Skill）：

```json
{
  "id": "compose",
  "name": "品牌文案助手",
  "description": "按品牌、受众和渠道编写文案",
  "hostSurfaces": ["agent-home", "canvas"],
  "contextSchemaRef": "schemas/context.json",
  "skill": "assist",
  "recipes": ["social"],
  "defaults": {"tone": "友好", "channel": "社交媒体"},
  "output": "result",
  "prompt": "基于已知事实编写方案，不编造功效与价格。"
}
```

- `hostSurface` 是宿主入口枚举；工作台地址是 `pluginId.workbenchId`，保存在独立 `workbench` 选择中，不能把业务工作台 ID 填进 hostSurface。
- `operation` 可选，格式 `pluginId.operationId`，目前只支持本包。`operation`、`skill` 至少一个；两者都有时“确认执行操作”和“交给 Agent”是不同的显式入口。
- Skill 可以声明 `launchOperation`；必须在其 `operations` 列表中，且与绑定工作台的操作一致。该字段不自动运行技能或操作，也不创建另一份输入表单 Schema。
- `contextSchemaRef` 必须是本包 `schemas/` 下显式 object 根 Schema，最多 32 个顶层字段，字段名为字母开头的字母/数字/下划线，最长 80。不支持根 `$ref`；子字段和嵌套 `$ref` 仍可使用已有安全引用规则。有操作时必须与 Operation.inputSchemaRef 完全相同。
- 可用字段元数据包括 `title`（最多 160 字符）、`description`、`required`；枚举/布尔使用选择器，标量使用输入框，复杂结构使用 JSON 编辑器。暂不支持任意 UI、脚本、条件布局或远程 Schema。
- `output` 为 `result` 或 `canvas_optional`，表达输出意图，不授予投影权限。画布写入仍需要 Operation/Blueprint 与原宿主批准。

`recipes/social.json` 示例：

```json
{
  "id": "social",
  "name": "社交种草",
  "defaults": {"tone": "友好"},
  "requirements": {"channel": "社交媒体"},
  "prompt": "使用适合目标受众的简短表达。"
}
```

合并顺序为工作台默认值、配方默认值、配方字段要求、用户显式值。用户值能覆盖建议，不能违反要求；多个配方建议同字段不同值时，用户必须显式选择；多个配方要求相互矛盾时必须移除冲突配方。配方按 ID 排序，结果不取决于勾选顺序。每个值在安装时校验字段类型，启动时再校验完整输入。

配方没有权限/预算/模型选择字段；同名普通输入也不能影响宿主授权。方法提示不能替代系统规则。

## 调用顺序与快照

1. `GET /api/plugin-workbenches` 列出当前用户已启用发布的工作台。
2. `GET /api/plugin-workbenches/{pluginId.workbenchId}?releaseId=...&hostSurface=agent-home` 获取配置；画布入口另带真实 `canvasId`，所有权由后端验证。
3. `POST /api/plugin-workbench-previews` 提交 `{id, releaseId, recipeIds, input, context}`。`input` 只放用户显式编辑值，后端填入建议；`recipeIds` 使用空数组表示未选择，不能为 null。请求最多 64 KiB。
4. 检查 `valid`、`conflicts` 和 `validationMessage`，展示合并后的 `input` 与 `prompts`。只有有效结果的 `selection` 可以进入下一步。
5. 直接操作：原 `POST /api/plugin-invocations` 增加 `workbench: selection`，使用相同 operation、releaseId、最终 input 和 context。写操作沿用固定幂等键与审批。
6. Agent：在原 Agent/Thread 请求中带 `workbench: selection`（最多 32 KiB），仍由用户选择模型、权限、预算和发送目标。后端自动解析本发布绑定的 Skill，固定本轮上下文；不会自动发送对话。

`selection` 为 `{id, releaseId, recipeIds, input, digest}`；摘要绑定入口/画布、版本配置、配方与最终输入，提交时重验当前权限和版本，摘要不是授权令牌。配方正文与定义来自不可变 PluginRelease，Run 和 Agent/Thread 记录保存选择；升级不改历史。恢复到另一个 hostSurface/画布时要重新预览本轮，不能直接复用旧摘要。

同一工作台的新一轮可以显式修改输入；不同工作台必须新建会话，不能自动携带另一工作台的历史。原会话从通用首页或画布历史打开时会恢复其工作台选择。插件版本变化会要求重新打开，不静默替换已提交快照。

工作台不创建第二套任务引擎。`PluginRun.RequestJSON`、Agent 请求和 Thread 既有上下文保存选择，无新数据库迁移。Pipeline 派生如果改变工作台顶层输入，须回到工作台重新预览创建新运行；步骤输入沿用原规则。

## 示例与扩展边界

示例源码见 `examples/plugins/resource-workbench`、`brand-workbench`、`brief-workbench`。用现有 `plugin-contract -dir ... -out ...` 打包，再按原安装/启用流程操作。加入第四个同类工作台只需要包配置、Skill 和已有操作，无需新增 Agent 或 Task 业务分支。

纯 Markdown Skill 仍可独立使用。只读 inline 结果不创建持久 Run；刷新后重新检查即可。草稿按账号、工作台、发布、画布保存在本机浏览器，不等于云端保存；JSON 尚未合法时的中间文本不保证跨页面恢复。

跨包工作台组合、objectTypes、外部开放鉴权、Webhook 和公开市场仍未实现。关闭入口不能代替在途任务的取消/费用结算；旧后端不认识 hostApi 3.1，不能直接混跑处理这些任务。

## P10 品牌对象：hostApi 3.2

要求 `requires.hostApi: "^3.2.0"`。仍用同一包和宿主，不声明 objectTypes。完整样例见 `examples/plugins/brand-points` 和 `brand-script`；边界、配额和验收见 [P10 清单](../../../plans/qimu-workstudio-p10-acceptance.md)。

工作台增加 `"objectInputs": {"reference": "brand/v1"}`。reference 必须是 contextSchemaRef 根 properties 中的必填字段，最多声明 8 个对象字段；Manifest 必须申请 asset.read，运行时用户也必须实际授权。输入合同如下，可放在本包 Schema 的对应 property 中：

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["objectId", "version", "type", "schemaVersion"],
  "properties": {
    "objectId": {"type": "string", "minLength": 1, "maxLength": 80},
    "version": {"type": "integer", "minimum": 1, "maximum": 100},
    "type": {"const": "brand"},
    "schemaVersion": {"const": 1}
  }
}
```

表单展示当前账号的品牌选择器；选择固定版本，不自动跟随最新版。预览除原有 selection 外新增 `objects: {reference: {reference, brand, digest, source?, archived, currentVersion}}`。这些事实由服务器读出，不接受浏览器传入同名资料覆盖。交给 Agent 时把资料冻结在既有任务模型请求中；对象文本仅为数据，不授予操作权限。归档阻止新调用，已提交任务保留其快照。

可复用的 Host 操作：

| adapter | 输入 | 权限 / effects |
| --- | --- | --- |
| object.read | reference 完整引用 | asset.read / read |
| object.search | query 字符串、offset 整数 | asset.search / read |
| object.save | objectId?、expectedVersion、clientKey、brand、source? | asset.import / draft_write |

三者均为 `execution.kind: "host", mode: "inline"`。read/save 返回 ObjectView；search 返回 `{objects: [...]}` 索引列表。品牌字段为 name、audience、positioning、claims、restrictions，具体约束直接参考示例 Schema。除已有 Schema 校验，Adapter 会强校验领域值、归属、版本和大小；放宽包 Schema 不能放宽宿主权限。read 拒绝已归档对象；管理接口可读取归档历史。

save 不代表立即保存：经原插件审批后才提交对象版本。新建不带 objectId、expectedVersion=0；更新必须携带读到的版本。clientKey 标识一份不可变写入意图，不能复用来提交不同内容；调用层还需保留 plugin-invocations 的幂等键。审批期间版本或状态变化会失败，不用新版替换用户批准的旧输入。纯查询/整理请求不能触发 save。

API 管理入口为 `GET/POST /api/business-objects`、`GET /api/business-objects/:id/versions/:version`、`PATCH /api/business-objects/:id/archive`，仅面向已登录的当前账号，详见 OpenAPI。不是对外开放鉴权平台。没有 DELETE，所有历史版本保留；source 派生来源记录在新对象 v1。不同 type/schemaVersion 显式拒绝，本轮无跨 Schema 转换器。

两个示例工作台仅绑定 Skill，点击“交给 Agent”启动现有模型流程；其 read/search/save 可通过技能和现有操作入口使用，不把只读操作假装成生成任务。品牌资料不含媒体，后续媒体引用和自定义对象贡献需独立设计资源保护。
