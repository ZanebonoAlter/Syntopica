# 语义标签板块系统 — 后端完整设计

> 包含用户故事、顺序图、类图、状态图，覆盖语义标签板块系统的后端完整设计。

---

## 白话概述：这个系统到底在做什么？

**一句话：把你的 RSS 订阅文章自动归到几个"长期话题板块"里，每个板块每天生成一份简报。**

举个例子——

你订阅了科技、国际新闻、财经几个 RSS 源。系统读每一篇文章时：
1. 从文章里抽出关键标签，比如读了篇"伊朗导弹袭击以色列"的新闻，抽取出事件标签「伊朗袭击以色列」、人物标签「内塔尼亚胡」
2. 同时给这些标签贴上"小标签"（辅助标签），比如「伊朗」「导弹袭击」「中东冲突」「地缘政治」
3. 系统积累了大量"小标签"后，自动发现「伊朗」「霍尔木兹海峡」「中东冲突」「地缘政治」这四个小标签经常一起出现，于是建议你建一个"中东局势"板块
4. 以后系统读到任何一篇文章，只要它的"小标签"跟"中东局势"板块能对上，文章就自动归入这个板块
5. 每天早晨，系统把"中东局势"板块下当天所有事件汇总成一份简报给你看

**核心思路：** 用"小标签"做中介，让文章和板块之间不再靠模糊的"相似度猜"，而是靠"小标签有没有交集"来精确判断。

---

## 术语表 —— 用人话解释每个概念

| 术语 | 英文 | 一句人话 | 打个比方 |
|---|---|---|---|
| **标签** | Tag | 从文章里抽取的关键词/事件/人物 | 你给文章加的"话题标签" |
| **辅助标签** | Auxiliary Label | 给标签再贴一层"小标签"，作为标签的语义锚点 | 就像你给"iPhone 16"贴上「苹果」「手机」「消费电子」这些小标签；系统用这些小标签判断一篇文章该归哪个板块 |
| **板块** | SemanticBoard | 一个长期存在的话题领域 | 比如"AI圈""中东局势""新能源"，这些板块不是临时的，而是一直在，每天收集当天相关新闻 |
| **每日简报** | NarrativeBoard | 每天自动生成的一份板块报道 | 每天早上，"AI圈"板块把昨天所有 AI 相关的新闻打包成一份摘要给你 |
| **板块构成** | Board Composition | 某个板块由哪些"小标签"组成 | "AI圈"板块由「AI」「大语言模型」「OpenAI」「深度学习」这些小标签构成 |
| **Embedding** | Embedding | 把文字变成一串数字，让计算机能算两个词"有多像" | 相当于给每个词一个坐标，靠得近的坐标意思相近 |
| **合并嵌入** | Merge Embedding | 只用标签名本身算的坐标 | 只比较「AI」和「人工智能」这两个词长得像不像 |
| **存储嵌入** | Storage Embedding | 用标签名 + 解释文字一起算的坐标 | 比较「AI + 人工智能技术领域」和「显卡风扇散热技术」——带解释后就能区分开，不会把完全不相关的词混到一起 |
| **聚类** | Clustering | 把一堆相似的东西自动分组 | 就像把一袋混合糖果按口味分成几堆——系统把相似的"小标签"分堆，然后问你这堆能不能变成一个新板块 |
| **回填** | Backfill | 把历史数据按最新规则重新算一遍归属 | 比如你新增了"量子计算"板块，回填就是把过去所有文章中跟量子计算相关的重新归到这个板块下 |
| **冷启动** | Cold Start | 系统刚上线、还没有板块的时候 | 刚开始用，系统还没有任何板块，需要积累一段时间数据后，由你手动触发创建第一批板块 |
| **别名** | Alias | 同一个意思的不同叫法 | 「AI」「人工智能」「Artificial Intelligence」其实是同一个东西，合并后「AI」是主名，其他都是别名 |

---

## 数据怎么流转 —— 一张图看懂

