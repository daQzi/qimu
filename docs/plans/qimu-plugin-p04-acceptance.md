# P04：远程 API、连接与可恢复单任务

用户已确认 P04 验收通过，后续进入 P05。以下验证记录为 P04 交付时的历史事实；真实供应商接入仍需独立验证。

> 状态：通用机制代码完成，自动验证通过，待用户页面验收；真实供应商仍待接入。P00–P03 已由用户验收通过；P05 尚未开始。
> 分支：`codex/plugin-p04-remote-tasks`，独立 worktree。开始同步 main `16bd4b92`，开发期间再次同步 `cb68476e`；完成后仅合入 `codex/codex-plugin`。

## 1. 本阶段能力

- 应用包可贡献 `connectors`，HTTP operation 使用已有 operation_search/describe/invoke、用户批准和 PluginRun，不增加特定供应商的 Agent 工具。
- 用户连接按账号、插件、connector 隔离；地址、凭据有不可变修订。凭据复用 `.settings-key` 的 AES-GCM 加密，仅展示是否已配置。更换目标主机/协议必须重新填写凭据，已批准任务继续使用原连接修订。
- 提交前保存唯一 Task、固定 release/contractHash、提交身份、输入资源引用和审批。发送前另行加密保存实际 JSON 正文，幂等恢复使用原键和原正文，包括已签发的资源地址。
- Worker 复用现有队列和租约；每次执行提交、查询或导入，轮询后释放槽位。状态写入检查当前租约并在事务中提交，旧执行者不能发布迟到结果。
- 无幂等/查询能力的提交不明进入 paused，不能通过普通任务重试绕过确认。异步任务可绑定核实后的真实上游 ID，继续查询；新生成必须重新创建 Run 并报价批准。
- 平台服务费由管理员配置，默认 0；非零费用在批准时由现有账务预留，成功保存结果后结算，确定失败或停止本地处理退回平台费。提交不明保留待核对预留。供应商使用用户 Key 独立计费，平台不声称供应商费用已退回。
- 插件平台服务费与本轮既有模型费用共同进入 Agent 预算，批准和后续 Agent 入队在锁内复核。
- 上游完成后响应加密保存；导入失败只查询原任务刷新结果地址并重试导入，不重发生成。导入采用稳定上传身份，输出及输入资源进入删除保护。
- 取消回执不等于取消确认：异步取消继续查询原任务。取消不支持或结果不明时，明确显示外部任务可能继续运行；用户停止后迟到结果不能把 Run 改回成功。
- 插件中心新增连接配置、管理员服务费、远程操作调试；运行卡支持报价确认、状态刷新、停止、恢复查询/导入和人工绑定上游 ID。任务中心可打开关联插件运行。

## 2. 当前连接合同

支持固定 JSON POST 提交、GET 轮询/按提交键查询、POST/DELETE 取消；鉴权支持 none、bearer、自定义鉴权头。请求字段仅为 literal、`input.<field>`、`resource.<field>`；资源映射必须在 resources 声明 kind，并验证当前用户归属及批准时的资源版本。

路径只能使用受控 `{jobId}` 或 `{submissionKey}`，不允许脚本、任意请求头、路径穿越或包内凭据。幂等头必须声明 retentionSeconds（60–86400）；超过窗口不自动重放提交。响应字段用 JSON Pointer；未知状态暂停核实。

边界：最多 8 个输入资源、1 个媒体产物；请求正文 ≤256KB、JSON 响应 ≤256KB、发布结果 ≤64KB；下载不超过当前资源策略及 64MB。每个用户连接跨修订/跨 Worker 限制每秒 1 次 API 请求。音视频导入需可用的 ffprobe，图片检查实际图片头及尺寸；不是仅信任远程 Content-Type。

初始 URL、实际拨号和下载重定向均走出站校验。鉴权 API 不跟随重定向；下载不带连接凭据，最多 3 次重定向，每次重新检查地址。原始响应、下载签名和提交正文只存密文；公开结果只保留 Schema 允许的字段和资源引用，拒绝凭据回显及未导入的 HTTP 地址。

尚未开放：OAuth、任意 multipart/流式协议、Webhook、Pipeline、多产物批量导入及在途连接修订切换。旧凭据被供应商撤销时不会自动改用新凭据或地址。更高层的输入表单和多步骤流程在后续阶段实现。

## 3. 数据库与 main 同步

最终结构版本 **33**。main 本次新增 Agent 日志、消息、任务诊断与资源租约，和插件已验收的 v28–v30 编号发生交叉；按已保存的名称与校验和识别路径，保留既有记录及 AppliedAt。未知校验和继续拒绝。

| 版本 | 主线/新数据库 | 已验收插件 P02/P03 数据库 | 原 P01 v27 插件数据库 |
| --- | --- | --- | --- |
| 27 | 积分成本 | 保留积分成本 | 保留插件发布目录 |
| 28 | Agent 日志/消息 | 保留插件发布目录 | 保留补入的积分成本 |
| 29 | Agent 资源租约 | 保留插件单步运行 | 保留插件单步运行 |
| 30 | 插件发布目录 | 保留插件画布绑定 | 插件画布绑定 |
| 31 | 插件单步运行 | 补 Agent 日志/消息 | 补 Agent 日志/消息 |
| 32 | 插件画布绑定 | 补 Agent 资源租约 | 补 Agent 资源租约 |
| 33 | 远程连接、任务和引用 | 同左 | 同左 |

