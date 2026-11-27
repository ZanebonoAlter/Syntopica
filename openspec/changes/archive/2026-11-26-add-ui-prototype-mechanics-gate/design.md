# Design

## D0 两类原型：这个 change 是被一次澄清校准过来的

立项时的实现是「四项机械检查全扫」，写完 smoke 21 条全绿、归档门禁也挂上了。两次校准推翻了它：

**校准一（用户澄清）**：现存那个 504 文件/98M 的原型目录不是「原型成了垃圾桶」，是 **three.js 首页改版的视觉探索**——多版本实验页、渲染导出、blender 试点，这些都是那类工作的正常产物。用功能原型的规则查它没有意义。

**校准二（评审实测）**：另三项的静态判定是形式主义——
- EC-1：任何容器有一处 `padding: 24px` 就全过；`padding: 12px 24px`（合法的 24px 上下文）反被判红
- EC-2/EC-3：判「出现过 `:focus` 字样」——`outline: none` 过关，用浏览器默认焦点环的反而红

所以规范分两类原型，机器只查**危险操作确认**（它不依赖 CSS 解析），其余三项留作人核的设计下限。

**这条校准本身值得记**：我第一版的「四项全查」看起来更完整，实际是拿「覆盖面」换「判定可信度」，而不可信的判定会让人绕过门禁——比不查更糟。

## D1 判「有没有声明」而不是「渲染多大」

你选的是静态方案。它的取舍必须说清楚：

| 方案 | 能拦住 | 拦不住 | 成本 |
| --- | --- | --- | --- |
| 静态扫声明 | 「干脆没写尺寸」 | 「写了但只有 12px」 | 秒级，零依赖 |
| 浏览器实测 | 两者都拦 | 动态内容仍可能漏 | 每轮十几秒，98M 目录上更慢，树莓派抢资源 |

选静态的理由不是「够用」，而是**拦得住主要漏法**：真实原型里「忘记写尺寸」远多于「故意写小」。而且静态方案的误报可以靠改声明消除，实测方案的慢是每轮都要付。

**这条取舍写进规范注释**，避免后人以为这个门禁能保证视觉尺寸正确。

## D2 为什么是 24×24 而不是 44×44

44px 是移动端拇指标准（Apple HIG 44pt / Material 48dp）。Syntopica 是**单用户桌面 Web + 鼠标**：

- 桌面鼠标的瞄准精度远高于拇指
- 24×24 是 WCAG 2.2 AA 的 `Target Size (Minimum)` 标准
- 44px 用在桌面工具栏上会让一排图标明显变大变稀，反而降低信息密度

这也是规范第一次被写下来的地方——此前「44px」只活在 ISS-03 台账里，从未进权威文档。

## D3 只扫 HTML 与 CSS

现存原型目录的构成（实测）：

| 内容 | 数量 |
| --- | --- |
| `nodes/` | 86 文件 |
| `city-assembly-01/` | 41 |
| `city-grid-shots/` | 10 |
| `blender-pilot-07/` | 8 |
| `vendor3/`、`vendor/` | 2 |
| PNG | 9 |
| **真正的原型文件** | `index.html`（26688 行）、`fixture.json`、`space-skeleton.html` |

扫全目录只会把实验产物当原型检查，得到一堆无意义违规。所以脚本**只匹配 `*.html` 与 `*.css`**，且限定在 `ui-prototype/` 根与 `skeleton/` 子目录。

**反过来说**：那 500 个实验文件本身是个问题（原型目录成了工作区），但它属于那个 change 自己的事，不在本 change 范围。

## D4 挂载点必须能真拦

ISS-01 的教训写在这里：`diagrams-trace.sh` 当时只挂 `archive-readiness.sh`，而那是**作者自查脚本**，`spec-gate.ts` 才是真阻断点。结果归档能被绕过。

