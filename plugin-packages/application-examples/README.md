# 应用插件 P01/P02 验收包

这些包通过应用插件 v3 安装入口登记版本与技能。P01 只验收安装；P02 增加可信 Host 操作执行，1.2.0 包用于完整验收读取和快照审批。

| 包 | 用途 |
| --- | --- |
| method-guide-1.0.0.yingce-plugin | 纯技能示例；无操作权限申请，启用后可在“我的技能”读取目标澄清方法 |
| resource-helper-1.0.0.yingce-plugin | 资源助手；授予 media.read 后加入 check-source 技能；P01 操作尚未开放，P02 可执行只读检查 |
| resource-helper-1.1.0.yingce-plugin | 同一资源助手的新发布；用于验证上传新版不会自动改变当前用户的安装版本 |
| resource-helper-1.2.0.yingce-plugin | P02 可执行样例：同步读取视频元数据，并在用户确认后保存结构化快照；不调用模型、不生成媒体 |

源文件分别在 `backend/internal/plugins/contracts/testdata/method-guide`、`resource-helper` 和 `resource-helper-p02`。1.1.0 仅覆盖发布版本，方法正文相同；1.2.0 才新增 P02 操作。

在 backend 目录可重新生成到一个**尚不存在**的输出文件：

```sh
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/method-guide -out /tmp/method-guide-check.yingce-plugin
go run ./cmd/plugin-contract -dir internal/plugins/contracts/testdata/resource-helper -version 1.1.0 -out /tmp/resource-helper-upgrade-check.yingce-plugin
```

生成器不会覆盖文件；输出不同 ZIP 时间/压缩元数据不影响包内容摘要。生产启动时只扫描 plugin-packages 顶层的旧协议包，本目录不会被当作内置协议自动装载。

P01 安装与版本步骤见 [P01 验收说明](../../docs/plans/qimu-plugin-p01-acceptance.md)；1.2.0 操作、审批与 Agent 验收见 [P02 验收说明](../../docs/plans/qimu-plugin-p02-acceptance.md)。
