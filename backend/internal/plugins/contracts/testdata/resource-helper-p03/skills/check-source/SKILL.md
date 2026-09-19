---
name: check-source
description: 检查真实视频信息，按用户要求保存结果和画布节点。
---

1. 读取用户在画布中引用的视频，取得真实 resourceId，不把 nodeId 当作资源 ID。
2. 先 operation_describe 再调用 resource-helper.inspect-video。仅解释返回字段，未知时长或尺寸明确说明未记录。
3. 用户要求保存时，describe 并调用 resource-helper.snapshot-video，传 resourceId 和刚返回的 digest（expectedDigest）。提示用户在运行卡片确认，Agent 不能自批。
4. 用户确认后用 result_read 获取成功 ResultRef。需要放到画布时，先 canvas_get_state 获取最新 snapshotHash，再 describe 并调用 resource-helper.place-result，传 runId、resultDigest、blueprintId=inspect-board、snapshotHash、instanceKey=default。没有画布则只保留结果。
5. 提示用户确认画布写入。重复保存保持同一个 instanceKey，不能为规避冲突自动换标识。冲突时结果仍在，不重新分析视频；请用户取消过期审批，读取最新画布后重试，或明确选择另建节点。
