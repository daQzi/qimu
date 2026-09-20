# P06：并行、批次授权与局部派生验收

状态：P05 用户已验收。P06 代码及自动化验证完成，在独立 worktree 实施后仅合入 `codex/codex-plugin`，等待用户验收；不合入 main、不推送、不部署、不迁移业务库。

## 1. 本次交付及边界

增加声明式 DAG、operation 的 when/foreach、持久批次状态、当前批次统一批准、局部派生重做、五层执行槽限制与诊断。Agent 仍通过统一 operation 工具启动流程、plugin_input_submit 提交输入；新增 plugin_run_derive 创建新运行。批准收费的工具不向模型开放。

保留 P05 顺序执行器及其未结束运行；有 when/foreach/非链式依赖的新流程使用批次执行器。派生运行统一使用批次执行器。不增加重作业队列，仍使用原 Task Worker、原钱包预留/结算/退款与原 HTTP 连接器。

本次“批次授权”是当前已展开、最多 8 个待批准远程任务的精确授权，不是整条未知流程的无限授权。后续输入尚未确定、资源变化、下一波任务或报价变化，需要再次核对。Host 草稿与画布写操作保留逐项批准。

供应商 BYOK 金额目前未知：UI 必须显示并让用户单独接受供应商独立计费。授权总额只约束平台服务费，不能承诺限制供应商账单。未配置的平台服务费沿用 P04 明确的零服务费策略，不代表供应商免费；缺失/异常报价字段拒绝批次批准。

## 2. 插件作者接入

源包：`backend/internal/plugins/contracts/testdata/batch-helper-p06/`。
安装包：`plugin-packages/application-examples/batch-helper-1.0.0.yingce-plugin`。

```json
{
  "key": "remote",
  "type": "operation",
  "operation": "batch-helper.echo",
  "dependsOn": ["choose"],
  "foreach": {"from":"steps/choose#/items","itemKey":"/id","maxConcurrency":2},
  "when": {"equals":[{"from":"item#/enabled"},{"literal":true}]},
  "inputs": {"prompt":{"from":"item#/prompt"}}
}
```

`when` 支持 exists 引用或两个 binding 的 equals。不存在的 exists 为 false；equals/inputs 缺失引用拒绝执行，不猜值。item# 只允许在 foreach 项内使用。所有步骤结果引用必须声明直接 dependsOn；DAG 不要求数组本身拓扑排序。

itemKey 是每项内的 JSON Pointer，结果必须是唯一、非空、最多 120 UTF-8 字节的字符串。重排或修改一项内容时保持其 ID 不变；不能用完成顺序生成 ID。每步展开前完整校验，重复 key 不创建该步的任何远程任务。单步最多 256 项、整条批次流程累计最多 256 个实例（包括输入步骤）；样例表单限 250 项。

foreach 输出是按输入顺序排列的结果数组；条件为 false 的位置为 null，空集合输出空数组。单次输入/最终结果限制 64KB，累计输出 256KB，批次状态 1MB；大媒体使用资源引用。步骤数仍限 64；跨包、嵌套 pipeline、自定义脚本与 P07 输入 view 尚未开放。

## 3. 授权、并发与恢复

报价摘要绑定每个子 Run 的 operation/release/contractHash、完整请求摘要（包括模型配置）、连接版本、资源摘要、revision、approvalId、服务费。批准同时校验数量、额度与最多五分钟有效期。逐项执行原 Decide/Enqueue，但在同一目录事务中；任何一项余额不足、Agent 累计预算不足或报价过期/变化，Task、订单、钱包扣减和授权回执全部回滚。相同回执重放不增加任务或重复扣费。

并发限制同时作用于宿主、用户、连接、模型作用域和插件；单项实际受各层最小余量限制。宿主取现有 Worker 并发和 8 的较小值，用户默认 4，连接取现有渠道并发和 2 的较小值，模型默认 2，插件默认 4。可用下列环境变量进一步降低，值必须为 1–8，无效值报错：

```text
CANVAS_PLUGIN_HOST_CONCURRENCY
CANVAS_PLUGIN_USER_CONCURRENCY
CANVAS_PLUGIN_CONNECTION_CONCURRENCY
CANVAS_PLUGIN_MODEL_CONCURRENCY
CANVAS_PLUGIN_PLUGIN_CONCURRENCY
```

通用 HTTP 连接器没有已验证的供应商模型目录，因此当前同一连接的动作共享保守的模型槽，不依靠可任意填写的 input.model 分桶。此阶段不宣称按供应商真实模型 ID 跨连接识别限流；后续有可信模型映射后再细分。完整请求摘要绑定全部模型参数字段。foreach.maxConcurrency 另限制该步骤未结束子任务数，单流程最多 8 个未结束子任务。

