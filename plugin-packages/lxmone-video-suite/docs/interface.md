# 万有引力全模态套件接口字段

## 协议身份

- 插件 ID：`lxmone-video-suite`。
- 能力：`text`、`image`、`video`。
- 默认 Base URL：`https://lxmone.xyz/v1`。
- 鉴权驱动：`bearer`。
- 创建接口：`POST /v1/videos` 或 `POST /v1/videos/generations`（按模型所属 provider 决定）。
- 视频查询接口统一为 `GET /v1/videos/{{taskId}}`。
- 参考媒体要求公开可访问的 HTTPS URL（`requiresPublicMediaUrls`）。

以下旧视频形状保留用于已有渠道；它们不是当前全部可用模型清单。原形状来自中转站公开文档 `https://lxmone.xyz/docs/`：同一批模型的 `model`、`prompt`、时长、
分辨率与画幅字段名在不同渠道并不一致，因此按上游请求体拆成 8 个 provider，而不是一个通用协议。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |
| `apiKey` | secret | 是 | 万有引力中转站的 API Key，随请求发送 `Authorization: Bearer <apiKey>`。 |

## 新增协议入口

同一个包安装后提供文本、图片和视频协议，但每个渠道仍须选择与用途相符的 provider。
默认 Base URL 统一填写 `https://lxmone.xyz/v1`；不要填写完整接口路径，也不要重复追加 `/v1`。

| Provider | 用途 | 创建 / 查询 |
| --- | --- | --- |
| `lxmone-chat` | 全部 OpenAI Chat Completions 兼容文本模型 | `POST /v1/chat/completions` |
| `lxmone-claude` | 文档支持的 Claude 原生 Messages | `POST /v1/messages` |
| `lxmone-images` | 文生图与参考图编辑 | `POST /v1/images/generations` 或 `/v1/images/edits` |
| `lxmone-images-async` | 异步文生图与参考图编辑 | 同步路径追加 `/async`；`GET /v1/images/tasks/{taskId}` |
| `lxmone-video-current` | 当前公开模型，按完整 ID 自动选 A/B 路径和字段 | `POST /v1/videos` 或 `/v1/videos/generations`；统一 `GET /v1/videos/{taskId}` |

### 文本与 Agent

`lxmone-chat` 使用 Bearer 鉴权；`lxmone-claude` 使用 `x-api-key` 和 `anthropic-version: 2023-06-01`。
映射 `messages`、`instructions`、`temperature`、`top_p`、`max_tokens`、`tools`、`tool_choice`、
`response_format`、`stream` 等字段；具体字段是否支持以对应模型为准，Claude 不接受 OpenAI 特有字段。
扩展参数使用 `providerOptions.lxmone-chat` / `providerOptions.lxmone-claude`，`extra.max_tokens` 优先。
文本响应分离正文和 reasoning，读取 usage；两个入口都声明 Agent 请求及工具调用响应映射。
宿主负责流式处理，插件不自己拼接 SSE。上游文档只验收了部分模型的非流式文本，不能视为所有模型的工具/流式能力已经验收。

### 图片生成与编辑

没有 `images` 时发送 JSON 文生图；存在参考图时自动切换 edits，并以有序、重复的 `image[]`
multipart 文件字段上传，由宿主读取 URL / Data URL 并转成文件，不把本地路径交给上游。
因此 `gpt-image-2a` 和 `nana-banana-2.1` 等要求 multipart 的编辑也能使用；普通图片模型共用该编辑路径。
使用 `model`、`prompt`、`imageCount` → `n`、`aspectRatio` → 像素 `size`，默认 `n=1`、`size=1024x1024`、`response_format=url`。
插件声明像素尺寸控件；渠道应配置模型支持的实际尺寸，不是把 `16:9` 字符串作为像素尺寸提交。
当前统一限制最多 3 张参考图，不支持 mask；Qwen / Seedream 强校验 `n=1`，不发送它们不支持的原生扩展参数。
`gpt-image-2a` 的原生 4K 使用 `3840x2160`，拒绝 `4096x4096`。
此版本不开放 quality/background/output_format 等模型专用高级参数，不能把“所有图片模型入口”理解为“所有高级参数”。
同步解析 `data[].url` / `data[].b64_json`；异步解析 `task_id`、`status`、`result.data[].url`，
只在没有数组结果时使用 `image_url`，避免重复第一张图片；failed 停止轮询并保留错误。
异步图片结果保留 12 小时，宿主应立即转存。同步接口与异步接口分别选择，不自动重投创建请求。

### 当前视频自动路由

`lxmone-video-current` 按经过核对的模型 ID 选择顶层 reference 字段、camelCase reference 字段、
`assets`、`input.media + parameters`、URL 数组等请求结构，分别映射 `seconds` / `duration` / `duration_seconds`。
不改模型名，不把未知模型塞入通用模板；参考素材类型、数量、时长、分辨率和画幅按公开目录校验。
H3 B/C 和当前标注不支持纯文生的模型要求参考图。素材必须是上游可访问的公网 URL，宿主负责发布和 SSRF 校验。
Seedance HC 将 `generateAudio` 映射到 `generate_audio`，参考音频本身不代表生成声音。

已核对的当前模型及字段分组：

- `grok-imagine-video-1.5-fast`、`grok-imagine-video-1.5-lite`、`grok-imagine-video-1.5（按次）`：`model`、`prompt`、`seconds`、`size`、`aspect_ratio`、`reference_images`。
- `kling-4.0`：`model`、`prompt`、`duration`、`resolution`、`ratio`、`referenceImages`、`referenceVideos`、`referenceAudios`。
- `minimax-h3`、`sd-2.5-480p-plus`、`sd-2.5-480p-pro`、`sd-2.5-720p-plus`、`sd-2.5-720p-pro`、`sd2.0-480p`、`sd2.0-720p`、`seedance2.0-720p`、`seedance2.5-1`、`seedance2.5-1080s`、`seedance2.5-k`、`seedance2.5-w`、`wan-3.0`：`model`、`prompt`、`duration`、`resolution`、`aspect_ratio`、`reference_images`、`reference_videos`、`reference_audios`。
- `minimax-h3-a`、`minimax-h3-b`、`minimax-h3-c`：`model`、`prompt`、`duration_seconds`、`resolution`、`aspect_ratio`、`reference_images`、`reference_audios`、`mode`。
- `omni-flash-720p`、`seedance2.0-933-V`、`seedance2.5`、`wan30-720p`：`model`、`prompt`、`seconds`、`resolution`、`aspect_ratio`、`reference_images`、`reference_videos`、`reference_audios`。
- `seedance-2-0-fast-hc`、`seedance-2-0-hc`、`seedance-2-0-mini-hc`、`seedance-2-5-hc`：`model`、`prompt`、`duration_seconds`、`resolution`、`aspect_ratio`、`reference_images`、`reference_videos`、`reference_audios`、`generate_audio`。
- `seedance2.0-933`：`model`、`prompt`、`seconds`、`ratio`、`image_urls`、`video_urls`、`audio_urls`。
- `seedance2.5-480`、`seedance2.5-480s`、`seedance2.5-720`、`seedance2.5-720s`：`model`、`prompt`、`duration`、`size`、`assets`。
- `seedance2.5v`：`model`、`prompt`、`seconds`、`reference_images`、`ratio`、`videos`、`audios`。
- `wan3.0-video-p`、`wan3.0-video-prime-p`：`model`、`duration`、`resolution`、`input`、`parameters`。

当前视频统一查询 `/v1/videos/{taskId}`；优先下载 `metadata.direct_url` / `data.metadata.direct_url`，
没有免鉴权直链时，使用同一渠道 Key 调用 `GET /v1/videos/{taskId}/content`，不把 Key 拼进 URL。
旧的 8 个视频 provider ID 保留，已有模型绑定不改写；新增模型推荐选择当前自动路由入口。
旧入口仅维持原请求合同，不应据此认为已停用的模型还能调用。

## 旧视频 provider 字段映射

宿主向插件提交统一的视频生成请求，插件按 provider 映射到上游字段：

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | `model` | 上游模型 ID，透传，不做改名或别名。 |
| `prompt` | string | 是 | `prompt` / `input.prompt` | 视频提示词。 |
| `images` | media[] | 否 | `reference_images` / `images` / `image_url` / `input.media[]` / `input_reference` | 参考图；带 role 的 `first_frame`、`last_frame` 会落到支持首尾帧的 provider 专属字段。 |
| `videos` | media[] | 否 | `reference_videos` / `videos` / `input.media[]` | 参考视频。 |
| `audios` | media[] | 否 | `reference_audios` / `audio` / `audio_urls` / `input.media[]` | 参考音频。 |
| `duration` | integer | 否 | `seconds` / `duration` / `duration_seconds` | 时长秒数；缺省由各 provider 填默认值。 |
| `aspectRatio` | string | 否 | `aspect_ratio` / `parameters.ratio` | 画幅比例，默认 `16:9`。 |
| `resolution` | string | 否 | `resolution` / `size` / `parameters.resolution` | 分辨率档位；上游要求大写 `720P` 形式的 provider 会自动转换。 |
| `generateAudio` | boolean | 否 | `audio` / `generate_audio` | 是否生成或保留音频。 |

## 各 provider 的创建体

### `lxmone-wan-videos`（`wan3.0-video`、`wan3.0-video-prime`）

`POST /v1/videos`，顶层字段：`model`、`prompt`、`seconds`、`size`、`aspect_ratio`、
`reference_images[{url, role}]`、`reference_videos[]`、`reference_audios[]`、`prompt_extend`。

### `lxmone-wan-channel-s`（`wan3.0-video-s`、`wan3.0-video-prime-s`）

