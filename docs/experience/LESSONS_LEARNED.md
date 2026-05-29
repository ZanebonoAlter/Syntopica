# 项目经验总结（Lessons Learned）

这份文档记录本项目里已经反复踩过的坑，目标是减少重复调试时间。

## 适用范围

开始编码前，以下场景都应该先看这份文档：

### Go / Gin 后端
- 新增或修改 Gin handler
- 处理 JSON 请求体
- 使用 `c.GetRawData()`、`c.ShouldBindJSON()`
- 新增 API 字段或接口返回结构

### Vue / Nuxt 前端
- 修改 `fetchFeeds`、`fetchArticles` 等映射函数
- 新增后端字段并在前端展示
- 修改 TypeScript 类型定义
- 修改 Edit / Update 类对话框组件
- 处理 props 变化时 `ref` 不同步问题
- 处理 `v-model` 默认值问题

### 跨层数据流
- 后端新增字段，前端需要同步显示
- 修复 snake_case 和 camelCase 转换问题
- 修复数据库、后端、前端之间的数据不一致

---

## Go / Gin 经验

### 1. 请求体只能读一次

问题：
- 在同一个 handler 里先用 `c.GetRawData()` 读原始 body
- 然后再用 `c.ShouldBindJSON()` 绑定结构体
- 第二次读取会失败，常见表现是 EOF

错误示例：
```go
rawBody, _ := c.GetRawData()

var req UpdateRequest
if err := c.ShouldBindJSON(&req); err != nil {
    return
}
```

正确做法：
```go
rawBody, err := c.GetRawData()
if err != nil {
    return
}

var bodyMap map[string]interface{}
if err := json.Unmarshal(rawBody, &bodyMap); err != nil {
    return
}

var req UpdateRequest
if err := json.Unmarshal(rawBody, &req); err != nil {
    return
}
```

规则：
- 同一个 handler 里，不要同时使用 `c.GetRawData()` 和 `c.ShouldBindJSON()`
- 如果需要原始 JSON，就只读一次 raw body，然后多次 `json.Unmarshal()`

---

## Vue / Nuxt 经验

### 2. props 初始化成 ref 后，不会自动同步

问题：
- `const enabled = ref(props.feed.enabled)` 只会在组件初始化时取一次值
- 父组件更新 props 后，子组件里的 ref 不会自动更新

错误示例：
```ts
const enabled = ref(props.feed.contentCompletionEnabled ?? false)
```

正确做法：
```ts
const enabled = ref(props.feed.contentCompletionEnabled ?? false)

watch(() => props.feed, (newFeed) => {
  if (!newFeed) return
  enabled.value = newFeed.contentCompletionEnabled ?? false
}, { deep: true })
```

什么时候这样做：
- 对话框表单
- 编辑组件
- 任何需要本地可修改状态、但初始值来自 props 的地方

---

### 3. `v-model` 必须给明确默认值

问题：
- 后端可能返回 `undefined` 或 `null`
- checkbox、number input、select 很容易出现展示异常

推荐写法：
```ts
const enabled = ref(props.feed.contentCompletionEnabled ?? false)
const completionOnRefresh = ref(props.feed.completionOnRefresh ?? true)
const maxRetries = ref(props.feed.maxCompletionRetries ?? 3)
```

规则：
- 布尔值、数字值都优先用 `??`
- 不要用 `||` 处理默认值，否则 `0`、`false` 会被误伤

---

### 4. 后端新增字段后，前端映射必须同步更新

问题：
- 后端已经返回字段
- 但前端 store / composable 的映射函数漏掉了
- 最终 UI 仍然拿不到数据

典型位置：
- `front/app/stores/api.ts`
- `fetchFeeds()`
- `fetchArticles()`
- 类型定义文件，如 `front/app/types/feed.ts`、`front/app/types/article.ts`

检查清单：
- 后端 JSON 字段是否真的返回
- API 映射函数是否已接住字段
- TypeScript 类型是否已新增字段
- 组件是否用了正确字段名