这些变量是后端进程环境变量；Docker 部署需显式传入 backend 容器，仅写入 Compose 插值用的 .env 不会自动转交给进程。

执行槽在数据库中按目录事务原子获取，有效性跟随真实 Task 的 owner/lease expiry；Task 心跳续期同步延续槽有效期。每次提交、查询、导入或取消处理完成就释放槽，等待下一次轮询不占槽。旧 Worker 不能释放新 owner 的槽；重启后的过期槽不计入并发。

传输恢复、幂等查询和导入重试复用原 Task、submissionKey、attempt；用户重做创建新的派生 Run/子任务及 attempt。非幂等提交结果未知时，只查询/人工核实，不能自动重发。父流程停止或有子任务失败时先停止新准入、取消其他未结束子任务，收束后进入 cancelled/failed，不遗弃钱包或资源处理。权限撤回后继续跟踪已准入任务，但不展开下一批。

## 4. 派生运行

`parentRunId` 保持 P05 的“编排父运行”含义；新增 `derivedFromRunId` 专门记录“基于哪个历史运行重做”，避免历史查询与 Agent 恢复混淆。根运行历史仍能列出派生运行。

仅允许从已结束流程派生；未决子任务或取消后外部仍可能执行的任务会被拒绝，不能绕过未知提交保护。input 可替换入口，inputs 按 wait_input 步骤键覆盖表单；未覆盖的已提交表单继承，强制重做的表单需重新提供。forceSteps 计算传递依赖闭包。

默认不复用远程生成结果。用户明确勾选 reuseCompleted 后，以步骤 key + itemKey 匹配历史成功项，并比较规范化输入、固定发布/合同和重新准备得到的连接/资源摘要；强制重做闭包内不复用。输出需再次通过 Schema，资源引用复制到派生 Run，历史成功运行不被修改。更换一条映射会重做该项及结果发生变化的下游；无关项可以复用。画布/草稿写操作不缓存复用。

Agent 的 plugin_run_derive 默认不复用远程随机结果，要求有效的当前 Agent revision 与非只读权限，新运行绑定当前 Agent 预算。用户在 UI 选择明确复用时走同一派生领域接口，继续继承原 Agent 的累计预算约束。

## 5. API、数据和代码位置

| 接口 | 行为 |
| --- | --- |
| GET /api/plugin-runs/:id/batch-quote | 当前远程待批清单、总额、外部计费标记、摘要与有效期 |
| POST /api/plugin-runs/:id/batch-approval | digest/count/amountMicrocredits/expiresAt/acceptExternalBilling，整批原子批准 |
| POST /api/plugin-runs/:id/derive | input/inputs/forceSteps/reuseCompleted；Idempotency-Key 必需 |
| GET /api/plugin-batch-diagnostics | 当前账号 queued/activeSlots/unknown/importPending/waitingApproval/pausedPipelines/approvalConflicts/budgetConflicts |

GET Run 增加 batch.steps/items/done/reusedFrom、stepRuns、approvals、derivedFromRunId/attempt；输入卡片可以与其他正在执行的独立步骤并存。批量批准回执、运行事件和任务均持久化，SSE 继续沿用 P05。

schema 35 增量添加：plugin_pipeline_executions.batch_json、plugin_runs.derived_from_run_id/attempt、plugin_batch_approvals、plugin_execution_slots、plugin_batch_metrics。保留历史 migration 校验和和 P05 cursor/childRunId。PluginRunStep 仍是每个子 Run 的 invoke 记录，批次实例单独保存在 BatchJSON，避免全 Run 状态更新覆盖实例状态。

同时将 plugin_remote_executions.cancel_status 扩为 64 字符。故障注入发现 P04 的 unsupported_external_may_continue 超出原 PostgreSQL 32 字符字段，SQLite 无长度限制因而未暴露；此次通过增量迁移修复，避免取消事务失败。

全量检查同时补齐 `cmd/migrate-sqlite-postgres` 中 P01–P06 的插件表清单；原工具会因覆盖检查拒绝含插件表的数据库迁移。表覆盖与复合主键复制测试验证该工具仍先检查再迁移，不跳过未知表、不静默漏数据。本阶段没有运行真实业务数据库搬迁。

领域实现：plugins/pipeline_batch.go、pipeline_authorization.go、pipeline_derive.go；组合根：app/plugin_batch.go；持久化：repository/plugin_batch.go、plugin_pipeline.go；界面：PluginBatchControls + PluginRunCard。业务库升级前备份数据库、插件包、上传资源和加密密钥。回退应先阻止新调用、处置在途任务，再恢复匹配版本的备份；不能仅运行旧二进制读取新批次状态。

