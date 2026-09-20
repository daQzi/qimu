# P08：持久会话与首页、画布衔接

状态：P00–P07 已由用户验收。P08 在完整集成分支上使用独立 worktree 实施，完成后仅合入 `codex/codex-plugin`，等待本阶段验收。不合入 main、不推送、不部署、不调用真实收费供应商。

## 1. 已实现边界

- 首页 Agent 可先新建云端会话并原地对话，不再强制创建空画布。按需打开画布，画布内可返回首页继续同一个 Thread。
- 新增持久 Thread、不可变轮次引用和历史画布绑定。Thread 负责组织会话，不执行模型、不另建队列、不维护新钱包。
- 每轮仍对应既有 CloudAgentExecution/Task。消息、Task 与费用预留同事务提交；服务器决定父轮次、固定当轮请求/技能/偏好/画布上下文。插件 InvocationContext 继承服务器确定的 ThreadID。
- 云端历史可按页读取，旧画布的本机对话仍可从“本机旧对话”访问；没有自动迁移、删除或重做旧任务。
- 同一个 Agent 面板、事件消费层、审批和 PluginRunCard 用于首页与画布。模型消息按 runId 隔离；Agent 与插件各自保留原事件游标。
- 会话可从已有记录列表关联本账号 PluginRun/CreationRun。关联仅新增引用，不运行、不审批、不夺取原 CreationRun 的执行租约。插件引用使用原运行卡片；创作引用显示原状态，详细执行仍在原创作入口。
- 首页直接生成入口保持原实现，模型选择、任务权限、报价与费用校验均不被 Thread 绕过。

本阶段支持当前已经开放的画布、技能和偏好上下文。Workbench/Recipe 配置属于 P09，业务对象属于 P10；未添加可接收任意对象的占位字段，也没有声称已经实现任意业务工作台。

## 2. 接入合同

所有路径以 `/api` 为前缀，使用当前登录态，响应沿用 `{code,data,msg,reason}`。前端入口为 `services/api/agent-threads.ts`，不绕过 `http`。

| API | 请求及返回 | 行为 |
| --- | --- | --- |
| `POST /agent/threads` | `{clientKey,title?,canvasId?}` → `{thread}` | clientKey 8–128 字符，标题最多 80 字；相同账号同键同输入返回原会话，同键异输入 409 |
| `GET /agent/threads` | `canvasId?`, `offset?` → `{threads,hasMore}` | 每页 50；按更新时间倒序；canvasId 按历史绑定过滤 |
| `GET /agent/threads/:id` | `before?` → `{thread,entries,canvases,nextBefore?}` | 每页 20，页内升序；下一页使用 nextBefore，不把 Run ID 当游标 |
| `PATCH /agent/threads/:id/context` | `{revision,canvasId}` → `{thread}` | 只修改未来轮次；空 ID 解除当前绑定，历史绑定保留 |
| `POST /agent/threads/:id/messages` | `{revision,request}` → `{thread,run}` | request 为已有 Agent 请求；不得传 threadId，父轮次由服务器确定 |
| `POST /agent/threads/:id/references` | `{revision,clientKey,kind,runId}` → `{thread}` | kind 为 plugin/creation，必须是当前账号已有运行 |

Thread 的 `revision` 覆盖追加和绑定修改；`contextRevision` 仅在绑定变化时递增。每条 entry 保存 sequence、kind、runId、prompt、contextRevision 和当轮固定请求。已有 Run 的完整状态、技能内容快照、偏好和事件仍以原执行记录为准。

无画布请求示例（模型 ID 应从系统受管模型目录选择）：

```json
{
  "revision": 1,
  "request": {
    "hostSurface": "agent-home",
    "canvasId": "",
    "prompt": "帮我整理创意方向，暂不生成媒体",
    "logicalModelId": "从系统模型目录选择的实际ID",
    "permissionMode": "read_only",
    "skillIds": [],
    "contextScope": [],
    "budget": {"maxCredits": 1, "maxGenerationTasks": 0, "maxVideoSeconds": 0},
    "idempotencyKey": "由客户端生成并持久保存的唯一键"
  }
}
```

### 并发与恢复

1. 同一 Thread 的写入先获取数据库行锁；SQLite 在读取前获得写锁，PostgreSQL 不依赖进程内锁。每轮任务准入和收据写入必须共同提交。
2. 消息重试先核对原请求摘要，之后才核对当前 revision、偏好和画布。响应丢失后，原请求仍可找回同一 Run，不重复计费。
3. 上一轮处于运行、待审批或清理中时不能追加新轮次。运行中的插话沿用原 Agent interjection API。
4. 客户端发送前在账号/Thread 范围持久保存完整请求、幂等键和 revision。页面切换不会按新画布改写待确认请求；记录损坏会暂停发送。
5. 409 表示本次消息未被接受，客户端清理本次待提交记录并要求“重新读取”核对会话后再发；传输错误保留原请求供原样重试。不会自动换键重跑。
6. Agent SSE 按每个 runId 的事件 seq 恢复；插件卡片继续使用 PluginRun 自己的游标。读取历史不会回放画布写入或重新弹出历史审批。
7. 绑定变化不能修改旧 Run 的画布。旧 Run 的画布事件不会被应用到当前另一画布。新画布的后续操作仍需要原权限和审批。

