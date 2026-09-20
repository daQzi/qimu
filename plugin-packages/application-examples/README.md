# 应用插件验收包与作者模板

新插件从 [当前开发指南](../../docs/content/docs/plugins/application-plugin-v3-integration.md) 的 `-init` 命令开始，支持 skill/resource 模板和 `-package` 成品包校验。`delivery-check` 收尾样例由模板生成，不含真实模型；验收见 [P14A](../../docs/plans/qimu-plugin-closeout-acceptance.md)。本目录已有版本继续作为历史阶段样例。

这些包通过应用插件 v3 安装入口登记版本与技能。P01 只验收安装；P02 增加可信 Host 操作执行，1.2.0 包用于完整验收读取和快照审批。

| 包 | 用途 |
| --- | --- |
| method-guide-1.0.0.yingce-plugin | 纯技能示例；无操作权限申请，启用后可在“我的技能”读取目标澄清方法 |
| resource-helper-1.0.0.yingce-plugin | 资源助手；授予 media.read 后加入 check-source 技能；P01 操作尚未开放，P02 可执行只读检查 |
| resource-helper-1.1.0.yingce-plugin | 同一资源助手的新发布；用于验证上传新版不会自动改变当前用户的安装版本 |
| resource-helper-1.2.0.yingce-plugin | P02 可执行样例：同步读取视频元数据，并在用户确认后保存结构化快照；不调用模型、不生成媒体 |
| resource-helper-1.3.0.yingce-plugin | P03 样例：校验读取摘要、结果视图、经确认保存到画布、重复绑定与冲突保护；需要另外授权 canvas.read/canvas.write |
| remote-helper-1.0.0.yingce-plugin | P04 远程同步/异步协议样例：需要配置连接与凭据，支持批准、轮询、导入、取消与恢复；不代表真实模型能力 |
| pipeline-helper-1.0.0.yingce-plugin | P05 顺序流程：视频元数据检查 → 用户输入 → 批准远程回显 → 结果；支持草稿、重启和历史继续 |
| batch-helper-1.0.0.yingce-plugin | P06 批次流程：稳定 ID 映射、条件跳过、最多两项一波、精确批次批准和局部派生复用 |

源文件分别在 `backend/internal/plugins/contracts/testdata/method-guide`、`resource-helper` 和 `resource-helper-p02`。1.1.0 仅覆盖发布版本，方法正文相同；1.2.0 才新增 P02 操作。

在 backend 目录可重新生成到一个**尚不存在**的输出文件：

```sh
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/method-guide -out /tmp/method-guide-check.yingce-plugin
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/resource-helper -version 1.1.0 -out /tmp/resource-helper-upgrade-check.yingce-plugin
```

生成器不会覆盖文件；输出不同 ZIP 时间/压缩元数据不影响包内容摘要。生产启动时只扫描 plugin-packages 顶层的旧协议包，本目录不会被当作内置协议自动装载。

P01 安装与版本步骤见 [P01 验收说明](../../docs/plans/qimu-plugin-p01-acceptance.md)；1.2.0 操作、审批与 Agent 验收见 [P02 验收说明](../../docs/plans/qimu-plugin-p02-acceptance.md)。

1.3.0 源文件位于 `backend/internal/plugins/contracts/testdata/resource-helper-p03/`；完整操作见 [P03 验收说明](../../docs/plans/qimu-plugin-p03-acceptance.md)。可使用 `go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/resource-helper-p03 -out /tmp/resource-helper-p03-check.yingce-plugin` 重新打包到不存在的文件。

远程样例源文件位于 `backend/internal/plugins/contracts/testdata/remote-helper-p04/`；本地演示命令、测试凭据和恢复验收见 [P04 验收说明](../../docs/plans/qimu-plugin-p04-acceptance.md)。包中没有真实 Key，默认 example.invalid 仅为占位，必须由用户配置服务地址。
## P05 顺序流程示例

`pipeline-helper-1.0.0.yingce-plugin`：检查当前用户的视频元数据 → 等待填写 prompt → 用户批准远程回显 → 持久结果。复用 P04 的本地演示服务，不包含视频处理模型。使用说明、Schema 合同、恢复与人工验收见 `docs/plans/qimu-plugin-p05-acceptance.md`。

## P06 批次流程示例

`batch-helper-1.0.0.yingce-plugin` 的源文件在 `backend/internal/plugins/contracts/testdata/batch-helper-p06/`，复用同一演示 API。表单按顺序提供 items，每项含 id/prompt/enabled；收费必须核对批准，跳过项输出 null。完整操作见 `docs/plans/qimu-plugin-p06-acceptance.md`。它验证通用批次机制，不包含视频生成模型或真实密钥。