```
┌──────────────────────────────────────────────────────────────────────┐
│                        文章 → 标签 → 小标签 → 板块                     │
│                                                                      │
│  ① 文章到达                                                           │
│     │                                                                │
│     ▼                                                                │
│  ② LLM 抽取标签 + 小标签                                              │
│     ├─ 事件/人物标签 ──→ 自动附带 3-5 个小标签（如"伊朗袭击以色列"      │
│     │                      ──→「伊朗」「导弹袭击」「中东冲突」）         │
│     └─ 关键词标签 ──→ 自己直接做小标签（如"Claude Code" 就是它自身）    │
│     │                                                                │
│     ▼                                                                │
│  ③ 小标签入库（去重合并）                                             │
│     ├─ L1: 别名一模一样？→ 直接用已有                                  │
│     ├─ L2: 意思几乎一样？(相似≥95%) → 合并为一个                       │
│     └─ L3: 全新的 → 新建                                             │
│     │                                                                │
│     ▼                                                                │
│  ④ 标签匹配板块                                                       │
│     标签的"小标签" ──交集──→ 板块的"构成标签"                          │
│     有交集 → 挂载到该板块                                              │
│     没交集 → 暂时无归属                                                │
│     │                                                                │
│     ▼                                                                │
│  ⑤ 每日简报生成                                                       │
│     每个板块下当天的事件标签 → LLM 写摘要 → 生成每日简报                │
│                                                                      │
│  ★ 板块升级（手动触发）                                               │
│     高频小标签 → 聚类分堆 → LLM 建议 → 你确认 → 变成新板块             │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 用户故事

### US-1：读文章自动打小标签
> **我是** 一个 RSS 阅读用户  
> **我想** 系统读完每篇文章后，不仅抽出事件/人物标签，还自动给这些标签贴上 3-5 个"小标签"  
> **这样我** 就不用自己去想这篇文章该归到哪个板块了，系统靠小标签就能自动判断

**怎么算完成：**
- 读到"伊朗袭击以色列"这篇文章，LLM 抽取出事件标签的同时，附带上「伊朗」「导弹袭击」「中东」这些小标签，每个小标签还带一句简短解释
- 读到包含"Claude Code"的文章，这个关键词本身直接当小标签用，不再绕一圈去生成额外小标签
- 小标签的解释不能为空、不超过 500 字、不能只是重复标签名（比如「伊朗」的解释不能只是"伊朗"）

---

### US-2：差不多的小标签自动合并
> **我是** 一个 RSS 阅读用户  
> **我想** 系统能自动把意思相同的小标签合并起来（比如「AI」「人工智能」「artificial intelligence」）  
> **这样我** 的标签池不会越来越臃肿，同样的意思不会出现好几个版本

**怎么算完成：**
- 名字完全一样或别名命中 → 直接用已有的，零成本
- 用"仅标签名"算出来的相似度 ≥ 95% → 自动合并，热度低的那一方变成另一方的别名
- 全新的小标签 → 正常新建
- 合并时永远保留用得更多的那个作为主名

---

### US-3：标签自动归到对应板块
> **我是** 一个 RSS 阅读用户  
> **我想** 每篇文章的标签能自动判断属于哪个话题板块  
> **这样我** 就不用手动分类了，文章自动出现在对应的板块里

**怎么算完成：**
- 判断方法不看"标签和板块名字像不像"，而是看"标签的小标签跟板块的构成标签有没有交集"
- 一个标签可以同时属于多个板块（比如"霍尔木兹海峡"既属于"中东局势"也属于"能源安全"），最多 3 个
- 匹配结果存下来，方便后续查看和回溯

---

### US-4：高频小标签升级为新板块
> **我是** 一个 RSS 阅读用户  
> **我想** 当某个话题反复出现时，系统能提醒我是否要把它立为一个新板块  
> **这样我** 不会漏掉新冒出来的热门话题，板块库能自然生长

**怎么算完成：**
- 某个小标签出现了 5 次以上，系统把它放进"升级候选池"
- 我点击"生成建议"后，系统先把候选标签按相似度分堆，然后问 LLM "这堆能不能叫一个板块？"
- LLM 返回建议，我逐条确认或拒绝
- 确认后会创建新板块，并自动把构成的小标签关联上

---

### US-5：手动调整板块的构成标签
> **我是** 一个 RSS 阅读用户  
> **我想** 能手动往板块里加小标签、或把不合适的小标签移除  
> **这样我** 能纠正 LLM 有时候判断不太对的地方，精确控制板块的覆盖范围

**怎么算完成：**
- 创建/编辑板块时，系统根据相似度推荐一批候选小标签
- 我可以通过搜索找到特定小标签并勾选添加
- 添加/移除后系统不会自动重算历史归属，而是提醒我"要不要手动回填一下"

---

### US-6：每天早上自动生成每日简报
> **我是** 一个 RSS 阅读用户  
> **我想** 每天早上系统自动按板块把昨天的事件汇成简报  
> **这样我** 不用一篇文章一篇文章看，每个话题板块一眼就能知道昨天发生了什么

**怎么算完成：**
- 每个板块（按分类范围或全局范围）收集昨天归属于它的所有事件标签
- 一篇文章如果属于多个板块，可以同时出现在多份简报里
- 简报能跟前一天的同一板块简报接续（形成连续叙事）
- 简报的内容上下文用的是板块自身的名称和描述

---

### US-7：改规则后重新算一遍归属
> **我是** 一个 RSS 阅读用户  
> **我想** 改了板块构成或者匹配参数后，能一键把历史标签的归属重新算一遍  
> **这样我** 调整规则后，历史数据也能同步更新，不会新旧混在一起

**怎么算完成：**
- 三种回填模式：全部重算 / 只算没归属的 / 只算某个板块的
- 后台异步执行，能看进度
- 重复跑不会出问题（幂等）

---

### US-8：清理低质量的小标签
> **我是** 一个 RSS 阅读用户  
> **我想** 能把不好用的小标签禁用掉、把分散的别名合并、把不相关的从板块里移除  
> **这样我** 长期维护下标签池不会变成垃圾堆

**怎么算完成：**
- 禁用后的小标签不再参与板块匹配和升级候选
- 手动合并别名时，原来关联的标签自动迁移到目标
- 修复操作不会自动删历史数据，需要时我再手动回填

---

## 顺序图

### 1. 文章标签提取 & 小标签入库

```mermaid
sequenceDiagram
    autonumber
    participant Scheduler as 定时调度器
    participant Tagger as 标签提取器
    participant LLM as AI (大模型)
    participant AuxSvc as 小标签服务
    participant DB as 数据库
    participant Embedder as 向量化服务

    Scheduler->>Tagger: 处理这篇文章，提取标签

    par 两条路同时走 (互不影响)
        Tagger->>LLM: 找出文章里的事件和人物，每人/事顺便给3-5个小标签
        LLM-->>Tagger: 事件"伊朗袭击以色列"→【伊朗, 导弹袭击, 中东冲突】
        Note over Tagger: 失败了最多重试3次
    and 另一条路
        Tagger->>LLM: 找出文章里的关键词/术语，只给标签名+解释
        LLM-->>Tagger: 关键词"Claude Code"→解释"Anthropic的AI编程助手"
        Note over Tagger: 关键词不生成小标签，自己就是小标签
    end

    Tagger->>Tagger: 两条路结果合并，最多保留5个标签

    loop 每个事件/人物标签的小标签
        Tagger->>AuxSvc: 入库小标签「伊朗」(解释: 中东地区国家)
        AuxSvc->>DB: 第一步：名字/别名完全一样？
        alt 找到了 (L1命中)
            DB-->>AuxSvc: 已有，直接复用，使用次数+1
        else 没找到
            AuxSvc->>Embedder: 用标签名生成"合并用坐标"(只看名字像不像)
            Embedder-->>AuxSvc: 坐标向量
            AuxSvc->>DB: 第二步：跟已有的比，相似≥95%？
            alt 太像了 (L2命中)
                AuxSvc->>DB: 合并！热度低的变别名，归到热度高的
            else 真的不一样 (L3新建)
                AuxSvc->>Embedder: 用标签名+解释生成"存储用坐标"
                Embedder-->>AuxSvc: 完整坐标向量
                AuxSvc->>DB: 新建一条小标签记录
                AuxSvc->>DB: 建立"标签→小标签"关联
            end
        end
    end

    loop 每个关键词标签
        Tagger->>AuxSvc: 关键词"Claude Code"直接当小标签入库
        Note over AuxSvc: 走同样的 L1→L2→L3 流程
    end

    Tagger->>DB: 保存标签、文章与标签关联
    Tagger-->>Scheduler: 处理完成
