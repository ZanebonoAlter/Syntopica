<!-- domain: ui -->

# ui-icon-localization Specification

## Purpose
TBD - created by archiving change localize-icons. Update Purpose after archive.
## Requirements
### Requirement: 本地图标子集产物
项目 SHALL 维护一份本地图标子集产物（`app/assets/iconify-subset.json`），内容为源码中实际使用的全部 iconify 图标（当前全部为 `mdi` 前缀），从 `@iconify-icons/mdi` 全量数据中提取生成。产物 SHALL 纳入 git 管理，不依赖每个开发者本地执行生成脚本即可启动开发。

#### Scenario: 子集覆盖源码全部图标
- **WHEN** 用同一提取规则扫描 `app/` 源码得到图标名集合 S，与子集产物中的图标名集合比较
- **THEN** 子集产物包含 S 中的每一个图标名

#### Scenario: 新增图标后重新生成
- **WHEN** 开发者在源码中使用了子集产物之外的图标名
- **THEN** 通过重跑生成脚本（如 `pnpm generate:icons`）更新子集产物并提交

### Requirement: 启动时注册本地图标数据
前端 SHALL 通过 Nuxt plugin 在应用启动时将本地图标子集通过 `@iconify/vue` 的 `addCollection` 注册，使 `<Icon>` 组件命中本地数据，运行时 SHALL NOT 向 `api.iconify.design` 等外部 iconify API 发起请求。

#### Scenario: 离线渲染 mdi 图标
- **WHEN** 断网或代理环境下打开任意含 `<Icon icon="mdi:xxx">` 的页面，且该图标在子集中
- **THEN** 图标正常渲染，浏览器无指向 iconify API 域名的网络请求

#### Scenario: 未注册图标的兜底
- **WHEN** 渲染的图标名不在本地子集中
- **THEN** 允许 `@iconify/vue` 按默认行为尝试网络加载（视为开发疏漏，由子集覆盖校验兜底发现）

### Requirement: 图标子集一致性校验
项目 SHALL 提供可执行的校验方式（脚本或单元测试），断言「源码扫描得到的图标名集合」是「子集产物」的子集，用于在 pre-push / CI 阶段发现未重新生成子集的疏漏。

#### Scenario: 校验通过
- **WHEN** 子集产物已包含源码全部图标名，执行校验
- **THEN** 校验退出码为 0

#### Scenario: 校验失败提示修复方式
- **WHEN** 源码出现子集产物之外的图标名，执行校验
- **THEN** 校验以非 0 退出码失败，并输出缺失的图标名与重新生成命令

