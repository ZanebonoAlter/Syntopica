# detective-wall-scene

## Purpose

Three.js 3D 侦探照片墙场景，负责卡片、红线、迷雾、光照和后处理的创建与管理。接收 Vue 层传入的 section/relation 数据，映射为 3D 对象并渲染。在核心场景基础上叠加"侦探办公桌台灯下"的环境纵深层：前景桌面 + 远景墙 + 台灯道具 + 卷宗堆 + 半球光 + 环境贴图 + 方向性雾 + 尘埃微粒。

## Requirements

### Requirement: TopicWallScene 核心场景

TopicWallScene SHALL 管理 Three.js 场景的完整生命周期，暴露核心对象供 InteractionLayer 和 DirectorCamera 使用。

- `loadBoardData(sections, relations, dateRange, days)`：加载板块数据
- `clearScene()`：清空场景（切换板块前调用）
- `startRenderLoop()` / `stopRenderLoop()` / `onResize(width, height)`：渲染控制
- `addFrameCallback(fn)`：注册每帧回调（如 OrbitControls.update）
- `setSelectionLight(card | null)`：聚焦卡片上方红光
- `dispose()`：清理所有资源

暴露成员：scene, camera, renderer, css2d, composer, cardGroup, redStrings, fog, followLight, selectionLight

#### Scenario: loadBoardData

- **WHEN** loadBoardData 被调用
- **THEN** 系统 SHALL 调用 CardGroup.buildCards 创建卡片
- **AND** 创建对应的 RedString 红线
- **AND** 设置 FogSystem 密度

#### Scenario: 渲染循环

- **WHEN** 每帧渲染
- **THEN** 系统 SHALL 依次调用所有 frameCallbacks
- **AND** composer.render() 输出到屏幕

#### Scenario: dispose

- **WHEN** dispose() 被调用
- **THEN** 系统 SHALL 清理所有 geometry、material、texture，避免内存泄漏

### Requirement: CardGroup 卡片组

CardGroup SHALL 管理所有 PinCard 的创建、布局和批量动画。

- `buildCards(sections, relations, dateRange)`：调用布局算法创建所有卡片
- `getCardById(sectionId)` / `cards: PinCard[]`：卡片访问
- `staggerEntrance(intervalSec)` / `staggerExit(intervalSec)`：批量入场/退场动画
- `highlightSet(ids)` / `dimAll()` / `resetAll()`：高亮控制

#### Scenario: 构建与访问卡片

- **WHEN** `buildCards(sections, relations, dateRange)` 被调用
- **THEN** CardGroup SHALL 调用布局算法创建所有 PinCard，并可通过 `getCardById(sectionId)` / `cards` 访问

#### Scenario: 批量动画与高亮

- **WHEN** 数据加载完成或聚焦切换
- **THEN** CardGroup SHALL 通过 `staggerEntrance`/`staggerExit` 编排批量入场/退场动画，通过 `highlightSet(ids)`/`dimAll()`/`resetAll()` 控制高亮

### Requirement: PinCard 卡片

PinCard SHALL 表示单个话题卡片，包含 paper + pin + text + CSS2D tooltip。

- `data: SectionTimelineNode`：关联数据
- `group: THREE.Group`：3D 对象（paper + pin + text）
- `position: THREE.Vector3`：世界坐标
- `tooltip: CSS2DObject`：卡片上方 tooltip（data-card-id, pointer-events:auto）

#### Scenario: elevate / settle

- **WHEN** elevate() 被调用（悬停）
- **THEN** 卡片 SHALL 沿 Z 抬起 + 显示 tooltip
- **WHEN** settle() 被调用（取消悬停）
- **THEN** 卡片 SHALL 回到原位 + 隐藏 tooltip

#### Scenario: highlight / dim / reset

- **WHEN** highlight() 被调用
- **THEN** emissiveIntensity SHALL 增强到 0.2（非 0.5，避免盖住文字）
- **WHEN** dim() 被调用
- **THEN** opacity SHALL 降低（退入背景）
- **WHEN** reset() 被调用
- **THEN** 卡片 SHALL 恢复正常状态