## 3. 数据库与部署边界

新增 schema 36 `agent_threads`，保留 35 及以前的名称、校验和与时间：

- `agent_threads`：账号与 clientKey 唯一、创建请求摘要、标题、当前画布、两类 revision、最新 sequence、最后 Agent Run。
- `agent_thread_entries`：Thread/sequence 主键，Thread/clientKey 唯一；不可变运行引用和上下文 JSON。
- `agent_thread_canvases`：Thread/画布复合主键，仅保存历史绑定。

SQLite→PostgreSQL 搬迁工具已加入三张表。没有迁移本机业务库；自动验证只在临时 SQLite 和隔离 PostgreSQL 中执行。

部署验收环境时，先备份数据库、资源、插件包与加密密钥，按现有部署机制运行 `migrate-schema up`，随后 `migrate-schema verify` 确认为 36。开发命令也必须显式指定已确认的数据目录，不能默认使用 backend/data；不要把本次测试数据库作为业务库。

回退应先关闭新增入口，保留 v36 数据与运行读取/审批/取消能力。旧二进制会拒绝更高 schema，不可直接覆盖部署；不提供删表降级脚本，不删除已创建会话或账务来模拟回退。

## 4. 用户验收步骤

使用 `codex/codex-plugin` 对应的前后端和 schema 36 验收环境，保留现有登录账号、受管文本模型及 P07 示例插件。

1. **首页原地对话**：进入“创作 → Agent”，点“新建云端会话”。选择受管文本模型，输入普通问题。应原地收到回复，画布列表没有新增空画布。刷新后从“云端历史”恢复原消息。
2. **跨页面继续**：在此会话点“打开画布”，再发一轮；点“首页继续”后继续提问。URL 的 thread 值保持一致，已有回复、任务和费用不因跳转重复。画布可以按需新建一次。
3. **既有画布**：打开另一张已有画布，在 Agent 的“云端历史”选中该会话，点“使用当前画布继续”。历史轮次保留旧画布，下一轮才使用新画布；不确认绑定时不能向错误画布追加新轮次。
4. **审批/输入**：引用 P07 技能触发需要确认的插件操作；在待审批或待输入时切换首页与画布，应看到同一个 Run/approval/input。只批准一次，表单提交后不出现重复任务或费用。旧节点仍能定位、输入、继续对话。
5. **双标签页**：两页打开同一个 thread，同时发不同的新轮次。只能一个成功追加；另一页提示重新读取。重新读取后能看到已接受的轮次，不自动重发被拒绝的消息。
6. **断网恢复**：提交后断网或刷新，恢复连接后原样重试待确认消息。模型、消息和预算不能被新草稿覆盖；任务中心不出现同一消息对应的第二笔任务。
7. **旧对话与账号隔离**：画布不带 thread 参数时仍能恢复本机旧对话。切换账号后不能看到上一账号云端会话；把旧账号 thread URL 粘贴给另一账号应返回不可访问，不自动创建替代会话。
8. **引用与直接生成**：云端历史内选择本人已有插件/创作记录关联，检查没有重新执行或授权。切回直接生成，正常完成一次原流程，报价和生成检查照常生效。
9. **布局**：检查首页内嵌滚动、画布面板缩放/拖拽、明暗主题、中文输入、弹层焦点和更早消息分页。更换入口不应把已结束的历史审批变成新审批。

## 5. 验证范围

自动检查覆盖消息/任务/费用原子回滚、同键重试、双实例 PostgreSQL 并发、账户隔离、跨画布续聊、上下文固定、引用不执行、HTTP 严格解码和鉴权、schema 35→36 重复迁移与插件历史保留，以及前端事件恢复和待提交记录隔离。

验证结果：

- P08 的 app/handler/database 专项以及 SQLite/PostgreSQL 迁移均通过，三包相关 `-race` 检测通过。
- 搬迁清单校验发现 GORM 对 Canvas 的复数处理与手写查询不一致，已显式固定 `agent_thread_canvases`，补充按画布列会话的测试；app/handler/database/搬迁工具受影响测试重新执行 `-race` 后全部通过。
- 扩展回归覆盖 CloudAgent、Creation 与 P03–P08。首次发现既有节点目录断言漏计 P07 的 plugin-input，以及隔离环境缺少 Redis；补齐名单和权限断言、隔离 Redis 后，两项重跑通过。其余选定回归通过。
- 前端 42 项相关测试通过，覆盖共享事件消费、原 SSE 恢复、账号/会话幂等缓存、面板组件、原对话与画布同步；最终 `bun run build` 通过。构建仍有既有大 chunk 提示。
- OpenAPI YAML 解析及新增路径存在性检查通过；没有替换原客户端协议。
- 开始前和提交前均 fetch fork origin，本地主线与 origin/main 保持 `9afe061d`，阶段分支合入 main 无新增差异。没有更新远端分支。

前端通过生产构建验证实际页面与类型。浏览器点击流程和真实模型效果仍由上述用户验收确认；没有启动本地开发服务，也没有把组件测试当作浏览器验收。通过后再开始 P09。
