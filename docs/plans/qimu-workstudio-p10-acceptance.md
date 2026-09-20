# P10：业务对象与跨插件复用

状态：代码完成，待用户验收。P09 已由用户确认通过。本阶段仅合入 `codex-workstudio`，不自动推送、部署、修改原插件 PR 或进入 P11。

## 1. 本阶段做了什么

- 新增“品牌资料”管理页 `/business-objects`，从插件工作台进入；可新建、编辑、检索、查看历史、基于旧版本另建、归档和恢复。
- 品牌是账号私有的文本资料，包含名称、受众、定位、已确认卖点、表达限制。新增两个数据库表，内容每次保存生成不可变版本。
- 插件引用 `objectId + version + type + schemaVersion`，多个插件可以使用同一个品牌版本，不复制素材。品牌修改不会改写旧版本或已提交 Agent 输入。
- 新增三个 Host Adapter：`object.read`、`object.search`、`object.save`。写操作复用原 Run/审批/幂等，不新增执行器。
- 工作台通过 `objectInputs` 声明品牌字段，通用表单显示品牌选择器；服务端解析真实资料、校验归属和授权，并冻结到 Agent 的模型请求。
- 两个独立示例：“品牌卖点整理”与“品牌广告脚本”，共用版本资料、页面和 Agent。前者另演示检索与经审批保存，后者仅申请读取。
- 浏览器在发送品牌保存前保留账号范围内的完整请求和幂等键；结果不明可刷新后原样重试。普通未提交表单不是云端草稿。

## 2. 为什么使用独立对象域

现有 `Asset/AssetVersion` 面向项目素材、媒体和可更新的 PayloadJSON，不能直接保证品牌的字段合同、账号归属、乐观并发和历史不可变性。P10 采用 `internal/objects` 管理品牌规则，通过既有权限/Host/Workbench/Agent 组合接入；没有开放通用 PayloadJSON 写口、插件 SQL 或自定义表。

此轮品牌 v1 不含媒体引用：不创建、复制或删除媒体。图片、Logo、人物/场景等资源关系、项目共享、向量检索、任意 objectTypes 都后置。扩展新对象族须先实现服务端领域合同和授权，再开放对应类型；相同字段名不代表兼容。

## 3. 冻结合同和保留规则

| 项目 | 当前规则 |
| --- | --- |
| 类型 | 仅 `brand/v1`，hostApi `^3.2.0`；旧 3.0/3.1 包按原能力运行 |
| 必填 | name 1–160 字符、audience 1–1000、positioning 1–2000；所有文本无首尾空白 |
| 列表 | claims/restrictions 必须为数组，可为空；各最多 20 项、每项 1–500 字符 |
| 大小/数量 | 内容 JSON 最多 16 KiB；账号最多 100 对象（含归档），对象最多 100 版本 |
| 保存 | 新建 expectedVersion=0；修改指定 objectId 和预期版本，冲突 409；clientKey 8–128 字符 |
| 重试 | 同一对象/clientKey 只允许同一请求；新建 ID 由账号和 clientKey 确定；网络结果不明不能改正文重试 |
| 检索 | 当前账号名称检索、归档筛选；每页 30，offset 0–1000；无任意字段/SQL 查询 |
| 归档 | 隐藏于默认检索并拒绝新的插件读取/工作台准入；历史管理读取保留；恢复后可以继续用旧版 |
| 引用保护 | 不提供硬删除和版本回收，所有版本留存；Run 原请求/结果、Agent 原始任务输入保留引用/事实，原产物沿 Run 来源追溯 |
| 派生 | 新建时明确 source 完整引用；校验当前账号；来源记录在派生对象 v1，可从该版本追溯。后续编辑不改变原 v1 |
| Schema 迁移 | 不做字段同名隐式转换；product 或 schemaVersion=2 直接拒绝。本轮无 v2 转换器，未来需显式转换为新对象并保留来源 |
| 授权 | read/工作台解析使用 asset.read，search 使用 asset.search，save 使用 asset.import + draft_write 审批 |

品牌管理页是用户直接编辑入口，按登录账号和插件中心功能门禁校验；插件没有浏览器管理 API 的执行权限，必须通过已有插件宿主适配器。引用中的 ID 不是授权令牌。

不新增引用计数表：当前“全部版本保留、禁止硬删除”保证历史可追溯。未来引入物理清理前必须补齐所有引用来源扫描、派生保护和清理失败处理，不能直接删除历史行。

## 4. 验收准备

示例源码：`examples/plugins/brand-points`、`examples/plugins/brand-script`。包不自动安装或授权。在仓库根执行（目标文件须不存在）：

```sh
mkdir -p .local/workstudio-p10-packages
cd backend
go run ./cmd/plugin-contract -dir ../examples/plugins/brand-points -out ../.local/workstudio-p10-packages/brand-points-1.0.0.yingce-plugin
go run ./cmd/plugin-contract -dir ../examples/plugins/brand-script -out ../.local/workstudio-p10-packages/brand-script-1.0.0.yingce-plugin
```

