# 项目重组设计文档

**日期:** 2026-03-08
**状态:** 已确认
**范围:** 根目录、文档入口、前端目录、后端目录、仓库清理

---

## 1. 目标

把当前项目从“历史文档和真实代码脱节”的状态，整理成一个可读、可维护、可继续扩展的仓库结构。

本次重组要同时解决 4 个问题：

1. 根目录入口太乱，读者不知道从哪进
2. 文档分散且失效引用很多
3. 前端目录按技术层堆积，业务边界不清
4. 后端目录按 handlers/services/models 横向切开，理解一个功能要跨很多目录

---

## 2. 现状判断

当前真实运行主线是：

- `front/` - Nuxt 4 前端
- `backend-go/` - Go 后端
- `docs/` - 零散文档和计划
- `tests/` - 独立测试目录

当前主要漂移点：

- `README.md` 仍引用不存在的 `crawl-service/`、`docs/QUICKSTART.md`、`CLAUDE.md`
- `PROJECT_STRUCTURE.md` 描述的是一套历史结构，不是当前仓库现实
- `front/app/composables/api/`、`front/app/services/`、`front/app/stores/api.ts` 三层职责重叠
- `backend-go` 文档仍大量描述 Python 兼容和旧数据库迁移叙事
- 仓库内夹杂生成产物和本地环境目录，影响判断真实结构

---

## 3. 设计原则

### 3.1 入口唯一

- 项目总入口只认 `README.md`
- 文档总入口只认 `docs/README.md`
- 前后端详细架构统一收进 `docs/architecture/`

### 3.2 目录按领域组织

- 前端从“大组件仓库”改成“按业务 feature 分组”
- 后端从“按技术层切分”改成“按 domain 分组”

### 3.3 文档必须描述真实状态

- 不保留已不存在目录的说明
- 不保留只服务一次性迁移的历史整理文档在根目录占位
- 所有引用路径必须存在

### 3.4 渐进重构

- 先修文档入口，再迁代码目录
- 先保行为不变，再收缩结构边界
- 路由、API 路径、主要运行命令先不改语义

---

## 4. 目标结构

### 4.1 顶层目录

```text
my-robot/
├── README.md
├── AGENTS.md
├── docs/
│   ├── README.md
│   ├── architecture/
│   ├── guides/
│   ├── operations/
│   ├── history/
│   └── plans/
├── front/
├── backend-go/
├── tests/
└── tools/                  # 仅在真实脚本存在时保留
```

### 4.2 前端目录

```text
front/app/
├── app.vue
├── pages/
├── api/
├── features/
│   ├── shell/
│   ├── categories/
│   ├── feeds/
│   ├── articles/
│   ├── summaries/
│   ├── digest/
│   └── preferences/
├── stores/
├── shared/
│   ├── components/
│   ├── composables/
│   ├── types/
│   ├── utils/
│   └── styles/
└── plugins/
```

### 4.3 后端目录

```text
backend-go/
├── cmd/
├── internal/
│   ├── app/
│   ├── platform/
│   │   ├── config/
│   │   ├── database/
│   │   ├── middleware/
│   │   └── ws/
│   ├── domain/
│   │   ├── categories/
│   │   ├── feeds/
│   │   ├── articles/
│   │   ├── summaries/
│   │   ├── preferences/
│   │   ├── content-processing/
│   │   └── digest/
│   └── jobs/
```

---

## 5. 文档信息架构

### 5.1 根 README

只保留：

- 项目简介
- 真实架构概览
- 启动方式
- 文档导航

### 5.2 docs/README.md

作为唯一文档地图，分成：

- 我想先了解项目
- 我想开发前端
- 我想开发后端
- 我想看数据流和业务流程
- 我想排查问题

### 5.3 文档分层

- `docs/architecture/` - 架构、数据流、目录规则
- `docs/guides/` - 功能说明和业务流程
- `docs/operations/` - 开发、数据库、编码安全、排障
- `docs/history/` - lessons learned、历史记录
- `docs/plans/` - 设计和实施计划

---

## 6. 前端重组策略

### 6.1 目标

让开发者面对前端时，只需要先判断“这是哪个 feature”，再进入该 feature 目录，而不是先猜组件/服务/composable/store 该落在哪条横向技术层。

### 6.2 主要调整

- `composables/api/` 迁为顶层 `api/`
- `components/` 中的领域组件拆到对应 `features/*/components/`
- `services/` 中的薄封装尽量删除，逻辑合回 feature composable 或 store
- `shared/` 只保留跨 feature 通用资源
- `stores/api.ts` 逐步降级，减少 `syncToLocalStores()` 这种手工同步模式

### 6.3 前端入口保留策略

- `app.vue` 继续承担应用启动壳
- `pages/` 保留为 Nuxt 路由入口
- 页面只组装 feature，不堆业务细节

---

## 7. 后端重组策略

### 7.1 目标

让开发者理解一个后端功能时，不需要来回跳 `handlers/`、`services/`、`models/`、`schedulers/`，而是进入对应领域目录即可看全。

### 7.2 主要调整

- `cmd/server/main.go` 中的启动装配抽到 `internal/app/`
- `pkg/database/` 并入 `internal/platform/database/`
- `config/`、`middleware/`、`ws/` 统一纳入 `platform/`
- 业务逻辑按 domain 收拢
- scheduler 的执行壳归拢到 `internal/jobs/`

### 7.3 领域划分

- `categories/`
- `feeds/`
- `articles/`
- `summaries/`
- `preferences/`
- `content-processing/` - Firecrawl、内容补全、抓取处理
- `digest/`

---

## 8. 风险与控制

### 8.1 风险

- 前端 import 路径会大量变化
- `apiStore` 拆薄过程中可能引入状态回归
- Go 包路径迁移会影响编译与测试
- 文档若不同步迁移，会再次产生失真

### 8.2 控制策略

- 先做文档归真，再做目录迁移
- 每一阶段结束都运行对应验证命令
- 暂时不修改 API 路径和页面路由语义
- 优先做“移动 + 引用修复”，延后做“深层逻辑拆分”

---

## 9. 验收标准

- `README.md` 不再引用不存在路径
- `docs/README.md` 能成为唯一文档入口
- 前端新增一个功能时，能明确落入某个 `features/*`
- 后端新增一个接口时，能明确落入某个 `domain/*`
- 架构文档与真实目录一致
- 前端类型检查和构建可通过
- 后端测试与启动命令可通过