#### Scenario: tooltip 内容

- **WHEN** tooltip 显示
- **THEN** 内容 SHALL 为话题名 + 状态（中文）
- **AND** 标题超 14 字符加 `…` 截断

### Requirement: RedString 红线

RedString SHALL 表示两个话题之间的关系连线，使用 CatmullRomCurve3 with 2 points（直线，不弯曲）。

- `fromId / toId / distance`：两端节点 ID 和距离
- `draw(progress)`：drawProgress 0→1 逐渐出现
- `highlight() / dim() / reset()`：线宽增大 + 发光 / 退入背景 / 恢复正常

#### Scenario: 绘制与高亮

- **WHEN** BFS 生命线展开需要呈现红线
- **THEN** `draw(progress)` SHALL 按 drawProgress 0→1 逐渐出现（CatmullRomCurve3 with 2 points，直线）
- **AND** `highlight()` SHALL 增大线宽 + 发光，`dim()` SHALL 退入背景，`reset()` SHALL 恢复正常

### Requirement: FogSystem 迷雾

FogSystem SHALL 根据天数设置迷雾密度，提供动画过渡。

- `setDensityForDays(days)`：7→0.08, 14→0.05, 30→0.03, 60→0.02
- `animateToDensity(density, durationSec)`：GSAP 过渡
- `disable()` / `enable(days)`：完整生命周期模式控制

#### Scenario: 按天数设置迷雾密度

- **WHEN** `setDensityForDays(days)` 被调用
- **THEN** 迷雾密度 SHALL 按天数映射设置（7→0.08, 14→0.05, 30→0.03, 60→0.02）
- **AND** `animateToDensity(density, durationSec)` SHALL 以 GSAP 过渡动画到目标密度；`disable()`/`enable(days)` SHALL 服务完整生命周期模式

### Requirement: 布局算法

系统 SHALL 按日期列 × 行排列卡片：

1. 提取唯一日期排序
2. dateX = indexOf(date) × COL_W (3.0)
3. 同一日期内按 article_count 降序，分配 row × ROW_H (2.2)
4. Z = random(-0.15, 0.15)
5. 每张卡片 rotation.z = random(-3°, 3°)

#### Scenario: 日期列 × 行排列

- **WHEN** buildCards 为卡片布局
- **THEN** 系统 SHALL 提取唯一日期排序，dateX = indexOf(date) × COL_W (3.0)
- **AND** 同一日期内 SHALL 按 article_count 降序分配 row × ROW_H (2.2)，Z = random(-0.15, 0.15)，rotation.z = random(-3°, 3°)

### Requirement: 后处理管线

后处理 SHALL 使用 EffectComposer 管线：RenderPass → BloomEffect → VignetteEffect → FilmGrainPass。

- BloomEffect：intensity 0.6, luminanceThreshold 0.8, luminanceSmoothing 0.3（只让红线 emissive red 发光）
- VignetteEffect：darkness 0.5（暗角聚焦）
- FilmGrainPass：intensity 0.04（轻微胶片颗粒，优先使用 three/examples 的 FilmPass）

#### Scenario: 后处理链路

- **WHEN** 每帧渲染
- **THEN** EffectComposer SHALL 依次执行 RenderPass → BloomEffect → VignetteEffect → FilmGrainPass
- **AND** Bloom 参数 SHALL 为 intensity 0.6 / luminanceThreshold 0.8 / luminanceSmoothing 0.3（只让红线 emissive red 发光）

### Requirement: 样式常量（基础）

系统 SHALL 使用以下色板和尺寸常量：