```

---

### 2. 标签匹配板块

```mermaid
sequenceDiagram
    autonumber
    participant Caller as 调用方 (入库时/回填时)
    participant Matcher as 板块匹配器
    participant DB as 数据库
    participant Config as 配置中心

    Caller->>Matcher: 算一下这个标签该归哪个板块

    Matcher->>DB: 查这个标签有哪些小标签
    DB-->>Matcher: 【伊朗, 导弹袭击, 中东冲突】

    Matcher->>DB: 查出所有活跃板块及其构成标签
    DB-->>Matcher: "中东局势"板块→构成标签:【伊朗, 中东, 地缘政治, ...】

    Matcher->>Config: 拿匹配参数 (相似度门槛、命中率门槛等)
    Config-->>Matcher: {门槛0.6, 命中率50%, 最像0.8, ...}

    Matcher->>Matcher: 逐个板块算匹配分

    loop 每个板块
        alt 小标签跟板块构成标签直接撞上了
            Matcher->>Matcher: 直接挂载！原因: 直接命中，得分1.0
        else 没直接撞上，算间接分
            Matcher->>Matcher: 算"命中率"=多少个小标签跟板块长得像 / 总小标签数
            Matcher->>Matcher: 算"最像分"=所有小标签中跟板块最像的那个的相似度

            alt 命中率超过一半
                Matcher->>Matcher: 挂载！原因: 命中率高
            else 有某个小标签特别像
                Matcher->>Matcher: 挂载！原因: 高度相似
            else 加权综合分过线
                Matcher->>Matcher: 挂载！原因: 综合分达标
            else
                Matcher->>Matcher: 不归这个板块
            end
        end
    end

    Matcher->>Matcher: 按得分排序，最多取3个板块

    Matcher->>DB: 写入标签→板块归属记录
    DB-->>Matcher: 写入完成

    Matcher-->>Caller: 匹配结果: 归入【中东局势】【能源安全】