本 change 两处都挂，判定逻辑只在 `ui-prototype-check.sh` 一份（`spec-gate` 通过 spawn 调用，与检查⑧同一模式）。

## D5 双通道识别危险操作

| 通道 | 判据 | 现状 |
| --- | --- | --- |
| 文字关键词 | 删除/清空/重置/移除/彻底/销毁 | 立即可用 |
| `data-destructive` | 元素或祖先带该属性 | 新约定，存量 0 处 |

两通道是「或」：任一命中即要求同一元素（或 `onclick`/`<form>` 链路）上有确认——`data-confirm`、`confirm(`、`<dialog>`、或文字含「确认」的按钮。

为什么不用单一通道：
- 只认 `data-*` → 不标就检查不到，等于要么全绿要么全漏
- 只认文字 → 「移除筛选条件」这种无害操作会被误判

双通道的代价是有误报，但误报可以通过改文案消除——这比漏报好，漏报是无声的。

## 核心逻辑（伪代码）

### 关键数据结构

```
Finding = {
  code:  "ui-tap-target" | "ui-focus-missing" | "ui-hover-missing" | "ui-destructive"
  file:  string        // 仓库根相对
  line:  number        // 命中行
  detail: string       // 元素名 / 缺失的声明
}
ScanInput = { htmlFiles: string[], cssText: string }   // cssText 含 <style> 内联 + *.css 外链
```

### 主流程

```
main(changeDir):
    if not ui-impact(major):        exit 0     # none/minor 不查
    if not exists(ui-prototype/):   exit 0     # 无原型可查

    files = glob(ui-prototype/*.html) + glob(ui-prototype/skeleton/**/*.html)
    if findSkipMarker(files) 有非空理由:  exit 0   # 显式豁免，理由留痕
    if files 为空:                      exit 0

    css    = 所有 <style>...</style> 内容 + 所有 *.css 文本
    clickable = 匹配 button / a[href] / [role=button] / input[type=submit|button]

    findings = []

    # EC-1 尺寸声明：全局存在性。精确到元素要 CSS 选择器引擎，与零依赖不可兼得
    # （曾写成“逐元素匹配”，是过度承诺，已改）。缺省时列出可点元素清单帮定位。
    if clickable 非空 且 not css 含 ≥24px 的 min-height|min-width|padding:
        findings += ui-tap-target(附可点元素清单)

    # EC-2 / EC-3 伪类存在性（全局判定）
    if not css 含 ":focus" 或 ":focus-visible": findings += ui-focus-missing
    if not css 含 ":hover":                     findings += ui-hover-missing     # 载体分流：状态图形态豁免

    # EC-4 危险操作确认（逐元素：这里只需看邻近文本与标签属性，不需选择器匹配）
    for el in clickable:
        if isDestructive(el)  且 not hasConfirmation(el):
            findings += ui-destructive(el)

    if findings 非空: 打印分组清单 → exit 1
    else:             exit 0
```

### 失败与边界分支

- **`ui-impact` 声明缺失或非法**：exit 0（fail-open）——声明完整性归 `ui-design-gate` 前置门禁，不在这里重复判
- **`<style>` 与外部 CSS 都没有**：EC-2/EC-3 直接红（无样式 = 无可点反馈），除非有跳过标记
- **画布类无 HTML 原型**：`files` 为空 → exit 0。V3–V5 由 `interaction-table.md` 的「可访问与动效声明」承接，那是人写的，机器不判
- **一个 HTML 都没有但目录存在**：exit 0 并打一行提示（不猜意图）
- **正则解析的固有风险**：不做完整 HTML 解析，用行级正则找标签。嵌套/跨行标签会漏 → 这是明确接受的精度上限，宁可漏报不误报
- **跳过标记**：`data-ui-mechanics-skip="<理由>"` 放在 `<html>` 或 `<body>` 上，理由非空才生效（与 `ui-skip` 同纪律：显式留痕优于隐形跳过）
