# desktop/lab/embeddedpg —— embedded PG 真实负载验证实验台

迭代 0 议题 **0-7**（embedded PG 真实负载验证）的可复跑实验台。结论见
[`docs/srs/syntopica-client/iter0-findings.md`](../../../docs/srs/syntopica-client/iter0-findings.md) 的
「0-7 embedded PG 真实负载验证」节。

> ⚠️ **红线**：本目录的一切产物都是**一次性实验品**。真库（Docker `syntopica-postgres`）
> 只允许**只读**访问；备份演练的输出目录指向 `_tmp/`，绝不写仓库的 `./backups/`（cron 的救命恢复通道）。
> 本目录有独立 `go.mod`（module `syntopica/lab/embeddedpg`），**不碰** `backend-go/go.mod`。

## 组件

| 路径 | 作用 |
| --- | --- |
| `cmd/lab/main.go` | 主控：`provision`（下载 PG）/ `start`（起实例 + 装 pgvector）/ `bench`（冷启动计时） |
| `cmd/fetch/main.go` | 缓存预置工具：可续传下载 PG 包 + supervisor，SHA-256 校验后按库的缓存布局落盘 |
| `scripts/install-pgvector.sh` | 把 pgvector 三件套（`vector.so` + `vector.control` + `vector--*.sql`）装进 PG 安装目录 |
| `scripts/backup-key-tables-embedded.sh` | `scripts/db/backup-key-tables.sh` 的 embedded PG 平移版（EXCLUDED 语义原样保留） |
| `scripts/pg-rss.sh` | 统计 embedded PG 进程组（postmaster + 子进程）RSS |
| `scripts/bench-vectors.sh` | 三域向量查询 EXPLAIN ANALYZE 对比（tag / daily_report / discovery） |
| `_tmp/` | 全部运行时产物（pgdata / 安装目录 / 下载缓存 / 备份档 / 结果）——已 gitignore |

## 跑完一遍留下的产物（2026-11-04 那次实测）

| 路径 | 内容 | 可否删 |
| --- | --- | --- |
| `_tmp/install/postgresql-18.6.1/` | PG 18.6.1 运行时 + 已装入的 pgvector 0.8.7（131MB） | 可删（重跑 provision + install-pgvector 即可重建） |
| `_tmp/cache/` | 下载缓存（只剩 supervisor 二进制，PG 包已被清掉） | 可删（`fetch pg` 重下 39.9MB） |
| `_tmp/backups/pg-key-20261104-192226.dump` | 备份演练产出的档（1021MB，与 cron 档同量级） | 可删（跑一次备份脚本 9.9min 重建） |
| `_tmp/results/` | 全部实测数字（bench JSON / 三域 EXPLAIN 对比 / count 对账 / 设置快照 / 日志） | **建议保留**（结论的证据） |
| `_tmp/lab`、`_tmp/fetch`、`_tmp/syntopica-server` | 编译产物 | 可删（重新 build） |

## 复跑步骤

```bash
cd desktop/lab/embeddedpg

# 1) 编译实验台
go build -o _tmp/lab ./cmd/lab && go build -o _tmp/fetch ./cmd/fetch

# 2) 拿 PG 18.6.1 二进制（embedded-postgres v2.0.1 的默认版本）
#    直接跑 provision 走库自带的 DownloadProvider；本机代理慢时用 fetch 预置：
LAB_CACHE=$PWD/_tmp/cache ./_tmp/fetch pg          # PG 包（39.9MB）+ SHA 校验 + 解压
LAB_CACHE=$PWD/_tmp/cache ./_tmp/fetch supervisor  # v2.0.1 supervisor 二进制
./_tmp/lab provision                               # 复制成「安装目录」（LocalProvider 用）

# 3) 装 pgvector（首选：从官方镜像抠已编译好的 arm64 产物，不本地编译）
bash scripts/install-pgvector.sh pgvector/pgvector:0.8.7-pg18-trixie
#    兜底：本地 PGDG 装好的 <pgroot> 目录
#    bash scripts/install-pgvector.sh /usr/lib/postgresql/18

# 4) 起实例（DataDir 持久，Ready hook 自动 CREATE EXTENSION vector）
LAB_HOLD=1 ./_tmp/lab start          # 打印 DSN 后驻留；Ctrl-C 优雅停
#    DSN 形态：postgresql://postgres:postgres@127.0.0.1:55433/syntopica?sslmode=disable

# 5) 冷启动计时（fresh=含 initdb；warm=复用 data dir）
LAB_BENCH_MODE=fresh LAB_RUNS=3 ./_tmp/lab bench
LAB_BENCH_MODE=warm  LAB_RUNS=5 ./_tmp/lab bench

# 6) 导入真档做真实负载验证（新建一次性库，绝不碰真库）
DSN=.../restore2 ./_tmp/install/postgresql-18.6.1/bin/pg_restore \
  -d "$DSN" --no-owner --no-privileges -L <(pg_restore -l dump | grep -v 'SCHEMA - public') -j 4 dump

# 7) 三域向量查询对比（两边跑同一段 SQL）
OUT=_tmp/results/vectors-embedded.txt bash scripts/bench-vectors.sh "$DSN" embedded
OUT=_tmp/results/vectors-docker.txt  bash scripts/bench-vectors.sh <docker真库DSN> docker

# 8) 备份/恢复演练（输出到 _tmp，不碰 ./backups/）
PGHOST=127.0.0.1 PGPORT=55433 PGUSER=postgres PGDATABASE=restore2 \
  BACKUP_DIR=$PWD/_tmp/backups bash scripts/backup-key-tables-embedded.sh
```

## 踩过的坑（结论级，供后续 change 复用）

1. **PG 15+ 的 dump 里带 `CREATE SCHEMA public`** → 往「已有 public schema + 已装扩展」的新库 restore 会炸
   （`schema "public" already exists`；加 `--clean` 又会撞 `DROP SCHEMA public` 与 vector 扩展的依赖）。
   可复跑解法：`pg_restore -l` 出 TOC → 滤掉 `SCHEMA - public` 行 → `pg_restore -L <toc>`。
2. **恢复前的扩展安装顺序**：dump 不含 `CREATE EXTENSION vector`（`-n public` 不带扩展），而表定义里
   直接用 `vector(2560)` 类型 → 必须**先** `CREATE EXTENSION vector` 再 restore。
3. **embedded-postgres v2 的 `DownloadProvider` 用 2 分钟超时的 http.Client**：慢代理下 39.9MB 的包必超时
   （实测 EOF / context deadline）。解法：用 `cmd/fetch` 续传下载 + SHA 校验后按缓存布局预置。
4. **`StartParameters` 不接受生命周期托管的键**：`listen_addresses` / `unix_socket_directories` /
   `data_directory` / `port` 等由库自己管，塞进去直接报错。
5. **pgvector 0.8.7 的 hnsw/ivfflat 索引不支持 >2000 维**；Syntopica 的向量是 **2560 维** →
   生产索引用的是 `((embedding)::halfvec(2560))`（halfvec 上限 4000 维），照抄这个写法。
6. **PG 停机不是瞬时的**：上一个实例还在收尾时起下一个 → `lock file "postmaster.pid" already exists`。
   桌面端关停流程要等 postmaster 真退出。
7. **扩展 `.so` 的跨小版本 ABI**：容器里的 `vector.so`（PG 18.4 编）直接放进 embedded 的 PG 18.6.1 能加载
   （PG 同 major 内 ABI 稳定）；但更稳的是同 minor 的产物（`pgvector/pgvector:0.8.7-pg18-trixie` 内是 PG 18.6）。