```

---

### 3. 板块升级建议 & 确认

```mermaid
sequenceDiagram
    autonumber
    actor 用户 as 我(用户)
    participant API as 板块API
    participant Upgrade as 升级服务
    participant DB as 数据库
    participant Embedder as 向量化服务
    participant LLM as AI (大模型)

    用户->>API: 看看有哪些小标签够格升级了
    API->>Upgrade: 查升级候选
    Upgrade->>DB: 找出用了5次以上但还没升级的小标签
    DB-->>Upgrade: 25个候选标签
    Upgrade->>Embedder: 把它们按相似度分成8-10堆
    Embedder-->>Upgrade: 分了8堆: {堆1:【伊朗,中东,地缘政治】, 堆2:【AI,LLM,深度学习】, ...}
    Upgrade-->>API: 8个候选簇
    API-->>用户: 展示: 候选标签分了8堆

    用户->>API: 生成升级建议
    API->>Upgrade: 每堆拿去问LLM

    loop 处理每一堆
        Upgrade->>DB: 补充"同时出现的事件"作为上下文
        DB-->>Upgrade: 近30天跟这些标签一起出现的事件
    end

    Upgrade->>LLM: 这堆标签+关联事件，你看适合成立一个板块吗？
    LLM-->>Upgrade: 堆1→建议新建"中东局势"板块 / 堆2→建议归入已有的"AI圈"板块 / 堆3→跳过不够格
    Upgrade->>DB: 暂存建议(不真写)
    Upgrade-->>API: 建议列表
    API-->>用户: 展示: 3条建议等你确认

    loop 我逐条看
        用户->>API: 确认这条建议
        alt 新建板块
            API->>DB: 创建"中东局势"板块 + 绑定构成标签
        else 并入已有板块
            API->>DB: 把候选标签加进已有板块的构成列表
        end
        API-->>用户: ✅ 已处理，继续看下一条
    end

    用户->>API: 手动触发回填(可选)
    API->>API: 把历史标签的归属重新算一遍
    API-->>用户: 回填任务已启动
```

---

### 4. 回填流程

```mermaid
sequenceDiagram
    autonumber
    actor 用户 as 我(用户)
    participant API as 板块API
    participant Queue as 后台任务队列
    participant Matcher as 板块匹配器
    participant DB as 数据库

    用户->>API: 触发回填 (全部重算/只算无归属的/只算某个板块)

    API->>DB: 根据模式查出要重算的标签ID列表
    DB-->>API: 150个标签需要重算

    API->>Queue: 提交回填任务
    API-->>用户: 任务ID=42, 总共150个

    用户->>API: 查进度
    API->>Queue: 任务42怎么样了
    Queue-->>API: 已处理120个, 失败0个
    API-->>用户: 进度 80%

    Note over Queue: === 后台默默干活 ===

    loop 挨个处理标签
        Queue->>Matcher: 重新算这个标签归哪个板块
        Matcher->>DB: 查小标签+板块构成
        Matcher->>Matcher: 匹配计算
        Matcher->>DB: 覆盖旧的归属记录
        Queue->>Queue: 处理数+1
    end

    Queue->>DB: 更新任务状态: 完成
```

---

### 5. 每日简报生成

```mermaid
sequenceDiagram
    autonumber
    participant Scheduler as 每日定时器
    participant Generator as 简报生成器
    participant DB as 数据库
    participant LLM as AI (大模型)

    Scheduler->>Generator: 到点了，生成今天的简报

    Generator->>DB: 有哪些活跃板块？
    DB-->>Generator: "中东局势""AI圈""新能源" ...

    loop 每个范围 (全局/按订阅分类)
        Generator->>DB: 这个范围下各板块今天有哪些事件标签？
        DB-->>Generator: "中东局势"板块: 今天有【伊朗演习, 沙特油价, 红海航运】

        loop 每个有事件的板块
            Generator->>DB: 找昨天的同一板块简报(用于接续)
            DB-->>Generator: 昨天的"中东局势"简报

            Generator->>DB: 创建今天的简报记录
            Note over DB: 记下: 属于哪个板块, 包含哪些事件, 续接昨天的简报

            Generator->>LLM: 写一段摘要: 板块"中东局势"+ 今天3个事件
            LLM-->>Generator: "今日中东局势持续升温：伊朗在中东地区..."
            Generator->>DB: 保存摘要
        end
    end

    Note over Generator: 注意：没有板块或板块下没事件的，不生成(不报错)

    Generator-->>Scheduler: 生成完毕
