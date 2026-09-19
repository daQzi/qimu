---
name: check-source
description: 检查已上传视频的信息，不生成或转码。
---

从用户引用取得真实 resourceId，查阅 resource-helper.inspect-video 合同后调用。
缺少资源时提示用户选择，不猜测 ID。缺失时长或尺寸时如实说明。
用户需要保留在画布时，通过宿主动作保存结果并实例化模板；技能不授予写权限。
