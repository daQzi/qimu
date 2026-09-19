# P03 交付与验收：插件结果卡片与画布绑定

> 状态：用户已确认 P03 验收通过，提交 `af6e404b` 已合入 `codex/codex-plugin`；当前继续 P04。本文保留 P03 交付时的 schema 30 和验收记录。
> 阶段分支：`codex/plugin-p03-canvas-results`，独立 worktree；开始时已核对并同步 local main `308cf424`（v1.5.4）。完成后合入 `codex/codex-plugin`，不合并 main、不推送。

## 1. 本阶段交付

- 复用 P02 的检索、describe、invoke 和用户审批，新增可信 Host Adapter `canvas.blueprint.instantiate`。Agent 不增加视频助手专用工具或业务分支。
- 读取接口返回真实结果摘要 `digest`。1.3.0 的保存动作要求 `expectedDigest`，后端重新检查资源，防止把变化后的数据当作用户刚看到的结果保存。
- 运行卡片按包内 `key-value/v1` 视图渲染字段，显示真实运行状态、操作版本、来源和冲突原因。缺失字段显示“未记录”，不捏造时长、尺寸或进度百分比。
- 前后端注册 `plugin-result` 节点；节点绑定成功结果，不复制可被编辑的“结果事实”。运行事实仍在 PluginRun，画布是展示和交互入口。
- 成功结果可以直接保留，也可以经用户确认保存到当前画布。保存节点不再次检查原视频、不调用远程服务、不生成媒体。
- 蓝图模板 key 与真实 nodeId 持久绑定；同一结果、画布、发布、蓝图和 instanceKey 重复保存不重复建节点。两份待审批请求并发批准也只写入一次。
- 写入复用既有画布存储配额、历史快照和 revision 检查；审批前重新检查画布 snapshotHash、当前权限与固定发布。节点标题、位置、尺寸或绑定被用户修改，以及节点被删除，都不能被旧请求覆盖或恢复。
- 冲突保留原始成功结果，运行查询返回 `projectionStatus=conflict` 与原因；用户可取消旧审批后重新读取画布，或明确选择“另建结果节点”。

P03 不含远程 Connector、长任务、Pipeline、参数输入节点、首页 Agent UI、任意插件前端代码或公共结果分享。标准结果节点不作为生成参考素材，不开放输入/输出连线。

## 2. 数据库与升级

结构版本为 **30**：新增 `plugin_canvas_projections`，保存归属、源运行、发布、蓝图、模板 key→nodeId 映射、原始节点信息及投影回执。`plugin_runs` 新增 `projection_status/failure_message`，记录投影结果与可恢复冲突。

P01 和 main 的两条 v27 历史谱系继续按 P02 规则识别。v29→v30 不改写已有成功 ResultRef、资源引用和运行 revision。迁移可重复执行；本阶段只操作隔离测试数据库，不迁移真实业务库。

验收前备份数据库及数据目录，使用本分支构建的迁移工具执行 `migrate-schema up`、`verify`，前后端须来自同一分支。回退需要匹配的二进制与数据库备份，不能删除用户结果或把旧二进制直接指向 schema 30。

画布能力版本提升到 `canvas-capabilities/v5`。沿用现有 Agent 的版本安全策略：历史运行可读取，旧能力版本的在途 Agent 不静默使用新工具合同继续执行；请在新版环境发起新一轮。P02 已保存的插件结果和独立审批仍可查询。

## 3. 验收包与准备

[resource-helper-1.3.0.yingce-plugin](../../plugin-packages/application-examples/resource-helper-1.3.0.yingce-plugin)

1. 管理员在 `/admin/plugins` 导入 1.3.0。
2. 用户在 `/plugins` 选择并启用 1.3.0，授予 `media.read`、`resource.create`、`canvas.read`、`canvas.write`。上传新版不会自动替换已启用版本。
3. 打开一个已有画布，上传真实视频并等待资源就绪。确认画布已保存到服务器。
4. 新开一轮画布 Agent 对话，选择 `check-source` 技能并引用该视频。实际 Agent 对话仍使用已有模型和计费；下面插件操作自身不调用模型。

## 4. 页面验收

### A. 检查与保存结果

1. 对 Agent 说：“检查这个视频的信息，先不要生成内容。”
2. Agent 应从画布取得真实 resourceId，describe 后执行 inspect-video，并解释实际返回字段。画布不应立即新增节点。
3. 再要求保存结果；snapshot-video 携带刚读取的 digest，卡片显示“等待确认”。用户确认后才成功。
4. 不选择画布输出时，仍可查看该运行结果。刷新后可在插件中心用 runId 查询，不依赖当前 Agent 正在运行。
5. 插件中心也能直接“读取视频信息→保存元数据快照”，无需经过 Agent；1.3.0 未先读取时会提示先检查。

