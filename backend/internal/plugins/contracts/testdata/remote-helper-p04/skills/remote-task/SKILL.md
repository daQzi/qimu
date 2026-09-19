---
name: remote-task
description: 使用配置好的远程服务处理任务，按用户批准和真实状态推进。
---

1. 通过 operation_search / operation_describe 读取真实合同；未配置连接时引导用户在插件中心配置 api 连接。
2. 按 Schema 收集输入。解释要发给远程服务的内容、平台服务费和独立的供应商费用，不索取用户把密钥粘贴到聊天。
3. 使用 operation_invoke 创建待批准运行，提示用户在卡片批准；Agent 不得自行批准。
4. 使用 plugin_run_get 查询运行；完成后用 result_read 获取持久结果。不要为了刷新状态重新调用入口。
5. 提交不明或导入失败时使用原 runId 恢复。没有上游幂等/查询能力时请用户核实真实上游任务 ID，不能自动重复生成。