新增 `plugin_connections`、`plugin_connection_versions`、`plugin_connection_rates`、`plugin_operation_prices`、`plugin_remote_executions`、`plugin_run_resources`；Run/Task 双向关联为唯一身份。Task 新增 paused（不自动领取）；已有任务和结果不伪造连接或远程状态。

验收前备份数据库、插件包目录和 `.settings-key`，用本分支迁移工具执行 `migrate-schema up` 与 `verify`。本阶段只迁移隔离测试库。旧二进制不识别新任务时不能直接降级：先停止新提交、排空或明确处理已有任务，再使用匹配备份和二进制回退。

## 4. 无收费模型的页面验收

样例包：[remote-helper-1.0.0.yingce-plugin](../../plugin-packages/application-examples/remote-helper-1.0.0.yingce-plugin)。它是协议接入样例，不包含真实供应商、密钥或视频分析模型。

1. 使用 `codex/codex-plugin`，前后端版本一致，schema 33。新开一轮 Agent 对话，避免复用旧能力合同的在途检查点。
2. 可在隔离开发环境启动本地演示服务（本次未自动启动）：

   ```sh
   cd backend
   go run ./cmd/plugin-remote-demo -listen 127.0.0.1:18081
   ```

   它只创建带明确测试说明的固定图片和 JSON，不调用模型、不收费；进程退出会丢失其内存中的演示作业。后端需精确设置 `CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=127.0.0.1` 才能连接此本地测试地址。不要开启全部私网访问。容器部署应使用后端实际可访问的测试服务地址并精确放行该主机。

3. 管理员在 `/admin/plugins` 导入样例。用户在 `/plugins` 启用 remote-helper，并授予 connection.use/resource.create。
4. 打开“远程连接 api”，填写 `http://127.0.0.1:18081` 和公开测试凭据 `demo-key`，保存后只看到“凭据已配置”。刷新、切换账号验证不会回显或串用凭据。
5. 在 echo 调试栏填 `{"prompt":"hello"}` 创建运行，确认前不发送网络请求。点击“确认发送并执行”，应得到明确的模拟 JSON 结果。
6. preview 使用同样参数。批准后看到提交、查询、导入状态，完成后获得真实保存的测试图片资源引用。刷新后可用 runId 在插件中心或任务中心重新打开运行。
7. preview 参数用 `{"prompt":"disconnect"}`：演示服务先创建作业再断开提交连接，系统应按原提交键找回作业并完成，不能多生成一个作业。
8. 可查看演示服务的真实唯一作业计数：

   ```sh
   curl -H 'Authorization: Bearer demo-key' http://127.0.0.1:18081/stats
   ```

9. 对尚未结束的 preview 点击“停止此任务”。首次取消只回执收到，系统继续查询至确认；若任务抢先完成，则如实显示外部已完成、本地停止后续处理。
10. 管理员给 echo 配置小额平台服务费，用户创建报价后修改价格，旧批准应被拒绝；重新发起并批准后检查积分预留、成功结算以及重复批准不重复扣费。
11. 在途任务启动后修改连接配置，已有任务仍使用原修订。新建运行才使用新配置；改变目标而留空凭据必须被拒绝。
12. 画布 Agent 选择 remote-task 技能，按提示创建、确认并查询远程运行；只读模式不能调用远程写操作，Agent 不能自行批准。

提交不明且供应商无幂等/查询能力、取消失败、过期下载链接、失效租约、迟到成功和预算不足由自动故障测试覆盖。真实供应商的价格、鉴权、幂等保留期、取消语义与结果期限尚未选定，因此真实供应商接入仍待验收，不能据本地模拟结果宣称深度捕捉或一键出海已完成。

## 5. 验证与阶段终点

已完成：真实 HTTP 生命周期与越权负例、SQLite/PostgreSQL + Redis 专项、P01/P03 历史路径迁移、外部断线与查询恢复、过期产物仅导入重试、取消回执与确认区分、迟到成功保护、租约失效保护、费用预留/结算、既有模型费用计入 Agent 预算、凭据加密与输出脱敏。

完整 app 回归通过（250.984 秒）；database、handler、plugins/contracts、repository、protocol、canvas/capability/contract 全套回归通过；P04 race 检查通过（165.412 秒）。前端 11 个文件共 172 项测试通过，生产构建（含 TypeScript）通过；相关 Go vet 和 OpenAPI 检查通过。main 同步后的 P02/P03/P04 专项也通过。构建仍有既有大 chunk 提示。

浏览器页面尚未人工验收；组件渲染、接口测试和构建不能代替本节页面验收。没有迁移业务数据库、调用真实模型、推送或部署。

```sh
# backend，PostgreSQL/Redis 须使用隔离测试实例
go test ./internal/app ./internal/handler ./internal/database -run '^TestP04' -count=1
go test ./internal/plugins/... ./internal/repository ./internal/protocol ./internal/canvas/... -count=1
# web
bun test test/plugin-remote-contract.test.tsx test/plugin-v3-contract.test.ts test/plugin-result-view.test.tsx
bun run build
```

完成后暂停，确认通用机制通过请回复 **“P04 验收通过，继续 P05”**。真实供应商的小样联调在明确服务合同和费用范围后补做。