```

---

## 类图 — 数据模型（表结构）

```mermaid
classDiagram
    direction TB

    class SemanticLabel {
        <<一张表管两种东西>>
        +ID 编号
        +Label 名称 "如: 伊朗"
        +Slug 英文标识
        +Embedding 存储用坐标 "标签名+解释一起算的向量"
        +MergeEmbedding 合并用坐标 "只用标签名算的向量"
        +LabelType 类型 "小标签 还是 板块?"
        +Aliases 别名列表 "如:【波斯, Persia】"
        +RefCount 被用了多少次
        +Description 解释 "如: 中东地区国家"
        +DisplayOrder 排序
        +Source 来源 "LLM提取/关键词直入/LLM建议/手动创建"
        +Status 状态 "正常/已禁用"
        +Protected 是否受保护(手动创建的不被自动合并)
        +CreatedAt 创建时间
        +UpdatedAt 更新时间
    }

    class TopicTag {
        <<文章标签>>
        +ID 编号
        +Slug 英文标识
        +Label 标签名 "如: 伊朗袭击以色列"
        +Category 类别 "事件/人物/关键词"
        +Status 状态
        +Score 置信度
    }

    class TopicTagSemanticLabel {
        <<标签→小标签 关联表>>
        +TopicTagID 标签ID
        +SemanticLabelID 小标签ID
    }

    class TopicTagBoardLabel {
        <<标签→板块 归属表 (匹配结果落库)>>
        +TopicTagID 标签ID
        +SemanticBoardID 板块ID
        +Score 匹配得分
        +MatchReason 匹配原因 "直接命中/命中率高/高度相似/综合分"
    }

    class BoardComposition {
        <<板块由哪些小标签构成>>
        +BoardID 板块ID
        +AuxiliaryLabelID 小标签ID
    }

    class NarrativeBoard {
        <<每日简报>>
        +ID 编号
        +Name 名称
        +Description 描述
        +EventTagIDs 包含的事件标签ID列表
        +SemanticBoardID 来自哪个板块
        +PrevBoardIDs 续接昨天的哪些简报
        +ScopeType 范围 "全局/按订阅分类"
        +CreatedAt 生成日期
    }

    class NarrativeSummary {
        <<简报摘要>>
        +Title 标题
        +Summary 摘要内容
        +Status 状态
        +BoardID 属于哪个简报
        +RelatedTagIDs 关联标签
        +RelatedArticleIDs 关联文章
    }

    SemanticLabel "1" --> "0..*" TopicTagSemanticLabel : 小标签ID
    SemanticLabel "1" --> "0..*" TopicTagBoardLabel : 板块ID
    SemanticLabel "1" --> "0..*" BoardComposition : 板块ID
    SemanticLabel "1" --> "0..*" BoardComposition : 小标签ID (自引用)

    TopicTag "1" --> "0..*" TopicTagSemanticLabel : 标签ID
    TopicTag "1" --> "0..*" TopicTagBoardLabel : 标签ID

    NarrativeBoard "0..*" --> "1" SemanticLabel : 来源板块
    NarrativeBoard "1" --> "0..*" NarrativeSummary : 简报ID
```

---

## 类图 — 核心服务

```mermaid
classDiagram
    direction TB

    class AuxiliaryLabelService {
        <<小标签服务>>
        负责小标签的入库、去重、合并、查询
        +入库小标签(标签名, 解释, 来源) 返回已存在的或新建的
        +关键词直入池(标签名, 解释) 返回小标签
        -L1精确匹配(标签名) 别名/名字一样直接用
        -L2相似度合并(标签名, 合并坐标) 太像就合并
        -L3新建(标签名, 解释, 两个坐标) 全新的就创建
        +列出小标签(分页, 搜索)
        +禁用/启用小标签(ID)
        +手动合并别名(来源ID, 目标ID)
        +推荐候选标签(板块名, 板块解释, 分页, 搜索) 用于人工创建板块
    }

    class SemanticBoardMatcher {
        <<板块匹配器>>
        负责算出一个标签该归哪些板块
        +匹配标签到板块(标签ID) 返回匹配结果列表
        -加载标签的小标签列表
        -加载所有活跃板块及其构成标签
        -算匹配分(标签的小标签, 板块) 返回得分和原因
        -检查直接命中 小标签撞上构成标签
        -算间接匹配 命中率+最高相似度+加权
        +读取匹配参数
        +修改匹配参数
    }

    class BoardUpgradeService {
        <<板块升级服务>>
        负责把高频小标签升级为正式板块
        +获取升级候选 返回够了门槛的小标签
        +生成升级建议 LLM判断每堆能不能升级
        +执行升级 用户确认后写入
        -收集候选小标签 ref_count>=5
        -预聚类 按坐标分8-10堆
        -补充共现事件 每堆找关联事件当上下文
        -LLM判断 问AI这堆值不值得开板块
        -新建板块 创建+绑定构成标签
        -并入已有板块 加进已有板块的构成列表
    }

    class BackfillService {
        <<回填服务>>
        负责把历史标签按最新规则重算归属
        +触发回填(模式: 全部/无归属/指定板块) 返回任务ID
        +查询进度(任务ID) 返回处理了多少
        -收集待处理标签ID(模式)
        -挨个处理标签 调匹配器重算归属
    }

    class NarrativeGenerator {
        <<简报生成器>>
        负责每天从板块生成简报
        +生成每日简报(日期, 范围)
        -加载所有活跃板块
        -按范围收集每个板块今天的事件
        -找昨天的同板块简报(用于续接)
        -调LLM写摘要
    }

    class SemanticBoardAPI {
        <<API层 (对外接口)>>
        +板块列表/详情/创建/编辑/删除
        +板块构成: 查看/添加/移除小标签
        +推荐候选标签 (创建/编辑板块时)
        +升级候选列表/生成建议/确认执行
        +触发回填/查询进度
        +读取/修改匹配参数
    }

    class EmbeddingService {
        <<向量化服务>>
        +生成合并用坐标(标签名) 只用名字算向量
        +生成存储用坐标(标签名, 解释) 名字+解释一起算向量
        +生成板块坐标(板块名, 描述)
    }

    class TagExtractionService {
        <<标签提取器>>
        +从文章提取标签(文章)
        -提取事件人物(文章内容) LLM调用1: 事件+人物+小标签
        -提取关键词(文章内容) LLM调用2: 关键词+解释
        -合并两条路的结果
    }

    SemanticBoardAPI --> AuxiliaryLabelService
    SemanticBoardAPI --> SemanticBoardMatcher
    SemanticBoardAPI --> BoardUpgradeService
    SemanticBoardAPI --> BackfillService

    BackfillService --> SemanticBoardMatcher

    BoardUpgradeService --> EmbeddingService
    AuxiliaryLabelService --> EmbeddingService

    TagExtractionService --> AuxiliaryLabelService