`POST /v1/videos`，`input.media[{type, url}]` 与 `input.prompt`，参数放在
`parameters{resolution, ratio, duration, audio, prompt_extend}`。

### `lxmone-seedance-videos`（`seedance-2-pro/fast/mini`、`seedance-2.5-pro`、`seedance2.0-a`~`f`、`seedance2.5-a`）

`POST /v1/videos`，字段：`model`、`prompt`、`duration_seconds`、`resolution`、`aspect_ratio`、
`reference_images[]`、`reference_videos[]`、`reference_audios[]`。

### `lxmone-h3-max-videos`（`minimax-h3-max`）

官方文档要求 H3 Max 走 `POST /v1/videos`（不要提交到 `/v1/videos/generations`，也要避开
`input_reference` 与 `audio` 这组独立工作流字段），字段为：`model`、`prompt`、`seconds`、
`resolution`、`aspect_ratio`、`reference_images[]`、`reference_videos[]`、`reference_audios[]`。
上游把客户端传入的 720p 映射为 768p，参考素材上限为图 / 视频 / 音频各 12 段。

### `lxmone-sd-videos`（`sd-2.0-a`、`sd-2.0-b`、`sd-2.0-c`、`sd-2.5-a`、`sd-2.5`）

`POST /v1/videos`，字段：`model`、`prompt`、`duration`、`resolution`、`aspect_ratio`、
`images[]`、`videos[]`、`audio[]`。

### `lxmone-sd-mini-videos`（`sd-mini`）

`POST /v1/videos`，字段：`model`、`prompt`、`seconds`、`resolution`、`aspect_ratio`、
`image_url`、`reference_image_urls[]`、`audio_urls[]`。

### `lxmone-grok-videos`（`grok-imagine-video`、`grok-imagine-video-1.5`、`grok-imagine-video-1.5S`）

`POST /v1/videos/generations`，字段：`model`、`prompt`、`duration`、`aspect_ratio`、`resolution`、
`generate_audio`，以及互斥的 `image{url}`（首帧）或 `reference_images[{url}]`（参考图）。

### `lxmone-h3-workflow`（`minimax-h3-a`~`minimax-h3-e`）

`POST /v1/videos/generations`，字段：`model`、`prompt`、`seconds`、`resolution`、`aspect_ratio`、
`input_reference[]`、`first_frame`、`last_frame`、`audio`。

## 旧视频 provider 响应与错误

创建成功后，插件从上游响应里按顺序取 `id`、`task_id`、`taskId`、`data.id` 得到统一 `taskId`，
轮询时按 `status`、`state`、`data.status` 取状态，`completed`/`succeeded` 归一为 `succeeded`，
`failed`/`expired`/`error` 归一为 `failed`，其余按进行中处理。

相对 `metadata.url` 留给鉴权 `/content` 下载，不作为公开下载地址。视频地址优先取 `metadata.direct_url`、`data.metadata.direct_url`，再按
`metadata.url`、`metadata.video_url`、`video_url`、`videoUrl`、`result_url`、`url`、
`data.metadata.url`、`data.video_url`、`output.url` 依次取值；上游返回的是临时地址
（`resultEphemeral`），由宿主立即下载并转存为项目资源。

H3 工作流完成响应可能同时包含相对路径 `metadata.url` 和完整 HTTPS 地址
`metadata.direct_url`，必须优先选择后者，避免把缺少主机的路径交给外部资源下载器。
下载仍执行宿主的地址与 SSRF 校验。

