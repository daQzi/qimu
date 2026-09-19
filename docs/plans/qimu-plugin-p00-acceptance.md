# P00 交付与用户验收：插件最小合同

> 阶段状态：代码完成待用户验收。尚未进入 P01。
> 起始版本：`9eb2fcd4`（v1.5.1）。关联[统一实施手册](qimu-agent-plugin-platform-execution-runbook.md)。

整合基线更新：提交前发现 main 已推进到 `0a06783f`，包含后续模型选择、图片工作流及视频 Token 计费修复；P00 提交已重放到此基线，保留全部新改动，仅合并待测试文档中的新增章节。整合后的相关测试与前端构建再次通过。

## 1. 本阶段完成什么

- 一份共享 JSON Schema，由 Go 和 TypeScript 分别消费，定义 Manifest、Operation、SkillBinding、ResultRef、调用上下文、Run/Step、返回结果和 Pipeline 合同。
- 单独的 P00 离线校验模块，检查字段、权限申请范围、包内引用、依赖环、Schema 环和 Pipeline 依赖；没有接入实际安装或执行路由。
- resource-helper 七文件样例，以及前后端共同执行的正反例测试集。
- 文件内容摘要与操作合同摘要规则、固定 golden 数据；改变任何包文件都会改变版本内容身份。
- 一个可运行的离线校验命令，便于不启动服务器、不调用模型地验收。

本阶段没有数据库迁移、Cloud Agent 业务逻辑修改、HTTP API 开放或新画布节点。当前产品仍只接收原有 v1/v2 安装合同；把 v3 样例上传到现有插件页应被拒绝，这是预期行为。

## 2. 与旧设计基线的差异

设计时基线为 `4eaa28b8`；开工时 main 已是 `9eb2fcd4`。后者包含 Agent 策略编译 v4、结构化事实上下文、读取参数纠错、媒体回写错误分层及画布同步修复。这些代码保留，本阶段没有覆盖或回滚。

P00 以当前代码为验证基线，后续 P02 必须接入新的策略和恢复路径，不能按旧设计行号还原已被修正的逻辑。

## 3. 合同冻结决定

| 项目 | P00 决定 |
| --- | --- |
| 唯一 Schema 源 | `backend/internal/plugins/contracts/schema.json`；前端直接导入，后端 embed |
| Profile | `profile.json`，`p00-contract/1`，runtimeEnabled=false；不是执行功能开关 |
| 校验器 | Go jsonschema/v6 6.0.2；前端 Ajv 8.17.1，Draft 2020-12；均不注册远程加载器 |
| 结构与语义 | Schema 检查字段/类型；代码检查引用/依赖/已知 Adapter 最低要求；真正用户授权仍待 P02 |
| 包贡献 | 离线样例支持 skills/operations/views/canvasBlueprints；新 HTTP/Pipeline 执行结构可独立校验，但包登记尚未开放 |
| 样例执行器 | 仅离线认可 resource.inspect 的 inline/read 合同，要求 media.read；此时没有注册真实 Adapter |
| 版本子集 | 发布和依赖暂为严格三段正式版本，依赖锁为精确版本；hostApi 为 ^3.0.0。预发布/构建后缀及一般依赖范围暂拒绝，P01 扩展时增加共同用例 |
| 目录与资源 | P00 输入是已展开的 UTF-8 文本文件；ZIP 解压与上传安全接入在 P01，不取代当前 ZIP 校验器 |
| 路径 | 包内 ASCII 相对路径；Schema ref 以包根为基准，允许当前文件片段；拒绝穿越、URL、远程资源、递归引用 |
| JSON | 拒绝重复键、多值正文、超深结构、超出安全数值范围的值；不静默覆盖旧键 |
| 归属 | qimu/qimu-* 保留；已登记 ID 由可信 catalog 通过 ReservedIDs 提供；publisher 字段不是可信身份证明 |
| 状态 | Run/Step 枚举沿用架构设计；unknown 是 submissionState，非新增 Task 状态；P00 只校验记录形状，不执行迁移或状态转换 |
| Pipeline | 固定 operation/wait_input、显式 dependsOn、受控绑定/条件/foreach 与输出；检查 DAG 和显式依赖，不运行步骤 |
| 权限效果 | read/draft_write/generation/external_write/delete/publish 是分类，不能自行授权；实际最低权限由可信 Adapter/宿主约束决定 |

### 3.1 上限

压缩包 48 MiB、manifest 512 KiB、256 文件、单文件 16 MiB 的既有上限保留。新离线合同增加累计展开 64 MiB、单 JSON 512 KiB、JSON/Schema 引用深度 32；工作流合同上限 64 步、声明并发最多 8。后续运行实际并发还要取用户/宿主/供应商限制的最小值。

Profile 中 inlineResultBytes=64 KiB、maxItems=256 为后续执行层的冻结上限声明；P00 不声称已经对运行中的结果或 foreach 数量做限流。元数据查询应返回已有值，不能借 inline 隐藏下载、探测或付费分析。

### 3.2 摘要规范

每个文件先对原始 UTF-8 字节做 SHA-256。文件名限制 ASCII，按字典序排序，拼接：

```text
qimu-package-v1\n
<path>\0<file-sha256-hex>\n
...
```

对拼接字节再做 SHA-256 得到 packageDigest。操作摘要是以下精确字符串的 SHA-256：

```text
qimu-operation-v1\n<packageDigest>\n<pluginId.operationId>\n
```