```

---

## 状态图

### 1. 小标签的一生

```mermaid
stateDiagram-v2
    [*] --> 入库判定: 新小标签到达

    state 入库判定 {
        [*] --> L1_别名命中: 算名字
        L1_别名命中 --> 直接复用: 别名/名字一样
        L1_别名命中 --> L2_相似度检查: 没找到
        L2_相似度检查 --> 合并到已有: 太像了(≥95%)
        L2_相似度检查 --> L3_新建: 真的不一样
        L3_新建 --> 活跃中: 创建记录

        直接复用 --> 活跃中: 使用次数+1
        合并到已有 --> 活跃中: 热度低的变别名
    }

    state 活跃中 {
        [*] --> 正常工作中
        正常工作中 --> 升级候选: 被用了5次以上
        升级候选 --> 已构成板块: 被纳入板块构成
    }

    活跃中 --> 已禁用: 我手动禁用
    已禁用 --> 活跃中: 我重新启用

    活跃中 --> 已合并为别名: 手动合并 或 自动合并
    已合并为别名 --> [*]: 标签名加入目标的别名列表
```

---

### 2. 标签提取任务状态

```mermaid
stateDiagram-v2
    [*] --> 排队中: 文章到了，创建提取任务

    排队中 --> 处理中: 调度器取走任务

    state 处理中 {
        [*] --> 两条路同时跑: 发起两个LLM调用

        state 两条路同时跑 {
            [*] --> 事件人物分支: 找出事件+人物+小标签
            [*] --> 关键词分支: 找出关键词+解释

            事件人物分支 --> 事件人物重试: 失败了
            事件人物重试 --> 事件人物分支: 重试(最多3次)
            事件人物重试 --> 事件人物失败: 3次都失败
            事件人物分支 --> 事件人物成功: 解析成功

            关键词分支 --> 关键词重试: 失败了
            关键词重试 --> 关键词分支: 重试(最多3次)
            关键词重试 --> 关键词失败: 3次都失败
            关键词分支 --> 关键词成功: 解析成功
        }

        两条路同时跑 --> 合并结果: 两条路都结束了(不管成败)
        合并结果 --> 入库保存: 合并+去重
        入库保存 --> 小标签入库: 事件人物的→走L1/L2/L3
        小标签入库 --> 关键词直入: 关键词→直接当小标签
    }

    处理中 --> 提取完成: 入库完成
    处理中 --> 部分成功: 事件失败了但关键词OK
    处理中 --> 退化兜底: 两条路全挂了，用简单规则兜底

    提取完成 --> [*]
    部分成功 --> [*]
    退化兜底 --> [*]
```

---

### 3. 板块升级流程

```mermaid
stateDiagram-v2
    [*] --> 空闲等待: 等小标签积累

    空闲等待 --> 候选够了: 用够5次的小标签达标

    候选够了 --> 已分堆: 用户点"查看候选"
    已分堆 --> 已补上下文: 每堆补充共现事件
    已补上下文 --> LLM判断中: 用户点"生成建议"
    LLM判断中 --> 建议已生成: LLM返回建议

    state 建议已生成 {
        [*] --> 等确认: 建议列表给你看
        等确认 --> 新建板块: 确认"创建新板块"
        等确认 --> 归入已有: 确认"并入已有板块"
        等确认 --> 跳过: 拒绝/跳过

        新建板块 --> 板块创建完成: 写入新板块+构成标签
        归入已有 --> 构成已更新: 写入构成标签

        板块创建完成 --> 等确认: 面板不关，继续看
        构成已更新 --> 等确认: 面板不关，继续看
        跳过 --> 等确认: 面板不关，继续看
    }

    建议已生成 --> 重新生成: 用户点"重新生成"
    重新生成 --> 已分堆: 重新分堆+LLM判断

    建议已生成 --> 提示回填: 至少确认了一条
    提示回填 --> 回填中: 用户手动触发
    回填中 --> 空闲等待: 回填完，候选池刷新了