错误信息取 `error.message`、`message`、`fail_reason`，错误码路径为 `error.message` 与 `error.code`。
HTTP 失败、业务错误码或状态进入 `failed` 时，任务以失败结束并把上游原文回传给用户。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "lxmone-video-suite",
  "name": "万有引力全模态套件",
  "version": "1.1.0",
  "author": "Yingce / 万有引力",
  "description": "万有引力文本对话、Claude Messages、图片生成/编辑与异步生图、视频生成套件。保留原视频协议，新增当前模型专用路由。",
  "permissions": [
    "generation.run",
    "media.read"
  ],
  "configuration": {
    "fields": [
      {
        "name": "apiKey",
        "type": "secret",
        "label": "API Key",
        "required": true
      }
    ]
  },
  "contributes": {
    "providers": [
      {
        "id": "lxmone-wan-videos",
        "label": "万有引力 Wan 3.0 视频（现有渠道）",
        "description": "wan3.0-video / wan3.0-video-prime：POST /v1/videos，顶层 reference_images、reference_videos、reference_audios。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "seconds": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 10
              }
            },
            "size": {
              "$coalesce": [
                {
                  "$upper": {
                    "$ref": "request.resolution"
                  }
                },
                "720P"
              ]
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "reference_images": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$sortByOrder": {
                      "$ref": "request.images"
                    }
                  },
                  "as": "media",
                  "in": {
                    "url": {
                      "$ref": "media.value"
                    },
                    "role": {
                      "$coalesce": [
                        {
                          "$ref": "media.role"
                        },
                        "reference_image"
                      ]
                    }
                  }
                }
              }
            },
            "reference_videos": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.videos"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "reference_audios": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.audios"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "prompt_extend": false
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-wan-channel-s",
        "label": "万有引力 Wan 3.0 视频（S 渠道）",
        "description": "wan3.0-video-s / wan3.0-video-prime-s：POST /v1/videos，原生 input.media 与 parameters。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "input": {
              "prompt": {
                "$ref": "request.prompt"
              },
              "media": {
                "$omitEmpty": {
                  "$concatArrays": [
                    {
                      "$map": {
                        "from": {
                          "$sortByOrder": {
                            "$ref": "request.images"
                          }
                        },
                        "as": "media",
                        "in": {
                          "type": {
                            "$coalesce": [
                              {
                                "$ref": "media.role"
                              },
                              "reference_image"
                            ]
                          },
                          "url": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    {
                      "$map": {
                        "from": {
                          "$ref": "request.videos"
                        },
                        "as": "media",
                        "in": {
                          "type": "reference_video",
                          "url": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    {
                      "$map": {
                        "from": {
                          "$ref": "request.audios"
                        },
                        "as": "media",
                        "in": {
                          "type": "reference_audio",
                          "url": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  ]
                }
              }
            },
            "parameters": {
              "resolution": {
                "$coalesce": [
                  {
                    "$upper": {
                      "$ref": "request.resolution"
                    }
                  },
                  "720P"
                ]
              },
              "ratio": {
                "$coalesce": [
                  {
                    "$ref": "request.aspectRatio"
                  },
                  "16:9"
                ]
              },
              "duration": {
                "$if": {
                  "condition": {
                    "$gt": [
                      {
                        "$ref": "request.duration"
                      },
                      0
                    ]
                  },
                  "then": {
                    "$ref": "request.duration"
                  },
                  "else": 5
                }
              },
              "audio": {
                "$ref": "request.generateAudio"
              },
              "prompt_extend": false
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-seedance-videos",
        "label": "万有引力 Seedance 视频",
        "description": "seedance-2-pro/fast/mini、seedance-2.5-pro、seedance2.0-a~f、seedance2.5-a。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "duration_seconds": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 5
              }
            },
            "resolution": {
              "$lower": {
                "$coalesce": [
                  {
                    "$ref": "request.resolution"
                  },
                  "720p"
                ]
              }
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "reference_images": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$sortByOrder": {
                      "$ref": "request.images"
                    }
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "reference_videos": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.videos"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "reference_audios": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.audios"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-h3-max-videos",
        "label": "万有引力 MiniMax H3 Max 视频",
        "description": "minimax-h3-max：POST /v1/videos，seconds、resolution 与 reference_images、reference_videos、reference_audios 扁平 URL 数组。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "seconds": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 5
              }
            },
            "resolution": {
              "$lower": {
                "$coalesce": [
                  {
                    "$ref": "request.resolution"
                  },
                  "768p"
                ]
              }
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "reference_images": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$sortByOrder": {
                      "$ref": "request.images"
                    }
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "reference_videos": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.videos"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "reference_audios": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.audios"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-sd-videos",
        "label": "万有引力 SD 视频",
        "description": "sd-2.0-a/b/c、sd-2.5-a、sd-2.5：POST /v1/videos，duration 与 images、videos、audio 数组。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "duration": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 5
              }
            },
            "resolution": {
              "$lower": {
                "$coalesce": [
                  {
                    "$ref": "request.resolution"
                  },
                  "720p"
                ]
              }
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "images": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$sortByOrder": {
                      "$ref": "request.images"
                    }
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "videos": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.videos"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "audio": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.audios"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-sd-mini-videos",
        "label": "万有引力 SD Mini 视频",
        "description": "sd-mini：POST /v1/videos，主图 image_url、额外参考图 reference_image_urls、音频 audio_urls。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "seconds": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 5
              }
            },
            "resolution": {
              "$lower": {
                "$coalesce": [
                  {
                    "$ref": "request.resolution"
                  },
                  "720p"
                ]
              }
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "image_url": {
              "$omitEmpty": {
                "$first": {
                  "$map": {
                    "from": {
                      "$sortByOrder": {
                        "$ref": "request.images"
                      }
                    },
                    "as": "media",
                    "in": {
                      "$ref": "media.value"
                    }
                  }
                }
              }
            },
            "reference_image_urls": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$filter": {
                      "from": {
                        "$sortByOrder": {
                          "$ref": "request.images"
                        }
                      },
                      "as": "media",
                      "where": {
                        "$gt": [
                          {
                            "$ref": "media.order"
                          },
                          0
                        ]
                      }
                    }
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "audio_urls": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$ref": "request.audios"
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-grok-videos",
        "label": "万有引力 Grok Imagine 视频",
        "description": "grok-imagine-video / 1.5 / 1.5S：POST /v1/videos/generations，duration、image、reference_images。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos/generations",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "duration": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 6
              }
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "resolution": {
              "$lower": {
                "$coalesce": [
                  {
                    "$ref": "request.resolution"
                  },
                  "720p"
                ]
              }
            },
            "image": {
              "$omitEmpty": {
                "$if": {
                  "condition": {
                    "$eq": [
                      {
                        "$len": {
                          "$ref": "request.images"
                        }
                      },
                      1
                    ]
                  },
                  "then": {
                    "url": {
                      "$first": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  },
                  "else": null
                }
              }
            },
            "reference_images": {
              "$omitEmpty": {
                "$if": {
                  "condition": {
                    "$gt": [
                      {
                        "$len": {
                          "$ref": "request.images"
                        }
                      },
                      1
                    ]
                  },
                  "then": {
                    "$map": {
                      "from": {
                        "$sortByOrder": {
                          "$ref": "request.images"
                        }
                      },
                      "as": "media",
                      "in": {
                        "url": {
                          "$ref": "media.value"
                        }
                      }
                    }
                  },
                  "else": null
                }
              }
            },
            "generate_audio": {
              "$ref": "request.generateAudio"
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-h3-workflow",
        "label": "万有引力 MiniMax H3 工作流",
        "description": "minimax-h3-a~e：POST /v1/videos/generations，input_reference、first_frame、last_frame、audio。",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/videos/generations",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "seconds": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.duration"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.duration"
                },
                "else": 5
              }
            },
            "resolution": {
              "$lower": {
                "$coalesce": [
                  {
                    "$ref": "request.resolution"
                  },
                  "720p"
                ]
              }
            },
            "aspect_ratio": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "16:9"
              ]
            },
            "input_reference": {
              "$omitEmpty": {
                "$map": {
                  "from": {
                    "$filter": {
                      "from": {
                        "$ref": "request.images"
                      },
                      "as": "media",
                      "where": {
                        "$and": [
                          {
                            "$ne": [
                              {
                                "$ref": "media.role"
                              },
                              "first_frame"
                            ]
                          },
                          {
                            "$ne": [
                              {
                                "$ref": "media.role"
                              },
                              "last_frame"
                            ]
                          }
                        ]
                      }
                    }
                  },
                  "as": "media",
                  "in": {
                    "$ref": "media.value"
                  }
                }
              }
            },
            "first_frame": {
              "$omitEmpty": {
                "$first": {
                  "$map": {
                    "from": {
                      "$filter": {
                        "from": {
                          "$ref": "request.images"
                        },
                        "as": "media",
                        "where": {
                          "$eq": [
                            {
                              "$ref": "media.role"
                            },
                            "first_frame"
                          ]
                        }
                      }
                    },
                    "as": "media",
                    "in": {
                      "$ref": "media.value"
                    }
                  }
                }
              }
            },
            "last_frame": {
              "$omitEmpty": {
                "$first": {
                  "$map": {
                    "from": {
                      "$filter": {
                        "from": {
                          "$ref": "request.images"
                        },
                        "as": "media",
                        "where": {
                          "$eq": [
                            {
                              "$ref": "media.role"
                            },
                            "last_frame"
                          ]
                        }
                      }
                    },
                    "as": "media",
                    "in": {
                      "$ref": "media.value"
                    }
                  }
                }
              }
            },
            "audio": {
              "$omitEmpty": {
                "$first": {
                  "$map": {
                    "from": {
                      "$sortByOrder": {
                        "$ref": "request.audios"
                      }
                    },
                    "as": "media",
                    "in": {
                      "$ref": "media.value"
                    }
                  }
                }
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}",
          "contentType": "application/json"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              },
              "taskId"
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              },
              {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$first": {
                          "$split": [
                            {
                              "$ref": "response.metadata.url"
                            },
                            "://"
                          ]
                        }
                      },
                      [
                        "https",
                        "http"
                      ]
                    ]
                  },
                  "then": {
                    "$ref": "response.metadata.url"
                  },
                  "else": null
                }
              },
              {
                "$ref": "response.metadata.video_url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.videoUrl"
              },
              {
                "$ref": "response.result_url"
              },
              {
                "$ref": "response.url"
              },
              {
                "$ref": "response.data.metadata.url"
              },
              {
                "$ref": "response.data.video_url"
              },
              {
                "$ref": "response.output.url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        }
      },
      {
        "id": "lxmone-chat",
        "label": "万有引力文本对话",
        "capabilities": [
          "text"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "messages",
            "type": "message[]",
            "required": true,
            "mapping": "provider message container",
            "description": "包含历史消息和当前用户输入。"
          },
          {
            "name": "instructions",
            "type": "string",
            "required": false,
            "mapping": "system/instructions",
            "description": "系统指令。"
          },
          {
            "name": "temperature",
            "type": "number",
            "required": false,
            "mapping": "temperature",
            "description": "采样温度。"
          },
          {
            "name": "top_p",
            "type": "number",
            "required": false,
            "mapping": "top_p",
            "description": "核采样参数。"
          },
          {
            "name": "max_tokens",
            "type": "integer",
            "required": false,
            "mapping": "max_tokens/max_output_tokens",
            "description": "最大输出 token。"
          },
          {
            "name": "tools",
            "type": "array",
            "required": false,
            "mapping": "tools/toolConfig",
            "description": "工具定义。"
          },
          {
            "name": "tool_choice",
            "type": "object|string",
            "required": false,
            "mapping": "tool_choice",
            "description": "工具选择策略。"
          },
          {
            "name": "response_format",
            "type": "object",
            "required": false,
            "mapping": "response_format/text",
            "description": "结构化输出配置。"
          },
          {
            "name": "stream",
            "type": "boolean",
            "required": false,
            "mapping": "stream",
            "description": "流式开关；后台任务当前以最终响应归一。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/chat/completions",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "messages": {
              "$ref": "request.messages"
            },
            "temperature": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.temperature"
              }
            },
            "top_p": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.top_p"
              }
            },
            "max_tokens": {
              "$omitEmpty": {
                "$coalesce": [
                  {
                    "$ref": "request.extra.max_tokens"
                  },
                  {
                    "$ref": "request.providerOptions.lxmone-chat.max_tokens"
                  }
                ]
              }
            },
            "tools": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.tools"
              }
            },
            "tool_choice": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.tool_choice"
              }
            },
            "response_format": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.response_format"
              }
            },
            "stream": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.stream"
              }
            },
            "stop": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.stop"
              }
            },
            "seed": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.seed"
              }
            },
            "frequency_penalty": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.frequency_penalty"
              }
            },
            "presence_penalty": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.presence_penalty"
              }
            },
            "logprobs": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.logprobs"
              }
            },
            "top_logprobs": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.top_logprobs"
              }
            },
            "user": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-chat.user"
              }
            }
          }
        },
        "agent": {
          "method": "POST",
          "path": "/v1/chat/completions",
          "contentType": "application/json",
          "body": {
            "$merge": [
              {
                "$ref": "request.extra.agent.chatCompletion"
              },
              {
                "model": {
                  "$ref": "request.model"
                }
              }
            ]
          }
        },
        "response": {
          "status": "succeeded",
          "textPaths": [
            "choices.0.message.content",
            "choices.0.text"
          ],
          "reasoningPaths": [
            "choices.0.message.reasoning_content"
          ],
          "usage": {
            "$ref": "response.usage"
          },
          "errorPaths": [
            "error.message",
            "error.code",
            "error.type"
          ],
          "messagePaths": [
            "error.message"
          ]
        },
        "agentResponse": {
          "textPaths": [
            "choices.0.message.content",
            "choices.0.text"
          ],
          "reasoningPaths": [
            "choices.0.message.reasoning_content"
          ],
          "toolCallsPath": "choices.0.message.tool_calls",
          "toolCallIdPaths": [
            "id"
          ],
          "toolCallNamePaths": [
            "function.name"
          ],
          "toolCallArgumentsPaths": [
            "function.arguments"
          ]
        }
      },
      {
        "id": "lxmone-claude",
        "label": "万有引力 Claude 原生",
        "capabilities": [
          "text"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "anthropic",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "messages",
            "type": "message[]",
            "required": true,
            "mapping": "provider message container",
            "description": "包含历史消息和当前用户输入。"
          },
          {
            "name": "instructions",
            "type": "string",
            "required": false,
            "mapping": "system/instructions",
            "description": "系统指令。"
          },
          {
            "name": "temperature",
            "type": "number",
            "required": false,
            "mapping": "temperature",
            "description": "采样温度。"
          },
          {
            "name": "top_p",
            "type": "number",
            "required": false,
            "mapping": "top_p",
            "description": "核采样参数。"
          },
          {
            "name": "max_tokens",
            "type": "integer",
            "required": false,
            "mapping": "max_tokens/max_output_tokens",
            "description": "最大输出 token。"
          },
          {
            "name": "tools",
            "type": "array",
            "required": false,
            "mapping": "tools/toolConfig",
            "description": "工具定义。"
          },
          {
            "name": "tool_choice",
            "type": "object|string",
            "required": false,
            "mapping": "tool_choice",
            "description": "工具选择策略。"
          },
          {
            "name": "response_format",
            "type": "object",
            "required": false,
            "mapping": "response_format/text",
            "description": "结构化输出配置。"
          },
          {
            "name": "stream",
            "type": "boolean",
            "required": false,
            "mapping": "stream",
            "description": "流式开关；后台任务当前以最终响应归一。"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/messages",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "max_tokens": {
              "$coalesce": [
                {
                  "$ref": "request.extra.max_tokens"
                },
                {
                  "$ref": "request.providerOptions.lxmone-claude.max_tokens"
                },
                4096
              ]
            },
            "system": {
              "$omitEmpty": {
                "$coalesce": [
                  {
                    "$ref": "request.instructions"
                  },
                  {
                    "$ref": "request.providerOptions.lxmone-claude.system"
                  }
                ]
              }
            },
            "messages": {
              "$filter": {
                "from": {
                  "$ref": "request.messages"
                },
                "as": "message",
                "where": {
                  "$ne": [
                    {
                      "$ref": "message.role"
                    },
                    "system"
                  ]
                }
              }
            },
            "temperature": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.temperature"
              }
            },
            "top_p": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.top_p"
              }
            },
            "top_k": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.top_k"
              }
            },
            "stop_sequences": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.stop_sequences"
              }
            },
            "tools": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.tools"
              }
            },
            "tool_choice": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.tool_choice"
              }
            },
            "metadata": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.metadata"
              }
            },
            "stream": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.stream"
              }
            },
            "thinking": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.thinking"
              }
            },
            "service_tier": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.service_tier"
              }
            }
          },
          "headers": {
            "anthropic-version": {
              "$coalesce": [
                {
                  "$ref": "request.providerOptions.lxmone-claude.anthropic-version"
                },
                "2023-06-01"
              ]
            },
            "anthropic-beta": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.lxmone-claude.anthropic-beta"
              }
            }
          }
        },
        "agent": {
          "method": "POST",
          "path": "/v1/messages",
          "contentType": "application/json",
          "body": {
            "$merge": [
              {
                "$ref": "request.extra.agent.claude"
              },
              {
                "model": {
                  "$ref": "request.model"
                }
              }
            ]
          },
          "headers": {
            "anthropic-version": "2023-06-01"
          }
        },
        "response": {
          "status": "succeeded",
          "text": {
            "$map": {
              "from": {
                "$filter": {
                  "from": {
                    "$ref": "response.content"
                  },
                  "as": "part",
                  "where": {
                    "$eq": [
                      {
                        "$ref": "part.type"
                      },
                      "text"
                    ]
                  }
                }
              },
              "as": "part",
              "in": {
                "$ref": "part.text"
              }
            }
          },
          "reasoning": {
            "$map": {
              "from": {
                "$filter": {
                  "from": {
                    "$ref": "response.content"
                  },
                  "as": "part",
                  "where": {
                    "$eq": [
                      {
                        "$ref": "part.type"
                      },
                      "thinking"
                    ]
                  }
                }
              },
              "as": "part",
              "in": {
                "$ref": "part.thinking"
              }
            }
          },
          "usage": {
            "$ref": "response.usage"
          },
          "errorPaths": [
            "error.message",
            "error.code",
            "error.type"
          ],
          "messagePaths": [
            "error.message"
          ]
        },
        "agentResponse": {
          "textPaths": [
            "content.0.text"
          ],
          "reasoningPaths": [
            "content.0.thinking"
          ],
          "toolCallsPath": "content",
          "toolCallIdPaths": [
            "id"
          ],
          "toolCallNamePaths": [
            "name"
          ],
          "toolCallArgumentsPaths": [
            "input"
          ]
        }
      },
      {
        "id": "lxmone-images",
        "label": "万有引力图片（生成 / 编辑）",
        "capabilities": [
          "image"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "图片模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "图片提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "provider image/reference fields",
            "description": "参考图或编辑源图，role 由业务层确定。"
          },
          {
            "name": "imageCount",
            "type": "integer",
            "required": false,
            "mapping": "n/sample_count",
            "description": "输出数量。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "size/aspect_ratio",
            "description": "比例或尺寸，语义按协议说明。"
          }
        ],
        "imageParameters": [
          {
            "name": "size",
            "label": "画幅 / 像素尺寸",
            "type": "string",
            "binding": "aspectRatio",
            "encoding": "pixels",
            "pricing": true
          },
          {
            "name": "count",
            "label": "生成数量",
            "type": "integer",
            "binding": "imageCount",
            "min": 1,
            "max": 100,
            "step": 1,
            "default": 1
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/images/generations",
          "pathTemplate": {
            "$if": {
              "condition": {
                "$gt": [
                  {
                    "$len": {
                      "$ref": "request.images"
                    }
                  },
                  0
                ]
              },
              "then": "/v1/images/edits",
              "else": "/v1/images/generations"
            }
          },
          "contentType": "application/json",
          "contentTypeTemplate": {
            "$if": {
              "condition": {
                "$gt": [
                  {
                    "$len": {
                      "$ref": "request.images"
                    }
                  },
                  0
                ]
              },
              "then": "multipart/form-data",
              "else": "application/json"
            }
          },
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "n": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.imageCount"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.imageCount"
                },
                "else": 1
              }
            },
            "size": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "1024x1024"
              ]
            },
            "response_format": "url"
          },
          "files": [
            {
              "name": "image[]",
              "source": {
                "$sortByOrder": {
                  "$ref": "request.images"
                }
              }
            }
          ]
        },
        "validations": [
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.images"
                  }
                },
                3
              ]
            },
            "message": "万有引力图片编辑最多支持 3 张源图。"
          },
          {
            "assert": {
              "$or": [
                {
                  "$not": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "qwen-2.1",
                        "Seedream-5-Pro",
                        "Seedream-5-Pro-2K"
                      ]
                    ]
                  }
                },
                {
                  "$lte": [
                    {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.imageCount"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.imageCount"
                        },
                        "else": 1
                      }
                    },
                    1
                  ]
                }
              ]
            },
            "message": "Qwen / Seedream 每次仅支持 n=1。"
          },
          {
            "assert": {
              "$eq": [
                {
                  "$len": {
                    "$filter": {
                      "from": {
                        "$ref": "request.images"
                      },
                      "as": "media",
                      "where": {
                        "$eq": [
                          {
                            "$ref": "media.role"
                          },
                          "mask"
                        ]
                      }
                    }
                  }
                },
                0
              ]
            },
            "message": "当前万有引力图片接入不支持 mask，请使用普通参考图编辑。"
          },
          {
            "assert": {
              "$or": [
                {
                  "$ne": [
                    {
                      "$ref": "request.model"
                    },
                    "gpt-image-2a"
                  ]
                },
                {
                  "$ne": [
                    {
                      "$ref": "request.aspectRatio"
                    },
                    "4096x4096"
                  ]
                }
              ]
            },
            "message": "gpt-image-2a 不支持 4096x4096；原生 4K 请填写 3840x2160。"
          }
        ],
        "response": {
          "status": "succeeded",
          "images": {
            "$map": {
              "from": {
                "$ref": "response.data"
              },
              "as": "item",
              "in": {
                "url": {
                  "$omitEmpty": {
                    "$ref": "item.url"
                  }
                },
                "dataUrl": {
                  "$if": {
                    "condition": {
                      "$ref": "item.b64_json"
                    },
                    "then": {
                      "$concat": [
                        "data:image/png;base64,",
                        {
                          "$ref": "item.b64_json"
                        }
                      ]
                    },
                    "else": null
                  }
                }
              }
            }
          },
          "usage": {
            "$ref": "response.usage"
          },
          "errorPaths": [
            "error.message",
            "error.code",
            "error.type"
          ],
          "messagePaths": [
            "error.message"
          ],
          "resultEphemeral": true
        }
      },
      {
        "id": "lxmone-images-async",
        "label": "万有引力图片（异步生成 / 编辑）",
        "capabilities": [
          "image"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "图片模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "图片提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "provider image/reference fields",
            "description": "参考图或编辑源图，role 由业务层确定。"
          },
          {
            "name": "imageCount",
            "type": "integer",
            "required": false,
            "mapping": "n/sample_count",
            "description": "输出数量。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "size/aspect_ratio",
            "description": "比例或尺寸，语义按协议说明。"
          }
        ],
        "imageParameters": [
          {
            "name": "size",
            "label": "画幅 / 像素尺寸",
            "type": "string",
            "binding": "aspectRatio",
            "encoding": "pixels",
            "pricing": true
          },
          {
            "name": "count",
            "label": "生成数量",
            "type": "integer",
            "binding": "imageCount",
            "min": 1,
            "max": 100,
            "step": 1,
            "default": 1
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/images/generations/async",
          "pathTemplate": {
            "$if": {
              "condition": {
                "$gt": [
                  {
                    "$len": {
                      "$ref": "request.images"
                    }
                  },
                  0
                ]
              },
              "then": "/v1/images/edits/async",
              "else": "/v1/images/generations/async"
            }
          },
          "contentType": "application/json",
          "contentTypeTemplate": {
            "$if": {
              "condition": {
                "$gt": [
                  {
                    "$len": {
                      "$ref": "request.images"
                    }
                  },
                  0
                ]
              },
              "then": "multipart/form-data",
              "else": "application/json"
            }
          },
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "prompt": {
              "$ref": "request.prompt"
            },
            "n": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$ref": "request.imageCount"
                    },
                    0
                  ]
                },
                "then": {
                  "$ref": "request.imageCount"
                },
                "else": 1
              }
            },
            "size": {
              "$coalesce": [
                {
                  "$ref": "request.aspectRatio"
                },
                "1024x1024"
              ]
            },
            "response_format": "url"
          },
          "files": [
            {
              "name": "image[]",
              "source": {
                "$sortByOrder": {
                  "$ref": "request.images"
                }
              }
            }
          ]
        },
        "validations": [
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.images"
                  }
                },
                3
              ]
            },
            "message": "万有引力图片编辑最多支持 3 张源图。"
          },
          {
            "assert": {
              "$or": [
                {
                  "$not": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "qwen-2.1",
                        "Seedream-5-Pro",
                        "Seedream-5-Pro-2K"
                      ]
                    ]
                  }
                },
                {
                  "$lte": [
                    {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.imageCount"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.imageCount"
                        },
                        "else": 1
                      }
                    },
                    1
                  ]
                }
              ]
            },
            "message": "Qwen / Seedream 每次仅支持 n=1。"
          },
          {
            "assert": {
              "$eq": [
                {
                  "$len": {
                    "$filter": {
                      "from": {
                        "$ref": "request.images"
                      },
                      "as": "media",
                      "where": {
                        "$eq": [
                          {
                            "$ref": "media.role"
                          },
                          "mask"
                        ]
                      }
                    }
                  }
                },
                0
              ]
            },
            "message": "当前万有引力图片接入不支持 mask，请使用普通参考图编辑。"
          },
          {
            "assert": {
              "$or": [
                {
                  "$ne": [
                    {
                      "$ref": "request.model"
                    },
                    "gpt-image-2a"
                  ]
                },
                {
                  "$ne": [
                    {
                      "$ref": "request.aspectRatio"
                    },
                    "4096x4096"
                  ]
                }
              ]
            },
            "message": "gpt-image-2a 不支持 4096x4096；原生 4K 请填写 3840x2160。"
          }
        ],
        "response": {
          "images": {
            "$map": {
              "from": {
                "$coalesce": [
                  {
                    "$ref": "response.result.data"
                  },
                  {
                    "$ref": "response.data"
                  },
                  {
                    "$if": {
                      "condition": {
                        "$ref": "response.image_url"
                      },
                      "then": [
                        {
                          "url": {
                            "$ref": "response.image_url"
                          }
                        }
                      ],
                      "else": []
                    }
                  }
                ]
              },
              "as": "item",
              "in": {
                "url": {
                  "$omitEmpty": {
                    "$ref": "item.url"
                  }
                },
                "dataUrl": {
                  "$if": {
                    "condition": {
                      "$ref": "item.b64_json"
                    },
                    "then": {
                      "$concat": [
                        "data:image/png;base64,",
                        {
                          "$ref": "item.b64_json"
                        }
                      ]
                    },
                    "else": null
                  }
                }
              }
            }
          },
          "usage": {
            "$ref": "response.usage"
          },
          "errorPaths": [
            "error.message",
            "error.code",
            "error.type"
          ],
          "messagePaths": [
            "error.message",
            "error",
            "message"
          ],
          "resultEphemeral": true,
          "taskIdPaths": [
            "task_id",
            "id"
          ],
          "statusPaths": [
            "status"
          ]
        },
        "poll": {
          "method": "GET",
          "path": "/v1/images/tasks/{{taskId}}"
        }
      },
      {
        "id": "lxmone-video-current",
        "label": "万有引力视频（当前模型自动路由）",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://lxmone.xyz/v1",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "上游模型 ID。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "prompt",
            "description": "视频提示词。"
          },
          {
            "name": "images",
            "type": "media[]",
            "required": false,
            "mapping": "reference_images",
            "description": "参考图片，宿主发布为公开 URL 后提交。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "required": false,
            "mapping": "reference_videos",
            "description": "参考视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": false,
            "mapping": "reference_audios",
            "description": "参考音频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": false,
            "mapping": "seconds/duration/duration_seconds",
            "description": "输出时长，单位秒。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "required": false,
            "mapping": "aspect_ratio",
            "description": "画幅比例。"
          },
          {
            "name": "resolution",
            "type": "string",
            "required": false,
            "mapping": "resolution/size",
            "description": "输出分辨率档位。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "required": false,
            "mapping": "audio/generate_audio",
            "description": "是否生成或保留音频。"
          }
        ],
        "description": "按完整模型 ID 区分当前 A/B 协议与参考素材字段；不猜测未知模型。",
        "create": {
          "method": "POST",
          "path": "/v1/videos",
          "pathTemplate": {
            "$if": {
              "condition": {
                "$in": [
                  {
                    "$ref": "request.model"
                  },
                  [
                    "grok-imagine-video-1.5-fast",
                    "grok-imagine-video-1.5-lite",
                    "grok-imagine-video-1.5（按次）",
                    "minimax-h3-a",
                    "minimax-h3-b",
                    "minimax-h3-c",
                    "seedance-2-0-fast-hc",
                    "seedance-2-0-hc",
                    "seedance-2-0-mini-hc",
                    "seedance-2-5-hc"
                  ]
                ]
              },
              "then": "/v1/videos/generations",
              "else": "/v1/videos"
            }
          },
          "contentType": "application/json",
          "body": {
            "$switch": {
              "cases": [
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "grok-imagine-video-1.5-fast",
                        "grok-imagine-video-1.5-lite",
                        "grok-imagine-video-1.5（按次）"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "seconds": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 6
                      }
                    },
                    "size": {
                      "$coalesce": [
                        {
                          "$ref": "request.providerOptions.lxmone-video-current.size"
                        },
                        "1280x720"
                      ]
                    },
                    "aspect_ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "reference_images": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "url": {
                              "$ref": "media.value"
                            },
                            "role": {
                              "$if": {
                                "condition": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "grok-imagine-video-1.5-lite"
                                    ]
                                  ]
                                },
                                "then": "first_frame",
                                "else": "reference_image"
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "kling-4.0"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "duration": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "resolution": {
                      "$coalesce": [
                        {
                          "$ref": "request.resolution"
                        },
                        {
                          "$switch": {
                            "cases": [
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "grok-imagine-video-1.5-fast",
                                      "grok-imagine-video-1.5-lite",
                                      "grok-imagine-video-1.5（按次）",
                                      "kling-4.0",
                                      "minimax-h3",
                                      "minimax-h3-a",
                                      "minimax-h3-b",
                                      "minimax-h3-c",
                                      "omni-flash-720p",
                                      "sd-2.5-720p-plus",
                                      "sd-2.5-720p-pro",
                                      "sd2.0-720p",
                                      "seedance-2-0-fast-hc",
                                      "seedance-2-0-hc",
                                      "seedance-2-0-mini-hc",
                                      "seedance-2-5-hc",
                                      "seedance2.0-720p",
                                      "seedance2.0-933",
                                      "seedance2.0-933-V",
                                      "seedance2.5",
                                      "seedance2.5-1",
                                      "seedance2.5-1080s",
                                      "seedance2.5-720",
                                      "seedance2.5-720s",
                                      "seedance2.5-k",
                                      "seedance2.5-w",
                                      "seedance2.5v",
                                      "wan-3.0",
                                      "wan3.0-video-p",
                                      "wan3.0-video-prime-p",
                                      "wan30-720p"
                                    ]
                                  ]
                                },
                                "then": "720p"
                              },
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "sd-2.5-480p-plus",
                                      "sd-2.5-480p-pro",
                                      "sd2.0-480p",
                                      "seedance2.5-480",
                                      "seedance2.5-480s"
                                    ]
                                  ]
                                },
                                "then": "480p"
                              }
                            ],
                            "default": "720p"
                          }
                        }
                      ]
                    },
                    "ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "referenceImages": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "referenceVideos": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.videos"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "referenceAudios": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "minimax-h3",
                        "sd-2.5-480p-plus",
                        "sd-2.5-480p-pro",
                        "sd-2.5-720p-plus",
                        "sd-2.5-720p-pro",
                        "sd2.0-480p",
                        "sd2.0-720p",
                        "seedance2.0-720p",
                        "seedance2.5-1",
                        "seedance2.5-1080s",
                        "seedance2.5-k",
                        "seedance2.5-w",
                        "wan-3.0"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "duration": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "resolution": {
                      "$coalesce": [
                        {
                          "$ref": "request.resolution"
                        },
                        {
                          "$switch": {
                            "cases": [
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "grok-imagine-video-1.5-fast",
                                      "grok-imagine-video-1.5-lite",
                                      "grok-imagine-video-1.5（按次）",
                                      "kling-4.0",
                                      "minimax-h3",
                                      "minimax-h3-a",
                                      "minimax-h3-b",
                                      "minimax-h3-c",
                                      "omni-flash-720p",
                                      "sd-2.5-720p-plus",
                                      "sd-2.5-720p-pro",
                                      "sd2.0-720p",
                                      "seedance-2-0-fast-hc",
                                      "seedance-2-0-hc",
                                      "seedance-2-0-mini-hc",
                                      "seedance-2-5-hc",
                                      "seedance2.0-720p",
                                      "seedance2.0-933",
                                      "seedance2.0-933-V",
                                      "seedance2.5",
                                      "seedance2.5-1",
                                      "seedance2.5-1080s",
                                      "seedance2.5-720",
                                      "seedance2.5-720s",
                                      "seedance2.5-k",
                                      "seedance2.5-w",
                                      "seedance2.5v",
                                      "wan-3.0",
                                      "wan3.0-video-p",
                                      "wan3.0-video-prime-p",
                                      "wan30-720p"
                                    ]
                                  ]
                                },
                                "then": "720p"
                              },
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "sd-2.5-480p-plus",
                                      "sd-2.5-480p-pro",
                                      "sd2.0-480p",
                                      "seedance2.5-480",
                                      "seedance2.5-480s"
                                    ]
                                  ]
                                },
                                "then": "480p"
                              }
                            ],
                            "default": "720p"
                          }
                        }
                      ]
                    },
                    "aspect_ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "reference_images": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_videos": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.videos"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_audios": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "minimax-h3-a",
                        "minimax-h3-b",
                        "minimax-h3-c"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "duration_seconds": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "resolution": {
                      "$coalesce": [
                        {
                          "$ref": "request.resolution"
                        },
                        {
                          "$switch": {
                            "cases": [
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "grok-imagine-video-1.5-fast",
                                      "grok-imagine-video-1.5-lite",
                                      "grok-imagine-video-1.5（按次）",
                                      "kling-4.0",
                                      "minimax-h3",
                                      "minimax-h3-a",
                                      "minimax-h3-b",
                                      "minimax-h3-c",
                                      "omni-flash-720p",
                                      "sd-2.5-720p-plus",
                                      "sd-2.5-720p-pro",
                                      "sd2.0-720p",
                                      "seedance-2-0-fast-hc",
                                      "seedance-2-0-hc",
                                      "seedance-2-0-mini-hc",
                                      "seedance-2-5-hc",
                                      "seedance2.0-720p",
                                      "seedance2.0-933",
                                      "seedance2.0-933-V",
                                      "seedance2.5",
                                      "seedance2.5-1",
                                      "seedance2.5-1080s",
                                      "seedance2.5-720",
                                      "seedance2.5-720s",
                                      "seedance2.5-k",
                                      "seedance2.5-w",
                                      "seedance2.5v",
                                      "wan-3.0",
                                      "wan3.0-video-p",
                                      "wan3.0-video-prime-p",
                                      "wan30-720p"
                                    ]
                                  ]
                                },
                                "then": "720p"
                              },
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "sd-2.5-480p-plus",
                                      "sd-2.5-480p-pro",
                                      "sd2.0-480p",
                                      "seedance2.5-480",
                                      "seedance2.5-480s"
                                    ]
                                  ]
                                },
                                "then": "480p"
                              }
                            ],
                            "default": "720p"
                          }
                        }
                      ]
                    },
                    "aspect_ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "reference_images": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_audios": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "mode": {
                      "$omitEmpty": {
                        "$if": {
                          "condition": {
                            "$in": [
                              {
                                "$ref": "request.model"
                              },
                              [
                                "minimax-h3-b",
                                "minimax-h3-c"
                              ]
                            ]
                          },
                          "then": "高质模式",
                          "else": null
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "omni-flash-720p",
                        "seedance2.0-933-V",
                        "seedance2.5",
                        "wan30-720p"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "seconds": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "resolution": {
                      "$coalesce": [
                        {
                          "$ref": "request.resolution"
                        },
                        {
                          "$switch": {
                            "cases": [
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "grok-imagine-video-1.5-fast",
                                      "grok-imagine-video-1.5-lite",
                                      "grok-imagine-video-1.5（按次）",
                                      "kling-4.0",
                                      "minimax-h3",
                                      "minimax-h3-a",
                                      "minimax-h3-b",
                                      "minimax-h3-c",
                                      "omni-flash-720p",
                                      "sd-2.5-720p-plus",
                                      "sd-2.5-720p-pro",
                                      "sd2.0-720p",
                                      "seedance-2-0-fast-hc",
                                      "seedance-2-0-hc",
                                      "seedance-2-0-mini-hc",
                                      "seedance-2-5-hc",
                                      "seedance2.0-720p",
                                      "seedance2.0-933",
                                      "seedance2.0-933-V",
                                      "seedance2.5",
                                      "seedance2.5-1",
                                      "seedance2.5-1080s",
                                      "seedance2.5-720",
                                      "seedance2.5-720s",
                                      "seedance2.5-k",
                                      "seedance2.5-w",
                                      "seedance2.5v",
                                      "wan-3.0",
                                      "wan3.0-video-p",
                                      "wan3.0-video-prime-p",
                                      "wan30-720p"
                                    ]
                                  ]
                                },
                                "then": "720p"
                              },
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "sd-2.5-480p-plus",
                                      "sd-2.5-480p-pro",
                                      "sd2.0-480p",
                                      "seedance2.5-480",
                                      "seedance2.5-480s"
                                    ]
                                  ]
                                },
                                "then": "480p"
                              }
                            ],
                            "default": "720p"
                          }
                        }
                      ]
                    },
                    "aspect_ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "reference_images": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_videos": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.videos"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_audios": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "seedance-2-0-fast-hc",
                        "seedance-2-0-hc",
                        "seedance-2-0-mini-hc",
                        "seedance-2-5-hc"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "duration_seconds": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "resolution": {
                      "$coalesce": [
                        {
                          "$ref": "request.resolution"
                        },
                        {
                          "$switch": {
                            "cases": [
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "grok-imagine-video-1.5-fast",
                                      "grok-imagine-video-1.5-lite",
                                      "grok-imagine-video-1.5（按次）",
                                      "kling-4.0",
                                      "minimax-h3",
                                      "minimax-h3-a",
                                      "minimax-h3-b",
                                      "minimax-h3-c",
                                      "omni-flash-720p",
                                      "sd-2.5-720p-plus",
                                      "sd-2.5-720p-pro",
                                      "sd2.0-720p",
                                      "seedance-2-0-fast-hc",
                                      "seedance-2-0-hc",
                                      "seedance-2-0-mini-hc",
                                      "seedance-2-5-hc",
                                      "seedance2.0-720p",
                                      "seedance2.0-933",
                                      "seedance2.0-933-V",
                                      "seedance2.5",
                                      "seedance2.5-1",
                                      "seedance2.5-1080s",
                                      "seedance2.5-720",
                                      "seedance2.5-720s",
                                      "seedance2.5-k",
                                      "seedance2.5-w",
                                      "seedance2.5v",
                                      "wan-3.0",
                                      "wan3.0-video-p",
                                      "wan3.0-video-prime-p",
                                      "wan30-720p"
                                    ]
                                  ]
                                },
                                "then": "720p"
                              },
                              {
                                "when": {
                                  "$in": [
                                    {
                                      "$ref": "request.model"
                                    },
                                    [
                                      "sd-2.5-480p-plus",
                                      "sd-2.5-480p-pro",
                                      "sd2.0-480p",
                                      "seedance2.5-480",
                                      "seedance2.5-480s"
                                    ]
                                  ]
                                },
                                "then": "480p"
                              }
                            ],
                            "default": "720p"
                          }
                        }
                      ]
                    },
                    "aspect_ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "reference_images": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_videos": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.videos"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "reference_audios": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "generate_audio": {
                      "$ref": "request.generateAudio"
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "seedance2.0-933"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "seconds": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "image_urls": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "video_urls": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.videos"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "audio_urls": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "seedance2.5-480",
                        "seedance2.5-480s",
                        "seedance2.5-720",
                        "seedance2.5-720s"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "duration": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 8
                      }
                    },
                    "size": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "assets": {
                      "$omitEmpty": {
                        "$concatArrays": [
                          {
                            "$map": {
                              "from": {
                                "$sortByOrder": {
                                  "$ref": "request.images"
                                }
                              },
                              "as": "media",
                              "in": {
                                "category": "image",
                                "url": {
                                  "$omitEmpty": {
                                    "$ref": "media.url"
                                  }
                                },
                                "data_base64": {
                                  "$omitEmpty": {
                                    "$if": {
                                      "condition": {
                                        "$not": {
                                          "$ref": "media.url"
                                        }
                                      },
                                      "then": {
                                        "$dataPayload": {
                                          "$ref": "media.dataUrl"
                                        }
                                      },
                                      "else": null
                                    }
                                  }
                                }
                              }
                            }
                          },
                          {
                            "$map": {
                              "from": {
                                "$sortByOrder": {
                                  "$ref": "request.audios"
                                }
                              },
                              "as": "media",
                              "in": {
                                "category": "audio",
                                "url": {
                                  "$ref": "media.value"
                                }
                              }
                            }
                          }
                        ]
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "seedance2.5v"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "prompt": {
                      "$ref": "request.prompt"
                    },
                    "seconds": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "reference_images": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.images"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "ratio": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "videos": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.videos"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    },
                    "audios": {
                      "$omitEmpty": {
                        "$map": {
                          "from": {
                            "$sortByOrder": {
                              "$ref": "request.audios"
                            }
                          },
                          "as": "media",
                          "in": {
                            "$ref": "media.value"
                          }
                        }
                      }
                    }
                  }
                },
                {
                  "when": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "wan3.0-video-p",
                        "wan3.0-video-prime-p"
                      ]
                    ]
                  },
                  "then": {
                    "model": {
                      "$ref": "request.model"
                    },
                    "duration": {
                      "$if": {
                        "condition": {
                          "$gt": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        "then": {
                          "$ref": "request.duration"
                        },
                        "else": 5
                      }
                    },
                    "resolution": {
                      "$upper": {
                        "$coalesce": [
                          {
                            "$ref": "request.resolution"
                          },
                          {
                            "$switch": {
                              "cases": [
                                {
                                  "when": {
                                    "$in": [
                                      {
                                        "$ref": "request.model"
                                      },
                                      [
                                        "grok-imagine-video-1.5-fast",
                                        "grok-imagine-video-1.5-lite",
                                        "grok-imagine-video-1.5（按次）",
                                        "kling-4.0",
                                        "minimax-h3",
                                        "minimax-h3-a",
                                        "minimax-h3-b",
                                        "minimax-h3-c",
                                        "omni-flash-720p",
                                        "sd-2.5-720p-plus",
                                        "sd-2.5-720p-pro",
                                        "sd2.0-720p",
                                        "seedance-2-0-fast-hc",
                                        "seedance-2-0-hc",
                                        "seedance-2-0-mini-hc",
                                        "seedance-2-5-hc",
                                        "seedance2.0-720p",
                                        "seedance2.0-933",
                                        "seedance2.0-933-V",
                                        "seedance2.5",
                                        "seedance2.5-1",
                                        "seedance2.5-1080s",
                                        "seedance2.5-720",
                                        "seedance2.5-720s",
                                        "seedance2.5-k",
                                        "seedance2.5-w",
                                        "seedance2.5v",
                                        "wan-3.0",
                                        "wan3.0-video-p",
                                        "wan3.0-video-prime-p",
                                        "wan30-720p"
                                      ]
                                    ]
                                  },
                                  "then": "720p"
                                },
                                {
                                  "when": {
                                    "$in": [
                                      {
                                        "$ref": "request.model"
                                      },
                                      [
                                        "sd-2.5-480p-plus",
                                        "sd-2.5-480p-pro",
                                        "sd2.0-480p",
                                        "seedance2.5-480",
                                        "seedance2.5-480s"
                                      ]
                                    ]
                                  },
                                  "then": "480p"
                                }
                              ],
                              "default": "720p"
                            }
                          }
                        ]
                      }
                    },
                    "input": {
                      "prompt": {
                        "$ref": "request.prompt"
                      },
                      "media": {
                        "$omitEmpty": {
                          "$concatArrays": [
                            {
                              "$map": {
                                "from": {
                                  "$sortByOrder": {
                                    "$ref": "request.images"
                                  }
                                },
                                "as": "media",
                                "in": {
                                  "type": "reference_image",
                                  "url": {
                                    "$ref": "media.value"
                                  }
                                }
                              }
                            },
                            {
                              "$map": {
                                "from": {
                                  "$sortByOrder": {
                                    "$ref": "request.videos"
                                  }
                                },
                                "as": "media",
                                "in": {
                                  "type": "reference_video",
                                  "url": {
                                    "$ref": "media.value"
                                  }
                                }
                              }
                            },
                            {
                              "$map": {
                                "from": {
                                  "$sortByOrder": {
                                    "$ref": "request.audios"
                                  }
                                },
                                "as": "media",
                                "in": {
                                  "type": "reference_audio",
                                  "url": {
                                    "$ref": "media.value"
                                  }
                                }
                              }
                            }
                          ]
                        }
                      }
                    },
                    "parameters": {
                      "duration": {
                        "$if": {
                          "condition": {
                            "$gt": [
                              {
                                "$ref": "request.duration"
                              },
                              0
                            ]
                          },
                          "then": {
                            "$ref": "request.duration"
                          },
                          "else": 5
                        }
                      },
                      "resolution": {
                        "$upper": {
                          "$coalesce": [
                            {
                              "$ref": "request.resolution"
                            },
                            {
                              "$switch": {
                                "cases": [
                                  {
                                    "when": {
                                      "$in": [
                                        {
                                          "$ref": "request.model"
                                        },
                                        [
                                          "grok-imagine-video-1.5-fast",
                                          "grok-imagine-video-1.5-lite",
                                          "grok-imagine-video-1.5（按次）",
                                          "kling-4.0",
                                          "minimax-h3",
                                          "minimax-h3-a",
                                          "minimax-h3-b",
                                          "minimax-h3-c",
                                          "omni-flash-720p",
                                          "sd-2.5-720p-plus",
                                          "sd-2.5-720p-pro",
                                          "sd2.0-720p",
                                          "seedance-2-0-fast-hc",
                                          "seedance-2-0-hc",
                                          "seedance-2-0-mini-hc",
                                          "seedance-2-5-hc",
                                          "seedance2.0-720p",
                                          "seedance2.0-933",
                                          "seedance2.0-933-V",
                                          "seedance2.5",
                                          "seedance2.5-1",
                                          "seedance2.5-1080s",
                                          "seedance2.5-720",
                                          "seedance2.5-720s",
                                          "seedance2.5-k",
                                          "seedance2.5-w",
                                          "seedance2.5v",
                                          "wan-3.0",
                                          "wan3.0-video-p",
                                          "wan3.0-video-prime-p",
                                          "wan30-720p"
                                        ]
                                      ]
                                    },
                                    "then": "720p"
                                  },
                                  {
                                    "when": {
                                      "$in": [
                                        {
                                          "$ref": "request.model"
                                        },
                                        [
                                          "sd-2.5-480p-plus",
                                          "sd-2.5-480p-pro",
                                          "sd2.0-480p",
                                          "seedance2.5-480",
                                          "seedance2.5-480s"
                                        ]
                                      ]
                                    },
                                    "then": "480p"
                                  }
                                ],
                                "default": "720p"
                              }
                            }
                          ]
                        }
                      },
                      "ratio": {
                        "$coalesce": [
                          {
                            "$ref": "request.aspectRatio"
                          },
                          "adaptive"
                        ]
                      }
                    }
                  }
                }
              ],
              "default": null
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}"
        },
        "result": {
          "method": "GET",
          "path": "/v1/videos/{{taskId}}/content"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.taskId"
              },
              {
                "$ref": "response.request_id"
              },
              {
                "$ref": "response.data.id"
              }
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$ref": "response.state"
              },
              {
                "$ref": "response.data.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.error.message"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.fail_reason"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.metadata.direct_url"
              },
              {
                "$ref": "response.data.metadata.direct_url"
              }
            ]
          },
          "errorPaths": [
            "error.message",
            "error.code"
          ],
          "resultEphemeral": true,
          "usage": {
            "$ref": "response.usage"
          }
        },
        "validations": [
          {
            "assert": {
              "$in": [
                {
                  "$ref": "request.model"
                },
                [
                  "grok-imagine-video-1.5-fast",
                  "grok-imagine-video-1.5-lite",
                  "grok-imagine-video-1.5（按次）",
                  "kling-4.0",
                  "minimax-h3",
                  "minimax-h3-a",
                  "minimax-h3-b",
                  "minimax-h3-c",
                  "omni-flash-720p",
                  "sd-2.5-480p-plus",
                  "sd-2.5-480p-pro",
                  "sd-2.5-720p-plus",
                  "sd-2.5-720p-pro",
                  "sd2.0-480p",
                  "sd2.0-720p",
                  "seedance-2-0-fast-hc",
                  "seedance-2-0-hc",
                  "seedance-2-0-mini-hc",
                  "seedance-2-5-hc",
                  "seedance2.0-720p",
                  "seedance2.0-933",
                  "seedance2.0-933-V",
                  "seedance2.5",
                  "seedance2.5-1",
                  "seedance2.5-1080s",
                  "seedance2.5-480",
                  "seedance2.5-480s",
                  "seedance2.5-720",
                  "seedance2.5-720s",
                  "seedance2.5-k",
                  "seedance2.5-w",
                  "seedance2.5v",
                  "wan-3.0",
                  "wan3.0-video-p",
                  "wan3.0-video-prime-p",
                  "wan30-720p"
                ]
              ]
            },
            "message": "此模型尚无已核对的万有引力视频映射，请按最新文档更新插件，勿改用近似模型名。"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.images"
                  }
                },
                {
                  "$switch": {
                    "cases": [
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "grok-imagine-video-1.5-fast",
                              "grok-imagine-video-1.5（按次）",
                              "omni-flash-720p"
                            ]
                          ]
                        },
                        "then": 7
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "grok-imagine-video-1.5-lite"
                            ]
                          ]
                        },
                        "then": 1
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "kling-4.0",
                              "wan-3.0",
                              "wan3.0-video-p",
                              "wan30-720p"
                            ]
                          ]
                        },
                        "then": 10
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "minimax-h3",
                              "minimax-h3-b",
                              "minimax-h3-c",
                              "sd2.0-480p",
                              "sd2.0-720p",
                              "seedance2.0-720p",
                              "seedance2.0-933-V"
                            ]
                          ]
                        },
                        "then": 9
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "minimax-h3-a"
                            ]
                          ]
                        },
                        "then": 0
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "sd-2.5-480p-plus",
                              "sd-2.5-480p-pro",
                              "sd-2.5-720p-plus",
                              "sd-2.5-720p-pro",
                              "seedance2.5",
                              "seedance2.5-1",
                              "seedance2.5-1080s",
                              "seedance2.5-480",
                              "seedance2.5-480s",
                              "seedance2.5-720",
                              "seedance2.5-720s",
                              "seedance2.5-k",
                              "seedance2.5-w",
                              "seedance2.5v"
                            ]
                          ]
                        },
                        "then": 30
                      }
                    ],
                    "default": 1000
                  }
                }
              ]
            },
            "message": "当前模型的参考images数量超过公开文档限制。"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.videos"
                  }
                },
                {
                  "$switch": {
                    "cases": [
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "grok-imagine-video-1.5-fast",
                              "grok-imagine-video-1.5-lite",
                              "grok-imagine-video-1.5（按次）",
                              "minimax-h3",
                              "minimax-h3-a",
                              "minimax-h3-b",
                              "minimax-h3-c",
                              "sd-2.5-480p-pro",
                              "sd-2.5-720p-pro"
                            ]
                          ]
                        },
                        "then": 0
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "kling-4.0",
                              "wan-3.0",
                              "wan30-720p"
                            ]
                          ]
                        },
                        "then": 5
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "omni-flash-720p",
                              "sd2.0-480p",
                              "sd2.0-720p",
                              "seedance2.0-720p",
                              "seedance2.0-933-V"
                            ]
                          ]
                        },
                        "then": 3
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "sd-2.5-480p-plus",
                              "sd-2.5-720p-plus",
                              "seedance2.5",
                              "seedance2.5-1",
                              "seedance2.5-1080s",
                              "seedance2.5-480",
                              "seedance2.5-480s",
                              "seedance2.5-720",
                              "seedance2.5-720s",
                              "seedance2.5-k",
                              "seedance2.5-w",
                              "seedance2.5v",
                              "wan3.0-video-p"
                            ]
                          ]
                        },
                        "then": 10
                      }
                    ],
                    "default": 1000
                  }
                }
              ]
            },
            "message": "当前模型的参考videos数量超过公开文档限制。"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.audios"
                  }
                },
                {
                  "$switch": {
                    "cases": [
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "grok-imagine-video-1.5-fast",
                              "grok-imagine-video-1.5-lite",
                              "grok-imagine-video-1.5（按次）",
                              "minimax-h3-a",
                              "minimax-h3-b"
                            ]
                          ]
                        },
                        "then": 0
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "kling-4.0",
                              "minimax-h3",
                              "minimax-h3-c",
                              "omni-flash-720p",
                              "sd2.0-480p",
                              "sd2.0-720p",
                              "seedance2.0-720p",
                              "seedance2.0-933-V"
                            ]
                          ]
                        },
                        "then": 3
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "sd-2.5-480p-plus",
                              "sd-2.5-480p-pro",
                              "sd-2.5-720p-plus",
                              "sd-2.5-720p-pro",
                              "seedance2.5",
                              "seedance2.5-1",
                              "seedance2.5-1080s",
                              "seedance2.5-480",
                              "seedance2.5-480s",
                              "seedance2.5-720",
                              "seedance2.5-720s",
                              "seedance2.5-k",
                              "seedance2.5-w",
                              "seedance2.5v",
                              "wan3.0-video-p"
                            ]
                          ]
                        },
                        "then": 10
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "wan-3.0",
                              "wan30-720p"
                            ]
                          ]
                        },
                        "then": 5
                      }
                    ],
                    "default": 1000
                  }
                }
              ]
            },
            "message": "当前模型的参考audios数量超过公开文档限制。"
          },
          {
            "assert": {
              "$or": [
                {
                  "$not": {
                    "$in": [
                      {
                        "$ref": "request.model"
                      },
                      [
                        "minimax-h3-b",
                        "minimax-h3-c",
                        "seedance2.5-1080s"
                      ]
                    ]
                  }
                },
                {
                  "$gt": [
                    {
                      "$len": {
                        "$ref": "request.images"
                      }
                    },
                    0
                  ]
                }
              ]
            },
            "message": "当前模型要求至少一张参考图片，不支持纯文生视频。"
          },
          {
            "assert": {
              "$switch": {
                "cases": [
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "grok-imagine-video-1.5-fast",
                          "grok-imagine-video-1.5-lite",
                          "grok-imagine-video-1.5（按次）",
                          "minimax-h3"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                1
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                15
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "kling-4.0"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                3
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                30
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "minimax-h3-a",
                          "minimax-h3-b",
                          "minimax-h3-c"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                3
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                15
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "omni-flash-720p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$ref": "request.duration"
                            },
                            [
                              5,
                              10
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "sd-2.5-480p-plus",
                          "sd-2.5-480p-pro",
                          "sd-2.5-720p-plus",
                          "sd-2.5-720p-pro",
                          "sd2.0-480p",
                          "sd2.0-720p",
                          "seedance2.0-720p",
                          "seedance2.5",
                          "seedance2.5-1",
                          "seedance2.5-480s",
                          "seedance2.5-720",
                          "seedance2.5-720s",
                          "seedance2.5-k",
                          "seedance2.5-w",
                          "seedance2.5v",
                          "wan30-720p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                4
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                30
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "seedance-2-0-fast-hc",
                          "seedance-2-0-hc",
                          "seedance-2-0-mini-hc",
                          "seedance-2-5-hc",
                          "seedance2.0-933",
                          "seedance2.5-1080s",
                          "wan3.0-video-p",
                          "wan3.0-video-prime-p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                1
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                30
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "seedance2.0-933-V"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                5
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                15
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "seedance2.5-480"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$and": [
                            {
                              "$gte": [
                                {
                                  "$ref": "request.duration"
                                },
                                4
                              ]
                            },
                            {
                              "$lte": [
                                {
                                  "$ref": "request.duration"
                                },
                                25
                              ]
                            }
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "wan-3.0"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.duration"
                            },
                            0
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$ref": "request.duration"
                            },
                            [
                              5,
                              10,
                              15,
                              30
                            ]
                          ]
                        }
                      ]
                    }
                  }
                ],
                "default": true
              }
            },
            "message": "duration 不在当前模型公开支持的范围内。"
          },
          {
            "assert": {
              "$switch": {
                "cases": [
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "grok-imagine-video-1.5-lite",
                          "seedance-2-0-fast-hc",
                          "seedance-2-0-hc",
                          "seedance-2-0-mini-hc",
                          "seedance-2-5-hc",
                          "seedance2.0-933-V",
                          "seedance2.5",
                          "seedance2.5-1080s",
                          "seedance2.5-w",
                          "wan-3.0"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.resolution"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.resolution"
                              }
                            },
                            [
                              "480p",
                              "720p"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "kling-4.0",
                          "seedance2.5-1"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.resolution"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.resolution"
                              }
                            },
                            [
                              "720p",
                              "1080p"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "minimax-h3-a",
                          "minimax-h3-b",
                          "minimax-h3-c"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.resolution"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.resolution"
                              }
                            },
                            [
                              "480p",
                              "720p",
                              "1080p",
                              "2k"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "omni-flash-720p",
                          "sd-2.5-720p-plus",
                          "sd-2.5-720p-pro",
                          "sd2.0-720p",
                          "seedance2.0-720p",
                          "seedance2.0-933",
                          "seedance2.5-720",
                          "seedance2.5-720s",
                          "wan30-720p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.resolution"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.resolution"
                              }
                            },
                            [
                              "720p"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "sd-2.5-480p-plus",
                          "sd-2.5-480p-pro",
                          "sd2.0-480p",
                          "seedance2.5-480",
                          "seedance2.5-480s"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.resolution"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.resolution"
                              }
                            },
                            [
                              "480p"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "seedance2.5-k",
                          "seedance2.5v",
                          "wan3.0-video-p",
                          "wan3.0-video-prime-p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.resolution"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.resolution"
                              }
                            },
                            [
                              "480p",
                              "720p",
                              "1080p"
                            ]
                          ]
                        }
                      ]
                    }
                  }
                ],
                "default": true
              }
            },
            "message": "resolution 不在当前模型公开支持的范围内。"
          },
          {
            "assert": {
              "$switch": {
                "cases": [
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "grok-imagine-video-1.5-lite"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.aspectRatio"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.aspectRatio"
                              }
                            },
                            [
                              "16:9",
                              "9:16",
                              "1:1",
                              "4:3",
                              "3:4"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "minimax-h3-a",
                          "minimax-h3-b",
                          "minimax-h3-c",
                          "seedance2.0-933-V",
                          "seedance2.5",
                          "seedance2.5-k"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.aspectRatio"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.aspectRatio"
                              }
                            },
                            [
                              "16:9",
                              "9:16"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "omni-flash-720p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.aspectRatio"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.aspectRatio"
                              }
                            },
                            [
                              "1:1",
                              "16:9",
                              "9:16",
                              "4:3",
                              "3:4"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "seedance2.5-480",
                          "seedance2.5-480s",
                          "seedance2.5-720",
                          "seedance2.5-720s"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.aspectRatio"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.aspectRatio"
                              }
                            },
                            [
                              "21:9",
                              "16:9",
                              "4:3",
                              "1:1",
                              "3:4",
                              "9:16"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "seedance2.5v"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.aspectRatio"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.aspectRatio"
                              }
                            },
                            [
                              "adaptive",
                              "16:9",
                              "9:16",
                              "1:1",
                              "4:3",
                              "3:4",
                              "21:9"
                            ]
                          ]
                        }
                      ]
                    }
                  },
                  {
                    "when": {
                      "$in": [
                        {
                          "$ref": "request.model"
                        },
                        [
                          "wan3.0-video-p",
                          "wan3.0-video-prime-p"
                        ]
                      ]
                    },
                    "then": {
                      "$or": [
                        {
                          "$eq": [
                            {
                              "$ref": "request.aspectRatio"
                            },
                            ""
                          ]
                        },
                        {
                          "$in": [
                            {
                              "$lower": {
                                "$ref": "request.aspectRatio"
                              }
                            },
                            [
                              "adaptive",
                              "16:9",
                              "9:16",
                              "1:1"
                            ]
                          ]
                        }
                      ]
                    }
                  }
                ],
                "default": true
              }
            },
            "message": "aspectRatio 不在当前模型公开支持的范围内。"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$add": [
                    {
                      "$len": {
                        "$ref": "request.images"
                      }
                    },
                    {
                      "$len": {
                        "$ref": "request.videos"
                      }
                    },
                    {
                      "$len": {
                        "$ref": "request.audios"
                      }
                    }
                  ]
                },
                {
                  "$switch": {
                    "cases": [
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "sd-2.5-480p-plus",
                              "sd-2.5-480p-pro",
                              "sd-2.5-720p-plus",
                              "sd-2.5-720p-pro",
                              "seedance2.5-k"
                            ]
                          ]
                        },
                        "then": 50
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "sd2.0-480p",
                              "sd2.0-720p",
                              "seedance2.0-720p"
                            ]
                          ]
                        },
                        "then": 15
                      },
                      {
                        "when": {
                          "$in": [
                            {
                              "$ref": "request.model"
                            },
                            [
                              "wan30-720p"
                            ]
                          ]
                        },
                        "then": 20
                      }
                    ],
                    "default": 1000
                  }
                }
              ]
            },
            "message": "参考素材总数超过当前模型公开限制。"
          }
        ]
      }
    ]
  },
  "documentation": "<当前插件的完整 documentation，由 README.md 与 docs/interface.md 拼接而成；为避免 JSON 递归，此处不重复展开正文。>"
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