启动阶段分支对应前后端，按既有流程迁移 schema 37；管理员上传两个包，用户选择版本并启用。生产升级前备份数据库、插件包、资源与加密密钥。本阶段仅迁移隔离测试库，未触碰现有业务库。

## 5. 用户验收步骤

1. 打开“插件工作台 → 品牌资料”。填写真实品牌、目标受众、定位、卖点和限制，保存为 v1。刷新后资料仍存在；空必填/过长条目应失败。
2. 进入“品牌卖点整理”，从选择器选刚才的品牌，预览应显示 objectId、v1 与真实品牌事实，不要求手填 JSON。
3. 交给 Agent，自己选择模型并发送整理目标。确认遵守品牌限制。再在“品牌广告脚本”选择同一品牌 v1，生成不同业务结果；两个插件共享资料而不是复制品牌。
4. 修改品牌名称或卖点，保存为 v2。品牌管理页可查看 v1/v2；之前工作台保存的引用仍为 v1，不自动升级。若要用新版，需显式重新选择并预览。已提交旧任务的输入不变。
5. 在旧版详情点击“基于此版本另建品牌”，确认生成新 ID、新对象 v1，来源仍指向原对象旧版；修改派生品牌不影响原品牌。
6. 归档原品牌，默认列表不显示，旧工作台新预览/调用失败。切换到“已归档”，旧版本仍可读；原任务/结果不消失。恢复后重新预览可继续。
7. 两个标签页同时读 v2：一个保存成功后，另一个用旧版本保存应明确冲突，不覆盖新版。保存时断网/刷新后，只能用原请求重试；确认列表后可解除重试锁定。
8. 在“品牌卖点整理”的技能中明确要求更新资料，或通过现有插件操作入口调用 save：审批前不能出现新版本；拒绝不写入；批准仅增加一版。审批期间资料已变更则旧审批失败，须读取新版后重新确认。
9. 切换第二账号：品牌列表和本机待确认请求不串数据；即使知道其他账号的 objectId，读取/修改/派生都失败。撤销插件读取授权后，工作台不能解析品牌。
10. 回归 P09 普通文字/JSON/配方工作台，以及原 Agent/插件/画布路径。检查窄屏、明暗主题、中文输入和键盘操作。

真实模型生成效果与浏览器交互由用户验收；没有自动选择付费服务、启动预览服务器或操作生产环境。

## 6. 自动验证

验证覆盖共享 Go/TypeScript 合同、两个包实际安装/启用/读取、SQLite/PostgreSQL 生命周期与审批、CAS 并发、归档/派生/账号隔离、Agent 输入冻结、HTTP 严格解析、schema 36→37 重复迁移与旧 Run 保留、搬迁表覆盖、真实选择器静态渲染、账号范围重试记录和前端生产构建。最终执行结果随本阶段交付记录列出，静态渲染不等同于浏览器验收。

本阶段已执行：

- `go test ./internal/plugins/... ./internal/objects/... ./cmd/migrate-sqlite-postgres -count=1` 通过。
- `go test ./internal/app ./internal/handler ./internal/database -run 'TestBusinessObject|TestWorkbench|TestP10Migration' -count=1` 通过；配置隔离 PostgreSQL 18，执行 SQLite/PostgreSQL 双库路径，未访问业务数据库。
- `go test -race ./internal/app -run 'TestBusinessObject|TestWorkbench' -count=1` 通过；覆盖并发 CAS、批准/拒绝、审批期间版本或归档变化和原 P09 回归。
- `bun test test/business-objects.test.tsx test/plugin-workbenches.test.tsx test/plugin-v3-contract.test.ts`：92 项通过。
- `bun run build`：类型检查和生产构建通过；保留既有大体积 chunk 提示，未做无关拆包。
- 两个示例经真实 `plugin-contract` CLI 打包验证；OpenAPI YAML 解析通过，已补路径与 Schema。
- 未启动开发服务器，未做浏览器交互或真实付费模型验收。测试容器仅用于本轮验证，交付前移除。

## 7. 迁移、回退和下一步

新增 schema 37 `business_objects`，历史迁移校验和不变。数据库表与 SQLite→PostgreSQL 搬迁清单均更新。schema 37 是加法迁移，无自动 down：不要删除对象/版本表来回退。优先停用示例和写入口，保留新后端的历史读取；旧二进制不了解 hostApi 3.2 与新 schema，不应直接接管在途任务。

本阶段起点是 P09 提交 `112285b2`，阶段分支 `codex/workstudio-p10`，每阶段开始/交付前同步 fork main，完成后仅合入 `codex-workstudio` 并暂停。P11 出海涉及真实视频样本、分析服务、费用与数据发送范围，须用户验收 P10 并授权下一阶段后另行推进。

本轮交付前再次 fetch fork origin；main 与 origin/main 均为 `9afe061d`，阶段分支已包含该提交，无需冲突处理。