```

---

### 4. 回填任务状态

```mermaid
stateDiagram-v2
    [*] --> 已创建: POST 触发回填

    已创建 --> 收集标签: 按模式查出要处理的标签
    收集标签 --> 排队中: 任务入队

    排队中 --> 处理中: 队列开始消费

    state 处理中 {
        [*] --> 匹配一个标签: 取下一个标签ID
        匹配一个标签 --> 归属已算好: 调匹配器重算
        归属已算好 --> 写入归属: 覆盖旧的记录

        写入归属 --> 匹配一个标签: 已处理+1, 继续
        写入归属 --> 这条失败了: 异常, 失败+1

        匹配一个标签 --> 全部完成: 所有标签都处理了
        这条失败了 --> 匹配一个标签: 继续下一个
    }

    处理中 --> 全部成功: status=完成
    处理中 --> 部分失败: 有失败的
    处理中 --> 严重错误: 整个任务挂了

    全部成功 --> [*]
    部分失败 --> [*]
    严重错误 --> [*]
```

---

### 5. SemanticLabel 状态切换

```mermaid
stateDiagram-v2
    [*] --> 活跃: 创建 (新建/LLM建议确认/手动创建)

    state 活跃 {
        [*] --> 正常: 正常参与匹配
        正常 --> 热门候选: 使用次数≥门槛
        热门候选 --> 构成板块中: 被纳入板块构成
    }

    活跃 --> 已禁用: 我手动禁用
    已禁用 --> 活跃: 我重新启用

    活跃 --> 已合并: 太像了自动合并 或 手动合并别名

    state 已合并 {
        [*] --> 别名已积累: 名字加入目标别名列表
        别名已积累 --> [*]: 标签关联迁到目标
    }

    已禁用 --> [*]: 可清理 (没有人用了)
