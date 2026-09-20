# 插件体系收尾 P14A：开发与交付验收

## 上游 PR 整合记录

用户确认向上游提交统一 PR 后，在独立 worktree 合入 upstream/main `0ea9f7d9`，保留模型选择强校验、任务/DNS 诊断、渠道批量调价及 Agent 入口拖动。HTTP 文档与待测试文档冲突保留双方内容；新增 `fileURLToPath` 修正拖动测试在中文目录下的路径解码，不修改产品行为。此前“不推送”是阶段交付边界，此次按用户新授权推送功能分支并创建 Draft PR，不合入上游 main。

同步后验证：后端模型选择/DNS/任务诊断、会话与 HTTP、schema 36、新插件及无画布 Agent 专项通过，双库用例配置隔离 PostgreSQL；出站请求、插件合同、作者工具与 CLI 测试通过。前端插件/会话及相邻用例 143 项通过，拖动专项单独运行 7 项通过，生产构建含类型检查通过。混跑拖动构建测试和面板测试出现模块解析问题，最终采用独立进程分组；不声称单进程全量测试通过。未运行生产迁移或收费模型，页面待验收状态不变。

本阶段提前完成 P14 中不依赖工作台的开发者交付部分。P00–P07 已由用户验收；P08 与本次收尾的页面验收仍需用户确认。工作台 P09–P10、一键出海 P11–P13 暂不推进；完整 P14 的独立作者、灰度部署和工作台组合演练未宣布完成。

实施分支：`codex/plugin-closeout`，从包含 P00–P08 的 `codex/codex-plugin` 创建独立 worktree。阶段开始已同步 fork origin/main；交付前再次同步。仅合入插件集成分支，不合 main、不推送、不部署、不迁移业务库。

## 1. 本次完成范围

| 交付 | 内容 |
| --- | --- |
| 作者工具 | 新增 skill/resource 模板创建、成品 ZIP 校验；保留目录校验、打包和版本覆盖 |
| 离线领域边界 | authoring 负责文件/模板/交付，复用 contracts 与真实 installer envelope；CLI 只处理参数和输出 |
| 包约束 | 有界读取、拒绝链接和越界包、独占创建不覆盖、稳定打包、打包后安装合同复验 |
| 新接入案例 | delivery-check：真实视频元数据 → 用户批准保存 → 用户批准画布投影；不注册特例工具或修改 Agent 主循环 |
| 生命周期验证 | 技能文件可见、未授权拒绝、真实 Adapter、重建服务不重复节点、新发布不强制升级、升级不改历史、停用保留结果、账号隔离 |
| 开发文档 | 接入指南按当前合同重写，包含远程协议限制、真实 HTTP 路由、升级/恢复及未开放边界 |

不新增执行器、业务 API、前端页面、数据库结构或运行配置。沿用 P08 的 schema 36。

## 2. 使用方需要验证什么

在已具备 P08 前后端及匹配数据库的隔离环境验收。不要为此次作者工具验证直接迁移生产数据库。

1. **安装包**：管理员在 /admin/plugins 导入 delivery-check 1.0.0；用户在 /plugins 选择版本、启用并授予四项权限。确认“我的技能”出现 check-source，内容引用 delivery-check 操作。
2. **Agent 使用**：在画布上传一段就绪视频，引用该视频并明确选择技能，输入“检查已有的视频信息，经我确认后保存到画布”。只应展示真实已有元数据，不应声称识别人物或生成新视频。
3. **两次写入确认**：保存结果、放到画布均先显示批准。批准前无节点；批准后出现结果节点，未知时长/尺寸不编造。
4. **恢复与重试**：刷新页面后从运行历史找到结果；重复相同投影不重复节点。移动或修改节点后遇到冲突，应保留用户编辑并提示刷新/另建，不能重跑原分析。
5. **升级**：导入 1.1.0 后用户仍停留在 1.0.0；手动切换后旧结果继续可读。重复安装同包不新增发布，同版本不同内容应被拒绝。
6. **停用与隔离**：停用插件后新调用失败；历史卡片与节点仍保留。切换另一账号，不应能读取原账号运行或素材。
7. **P08 补充验收**：若尚未确认，按 [P08 验收](qimu-plugin-p08-acceptance.md)验证首页→画布→首页的会话继续、账号切换及刷新恢复。