## 6. 用户手工验收步骤

1. 在测试环境更新集成分支、备份后按项目原方式迁移到 schema 35。安装 batch-helper 验收包，用户启用并授权 media.read/connection.use；按 P04 验收文档配置本地演示连接。example.invalid 不是可用服务地址。仍不需要真实收费模型。
2. 画布上传一段视频，引用 workflow 技能，让 Agent 调用 batch-helper.process，或在插件操作调试页传真实 resourceId。确认出现 choose 输入卡片。
3. 按以下内容提交：

```json
{"items":[{"id":"b","prompt":"第二项","enabled":true},{"id":"a","prompt":"第一项","enabled":true},{"id":"skip","prompt":"跳过","enabled":false}]}
```

4. 确认提交表单后尚未发远程请求；点击“核对当前批次费用”，检查两个真实子任务、服务费、有效期和供应商未知计费提示。未接受供应商独立计费时批准按钮不可用。批准后检查完成两项，结果顺序 b/a/null；刷新或关闭页面后从运行历史继续查看。
5. 在已结束卡片选择“基于此次运行重新执行”，只修改 a 的 prompt，可以交换 a/b 顺序但保留 ID，明确勾选复用。创建并批准新运行，确认只有 a 新增远程调用、b 显示已复用；旧成功结果与运行 ID 不变。再次不勾选复用重做，确认两项都重新执行。

演示服务的 /echo 返回实际 prompt，方便核对顺序；带演示凭据读取 GET /stats 的 echoRequests 可核对请求次数（首次 +2，修改一项并复用 +1，再不复用 +2）。原 uniqueSubmissions 仍只统计异步 /jobs。
6. 用三个以上启用项验证两项一波、下一波需核对；其中一次停止流程，确认不再启动后续项，在途子任务收束。重复 ID 的输入应该失败，不发送该批远程请求；不存在字段不可被默认补齐。
7. 测试环境设置服务费与不足余额，批准一批两项时确认整批不产生 Task/订单、不部分扣费。充值测试积分后刷新报价再批准；重复点击/重放不重复预留。访问诊断接口检查预算冲突、未知提交、导入积压与活动槽。
8. 验证亮/暗主题、窄屏、JSON 编辑滚动、多个子卡片、刷新、账号切换；复验 P05 顺序流程、原画布节点与 Agent 审批。此项需真实浏览器人工验收，不能用静态组件渲染代替。

## 7. 自动化记录

实施期间执行真实 HTTP 的 SQLite/PostgreSQL 生命周期、双调度器/双 Worker 槽、过期租约、整批预留/重复结算、未知提交、单项失败收束、撤权、资源引用及迁移测试；前端执行合同与真实组件渲染专项、类型检查和生产构建。最终结果以交付消息和本节完成记录为准。

完成记录：

- `go test -timeout 20m ./...` 全部通过，开启隔离 PostgreSQL 18 与 Redis 7；其中 app 用时 727 秒。此前默认 10 分钟全量超时，扩大测试时限后通过，未修改业务超时或跳过原用例。
- `go test -race -timeout 15m ./internal/app ./internal/plugins -run 'TestP06ScopedSlots|TestP06ConcurrentScheduler|TestP06Agent|TestP06Expansion|TestP06Dependency' -count=1` 通过（app 311 秒），覆盖 SQLite/PostgreSQL 双调度器、作用域并发、过期 owner 与 Agent 检查点。
- 后续跨账号 404、未知费用保守处理、展开大小、保守模型作用域，以及父流程取消后拒绝迟到子审批分别补充专项复验通过；P05 顺序流程和原子预算回归通过。迁移专项将 PostgreSQL 取消字段实际降至旧 32 字符，再执行 v35，验证完整取消状态可写且历史记录不变。
- 前端插件专项 109 项通过；生产构建含 TypeScript 检查通过。构建仍有既有 chunk 体积提示；未做无关拆包。`git diff --check` 通过。
- 全量发现的 SQLite→PostgreSQL 插件表清单缺失已修复并复验；中间旧代码的 race 运行因已修正字段而停止，以上完成记录采用最终专项，不将中止运行计为通过。
- 开始前、交付前同步 fork；最终 main/origin/main 为 `9afe061d`。初始同步的 Dockerfile/待测记录冲突保留双方有效内容；仅功能集成分支接收 P06。

不启动业务开发服务器，不使用实际视频供应商/收费模型，不修改生产数据；本地 Mock 仅验证连接器与执行机制，不证明视频生成效果。验收通过后再进入 P07。
