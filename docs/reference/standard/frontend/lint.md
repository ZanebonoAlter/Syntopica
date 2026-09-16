# 前端 Lint 配置（ESLint）

> **权威源**：本文件是前端静态检查配置的唯一权威。门禁命令见《开发执行规范》§5.1。
> Lint 在所有平台可用；typecheck/build 在 Windows + WSL 宿主必须走 Windows cmd（见 `shared/commit-pr.md` 跨平台节）。

## 配置文件

`front/eslint.config.js`（flat config + typescript-eslint）

## 启用规则

| 规则类别 | 说明 |
|---------|------|
| JS 基础 | `@eslint/js` recommended |
| TypeScript | `typescript-eslint` strict-ish |
| Vue | `eslint-plugin-vue` recommended |

## 安装（缺失时作为首个子任务）

```bash
pnpm add -D eslint @eslint/js typescript-eslint
```

## 运行

```bash
cd front
pnpm lint
```

## 资料来源

收敛自原《开发执行规范》§5.3 与 `development.md` §前端代码风格。