Agent 流程可能使用模型并产生现有模型费用；作者 CLI 和资源模板操作本身不调用远程模型。本轮自动测试使用真实宿主和隔离数据库，不调用付费供应商；浏览器与真实 LLM 行为需手工验收。

## 3. 作者从零接入

请用户或未参与实现的开发者只按[当前接入指南](../content/docs/plugins/application-plugin-v3-integration.md)操作，另起插件 ID（如 team-delivery-check）：

1. 用 -init 创建模板，修改名称、技能方法与视图标签。
2. 用 -dir 校验、-out 打包、-package 校验成品；故意重复输出应报错且旧包仍可用。
3. 不改启幕代码，完成上节安装、授权、Agent 与画布流程。
4. 用 -version 1.1.0 -out 新路径生成升级包，完成版本与历史验收。

resource 模板源代码纳入仓库；打包文件为本地验收产物，不提交构建产物。高级案例沿用 contracts/testdata 下的 remote-helper-p04、pipeline-helper-p05、batch-helper-p06、canvas-helper-p07，不能将演示 API 视为真实媒体供应商。

## 4. 自动验证

验证命令在 backend 执行。PostgreSQL/Redis 必须指向独立测试服务，测试自动创建隔离 schema：

```sh
go test ./cmd/plugin-contract ./internal/plugins/authoring -count=1
go test ./internal/plugins/contracts ./internal/protocol -count=1
go test -race ./cmd/plugin-contract ./internal/plugins/... ./internal/protocol -run 'Test(Templates|Authoring|CLI|Application)' -count=1
go test -race ./internal/app -run '^TestPluginAuthoringDeliveryLifecycle$' -count=1 -v
go test -race ./internal/app -run '^TestP06AtomicCreditReservation$' -count=1 -v
go test ./internal/app ./internal/handler -run '^Test(P03ObservedDigestAndRevocation|P03ProjectionHTTP|P04SafeRecoveryAndExpiredImport|P04ConnectionVersionQuoteAndIsolation|P04RemoteHTTP|P05RoutesBackgroundWorkerAndReplay|P06RevocationStopsNewWave|P07InputProjectionStrongBoundaries)$' -count=1 -v
```

后端跨库回归需设置 CANVAS_TEST_POSTGRES_DSN 与 CANVAS_TEST_REDIS_URL；未配置时 PostgreSQL 用例会跳过，不能算双库通过。

本次实际记录：

- 作者模板、CLI、合同与 ZIP 协议测试通过；新模板创建、1.0.0/1.1.0 打包与成品包校验实跑通过。
- 新插件安装→授权→技能读取→真实资源快照→画布→重建服务恢复→升级→停用链路，在 SQLite 和 PostgreSQL 均通过 race 验证（两种数据库未跳过）。
- 撤权停止新批次、输入投影边界、摘要校验、远程查询/导入恢复、连接版本/隔离和 P03/P04/P05 HTTP 路径专项通过；涉及 PostgreSQL 的用例使用隔离 Redis。
- 首轮 P03–P07 宽范围 race 回归在 app 包累计触发 10 分钟超时，当时运行到 P06AtomicCreditReservation/postgres；该整轮不记为通过。采样显示既有 fixture 重复初始化服务并解压内置协议包，开销较高。作者/领域/协议及 handler 包在该轮已通过；改为按以上命令分组验证，账务用例单独记录。
- 接入指南与收尾文档的相对链接、Git 空白错误和 Go 格式检查通过。
- 超时时所在的 P06AtomicCreditReservation 已单独在 SQLite/PostgreSQL 通过 race 验证，确认预算不足不部分扣费、重复批准不重复结算。宽范围整轮仍保留超时记录，不将分组通过写成全量通过。

