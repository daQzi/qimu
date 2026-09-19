---
name: workflow
description: 持久流程示例，读取视频后等待用户输入并确认远程处理。
---

使用 operation_describe 读取 pipeline-helper.process，再以用户选择的真实 resourceId 调用 operation_invoke。返回运行卡片后可以结束本轮，后台会继续。

用户回来时用 plugin_run_get 查看步骤和输入请求。按 Schema 询问用户，只有明确提供了必填内容才能用 plugin_input_submit 提交，不得猜测。也可指引用户在卡片填写。远程步骤必须由用户批准，填写表单不代表付费授权。使用示例服务时必须说明这只是 JSON 回显，不是视频处理模型。