```

---

## 完整 ER 图

```mermaid
erDiagram
    semantic_labels ||--o{ topic_tag_semantic_labels : "小标签ID"
    semantic_labels ||--o{ topic_tag_board_labels : "板块ID"
    semantic_labels ||--o{ board_composition : "板块ID"
    semantic_labels ||--o{ board_composition : "小标签ID (同一张表自引用)"
    semantic_labels ||--o{ narrative_boards : "来源板块ID"

    topic_tags ||--o{ topic_tag_semantic_labels : "标签ID"
    topic_tags ||--o{ topic_tag_board_labels : "标签ID"

    narrative_boards ||--o{ narrative_summaries : "简报ID"

    semantic_labels {
        SERIAL id PK "编号"
        VARCHAR label "名称"
        VARCHAR slug UK "英文标识"
        vector embedding "存储用坐标: 标签名+解释"
        vector merge_embedding "合并用坐标: 仅标签名"
        VARCHAR label_type "类型: 小标签 还是 板块"
        JSONB aliases "别名列表"
        INTEGER ref_count "被用了多少次"
        TEXT description "解释说明"
        INTEGER display_order "排序"
        VARCHAR source "来源: LLM提取/关键词直入/LLM建议/手动"
        VARCHAR status "状态: 正常/已禁用"
        BOOLEAN protected "受保护"
        TIMESTAMP created_at
        TIMESTAMP updated_at
    }

    topic_tags {
        SERIAL id PK
        VARCHAR slug UK
        VARCHAR label "标签名"
        VARCHAR category "类别: 事件/人物/关键词"
        VARCHAR status
        FLOAT score
        TIMESTAMP created_at
        TIMESTAMP updated_at
    }

    topic_tag_semantic_labels {
        BIGSERIAL id PK
        BIGINT topic_tag_id FK "标签ID"
        BIGINT semantic_label_id FK "小标签ID"
    }

    topic_tag_board_labels {
        BIGSERIAL id PK
        BIGINT topic_tag_id FK "标签ID"
        BIGINT semantic_board_id FK "板块ID"
        FLOAT score "匹配得分"
        VARCHAR match_reason "原因: 直接命中/命中率/最像/综合"
        TIMESTAMP created_at
        TIMESTAMP updated_at
    }

    board_composition {
        BIGSERIAL id PK
        BIGINT board_id FK "板块ID"
        BIGINT auxiliary_label_id FK "小标签ID"
    }

    narrative_boards {
        SERIAL id PK
        VARCHAR name "名称"
        TEXT description "描述"
        TEXT event_tag_ids "事件标签ID列表(JSON)"
        INTEGER semantic_board_id FK "来源板块"
        TEXT prev_board_ids "昨日简报ID列表(JSON)"
        VARCHAR scope_type "范围: 全局/按分类"
        INTEGER scope_category_id "分类ID"
        BOOLEAN is_system
        TIMESTAMP created_at
    }

    narrative_summaries {
        BIGSERIAL id PK
        VARCHAR title "标题"
        TEXT summary "摘要"
        VARCHAR status "状态"
        VARCHAR period "周期"
        INTEGER board_id FK "简报ID"
        TEXT related_tag_ids "关联标签(JSON)"
        TEXT related_article_ids "关联文章(JSON)"
    }
```

---

## 匹配算法 —— 用人话翻译

```
给一个标签 T，它有 n 个小标签，比如: 【伊朗, 导弹, 中东】
给一个板块 B，它有 m 个构成标签，比如: 【伊朗, 中东, 地缘政治, 霍尔木兹】

第一步：直接看有没有撞上的
  标签的小标签 = {伊朗, 导弹, 中东}
  板块的构成   = {伊朗, 中东, 地缘政治, 霍尔木兹}
  交集 = {伊朗, 中东} ← 直接撞上了！
  → 直接归入这个板块，原因: "直接命中"

第二步：如果没直接撞上，算"命中率"和"最像分"
  命中率 = 小标签里有多少个跟板块"长得像" / 总小标签数
  （"长得像" = 用存储坐标算余弦相似度 ≥ 0.6）
  最像分 = 所有小标签中跟板块最像的那个的相似度

第三步：三级判断
  规则1: 命中率 > 50% → 归入（大多数小标签都跟板块相关）
  规则2: 有某个小标签特别像板块 (≥0.8) → 归入（至少有一个强关联信号）
  规则3: 加权分 = 0.6×最像分 + 0.4×命中率 ≥ 门槛 → 归入
  三条都不满足 → 不归这个板块

最后：一个标签可能同时满足好几个板块的条件
  → 按得分从高到低排，最多取3个板块
```

---

## 核心配置参数

| 参数名 | 默认值 | 干啥用的 |
|---|---|---|
| `semantic_board_match_sim_threshold` | 0.6 | 小标签跟板块"多像"才算命中（计入命中率的最低门槛） |
| `semantic_board_match_direct_hit_rate` | 0.5 | 命中率超多少直接挂载 |
| `semantic_board_match_direct_max_sim` | 0.8 | 最像分超多少直接挂载 |
| `semantic_board_match_weight_sim` | 0.6 | 加权分里"最像"的权重 |
| `semantic_board_match_weight_density` | 0.4 | 加权分里"命中率"的权重 |
| `semantic_board_match_weighted_threshold` | 0.5 | 加权分门槛 |
| `semantic_board_match_max_boards` | 3 | 一个标签最多归几个板块 |
| `semantic_board_upgrade_ref_count_threshold` | 5 | 小标签被用了几次后才够格升级 |
| `semantic_board_merge_threshold` | 0.95 | L2自动合并的相似度门槛 |
| `semantic_board_cluster_distance` | 0.7 | 升级时分堆的距离门槛 |
| `semantic_board_cluster_max` | 10 | 升级时最多分几堆 |
| `semantic_board_cotag_window_days` | 30 | 升级时找共现事件看多少天内的 |
| `semantic_board_cotag_top_n` | 20 | 升级时最多取几个共现事件 |
| `semantic_board_cotag_dedup_threshold` | 0.85 | 共现事件去重的相似度门槛 |
| `semantic_board_cotag_hard_limit` | 15 | 每堆最多塞几个共现事件 |

---

## API 接口汇总

| 方法 | 路径 | 干啥的 |
|---|---|---|
| `GET` | `/api/semantic-boards` | 列出所有板块 |
| `GET` | `/api/semantic-boards/:id` | 看某个板块详情 |
| `POST` | `/api/semantic-boards` | 手动创建板块 |
| `PUT` | `/api/semantic-boards/:id` | 编辑板块 |
| `DELETE` | `/api/semantic-boards/:id` | 删板块 |
| `GET` | `/api/semantic-boards/:id/composition` | 看板块由哪些小标签构成 |
| `POST` | `/api/semantic-boards/:id/composition` | 往板块加小标签 |
| `DELETE` | `/api/semantic-boards/:id/composition/:labelId` | 从板块移除小标签 |
| `GET` | `/api/semantic-boards/suggest-auxiliaries` | 推荐候选小标签（创建板块时用） |
| `GET` | `/api/semantic-boards/:id/suggest-auxiliaries` | 推荐候选小标签（编辑已有板块时用） |
| `GET` | `/api/semantic-boards/upgrade-candidates` | 看哪些小标签够格升级了 |
| `POST` | `/api/semantic-boards/upgrade-suggest` | 让 LLM 生成升级建议 |
| `POST` | `/api/semantic-boards/upgrade-execute` | 确认执行某条升级建议 |
| `POST` | `/api/semantic-boards/backfill` | 触发回填重算归属 |
| `GET` | `/api/semantic-boards/backfill/:taskId` | 查回填进度 |
| `GET` | `/api/semantic-boards/matching-config` | 看匹配参数 |
| `PUT` | `/api/semantic-boards/matching-config` | 改匹配参数 |
| `GET` | `/api/auxiliary-labels` | 看所有小标签 |
| `PUT` | `/api/auxiliary-labels/:id/disable` | 禁用某小标签 |
| `PUT` | `/api/auxiliary-labels/:id/enable` | 启用某小标签 |
| `POST` | `/api/auxiliary-labels/merge-alias` | 手动合并别名 |
| `GET` | `/api/tags/:id/auxiliary-labels` | 看某个标签有哪些小标签 |
| `GET` | `/api/tags/:id/semantic-boards` | 看某个标签归了哪些板块 |