- CARD_PAPER=#FFFBEB, CARD_BORDER=rgba(26,26,26,0.12), PIN_COLOR=#DC2626
- STRING_COLOR=#DC2626, STRING_BASE_OPACITY=0.4, STRING_HIGHLIGHT_OPACITY=1.0
- BG_COLOR=#0a0f14, FOG_COLOR=#0a0f14
- STATUS_COLORS: emerging=#16a34a, continuing=#2563eb, split=#ea580c, merge=#9333ea, ending=#9ca3af
- AMBIENT_INTENSITY=0.15, SPOT_ANGLE=45°, SPOT_PENUMBRA=0.5
- FOLLOW_LIGHT：PointLight 每帧跟随相机位置
- SELECTION_LIGHT：红色 PointLight，聚焦卡片时移到其上方 (x, y+2, z+1) + intensity 1.0
- CARD_WIDTH=2.0, CARD_HEIGHT=1.4, CARD_DEPTH=0.05, PIN_RADIUS=0.08, COL_W=3.0, ROW_H=2.2

#### Scenario: 常量统一生效

- **WHEN** 场景、卡片、红线、光照构建
- **THEN** 系统 SHALL 使用本节统一常量（色板 CARD_PAPER/PIN_COLOR/STRING_COLOR、BG/FOG #0a0f14、STATUS_COLORS 五态着色、FOLLOW_LIGHT/SELECTION_LIGHT 行为、卡片尺寸与 COL_W/ROW_H 栅格）

### Requirement: 核心性能约束

系统 SHALL 满足以下核心性能约束：

- 单板块卡片数 <30 时帧率 ≥55fps (默认 7 天)
- 单板块卡片数 <100 时帧率 ≥30fps (60 天)
- dispose() 必须清理所有 geometry、material、texture
- 卡片创建后位置固定（不跑物理模拟），只有 hover 时 Z 轴微动
- 红线是直线（CatmullRomCurve3 with 2 points），不做贝塞尔弯曲

#### Scenario: 帧率预算

- **WHEN** 默认 7 天窗口（单板块卡片数 <30）
- **THEN** 帧率 SHALL ≥55fps；60 天窗口（卡片数 <100）帧率 SHALL ≥30fps

#### Scenario: 静态卡片与资源清理

- **WHEN** 卡片创建后与 dispose() 调用
- **THEN** 卡片位置 SHALL 固定（不跑物理模拟，仅 hover 时 Z 轴微动），红线 SHALL 保持直线，dispose() SHALL 清理所有 geometry、material、texture

### Requirement: TopicWallScene 环境层扩展

TopicWallScene SHALL 扩展以下环境层成员，行为不变部分包括 scene/camera/renderer/css2d/cardGroup/redStrings/fog/composer/followLight/selectionLight。

- `ambientEnv: AmbientEnv` — 程序化暖色环境贴图，影响 PBR 材质反射（图钉等金属）。
- `setDressing: SetDressing | null` — 桌面 + 远景墙 + 台灯 + 卷宗堆，场景环境几何。loadBoardData 时重建。
- `dust: DustParticles | null` — 台灯光锥内尘埃微粒。loadBoardData 时重建。

#### Scenario: loadBoardData 扩展

- **WHEN** loadBoardData 被调用
- **THEN** 系统 SHALL 重建 setDressing（定位台灯/卷宗到当前 latestDayX/minX）
- **AND** 系统 SHALL 重建 dust（以台灯位置为采样顶点）
- **AND** 主 spotlight SHALL 从台灯灯罩位置射向今天列卡片中心
- **AND** 方向雾 uFogOriginX SHALL 设为 latestDayX

#### Scenario: Render loop 扩展

- **WHEN** 每帧渲染
- **THEN** 系统 SHALL 调用 dust.update(dt)

#### Scenario: Dispose 扩展

- **WHEN** TopicWallScene.dispose() 被调用
- **THEN** 系统 SHALL 清理 ambientEnv、setDressing、dust

### Requirement: SetDressing 场景道具组

SetDressing SHALL 包含桌面、远景墙、台灯组、卷宗堆的 Group，所有几何使用 MeshStandardMaterial 并注入方向雾。

