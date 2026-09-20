# hostApi 3.3：系统模型操作与人工校验

适用启幕应用插件，不是外部 Codex 插件。最低 `requires.hostApi` 为 `^3.3.0`；3.0–3.2 的既有贡献继续按原合同运行。

## 1. 系统模型操作

```json
{
  "id": "analyze",
  "description": "分析已有视频，输出结构化报告",
  "inputSchemaRef": "schemas/input.json",
  "outputSchemaRef": "schemas/report.json",
  "requiredPermissions": ["generation.run", "media.read"],
  "effects": ["generation"],
  "context": {"requiresCanvas": false, "requiresProject": false},
  "execution": {
    "kind": "model",
    "mode": "task",
    "instruction": "识别镜头、人物、场景、道具和可见字幕，明确证据与未知项。",
    "resources": {"resourceId": "video"},
    "outputProfile": "video-report/v1"
  }
}
```

`resources` 最多 4 个输入字段，值为 image/video；没有媒体时写 `{}`。字段值必须是当前账号已就绪资源 ID，不接受 URL、data URL 或文件路径。当前不支持声明音频媒体理解。`instruction` 是包内固定指令，宿主附加实际输入、资源版本及输出 Schema；不会执行其中的代码。

调用输入的 `model` 二选一：

```json
{"logicalModelId": "实际逻辑模型ID"}
```

```json
{"channelId": "实际系统渠道ID", "model": "实际模型键"}
```

这些是结构示意，必须从部署的真实目录获取 ID。禁止自带密钥、接口 URL、任意协议请求和混用两类选择。业务 input Schema 应显式声明 model 和资源字段。

宿主在准备、批准和真正发请求前校验模型/资源，报价改变须重新发起确认。批准前没有 Task 或费用预留；批准后使用原 canvas_text Task、BillingOrder、能力校验和 provider，不是 BYOK HTTP 请求，也不收第二份插件服务费。前端展示实际模型、渠道域名、资源与授权额度。

系统模型任务逐项审批。原 BYOK 批次审批不包含模型任务；插件模型失败不自动切换付费路由，普通任务“重试”也不能绕过插件重新授权。模型成功但输出格式无效时 PluginRun 失败，原任务费用仍按原账务规则处理，不自动再次付费修复。

## 2. 输出合同

- `json`：一个合法 JSON 文档、不超过 64 KiB，拒绝重复键、过深结构、Markdown 包裹和额外正文，并通过 outputSchemaRef。
- `video-report/v1`：仅允许 `resources={"resourceId":"video"}`；额外校验来源、覆盖区间、镜头顺序、实体证据、ID/说话人引用和字幕/音轨标识。字段见示例包 schemas/report.json 及 internal/mediaanalysis/report.go。

视频报告在 Worker 内先执行本地 ffprobe，source.digest 使用实际文件 SHA-256，durationMs 使用探测时长，流/时基保存到 Task metadata；报价阶段仍显示资源版本指纹。镜头边界是模型推断，按需抽帧不是精确逐帧标定。管理员配置 text.references.videoAudio=true 后才允许音轨对白及 audioEvents（music/ambience/effect），无能力或无音轨时只能报告字幕并说明限制。

声明式输入视图可选 video-report-editor/v1 与 video-plan-editor/v1，分别绑定带 report 的修正表单、带 report/plan/confirmed 的确认表单；仍走统一 Schema、revision、幂等和领域校验。支持人物合并、原片定位、对白编辑、素材选择/上传；不会执行插件脚本。后端须安装 ffmpeg/ffprobe，官方构建镜像已包含。媒体上限 512 MiB、六小时，最多两个并发；工具执行两分钟超时。

宿主登录 API：POST /api/resources/:id/probe 返回 SHA-256/时长/流信息；GET /api/resources/:id/frame?atMs=1000 返回 JPEG，均检查当前账号归属并限流。抽帧请求时间须落在源视频范围内，不接受 URL、命令或本地路径。

输出转换与 Task 终态分离：原 Task 负责 provider/账务，PluginRun 负责结构化结果。后台调度和查询幂等同步结果；取消不被迟到结果覆盖。已有结果的 ResultRef、画布投影、固定版本和历史读取复用旧合同。

## 3. wait_input 初始草稿

```json
{
  "key": "review",
  "type": "wait_input",
  "dependsOn": ["analyze"],
  "formSchemaRef": "schemas/review.json",
  "inputs": {"report": {"from": "steps/analyze#"}},
  "inputValidator": "video-report/v1"
}
```

表单须为显式 object；每个输入字段属于其 properties。字面量在包校验时验证，运行时绑定再按字段 Schema 验证。草稿可以缺少其他必填字段，提交才校验完整表单；不会把预填当成确认，也不会因刷新/恢复覆盖编辑。

`inputValidator` 可省略；目前提供 video-report/v1、video-plan/v1 两个宿主校验器。流程输入必须带原视频 resourceId，表单带 report；方案校验另带 plan。校验器按流程原视频核对来源，不能在表单里换视频来绕过来源校验。校验失败不提交；派生输入重复执行相同校验。扩展校验器通过宿主注册和合同版本进行，不从社区包动态加载代码。

video-plan/v1 检查替换项实体/类型、重复映射、提示词、素材归属/就绪及 executable=false。确认方案和所选媒体加入已有保留引用；本期不做语音或视频编辑能力的虚假降级。

## 4. 完整示例与打包

示例目录：`examples/plugins/video-localization`。包含 Skill、5 个 Operation、4 步 Pipeline、View、Blueprint 和 Workbench。核心业务在包中，不修改 Agent 主循环。

在仓库根执行：

```sh
mkdir -p .local/artifacts
cd backend
go run ./cmd/plugin-contract -dir ../examples/plugins/video-localization -out ../.local/artifacts/video-localization-1.0.1.yingce-plugin
go run ./cmd/plugin-contract -package ../.local/artifacts/video-localization-1.0.1.yingce-plugin
```

工具不覆盖已有包；重新打包时指定新的输出文件。管理员通过原应用插件安装入口上传，账号启用并授权后，在工作台或 Agent 引用对应技能。配置与具体人工验证见 `docs/plans/qimu-workstudio-p11-acceptance.md`。
