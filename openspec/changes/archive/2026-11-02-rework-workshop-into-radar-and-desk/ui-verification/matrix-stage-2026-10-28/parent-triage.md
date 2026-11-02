# 首轮真实 S9 的父裁决与静态回退

## 结果统计校正

原report小结“23 PASS / 2 FAIL / 2 PARTIAL”合计27，与25步不符。父按第4节实际PASS/PARTIAL表格机械提取：**21 PASS、2 FAIL（15/25）、2 PARTIAL（22/24），共25步**。这是统计更正，不覆盖原报告/截图/请求证据，也不把未验补绿。部分缓存/迟到/空字段边界已有便宜层自动化证据，后续综合验收须逐项关联，不强求往真库造样本或把浏览器未验说成已验。

## 裁决

- F1/F2为Medium，必须修复并真实复验：深滚动恢复发生在编年异步增高之前而被钳制；缓存命中切档在router提交前读回旧范围，违反S9第15步默认范围重置。
- F3不是随意产品取舍：test-cases-matrix-stage.md第22步明确“容器内横滚仅在>31列”，MX-6明确≤31单屏。**修代码符合原契约，不改用例/设计以迎合实现。** 保日期/数值可识别、完整信息可查，禁止整页缩放/极小字体/隐藏字段假适配。
- F4补手机真实命中区≥24CSS px，视觉小竖线可保，但短窗左右端不能被重叠透明区遮蔽；保持实际plot映射、拖动无跳变与取消语义。
- F5父查ArticlePreviewModal与AppDialog实际源码：90vw是明确标deprecated的存量兼容width，既有reader本轮不重绘；不以此单独宣新布局错误。对话语义/初始焦点本次调用需补，可用opt-in小接口默认保旧、仅matrix reader启用，不做全站模态重构/改搜索高亮与iframe高度链/新增依赖。
- F6编年业务标签/标题按API大小写显示，不强制uppercase；系统kicker角色风格不需全改。

## 已执行安全回退（2026-10-28 04:11）

源代码未回滚，数据/后端未变。父先校验原备份SHA及当前待验index=a8b20ec…，解包到暂存目录，保留待验产物后换回验收前静态目录：

- 现站index SHA：`b2a61d733968037f0d6c06c8fa826e0ef694505e5766f4ce12199dbe18e64715`
- localhost:5100/tags/1974 HTTP200，返回SHA等于恢复后的index；/health成功。
- 后端PID仍1061075，无重启。
- 待验产物保留：`.pi/subagents/deploy-backups/matrix-rejected-s9-20261028-041126`
- 回退日志：`.pi/subagents/validation/matrix-s9-rollback-20261028-041126/`
- 原备份：`.pi/subagents/deploy-backups/matrix-before-20261028-025506.tar.gz`，已验证SHA `d4bc92e5546526977a6e9adb0ad26b9f3d6b30540bdd24d3bf85081aed4e756b`。

用户当前刷新回验收前总览/旧矩阵；修复、构建再铺后才复验新版。不要拿当前旧静态验证新源码，不勾8B.3/8B.4。