交付前再次 fetch fork 并同步 main，仍为 `9afe061d`，合入阶段分支无冲突。只使用本轮独立 PostgreSQL/Redis 测试容器；测试库、日志和示例成品不进入提交。生成的验收包内容摘要：1.0.0 为 `499ff42ea38a04806cc6b4613149a11c4ff58a65f854e35ed5c827411efa6dc0`，1.1.0 为 `46b6a95c3ff2906799d7e78cd6766ae9f2fae620c2f1d928d178615340db003f`。

此次不修改 web，不重复运行前端构建；文档目录当前无独立 package.json，文档通过链接与合同对照检查，不能声称文档站构建通过。

## 5. 受控发布与恢复清单

以下是未来发布时要执行的清单，本轮未执行生产部署：

1. **准备**：记录主线、插件集成版本、包摘要、依赖和用户授权；备份数据库、插件包目录、资源存储、.settings-key，检查备份可恢复。明文凭据不得出现在验收记录。
2. **隔离演练**：按项目 migrate-schema 工具执行 up/verify，确认数据库及当前 Worker 认识现有 PluginRun/Task；验证新包安装、撤权、在途运行恢复和账务。不要直接混跑不认识新任务的旧 Worker。
3. **小范围启用**：管理员安装样例，先让指定测试用户自行启用授权，再逐步扩大。当前不是自动白名单或公开分发系统。
4. **观察**：使用插件运行历史、任务中心、当前用户批次诊断、脱敏后台日志与管理员审计，检查 waiting/paused、导入积压、重复结算及权限拒绝。当前无插件专属诊断 ZIP。
5. **问题隔离**：用户停用、管理员 availability=false 或撤回问题发布，阻止新使用；全局准入开关 CANVAS_APPLICATION_PLUGINS_ENABLED=false 可用于停止安装/启用和新操作。保留在途任务处理能力，不将开关当作远程取消回执。
6. **恢复原任务**：核实供应商状态，按 Run 允许的动作恢复查询、绑定 jobId 或重试导入。提交不明不盲目重发；取消不支持时说明供应商仍可能继续扣费。
7. **恢复版本**：未撤回且依赖有效的旧发布可由用户明确切回；新权限重新确认。逻辑卸载保留历史和资源引用，孤立包先 preview 再另行决定 prune；prune 是不可恢复删除，不用于日常回退。
8. **整站回退**：先排空或明确处置在途任务，再使用匹配的二进制、数据库和密钥备份恢复。不能仅切 Git 分支就视作数据库回退；本次 authoring 代码自身无迁移。

## 6. 完成矩阵与停止点

| 范围 | 状态 / 后续 |
| --- | --- |
| 安装、技能、权限、统一操作、结果与画布 | P00–P03 已验收 |
| 远程连接、可靠 Task、持久流程、批次及交互 | P04–P07 已验收 |
| 首页与画布持久会话 | P08 代码已交付，保留独立手工验收 |
| 作者模板、打包校验、当前文档与接入案例 | 本次 P14A 收尾，待用户验收 |
| 通用工作台、Recipe、业务对象 | P09–P10 后置，不阻塞受控插件开发 |
| 一键出海与真实视频服务 | P11–P13 后置，独立验证协议、额度及质量 |
| 独立作者人工验证、生产灰度、完整工作台组合 | P14 剩余项，不能标记已完成 |
| 社区市场、签名审核、机器身份、Webhook、任意代码隔离 | 单独规划，未开放 |

通过本次验收后，可将“受控应用插件底座 + 开发交付工具”作为当前插件改造的停止点。后续应用优先独立开发插件；只有超出已开放合同的能力才修改底座。完成本阶段后暂停，不自动进入工作台或出海开发。
