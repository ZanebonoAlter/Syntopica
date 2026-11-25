## ADDED Requirements

### Requirement: 版块文章列表携带疑似 AI 判定状态

`GET /api/semantic-boards/:id/articles` 返回的每篇文章 SHALL 附带该文章的 AI 生成疑似度判定状态字段（未判定／疑似／已确认非 AI）与置信分。判定状态为未判定时该字段 SHALL 仍存在于响应项中（值为未判定），SHALL NOT 省略字段。

该字段 SHALL 为非正文类小字段，SHALL NOT 携带正文内容；接口 SHALL NOT 因此新增筛选参数。

#### Scenario: 归档列表项携带判定状态

- **WHEN** 客户端请求 `GET /api/semantic-boards/5/articles`
- **THEN** 每篇文章的响应项 SHALL 携带该文章的 AI 生成疑似度判定状态字段

#### Scenario: 未判定文章字段不被省略

- **WHEN** 列表中某篇文章判定状态为未判定
- **THEN** 该文章响应项 SHALL 仍携带判定状态字段（值为未判定），SHALL NOT 缺失该字段

#### Scenario: 判定字段不改变排序与筛选口径

- **WHEN** 客户端以既有筛选参数与排序参数请求版块文章列表
- **THEN** 返回结果的成员与顺序 SHALL NOT 因疑似 AI 判定状态而改变
