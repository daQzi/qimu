# 万有引力全模态套件（lxmone.xyz）

万有引力文本、图片和视频的声明式插件。版本 `1.1.0`；为保留已有渠道绑定，插件 ID 和包名仍为
`lxmone-video-suite`，原有 8 个视频 provider ID 不变。

## 接入步骤

1. 此插件属于官方包，不能通过 `/admin/plugins` 上传同 ID 包覆盖。源码部署替换官方目录内的 `lxmone-video-suite.yingce-plugin` 后重启后端，会重新导入并保留启用状态；容器部署需要将新包纳入镜像或实际配置的官方插件目录，不能只修改宿主机源码。更新后在 `/admin/plugins` 检查版本并启用。
   新增文本 provider 的流式和 Agent 输出限制还需要本次后端代码中的 wire 映射；已有部署只替换插件包时，不能视为流式/Agent 全链路升级完成。
2. 在系统渠道或个人自定义渠道中按用途选择下表的协议。Base URL 填 `https://lxmone.xyz/v1`，密钥填万有引力本站 API Key，不是模型厂商 Key。
3. 添加上游完整模型 ID，按模型配置尺寸、时长、分辨率和参考素材上限；不要用显示名称替代 ID，也不要混用近似名称。
4. 先分别用一个文本、图片和视频模型进行小额验收，确认结果转存后再批量使用。插件不会自动导入全站模型或价格。

| 协议 | 用途 | 示例模型 ID（以本站实时启用状态为准） |
| --- | --- | --- |
| `lxmone-chat` | 文本对话 / Agent | `deepseek-v4.1-flash`、`glm-5.3`、`qwen3.8-max` |
| `lxmone-claude` | Claude 原生 Messages / Agent | 按本站当前支持 Messages 的 Claude ID 配置 |
| `lxmone-images` | 同步图片生成 / 编辑 | `gpt-image-2a`、`nana-banana-2.1`、`qwen-2.1`、`Seedream-5-Pro` |
| `lxmone-images-async` | 异步图片生成 / 编辑 | 同上，适合等待时间较长的生图 |
| `lxmone-video-current` | 当前视频模型按 ID 自动路由 | `seedance2.5`、`seedance-2-5-hc`、`kling-4.0`、`minimax-h3-b` |

图片有参考图时自动切换 `/images/edits`，通过 multipart 上传有序文件；无参考图时走 JSON 文生图。
视频不是一个统一的 OpenAI 请求体，当前入口按已核对的模型 ID 区分字段和创建路径；未知模型拒绝提交。
旧视频入口仍保留，但旧文档中的模型可能已被上游停用，新渠道优先使用当前入口。

## 能力边界

- 本插件提供文本、图片、视频三类生成入口，不意味着文本模型均支持读图/视频，也不意味着所有模型均支持工具调用。
- 不自动同步模型目录、价格、账户权限或启用状态。只读目录 `GET /v1/sub2api/model-catalog` 可用于人工核对；可调用权限还需与 `GET /v1/models` 取交集。不要把站点展示价格直接作为平台计费配置。
- 不支持音频生成；视频参考音频和视频生成声音不等于独立 TTS。
- 不自动重新提交未知结果的付费请求。异步图片与视频完成后应及时转存，链接有有效期。
- 当前协议使用离线请求/响应测试；真实 Key、上游出图出片与宿主页面端到端验收需单独进行。

完整字段、当前视频模型分组、响应与错误处理见 [docs/interface.md](docs/interface.md)。
协议来源：[万有引力公开文档](https://lxmone.xyz/docs/)。

## 单包构建与验证

在仓库根目录执行，避免全量构建支付插件和改写其他包：

```sh
node plugin-packages/embed-documentation.mjs lxmone-video-suite
cd plugin-packages/lxmone-video-suite
zip -X -q ../lxmone-video-suite.yingce-plugin manifest.json README.md docs/interface.md
cd ../../backend
go test ./internal/protocol
```
