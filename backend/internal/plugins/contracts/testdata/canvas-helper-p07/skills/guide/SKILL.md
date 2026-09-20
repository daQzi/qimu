---
name: canvas-guide
description: 按节点填写映射、选择已有媒体并展示对比，不执行媒体生成。
---

# 画布交互引导

本示例只组织用户填写的映射和已导入媒体，不执行拉片分析、替换或收费模型生成。

1. 搜索并 describe `canvas-helper.process`，用空对象启动一次运行。保留返回的 Run ID。
2. 读取 `plugin_run_get`。若等待输入，使用返回的 canvasActions 投影对应 inputRequestId；先 describe `canvas-helper.place`，再读画布最新 snapshotHash，固定 instanceKey 为 default。不要猜输入 ID，不代填用户决定。
3. 画布保存仍须用户审批。告诉用户定位下一个待填写节点：填写映射并提交，下一步选择两个已上传媒体的资源 ID。无画布时可直接使用运行卡片。
4. 提交后读取同一 Run，等待下一输入或成功。不要重复启动 process。完成后用 results 蓝图展示表格、实体卡片和媒体对比。
5. result.continue/editor.focus 仅在线编辑器动作，不能当成后台 Adapter 调用；input.submit 复用已有 Run 输入接口，不代表模型授权。
6. 投影冲突先读取最新画布，用同一绑定重试；节点已经移动/删除时告知用户，按用户选择另建 instanceKey。不重新执行流程或已付费操作。
7. 删除或撤销节点仅影响展示，不取消 Run，不退费。要停止流程需使用独立取消运行操作。
