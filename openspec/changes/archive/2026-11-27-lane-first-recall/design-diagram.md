# 设计图：lane-first-recall

> 只画「看文字容易漏」的三处：双通道收集的先后与去重、两阶段召回的门槛位置、以及 `filterTagsByQuality` 二元开关被绕过的确切点。

## 1. 候选收集：双通道 + 两阶段召回

```mermaid
flowchart TD
    A[GenerateDailyReport Step1<br/>collectBoardTags] --> B[版块通道<br/>挂载 + fallback 现场匹配]
    A --> C{遍历该版块 active 泳道}

    C -->|逐泳道| D[锚点 = centroid<br/>缺失才退 embedding]
    D --> E{锚点可用?<br/>非空 / 不超 halfvec 上限 / 非 NaN}
    E -->|否| E1[skipped++, 跳过本泳道]
    E -->|是| F[阶段1 ANN：halfvec 距离取 id 池<br/>pool = TopK×4, 下限 16<br/>已排除本板 direction_mismatch]
    F --> G[阶段2 原列 float32 复核同一池<br/>重放同样过滤条件]
    G --> H{recallDecision<br/>按距离升序逐条收<br/>distance < maxDistance 且 ≤ TopK}
    H -->|一条都不收| H1[empty++<br/>该泳道当天留白]
    H -->|有| I[mergeNearestLaneRecall<br/>同一标签多泳道取最近]
    I --> C

    B --> J[去重合并]
    C -->|收集完毕| J
    J --> K[版块通道标签：保留 MatchReason/Score]
    K --> L[仅泳道通道标签：MatchReason = lane_recall]
    L --> M[跨版块排他<br/>laneRecallExclusivityAllowed]
    M --> N[filterTagsByQuality<br/>泳道证据直通 kept]
    N --> O[Step3 ClusterTagsLane]
```

## 2. `filterTagsByQuality` 的二元开关与绕过点

```mermaid
flowchart TD
    A[去重后的标签集] --> B[分区 kept / weighted]
    B --> C[kept = composite_hit ∪ direct_hit<br/>∪ hit_rate ∪ max_sim]
    B --> D[weighted = 其余<br/>单篇新事件恒在此]
    C --> E{len&#40;kept&#41; < 10 ?}
    E -->|是| F[weighted 全放回<br/>噪声也进]
    E -->|否| G[weighted 全丢<br/>11-19 起常见分支]
    D --> H{标签在 laneRecalled 集合?}
    H -->|是| I[直接并入 kept<br/>—— 不改二元开关本身，只给泳道证据一个独立入口]
    H -->|否| D2[留在 weighted]
    I --> E
```

> 为什么要独立入口而不是改开关：开关的两端各自都对应一次真实事故（10-04 非农、10-10 digest），
> 收紧的是「挂载层」这条永久事实防线。泳道证据是**单日行为**，绕过开关但不写挂载表，
> 两者语义不同层，混同即回到老路。

## 3. 排他判定与 gap=0 的短路位置

```mermaid
flowchart TD
    A[pending = 仅泳道通道带来的标签] --> B{gap <= 0 ?<br/>排他规则关闭}
    B -->|是| C[全部放行<br/>且不发那条 SQL —— 调用方短路保证无库可用]
    B -->|否| D[查这些标签在「其他版块」的挂载最高分]
    D --> E[laneRecallExclusivityVerdict<br/>bestOther - ownScore < gap]
    E --> F[放行的进入候选集]
    C --> F
    E -.->|gap<=0 时该式退化为 0-0<0 = false<br/>语义完全相反| G[因此 verdict 内部也自带同一短路<br/>单看该函数不会被误用]
```

## 4. L3 防污染闸（D4）

```mermaid
sequenceDiagram
    participant O as Orchestrator
    participant L2 as L2 裁决
    participant L3 as L3 自建新泳道
    O->>L2: 泳道召回标签 + 候选话题
    alt 判 keep
        L2-->>O: 归入已有泳道
    else 判 new 且属 laneRecalled
        L2-->>O: new
        O->>O: 直接丢弃（记 drop 日志）
        Note over L3: 不进入 L3 —— 泳道召回只有弱证据，<br/>不许凭它新建持久话题
    else 判 new 且非召回标签
        L2-->>O: new
        O->>L3: 允许自建新泳道（既有行为不变）
    end
```
