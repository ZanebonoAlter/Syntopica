<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约：lane-first-recall

## N/A Reason

本 change 只改日报生成的服务端召回链路（`collectBoardTags` 新增泳道 ANN 召回通道、`filterTagsByQuality` 对泳道证据放行、L2 判 `new` 不回 L3），不新增/修改任何界面元素、交互、文案或布局。用户可见变化是「日报内容变多、泳道不再被邻座内容填充」这类**数据结果**变化，由既有日报渲染与泳道矩阵直接呈现，不需要新的 UI 契约。

依据：proposal 头部 `ui-impact: none` 声明（此处不再重复注释分隔符字面量——lib 的 marker 解析要求全文恰好一次，正文引用会构成重复声明）——无新增入口、无新增配置控件（两个参数走 `ai_settings`，与 `persistent_topic_lane_l1_threshold` 同模式，不提供界面开关）、无布局模式变更。