---

### 5. snake_case 和 camelCase 只在边界转换

规则：
- 后端 JSON 用 snake_case
- 前端组件和 store 内部用 camelCase
- 转换位置放在 API 映射层，不要分散到各个组件里

示例：
```ts
const mappedFeed = {
  id: String(feed.id),
  contentCompletionEnabled: feed.content_completion_enabled,
  completionOnRefresh: feed.completion_on_refresh,
  maxCompletionRetries: feed.max_completion_retries,
}
```

发送回后端时：
```ts
await apiStore.updateFeed(id, {
  content_completion_enabled: enabled.value,
  completion_on_refresh: completionOnRefresh.value,
})
```

---

## 编码与乱码经验

### 6. Windows PowerShell 下，文本重写很容易把非 ASCII 内容写坏

高风险操作：
- `Set-Content`
- `Out-File`
- `>`、`>>`
- `git show > file`
- 任何对含中文文件的整文件 shell 重写

典型症状：
- 中文 UI 文案突然变成不可读字符
- 注释、标题、prompt 出现 mojibake
- 文件开头出现 BOM 相关异常显示

规则：
- 优先使用 UTF-8（无 BOM）
- 修改后马上重新打开文件检查
- 如果发现文本已经坏了，不要继续一点点补，直接整文件重写
- Windows 下不要用 `git show > file` 恢复含中文的文本文件

单独说明见：
- `docs/experience/ENCODING_SAFETY.md`

---

## 提交前检查清单

### Go 后端
- [ ] 没有同时用 `c.GetRawData()` 和 `c.ShouldBindJSON()`
- [ ] 新字段已经加到响应结构中
- [ ] handler 参数名和路由参数名一致
- [ ] nil / 空值情况已处理

### Vue / Nuxt 前端
- [ ] 后端新字段已经映射到前端对象
- [ ] TypeScript 类型已同步
- [ ] props 初始化成 ref 的地方已经加 watch 或改成 computed
- [ ] `v-model` 已给默认值

### 编码安全
- [ ] 修改过的中文文件已经重新打开检查
- [ ] 没有再出现乱码或异常字符
- [ ] 必要时已整文件按 UTF-8 重写

---

## 最重要的几条

1. Gin 请求体只能安全读取一次。
2. 前端字段映射漏一层，功能就等于没做完。
3. props 转 ref 后要考虑同步，不然表单状态会错。
4. `v-model` 默认值别偷懒，用 `??`。
5. Windows 下重写含中文文件时，要把编码当成高风险项处理。

6. **后端过滤与计数是两个独立问题。** 添加 JOIN 过滤时， 会因重复行膨胀，必须用  分开处理。
7. **归一化函数不能漏。** 后端返回 snake_case ()，前端用 camelCase ()，中间层  必须过一遍，不然  就是 Invalid Date。
8. **低质量 embedding 模型的阈值陷阱。** Qwen3-Embedding:4b 对完全不相关的概念（"Anthropic" vs "伊朗"）产生的余弦相似度可达 0.65，远高于直觉预期。默认  会导致大量跨域误匹配。需要实际测量后把阈值提到 0.72+，或者在 UI 标注推荐区间。

6. **后端过滤与计数是两个独立问题。** 添加 JOIN 过滤时，COUNT(*) 会因重复行膨胀，必须用 COUNT(DISTINCT id) 分开处理。
7. **归一化函数不能漏。** 后端返回 snake_case (pub_date)，前端用 camelCase (pubDate)，中间层 normalizeArticle 必须过一遍，不然 new Date(undefined) 就是 Invalid Date。
8. **低质量 embedding 模型的阈值陷阱。** Qwen3-Embedding:4b 对完全不相关的概念（Anthropic vs 伊朗）产生的余弦相似度可达 0.65，远高于直觉预期。默认 SimThreshold=0.6 会导致大量跨域误匹配。需要实际测量后把阈值提到 0.72+，或者在 UI 标注推荐区间。
