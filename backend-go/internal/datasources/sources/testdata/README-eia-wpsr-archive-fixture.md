# EIA WPSR 历史 fixture（board-signal-reports 阶段0核定）

真实官方响应存档，非合成数据。下载于 2026-09-22（北京时间），保留原始字节。

| 文件 | 来源 URL（真实请求，HTTP 200） | 周结日（当前周列） | 行数 |
| --- | --- | --- | --- |
| `eia-wpsr-archive-2026-08-26-table1.csv` | `https://www.eia.gov/petroleum/supply/weekly/archive/2026/2026_08_26/csv/table1.csv` | 8/21/26（+上周 8/14/26） | 59 |
| `eia-wpsr-archive-2019-01-04-table1.csv` | `https://www.eia.gov/petroleum/supply/weekly/archive/2019/2019_01_04/csv/table1.csv` | 12/28/18（+上周 12/21/18） | 55 |

结构要点（与现行 `ir.eia.gov/wpsr/table1.csv` 同构，可被 `parseEiaTable1` 同路径解析）：

- `STUB_1` 分区表头 + MDY 格式日期列（stocks 分区第二单元格为日期；supply 分区为 `STUB_1`,`STUB_2`）
- 目标行标签一致：`Crude Oil` / `Commercial (Excluding SPR)` / `Strategic Petroleum Reserve (SPR)`；`Crude Oil Supply` 分组下 `Domestic Production` / `Imports` / `Exports`
- 年代间总行数有差（2019=55 行、2026=59 行，后续年份追加行），解析按标签定位不受影响

获取方式备注：直接 GET 上述 URL 即可（www.eia.gov 归档为静态资源、无 key、无签名跳转）；现行主文件 `https://ir.eia.gov/wpsr/table1.csv` 会 302 到 CloudFront 签名 URL，需跟随重定向。WPSR 归档索引：`https://www.eia.gov/petroleum/supply/weekly/archive/`（年版 2011–2026，每版相对路径 `csv/table1.csv`）。
