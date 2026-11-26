<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

纯后端性能与存储路径改动：把辅助标签 L2 去重从进程内全量余弦换成数据库 halfvec HNSW 索引检索，不新增或修改任何用户可见的界面结构、交互或文案。proposal 已声明 `ui-impact: none`，本 change 无前端路径改动。
