# P09：通用工作台、Recipe 与动态输入栏

状态：用户已确认 P09 验收通过，授权继续 P10。用户授权从插件交付版本继续工作台阶段；不替代此前 P08/P14A 尚未记录的手工验收。

## 1. 本阶段交付

- 应用包可以贡献 `workbenches`、`recipes`，最低 `requires.hostApi: ^3.1.0`。原 `^3.0.0` 包仍按原能力使用。
- 一个通用路由 `/workbenches/:id?`，从应用插件中心、首页/画布 Agent 的“插件工作台”进入。没有针对品牌、会议或素材的独立 Agent 执行分支。
- Workbench 绑定本包 Operation 和/或 Skill；有 Operation 时直接使用该 Operation 的输入 Schema。纯 Skill 工作台用本包上下文 Schema，不制造空操作。
- 配方提供默认值、字段常量要求和方法提示；后端合并、解释冲突、校验输入，再生成固定选择。不会自动选择模型、改权限、改预算或调用外部服务。
- 通用表单支持文字、数字、枚举、布尔和结构化 JSON；支持字段标题、说明、必填标记及恢复建议值。复杂嵌套对象、本地 `$ref` 字段使用 JSON 输入，校验仍由真实 Schema 执行。
- 可以直接确认执行已声明操作，或把固定输入交给现有 Agent 会话。运行卡片、审批、费用、资源与画布投影沿用既有能力。
- 本机草稿按账号、工作台、发布、画布隔离，编辑自动保存，也可手动保存。显式提交前先保存完整请求和幂等键；网络结果不明时只允许原样重试。
- 切换工作台不携带旧会话和草稿；已有会话不可切换工作台 ID。相同工作台可以沿用原首页/画布会话，在新入口重新预览本轮输入。
- 原会话继续会恢复记录的工作台选择；停用、撤权或升级后不静默换版本。历史记录仍按原选择展示。

## 2. 三个独立示例

源码在 `examples/plugins/`，不自动安装、不自动授权。

| 包 | 输入 | 执行路径 |
| --- | --- | --- |
| `resource-workbench` | 当前账号已上传视频的 resourceId | 真实 Host 读取；也可交给检查技能指导后续保存/上画布 |
| `brand-workbench` | 品牌、受众、语气、渠道 | Markdown Skill + 用户选择的 Agent 文本模型；社交/专业/详情页配方 |
| `brief-workbench` | 会议记录、输出格式、是否列风险 | 同一个 Shell + Agent；仅新增包配置实现第三种业务 |

仓库根目录下打包（目标文件必须尚不存在）：

```sh
mkdir -p .local/workstudio-p09-packages
cd backend
go run ./cmd/plugin-contract -dir ../examples/plugins/resource-workbench -out ../.local/workstudio-p09-packages/resource-workbench-1.0.0.yingce-plugin
go run ./cmd/plugin-contract -dir ../examples/plugins/brand-workbench -out ../.local/workstudio-p09-packages/brand-workbench-1.0.0.yingce-plugin
go run ./cmd/plugin-contract -dir ../examples/plugins/brief-workbench -out ../.local/workstudio-p09-packages/brief-workbench-1.0.0.yingce-plugin
```

从管理员原插件上传入口安装，用户到“应用插件”选择版本并启用。素材包申请既有媒体/资源/画布权限；文案与会议包无需插件操作权限，但使用 Agent 模型仍按原计费和权限规则。

## 3. 用户验收步骤

1. 启动本阶段分支对应的前后端，按上节安装并启用三个包。在“应用插件 → 打开插件工作台”看到三个不同入口。
2. 打开素材检查，填真实视频 resourceId，预览、执行，确认返回真实时长/尺寸等元数据。缺少字段或其他账号资源应失败；直接读取不创建收费生成任务。
3. 打开品牌文案，填品牌和受众。选择“社交种草”，预览最终输入；再叠加“专业表达”，未手改语气时应显示冲突。显式选择语气后可继续，不能静默采用配方点击顺序。
4. 选择“商品详情页”，手改渠道为“社交媒体”，预览应提示违反要求。恢复建议或取消配方后可继续。
5. 修改字段后切换会议简报，再回品牌文案；本机草稿应保留且两个工作台互不串数据。刷新页面重复检查。切换账号、画布、插件版本后不加载另一个范围的草稿。
6. 品牌文案预览有效后点击“交给 Agent”，在下方新建云端会话，自己选择文本模型并发送目标。检查技能引用和文案结果；修改上方草稿不改写正在执行的输入。
7. 从会议简报进入同样的协作流程，确认无需新增页面或执行器。纯 Markdown Skill 在原技能库中仍可独立使用。
8. 从画布 Agent 打开工作台，确认 URL 带当前画布，插件不会自动获得写入权。模型/插件提出保存或投影时，仍须通过原确认流程；未批准不产生节点。
9. 已有工作台会话从首页或画布历史继续，确认仍携带原工作台选择。同一 Thread 改用另一个工作台应被拒绝并提示新建；切换入口不改写过去的记录。
10. 在有写操作的自定义工作台提交时断网/刷新，检查“原样重试待确认请求”；重试使用原请求编号，不能因改模型或表单重复收费。升级/停用插件后旧运行仍可查看，但旧入口不能静默切到新发布执行。
11. 检查明暗主题、窄屏、滚动、中文输入、键盘焦点、JSON 错误提示、原 Agent/插件/画布路径。

