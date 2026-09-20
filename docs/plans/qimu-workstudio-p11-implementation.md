# P11：一键出海分析与替换方案——实施记录

状态：P10 已验收。P11 在包含 P00–P10 的独立 worktree / 分支 `codex/workstudio-p11` 实施，交付后合入 `codex-workstudio`，不合入 main，不推送或部署。阶段前 main 已与 fork origin/main 同步至 `9afe061d`；最终 Git 状态以验收文档记录为准。

用户选定复用启幕现有视频理解模型，不接 Hypit；真实付费调用与样片效果由用户自行验收。开发过程不外发用户视频，不读取模型密钥进行真实服务试验。

## 1. 本次可交付范围与原方案差异

已接通：原视频资源 + 系统模型 + 国家/语言/风格 → 模型拉片 → 人工修正 → 生成替换建议 → 用户确认 → 固定结果与业务画布。插件为 `examples/plugins/video-localization`。

本次采用模型原生视频输入，沿用已有资源元数据与 provider 的 referenceVideos，不新增独立视频服务或本地 GPU 模型。下列原计划能力尚未交付，不能算已验证：

- 字节级原片 digest、ffprobe 流/时基探测、逐帧或按需抽帧 Adapter。当前 source.digest 是资源 ID、存储对象、ETag 和更新时间的版本指纹；durationMs 来自资源记录。它不是视频字节校验和，镜头边界是模型推断。
- 音轨转写、分离、说话人声音识别与背景音分析。现有 TextReferenceConfig 只证明视频数量/大小能力，不能证明音轨理解，故强制 audioAnalyzed=false、对白只允许可见字幕证据；没有字幕则为空，并记录限制。
- 最终配音、字幕合成、视觉替换和成片导出属于 P12/P13。方案 executable 一律 false，确认不触发生成。
- 复杂报告和方案先使用既有 JSON 输入编辑器；不声称已提供专用人物合并 UI、素材拾取器或时间线拉片 UI。

P11 的自动化验证覆盖本次软件链路，原计划的逐帧媒体证据与完整音频分析仍需后续补齐。用户验收时须确认该边界，不自动进入 P12。

## 2. 通用插件能力：hostApi 3.3

### 系统模型 Operation

新增 execution.kind=model、mode=task；声明 instruction、resources 和 outputProfile。resources 按输入字段映射 image/video；当前拒绝音频资源和任意 URL/密钥。outputProfile 支持 json 与 video-report/v1。必需 generation.run，有媒体时还需 media.read，效果为 generation。

调用输入中的 model 必须明确选择系统逻辑模型，或系统渠道 ID + 模型键；禁止两种混用和附带未知模型选择字段。插件本身不绑定供应商。

app/plugin_model.go 复用 prepareCreationTask/CreateTask、CreationPriceSignature、原任务队列、钱包和 provider：

1. 准备阶段只校验与报价，不发网络、不创建 Task、不预留费用。
2. 复核账号、资源归属/就绪、媒体数量与大小、模型能力、价格与资源版本。
3. 明确批准后，同一事务创建普通 canvas_text Task、原 BillingOrder 与 PluginRun 关联；仅预留模型费用，不额外叠加 HTTP 插件服务费。
4. Agent 调用复核剩余积分与生成任务数量。费用上限写入原订单，按原模型计费结算。
5. Worker 发起请求前再次验证批准的配置版本与资源；拒绝已停止运行。不为插件任务自动切换付费备用路由，不允许通过普通 Task 重试绕开审批。
6. 原 Task 成功后严格解析一个 JSON、按输出 Schema 验证，特定报告再做领域校验。格式失败保留原 Task 诊断，PluginRun 失败，不自动付费修复；已发生的模型费用仍按原 Task 结算。
7. 定时调度与运行查询可幂等回写成功/失败/取消，Worker 重启后可继续同步。取消沿用原 Task 的取消和费用处理路径；迟到结果不恢复已取消的插件运行。

模型任务逐项审批，不纳入现有 BYOK HTTP 批次合并审批；派生流程仍显示单项模型审批卡片。

### wait_input 预填和提交校验

wait_input.inputs 复用现有绑定语法，支持 literal、input#/field、steps/key#/field；引用上游须声明 dependsOn。前后端校验显式 object 表单字段与字面量类型，运行时校验实际预填字段。仅首次创建输入请求时写草稿，顺序与 DAG 都不覆盖用户已编辑内容。

提交保留完整 Schema、归属、revision/CAS 和幂等约束。inputValidator 引用受控宿主领域校验器（video-report/v1、video-plan/v1），不执行包内代码。校验失败留在待填写状态。派生输入走同一校验，不能绕过原片来源与实体关系约束。

### 版本与资源引用

成功结果、已提交输入、原模型任务保留固定插件发布。派生运行复用相同输入/合同/资源版本与报价的已成功模型结果；强制重做按依赖闭包传播。旧运行不修改；待确认草稿重新预填，用户继续编辑。

原视频和确认的替换素材加入现有 PluginRunResource 保留引用，阻止误删。拒绝尚未执行的模型审批会解除该次输入引用。没有新增数据库表或 migration，继续使用 schema 37。

## 3. video-localization 插件组成

- Skill localize：选择模型和原片，说明费用与限制，引导审核、画布投影和派生。
- process Pipeline：analyze → review → suggest → confirm。
- analyze：传视频到系统模型，结构化镜头、人物、场景、道具、字幕和不确定项。
- review：预填报告及国家/语言/风格；允许人工纠正，提交时验证时间范围、证据、唯一 ID、说话人关系和来源。
- suggest：只发送确认后的文本报告与目标要求，不再次发送原视频；另行报价，建议映射和提示词。
- confirm：编辑 target/prompt/materialId，显式 confirmed=true；验证实体类型、重复映射、素材归属与就绪、不可执行状态。
- View/Blueprint：分析实体卡片、输入节点、方案展示；通过既有 canvas.blueprint.instantiate、画布摘要和独立写入确认落图。
- Workbench：配置原片、模型、国家、语言和风格；可从首页或画布进入。工作台模型字段当前为结构化配置，Agent 可协助填写真实目录 ID。

分析结果的领域校验位于 internal/mediaanalysis，不把业务步骤写入 Agent 主循环。插件 Operation 的审批 UI 显示实际模型、授权积分额度、素材范围及真实失败语义。

## 4. 验证及交付方式

本地使用合成合同数据和 localhost 模拟模型测试执行语义；这些数据不代表用户样片的识别结果。覆盖 SQLite/PostgreSQL 的报价批准、标准 Task 关联、输出校验、人工修正、输入/结果画布投影、派生复用、顺序/DAG 草稿保留，以及普通文本 Worker、取消退款。最终测试命令与待验收项见同目录 P11 验收文档。

前端运行相关合同/展示测试和生产构建，不启动开发服务器；浏览器交互与真实模型识别效果由用户验收。出海插件使用现有作者工具校验、打包、安装和授权，不自动安装到用户业务环境。

回退：停用 video-localization，保留历史结果；必要时回退阶段提交。不需要回滚数据库，不删除原视频或旧版本方案。
