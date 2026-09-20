---
name: workflow
description: 持久流程示例，读取视频后等待用户输入并确认远程处理。
---

使用 operation_describe 读取 batch-helper.process，再以用户选择的真实 resourceId 调用 operation_invoke。返回运行卡片后可以结束本轮，后台会继续。

用户回来时用 plugin_run_get 查看步骤和输入请求。按 Schema 询问用户，只有明确提供了必填内容才能用 plugin_input_submit 提交，不得猜测。也可指引用户在卡片填写。远程步骤必须由用户批准，填写表单不代表付费授权。使用示例服务时必须说明这只是 JSON 回显，不是视频处理模型。

表单中的 items 按顺序填写，每项包含稳定且唯一的 id、prompt 和 enabled。用户删除或重新排列条目时，未改动条目的 id 必须保持不变；enabled=false 的条目保留 null 结果位置。可引导用户在运行卡片核对并批准当前批次，供应商费用未知，不得宣称免费。

用户明确要求重新执行时，可使用 plugin_run_derive 创建派生运行，按 choose 步骤提供新的 items；此工具不自动复用随机结果。需要局部复用时，请用户在已结束运行的卡片点击重新执行，修改映射并明确勾选复用。复用不会再次生成，不应把旧结果描述为新生成。