### B. 保存到画布

1. 在画布 Agent 的成功结果卡点击“保存到当前画布”；或让 Agent 调用 place-result。二者走同一后端操作。
2. 出现独立画布写入审批，确认前没有新节点。auto 模式也必须由用户确认。
3. 点击“确认保存到画布”后，应在已有节点右侧出现一个“视频资源信息”节点，显示来源、格式及已知尺寸/时长、操作版本。插件可显式传入布局位置；未指定时宿主自动避开已有节点。原文本、视频节点保持正常。
4. 如自动同步失败，先处理已有的本地/远端草稿冲突，再用“同步画布结果”重读；界面不能把服务端已保存说成没有保存。
5. 刷新画布，结果节点仍在且能读取同一成功结果。标题可通过已有铅笔入口修改，节点可拖动/缩放，内部滚动不能缩放画布。
6. 再次保存同一结果，确认后仍只有一组节点；不会重复调用视频检查或媒体生成。

### C. 用户编辑与冲突恢复

1. 发起画布保存，但在确认前编辑其他节点，再批准旧请求：应返回画布冲突，且没有部分新增的结果节点。原成功结果仍可读。
2. 取消旧审批，重新读取画布并发起保存，可继续使用原成功结果。
3. 修改已有结果节点的名称或位置，再对同一 instanceKey 重复保存：应提示绑定冲突，不覆盖用户修改。
4. 显式点击“另建结果节点”，确认后新增一组节点，原节点及其修改保留。
5. 删除结果节点后，重复旧保存不能自动恢复该节点；需要用户明确另建。删除展示节点不取消已完成运行。

### D. 权限与原有功能

1. 撤销 canvas.write 后，读取成功结果仍可用，新的保存或待审批写入必须失败。
2. 用户 B 不能读取 A 的 runId，不能将 A 的结果保存到自己的画布，也不能写入 A 的画布。
3. 插件停用后，历史结果仍可读取，新调用被拒绝；源视频仍受成功 PluginRun 的引用保护。
4. 检查已有文本/视频节点、标题编辑、连线、画布保存/刷新，以及 Agent 原有节点写入审批。
5. 公共分享页不会自动公开私有 PluginRun。未登录或无结果权限时应显示读取失败，不泄露结果。

## 5. 自动验证记录

已通过专项验证：SQLite/PostgreSQL 实际迁移、安装/启用/快照/投影、并发审批去重、服务重建后恢复、修改/删除冲突、另建绑定、过期审批、撤权、跨用户访问、伪造 digest、实际 Gin 路由，以及 Agent 通用 describe/invoke/result_read 路径。前端验证覆盖声明式字段渲染、转义、包合同、节点注册及原画布同步。

绑定插入故障测试确认：画布节点、历史快照、运行完成与绑定记录一起回滚；故障解除后同一审批可重试。并发检查通过。前端 8 个文件共 144 项测试通过；生产构建（含 TypeScript）通过，保留既有大 chunk 提示。相关 Go vet 与 OpenAPI YAML 检查通过。

完整回归首次发现 Token 定价准入测试的固定余额不足以覆盖新增能力描述；仅将该测试账号的余额补足至其已声明的一积分预算，未修改生产报价、预算或扣费逻辑。节点数量及创建菜单的测试期望随新增标准节点同步更新。

最终完整应用回归 `go test ./internal/app -count=1 -timeout=15m` 通过（408.236 秒）；plugins/contracts、database、handler、canvas/capability、repository 全套回归通过。最后的自动布局与投影回执调整另经 P02/P03 app/plugins/handler 专项验证通过。阶段结束复查 main 仍为 `308cf424`，无新增同步差异。

未启动开发服务进行浏览器自动操作，页面交互由用户按本清单验收；不将组件渲染和构建当作浏览器验收。

复验命令：

```sh
# backend；PostgreSQL 用独立测试库的 CANVAS_TEST_POSTGRES_DSN
go test ./internal/app ./internal/handler ./internal/database -run '^TestP03' -count=1
go test ./internal/plugins/... ./internal/canvas/... ./internal/repository -count=1
# web
bun test test/plugin-result-view.test.tsx test/plugin-v3-contract.test.ts test/canvas-node-registry.test.ts test/agent-canvas-sync.test.ts
bun run build
```

源码样例位于 `backend/internal/plugins/contracts/testdata/resource-helper-p03/`。离线打包器返回的 `runtimeEnabled=false` 表示校验器不执行插件，不代表已安装服务的三个可信 Adapter 不可用。

P03 完成合并后暂停。确认通过请回复 **“P03 验收通过，继续 P04”**。
