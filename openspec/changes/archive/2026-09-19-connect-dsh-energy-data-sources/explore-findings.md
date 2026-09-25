
## EIA/JODI 免费公开数据源只读 MCP 解析契约（实测）

# EIA/JODI 只读 MCP 解析契约（2026-09-02 实测，全程匿名无 key）

## 1. EIA WPSR table1.csv
- URL（固定，周更覆盖同一 URL）：https://ir.eia.gov/wpsr/table1.csv ；HTTP200，6,593B，text/plain，ETag 存在，Last-Modified=发布时刻（本次 Wed 2026-09-02 13:49 GMT，印证周三发布节奏）。
- 编码/格式：字节级 CP1252（非 ASCII 字节仅 0x96=en-dash "–"），CRLF 行尾。所有单元格带引号；数值含千分位逗号（"13,862"），解析须去逗号。日期格式 M/D/YY（8/28/26）。
- 缺失标记：Percent Change 类单元格为 "– –"（0x96 0x20 0x96 成对 en-dash）；用 UTF-8 强解会变乱码。映射 null，不得当 0。
- 结构（单文件两分区，各自表头）：
  - A 区（L1-20 库存摘要）：`"STUB_1","当周","上周","Difference","Percent Change","去年同期","Difference","Percent Change"`；行：Crude Oil / Commercial (Excluding SPR) / Strategic Petroleum Reserve (SPR) / Total Motor Gasoline(细分) / Fuel Ethanol / Jet / Distillate(细分) / Residual / Propane / Other Oils / Unfinished / Total Stocks (Incl/Excl SPR)。单位=百万桶（CSV 内不标注，依 WPSR 官方表定义，勿从数值猜）。
  - B 区（L21-58 供需）：`"STUB_1","STUB_2","当周","上周","Diff","去年同期","Diff","4周均当期","4周均同期","Pct","YTD当期","YTD同期","Pct"`；行分组前缀 "Crude Oil Supply "/"Other Supply "/"Products Supplied "/"Net Imports of Crude and Petroleum Products "（标签有尾随空格），行标签带脚注号如 "(1)     Domestic Production"。单位=千桶/日（同样 CSV 内不标注）。
- 四个目标行实测存在（8/28/26 期）：商业原油库存 = A 区 Crude Oil→"Commercial (Excluding SPR)" 424.460 MMbbl（注意顶层 "Crude Oil" 711.064 为含 SPR 总量，勿混用）；原油产量 = B 区 "(1) Domestic Production" 13,862 Mb/d；原油进口 = "(8) Imports" 6,770（"(9) Commercial Crude Oil" 同值）；原油出口 = "(12) Exports" 4,483 Mb/d。
- 范围/修订：仅美国、周度（期=周五截止）；CSV 无修订标记列、无发布日期字段（发布时刻只能取 HTTP Last-Modified）；后续周修订窗口未核实。

## 2. JODI Oil Primary（World Database）
- 精确下载 URL：https://www.jodidata.org/_resources/files/downloads/oil-data/annual-csv/primary/primaryyear2026.csv （明文 CSV，非 zip；HTTP200 4,760,442B≈4.76MB，application/octet-stream，Last-Modified 2026-08-20）。历史年同目录 `2002.csv`…`2025.csv`；Secondary 同理 `…/secondary/secondaryyear2026.csv`。下载页 https://www.jodidata.org/oil/database/data-downloads.aspx 为静态 HTML，链接可直接提取（另有 jodi-oil-country-note.xlsx、jodi_oil_data_availability_by_country.xlsx、jodi-oil-wdb-item-names-ver2017.pdf、world_ext.zip?iid=180）。
- 编码/表头：UTF-8 无 BOM；7 列 `REF_AREA,TIME_PERIOD,ENERGY_PRODUCT,FLOW_BREAKDOWN,UNIT_MEASURE,OBS_VALUE,ASSESSMENT_CODE`；2026 档 115,200 数据行。
- 维度全集（实测枚举）：REF_AREA 96 个 2 字母码（US="US"）；TIME_PERIOD=`YYYY-MM`（2026 档含 2026-01…06）；ENERGY_PRODUCT 4：CRUDEOIL/NGL/OTHERCRUDE/TOTCRUDE；FLOW_BREAKDOWN 10：CLOSTLV,DIRECUSE,INDPROD,OSOURCES,REFINOBS,STATDIFF,STOCKCH,TOTEXPSB,TOTIMPSB,TRANSBAK（无字面 production/import/export/closing stocks 名；映射：产量=INDPROD、进口=TOTIMPSB、出口=TOTEXPSB、期末库存=CLOSTLV）；UNIT_MEASURE 5：CONVBBL,KBBL,KBD,KL,KTONS——每 product×flow 五单位并存成多行，CONVBBL 取值 ~7400 疑为换算系数但语义未核实，不得当流量值用。
- 缺失标记（OBS_VALUE 非数值实测三种）：`-` 45,728 行、`..` 9,828 行、`x` 4,608 行（'x' 集中出现在 KBD×存量类组合）。一律原样映射 null 并保留标记，绝不转 0、不用缺失行凑结果；标记官方语义未核实。
- ASSESSMENT_CODE：实测值仅 1/2/3（分布 3:89,756 / 1:24,863 / 2:581）；官方语义在 item-names-ver2017.pdf，本轮未取，语义未核实。
- US 原油四流量实测可查（2026-06 原始行）：INDPROD/KBD=13818.0000(code1)；TOTIMPSB/KBD=5452.5000(code1)；TOTEXPSB/KBD=4223.3000(code2)；CLOSTLV/KBBL=668715.0000(code1，同组 KBD='x')。
- 时效/修订：TIME_PERIOD 是数据期非发布期；文件发布滞后数据月约 1.5~2 个月；整文件覆盖式更新（重下载即最新修订值，无修订历史列）；96 经济体参与≠每月全覆盖，须逐 国家×产品×流量×期 判缺失。

## 3. 建议工具形态（每源一个，固定只读，无任意 URL/路径/代码执行）
- `eia_wpsr_table1(section: enum{stocks,supply})`：固定 URL，返回分区行数组 `{section, group?, label, footnote_no?, values:[{column_meta, value|null}], unit:"MMbbl"|"Mb/d", period_week_ending, source_row_no}`；en-dash→null。
- `jodi_oil_primary(geo: enum(96码), product: enum(4), flow: enum(10), unit: enum(5), month: "YYYY-MM"|null)`：URL 仅年份模板化，返回 `{obs_value|null, missing_marker?, assessment_code, source_row_no}`；month=null 取该文件最新期。
- 红线：EIA 是美国数据不得称全球；JODI 缺失('-'/'..'/'x')不转 0 不凑数；MMbbl / Mb/d / KBBL / KBD / KL / KTONS 不得自动跨单位换算合并；周度(EIA)与月度(JODI)频度、数据期口径不同，跨源合并须显式要求。

## 证据文件（已落盘，小规模非敏感）
- openspec/changes/connect-dsh-energy-data-sources/evidence/eia-wpsr-table1-sample.csv（6,593B 全文件原件）
- openspec/changes/connect-dsh-energy-data-sources/evidence/jodi-us-crude-sample.csv（US×3 原油产品×10 流量×5 单位×6 期 = 900 行）
未核实项清单：EIA 修订窗口与发布时刻表（仅推断自 Last-Modified）；JODI ASSESSMENT_CODE 官方语义、CONVBBL 语义、三种缺失标记官方语义；两源商业再分发许可。

<!-- pinned 2026-09-09T01:27:07Z -->
