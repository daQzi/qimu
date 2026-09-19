---
name: check-source
description: 检查已上传视频的资源信息；仅在用户明确要求时保存元数据快照。
---

1. 从用户引用取得真实 resourceId，不猜测 ID。
2. 先 describe 并调用 resource-helper.inspect-video。
3. 未返回时长或尺寸时说明元数据缺失，不推测数值。
4. 只有用户明确要求保留快照时，才 describe 并调用 resource-helper.snapshot-video。
5. snapshot-video 返回待确认运行后，告知用户在运行卡片确认；Agent 不得自行批准。