几何构成：
- **桌面**：PlaneGeometry, y=STYLE.desk.y, rotation.x=-π/2, 深胡桃木纹 CanvasTexture, roughness 0.9
- **远景墙**：PlaneGeometry, z=STYLE.wall.farZ, 竖直, STYLE.wall.farColor, 靠雾衰减
- **台灯组**（5 primitive）：底座/立柱（黄铜 metalness 0.85）、灯罩（墨绿玻璃）、灯泡（emissive × bulbEmissive → Bloom 发光）。整组置于 latestDayX + STYLE.lamp.offset.x, 桌面 y, STYLE.lamp.offset.z
- **卷宗堆**（2 叠 × 3-4 个错落 BoxGeometry）：纸色纹理，微旋转，分布在 minX-1.5 与 latestDayX+4

#### Scenario: 构造

- **WHEN** SetDressing 构造（传入 latestDayX, minX, timelineWidth）
- **THEN** 系统 SHALL 创建桌面、远景墙、台灯组、卷宗堆
- **AND** 所有 MeshStandardMaterial SHALL 注入方向雾（共享 fogUniforms）

#### Scenario: 雾原点更新

- **WHEN** setFogOrigin(latestDayX) 被调用
- **THEN** 方向雾 uFogOriginX SHALL 更新为 latestDayX，density/range 保留

#### Scenario: 清理

- **WHEN** dispose(scene) 被调用
- **THEN** 系统 SHALL 从 scene 移除 group 并清理所有 geometry/material/texture

### Requirement: AmbientEnv 环境贴图

AmbientEnv SHALL 生成程序化暖色环境贴图（PMREMGenerator.fromScene），影响 PBR 材质反射。程序化场景包含深棕底 + 台灯位置暖色发光球 + 顶部冷光。不替换 scene.background。

#### Scenario: 生成

- **WHEN** AmbientEnv 构造（传入 scene, renderer）
- **THEN** 系统 SHALL 一次性生成 env map 并设为 scene.environment
- **AND** PMREMGenerator SHALL 仅构造时运行一次，不每帧

#### Scenario: 清理

- **WHEN** dispose() 被调用
- **THEN** 系统 SHALL 清理 env map

### Requirement: DustParticles 尘埃微粒

DustParticles SHALL 在台灯光锥内生成浮动尘埃微粒（PointsMaterial），使用 AdditiveBlending 且 depthWrite=false。

- `color`: STYLE.dust.color, `size`: STYLE.dust.size, `count`: STYLE.dust.count
- 小圆点 CanvasTexture

#### Scenario: 构造

- **WHEN** DustParticles 构造（传入 origin 台灯灯罩位置）
- **THEN** 系统 SHALL 椭球锥采样生成 count 个点，向墙面方向延伸

#### Scenario: 动画更新

- **WHEN** update(dt) 被调用
- **THEN** 微粒 SHALL 正弦漂移 + 越界 wrap

#### Scenario: 清理

- **WHEN** dispose(scene) 被调用
- **THEN** 系统 SHALL 清理 points geometry/material

### Requirement: Directional Fog Shader 注入

系统 SHALL 提供 injectDirectionalFog 函数，给 MeshStandardMaterial 注入 X 方向雾项。共享 fogUniforms 一处更新全局生效。

注入语义：
- 顶点输出世界 x（vWorldX）
- 片段在 fog_fragment 后叠加：dx = uFogOriginX - vWorldX
- 仅对"过去"（dx < 0）加重，今天/未来不动
- mix(fragColor, uDirFogColor, 1 - exp(-density * clamp(-dx / range, 0, 1)))

#### Scenario: 作用对象

- **WHEN** 方向雾注入
- **THEN** 桌面、远景墙、主软木墙、卷宗堆、台灯 SHALL 注入方向雾
- **AND** 卡片（CardGroup）SHALL NOT 注入方向雾（保持 highlight/dim 语义）
- **AND** 方向雾 SHALL 与全局 FogSystem（空间纵深）叠加，二者同色 #0a0f14

### Requirement: Style Constants 补充

STYLE 常量 SHALL 新增以下键值：