这里 `\n` 与 `\0` 表示实际换行/NUL，不是两个可见字符。不对 JSON 重序列化后再算包摘要，不依赖 ZIP 时间戳。空白改变也属于内容改变；发布版本不可就地覆盖。此规则尚未接入现有 registry，待 P01 使用。

本样例预期 packageDigest：

```text
c7bd49212024d6f370ce369057f823d419b4b5d6d309db2c82e5ec74663690bf
```

调用 requestDigest 尚未实现，P02 必须在服务端规范输入、资源身份、作用域与固定版本后计算；不得把本阶段包摘要直接当成调用幂等依据。

## 4. API、权限与数据边界

| 未来入口/操作 | P00 已冻结 | 待实施的强制行为 |
| --- | --- | --- |
| operation_search/describe | 操作命名、版本和 Schema | P01/P02 按用户启用状态与权限过滤 |
| plugin-invocations | operation、releaseId、input、可选 context；不允许顶层 userId/approved/price | P02 身份从登录态获取，input 再按目标操作 Schema 校验 |
| resource.inspect | media.read、read、inline、无画布前置 | P02 查真实资源归属和元数据；Schema 不能证明所有权 |
| 保存结果/画布绑定 | inline 与 ResultRef 分开；画布是可选输出位置 | P03 独立写权限、快照冲突和幂等，不自动继承读取授权 |
| HTTP/生成 | 任务执行合同与效果分类 | P04 凭证、出站、任务、批准、报价及结算；当前禁止执行 |
| Pipeline | 输入/步骤/输出/DAG 合同 | P05/P06 状态事务、等待输入、预算、取消和派生 |

RunRecord/StepRecord 是最小传输投影，不是完整数据库模型。完整持久化仍按[实施设计第 4 节](qimu-plugin-platform-v3-implementation.md#4-持久化模型与索引)：P01 增加发布/技能绑定/用户授权；P02 增加 Run/Step/Event；P04 增加 Connection 与 Task 关联。租约、预算和审批各自只有一个权威记录，不因传输类型新增第二套账本。

hostSurface 仅描述 agent-home/canvas/editor 容器；workbenchId 是未来注册配置 ID；canvasId/projectId/threadId 为可选上下文。产物由输出 Schema 决定，绑定画布不改变产物身份。本阶段不会用空 canvasId 或虚构 Thread 绕过现有 Agent 规则。

## 5. 自动验证与限制

使用 Go 1.26.8、Bun 1.3.9 执行，项目 go 指令仍为 1.25.0；没有切换生产环境。本机临时工具放在仓库忽略的 `.local/toolchains`，不会提交二进制或构建结果。

- 现有后端 protocol、skills、canvas/capability 测试通过。
- 现有 app 插件/Agent 策略/提示词/读取参数专项通过。
- 78 个前后端共享合同正反例通过；摘要一致性、旧安装入口仍拒绝 v3、包体限制和 Go vet 通过。
- 前端合同及既有插件权限/分类/状态共 97 项测试通过；前端 build（含类型检查）通过，仍有项目大包体提示，不影响构建成功。
- 离线校验合法样例返回 valid=true、runtimeEnabled=false；保留 ID 负例拒绝。

未启动应用、未联调浏览器、未运行付费模型、未验证数据库迁移。本阶段没有新增 UI 和线上执行路径，因此不把不存在的新插件功能列为页面验收任务。

## 6. 你需要验证的内容

### A. 运行合同测试

在 `qimu/backend`：

```sh
go test ./internal/plugins/contracts -count=1
```

预期：全部通过，覆盖合法/非法包、权限声明、引用、依赖与流程环、摘要及旧安装入口。

在 `qimu/web`：

```sh
bun test test/plugin-v3-contract.test.ts
```

预期：同一份语料全部通过，与后端使用同一个 fixtures 文件。

如果当前终端没有 go/bun，可以分别使用本次已准备的 `../.local/toolchains/go/bin/go` 和 `../.local/toolchains/bun-darwin-aarch64/bun` 替代命令名。这是本机便捷路径，不是部署依赖。

### B. 离线检查样例

在 `qimu/backend`：

```sh
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/resource-helper
```

预期：返回 JSON，valid=true、fileCount=7、runtimeEnabled=false，packageDigest 等于第 3 节记录。

再执行保留 ID 负例，不需要修改样例文件：

```sh
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/resource-helper -reserved-ids resource-helper
```

预期：失败退出，原因包含 scope_forbidden。它证明保留 ID 校验生效，不代表已接入用户权限数据库。

### C. 确认下一阶段的边界

- 接受当前第一版优先声明式包与 resource-helper 样例，后续再开放 HTTP/Pipeline。
- 包内容改动必须发布新版本；未知贡献和不支持的 Schema 明确报错。
- 认可 P01 只做安装/版本/技能目录，完整 Agent 执行与画布节点分别在 P02/P03。

P00 验收后回复“P00 验收通过，继续 P01”即可。此之前保持代码完成待验收，不自动进入下一阶段。

## 7. 提交与回退

本阶段在 `codex/plugin-p00-contracts` 上形成一次阶段提交，再快进合并到本地 main；没有自动推送远端或发布版本。提交包含本轮已确认的设计/实施文档及 P00 合同实现，不包含 `.local` 工具链、媒体、密钥或数据库。

回退可针对该阶段提交执行常规反向提交；由于没有迁移和执行挂接，无需删除数据库或用户资产。依赖锁随同回退即可；不对主分支执行 hard reset。
