# 灾备恢复链路图（补档样板）

> 本文件由 change `add-change-diagram-convention` 补入，作为「change 图表达」的参考样板。
> 性质：非 schema 制品，只新增、不改动本 change 的任何原制品（§12 补档范畴）。
> 事实来源：本目录 `proposal.md`（三条根因 L1/L2/L3）与 `design.md`（D1–D10）。

## 用户故事核心路径

**这张图回答**：一个值班人从「应用起不来」到「应用可用」要走哪几步，哪些判断会导致回退。

```mermaid
flowchart TD
    A[应用起不来 / 需要灾备恢复] --> B[选定备份档 + 同行数基线]
    B --> C{恢复脚本前置闸门}
    C -- "目标库名 = 生产库" --> C1[拒绝执行<br/>需显式覆盖 + 二次确认]
    C1 --> C
    C -- 新建一次性恢复库 --> D[预装 vector / pg_trgm]
    D -- 创建失败 --> D1[退出，不进入 restore]
    D --> E[过滤 dump TOC 的 SCHEMA public]
    E --> F[pg_restore]
    F --> G[count 完整性校验]
    G -- 不匹配 --> G1[报告差异，人工判读]
    G -- 通过 --> H[迁移状态对账]
    H --> I[启动应用]
    I --> J[Phase1 RunAutoMigrate<br/>建表 / 加列]
    J --> K[Phase2 RunMigrations<br/>版本迁移]
    K -- "档含 schema_migrations（L1 之后）" --> L[跳过已应用迁移]
    K -- "历史档不含（存量 7 份）" --> M[从零重放]
    M -- "旧写法 vector(4096)" --> N[维度不匹配<br/>应用退出]
    M -- "无限定 vector（L2）" --> L
    L --> O[应用可用]
```

**图不表达的约束**：count 校验与迁移对账只出报告，缺口不阻断退出码（可再生表由 AutoMigrate 重建）；恢复耗时量级 35min（SD 卡 1.7MB/s），不在图上体现。

## 状态流转

**这张图回答**：备份档「含/不含迁移历史」两种状态，在旧 DDL 与新 DDL 下分别落到哪个终态。

```mermaid
stateDiagram-v2
    [*] --> 备份档产出
    备份档产出 --> 含迁移历史: L1 之后产出
    备份档产出 --> 不含迁移历史: L1 之前产出（存量 7 份）
    含迁移历史 --> 迁移跳过: 启动 Phase2
    不含迁移历史 --> 迁移重放: 启动 Phase2
    迁移重放 --> 迁移失败: DDL 目标 vector(4096)
    迁移重放 --> 迁移完成: DDL 目标无限定 vector
    迁移跳过 --> 应用可用
    迁移完成 --> 应用可用
    迁移失败 --> 应用退出
    应用可用 --> [*]
    应用退出 --> [*]
```

**图不表达的约束**：「无限定 vector」是去掉 typmod、放宽而非收紧约束，因此对任意维度存量数据都成立；存量列统一（`unify-vector-column-dimension`）不在本图范围，仍保持 `vector(2560)`。

## 顺序图

**这张图回答**：备份、恢复、启动三段里，谁先调用谁，哪一步失败会拦住后续。

```mermaid
sequenceDiagram
    participant Cron as cron 04:00
    participant Backup as backup-key-tables.sh
    participant PG as 源库
    participant Disk as backups/
    participant Restore as restore-key-tables.sh
    participant NewPG as 一次性恢复库
    participant App as 应用进程

    Note over Cron,Disk: 每日备份
    Cron->>Backup: 触发
    Backup->>PG: pg_dump -Fc（排除可再生表，含 schema_migrations）
    PG-->>Backup: dump
    Backup->>PG: 逐表 count
    PG-->>Backup: 行数基线
    Backup->>Disk: 写 dump + 同行数基线
    Note over Disk: 轮转保留 7 份，基线与档同生命周期

    Note over Restore,NewPG: 灾难恢复
    Restore->>Restore: 目标库名 = 生产库？是则拒绝
    Restore->>NewPG: CREATE DATABASE
    Restore->>NewPG: CREATE EXTENSION vector / pg_trgm
    alt 扩展创建失败
        Restore-->>Restore: 退出，不进入 restore
    end
    Restore->>NewPG: pg_restore（过滤 SCHEMA public）
    Restore->>NewPG: count 校验 + 迁移状态对账
    NewPG-->>Restore: 完整性报告

    Note over App,NewPG: 应用启动
    App->>NewPG: Phase1 RunAutoMigrate（建表 / 加列）
    App->>NewPG: Phase2 RunMigrations（版本迁移）
    alt 档含 schema_migrations
        App-->>App: 跳过已应用
    else 历史档（不含）
        App-->>App: 从零重放（L2 保证不冲突）
    end
    App-->>App: 应用可用
```

**图不表达的约束**：备份与恢复都是单机磁盘操作，无异地同步与加密；恢复脚本不自动执行，必须人工触发（D3 硬闸）；演练按 D10 人工定期执行，未接 cron。