第 6–9 步的真实模型输出、浏览器交互待用户验收；本阶段没有调用真实付费模型，也未自动启动预览服务器。

## 4. 验证与实现边界

自动验证覆盖：Go/TypeScript 共同接受三个包和拒绝非法引用/默认值/旧宿主版本；确定性配方冲突、手改保留；SQLite/PostgreSQL 的安装、启用、预览、真实资源调用、写入审批、幂等、撤权与快照保留；Agent Thread 固定技能/选择、重复提交和切换隔离；HTTP 登录/严格请求校验；真实表单服务端渲染、草稿隔离；前端生产构建。具体运行结果随交付记录更新，不能据此宣称浏览器或真实供应商验收通过。

本阶段实际验证结果：

- `go test ./internal/plugins/... -count=1`：通过，涵盖现有插件域、作者工具、合同回归。
- `go test ./internal/plugins/contracts ./internal/plugins/authoring ./internal/plugins ./internal/app ./internal/handler -run 'TestWorkbench|TestSharedContractCorpus|TestTemplates|TestPluginAuthoringDeliveryLifecycle|TestAgentThread' -count=1`：通过；已配置临时 PostgreSQL，执行双库工作台与作者生命周期，未连接业务数据库。
- `go test -race ./internal/app -run 'TestWorkbench|TestAgentThread' -count=1`：通过，包含新版安装/启用不篡改旧工作台运行、旧快照拒绝启动。另已通过工作台合同 race 专项。
- `bun test test/plugin-workbenches.test.tsx test/plugin-v3-contract.test.ts test/agent-thread-consumer.test.ts test/agent-thread-pending.test.ts`：92 项通过。
- `bun run build`：类型检查与生产构建通过；构建仍提示大体积 chunk，不代表性能验收已完成。
- 未启动 dev server，未执行浏览器端交互或真实模型调用。复杂 JSON 编辑与主题/焦点等保留给上节手工验收。

- 没有新表或数据库迁移，沿用 schema 36；选择存在原 PluginRun.RequestJSON、Agent 请求和 Thread 上下文中。
- 只读 inline 结果不创建持久 Run，刷新后可重新读取；持久写入才通过 Run 卡片展示历史工作台快照。
- 工作台支持本包 Skill/Operation/Recipe；跨包工作台组合、业务对象类型和市场审核后置。
- `requirements` 是输入字段常量约束，不是权限、预算或模型能力声明。建议文本不能提升授权等级。
- `output` 只声明交付意图；`canvas_optional` 不会凭空生成画布或自动写入。可写入内容仍由真实 Operation/Blueprint 及宿主规则决定。
- `Skill.launchOperation` 约束工作台启动操作，引用该技能不会自动执行；普通技能库保持原方法引用方式。
- 有工作台快照的 Pipeline 可以沿用原运行派生机制；更改顶层启动输入必须回工作台重新预览创建新运行，不能携带旧摘要派生不同输入。步骤表单仍按原 Schema/审批处理。
- 草稿存在当前账号的本机浏览器存储，不等于云端保存；结构化字段中尚未构成合法 JSON 的中间文本不保证跨页面恢复。
- 原样重试遇到版本或权限变化会明确失败；先从插件运行历史核对，不清空未确认请求后盲目重发。

## 5. 分支、回退与下一步

P09 使用 `codex/workstudio-p09` 独立 worktree，基础为包含 P00–P08 和插件收尾的 `02b46988`。新的集成分支为 `codex-workstudio`。开始前本地 main 对齐 fork origin/main；原本地 main 指针另存 `archive/main-before-workstudio`，不丢弃原提交。提交前再次同步 fork。

本阶段只合入 `codex-workstudio`，不合并 main，不自动更新插件 PR #550，不推送/部署。验收通过后才开始 P10。

回退优先停用新增示例或关闭工作台入口，保留历史 Run 查询和处置。不要直接用不认识 hostApi 3.1 / 工作台快照的旧后端接管在途任务；如需回退二进制，应先完成或取消相关运行并备份数据。