```
desk:           { y: -1.6, zBack: -2, zFront: 12, color: '#3a2418' }
wall:           { backZ: -0.6, farZ: -4, farColor: '#06090c' }
lamp:           { offset: { x: 2.8, z: 5.2 }, brass: '#b08d3f', glass: '#1f3d2e',
                  bulb: '#ffe9b0', bulbEmissive: 1.5, spotColor: '#ffd9a0', spotIntensity: 2.0 }
dossier:        { stackColor: '#e8d4a5' }
directionalFog: { density: 1.2, range: 12, color: '#0a0f14' }
dust:           { count: 150, color: '#ffe9c8', size: 0.06 }
lighting:       { ..., hemiSky: '#3a2a1a', hemiGround: '#0a0f14', hemiIntensity: 0.55 }
```

#### Scenario: STYLE 环境层键值

- **WHEN** 环境层（SetDressing/AmbientEnv/DustParticles/光照）构建
- **THEN** 系统 SHALL 从 STYLE 读取 desk/wall/lamp/dossier/directionalFog/dust 及 lighting.hemi* 键值（如 lamp.offset {x:2.8, z:5.2}、directionalFog density 1.2/range 12、dust count 150）

### Requirement: Lighting 光照调整

光照配置 SHALL 做如下变更：

| 光源 | 变更 |
|------|------|
| 环境光 | AmbientLight(#fff, 0.15) → HemisphereLight(hemiSky, hemiGround, hemiIntensity) |
| 主聚光 | 暖色 STYLE.lamp.spotColor, STYLE.lamp.spotIntensity, 位置=台灯灯罩, target=今天列卡片中心, angle 0.5 penumbra 0.6 |
| 跟随灯 | 色调微暖 #fff0d0, 其余不变 |
| 选中灯 | 不变 |

#### Scenario: 光照配置生效

- **WHEN** 场景初始化光照
- **THEN** 环境光 SHALL 为 HemisphereLight(hemiSky, hemiGround, hemiIntensity)
- **AND** 主聚光 SHALL 为暖色（位置=台灯灯罩，target=今天列卡片中心，angle 0.5，penumbra 0.6）；跟随灯 SHALL 微暖 #fff0d0；选中灯 SHALL 保持不变

### Requirement: Scene Construction 调整

主软木墙 z SHALL 从 -0.16 移至 STYLE.wall.backZ (-0.6)（卡片更多浮出厚度）。软木墙材质 SHALL 注入方向雾。

#### Scenario: 主软木墙后退

- **WHEN** 场景构建主软木墙
- **THEN** 主墙 z SHALL 为 STYLE.wall.backZ (-0.6)（卡片更多浮出厚度），其材质 SHALL 注入方向雾

### Requirement: 性能约束

环境层 SHALL 满足以下性能约束：

- 几何总 draw call 增量 ≤ 12（桌面/远景墙/卷宗×6/台灯×5/尘埃×1）
- 方向雾 shader 注入仅增加常数级 ALU，不新增 pass
- PMREMGenerator 仅 constructor 一次性生成，不每帧
- dispose() 必须清理 env map、SetDressing 全部 geometry/material/texture、dust geometry/material
- 方向雾 uFogOriginX 在每次 loadBoardData 后更新
- 既有 perf 预算不变：7 天 < 30 卡片 ≥ 55fps；环境层增量不破坏该预算

#### Scenario: 环境层性能预算

- **WHEN** 环境层启用后渲染
- **THEN** 几何 draw call 增量 SHALL ≤12，方向雾注入 SHALL 仅增加常数级 ALU（不新增 pass），PMREMGenerator SHALL 仅在 constructor 一次性生成
- **AND** 既有预算 SHALL 保持：7 天 < 30 卡片 ≥ 55fps

#### Scenario: 环境层资源与雾原点

- **WHEN** dispose() 与 loadBoardData 执行
- **THEN** dispose() SHALL 清理 env map、SetDressing 全部 geometry/material/texture、dust geometry/material
- **AND** 方向雾 uFogOriginX SHALL 在每次 loadBoardData 后更新
