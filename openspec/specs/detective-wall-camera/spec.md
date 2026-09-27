# detective-wall-camera

## Purpose

导演相机系统，负责 Three.js 相机的运镜控制和板块切换转场动画。所有相机运动由 GSAP Timeline 编排。TBD.

## Requirements

### Requirement: DirectorCamera 运镜系统

DirectorCamera SHALL 提供预设镜头生成与 GSAP Timeline 驱动的相机过渡。

- 构造函数接收 `THREE.PerspectiveCamera`
- `hooks: DirectorCameraHooks` 由 WallCameraControls 注入，包含 `onTransitionStart`、`onTransitionComplete`、`onTargetUpdate`
- `currentShot: CameraShot` 记录当前镜头

#### Scenario: 预设镜头 todayFocus

- **WHEN** `todayFocus(dateX)` 被调用
- **THEN** 返回 CameraShot：position=(dateX, 5.5, 8), target=(dateX, 0, 0), fov=50, duration=0.6, ease='power2.inOut'

#### Scenario: 预设镜头 overview

- **WHEN** `overview(totalWidth)` 被调用
- **THEN** 返回 CameraShot：position=(totalWidth/2, 12, 16), target=(totalWidth/2, 0, 0), fov=55, duration=0.8, ease='power2.inOut'

#### Scenario: 预设镜头 topicFocus

- **WHEN** `topicFocus(card)` 被调用
- **THEN** 返回 CameraShot：position=(card.x, card.y+1, 5), target=(card.x, card.y, 0), fov=45, duration=0.5, ease='power2.out'

#### Scenario: 预设镜头 lifecycleFull

- **WHEN** `lifecycleFull(totalWidth)` 被调用
- **THEN** 返回 CameraShot：position=(totalWidth/2, 8, 14), target=(totalWidth/2, 0, 0), fov=60, duration=0.8, ease='power2.inOut'

#### Scenario: 运镜 transitionTo

- **WHEN** `transitionTo(shot)` 被调用
- **THEN** 返回 gsap.core.Timeline，插值 camera.position / target / fov
- **AND** 如果前一个 Timeline 仍在执行，自动 kill
- **AND** 在 GSAP onUpdate 中调用 `camera.lookAt(target)`
- **AND** 触发 hooks：onTransitionStart（orbit 禁用）→ onTargetUpdate（每帧同步 orbit target）→ onTransitionComplete（orbit 重启用）

#### Scenario: 即时跳转 snapTo

- **WHEN** `snapTo(shot)` 被调用（初始化用）
- **THEN** camera.position/target/fov 直接设置为 shot 值，无动画
- **AND** 触发 hooks 同步 orbit

### Requirement: WallCameraControls 平移/缩放

WallCameraControls SHALL 封装 OrbitControls 实现沿墙平移 + 缩放，禁止旋转。

#### Scenario: OrbitControls 配置

- **WHEN** WallCameraControls 构造
- **THEN** OrbitControls SHALL 配置为 enableRotate=false, enablePan=true, enableZoom=true
- **AND** mouseButtons: LEFT=PAN, MIDDLE=DOLLY, RIGHT=PAN
- **AND** minDistance=3, maxDistance=40

#### Scenario: 与 DirectorCamera 协调

- **WHEN** DirectorCamera 执行 transitionTo
- **THEN** onTransitionStart 触发 controls.enabled = false（禁用 orbit 避免位置冲突）
- **AND** onTargetUpdate 触发 controls.target.set(x,y,z)（运镜中同步 target）
- **AND** onTransitionComplete 触发 controls.enabled = true（重启用）

#### Scenario: 拖拽期间暂停 hover

- **WHEN** OrbitControls 触发 'start' 事件
- **THEN** interaction.setHoverSuspended(true)（拖拽时不 raycast）
- **WHEN** OrbitControls 触发 'end' 事件
- **THEN** interaction.setHoverSuspended(false)

#### Scenario: 渲染循环

- **WHEN** 每帧渲染
- **THEN** `controls.update()` SHALL 通过 scene.addFrameCallback 注册执行

### Requirement: 运镜约束

系统 SHALL 强制执行以下相机约束：

- 所有相机运动必须通过 `transitionTo()`，禁止直接修改 `camera.position`
- fov 变化范围 35~65
- 相机 Y 值不低于 2（不能降到卡片平面以下）
- `transitionTo`/`snapTo` 触发 hooks 以同步 orbit 状态

#### Scenario: 运镜统一入口与硬约束

- **WHEN** 场景需要移动相机（初始化直接就位用 `snapTo()`）
- **THEN** 系统 SHALL 通过 `transitionTo()` 编排运动，不直接修改 `camera.position`
- **AND** fov SHALL 被 clamp 在 35~65，相机 Y SHALL 不低于 2
- **AND** `transitionTo`/`snapTo` SHALL 触发 hooks 同步 orbit 状态

### Requirement: ChapterTransition 转场（暂未启用）

ChapterTransition SHALL 提供板块切换时的红色 wipe + 档案封面 + 卡片入场动画，当前无 BoardSelector 入口故暂不激活。

- 类文件保留供后续 BoardSelector 落地时复用
- Vue 层的 watch(boardId)、wipe/cover DOM、ChapterTransition 实例化均已移除
- 转场期间禁止用户交互（Raycaster disable）
- 转场 Timeline 被 kill 时清理中间状态
- 打字机效果不使用 GSAP TextPlugin，用 substring + onUpdate 手动实现
- 转场 DOM 由 Vue 渲染，ChapterTransition 只操作样式和动画

#### Scenario: 转场当前不触发

- **WHEN** 用户使用侦探墙（当前无 BoardSelector 入口）
- **THEN** ChapterTransition 转场 SHALL 不触发（Vue 层 watch(boardId)、wipe/cover DOM、实例化均已移除），ChapterTransition.ts 仅保留待后续复用

### Requirement: 依赖

构建 SHALL 满足以下依赖约束：

- `@types/three`（devDep）：补充 OrbitControls/CSS2DRenderer/Line2 的 jsm 类型
- 禁止 `front/three.d.ts`（declare module 'three'）遮蔽 @types/three

#### Scenario: 类型来源唯一

- **WHEN** TypeScript 编译侦探墙相关代码
- **THEN** OrbitControls/CSS2DRenderer/Line2 的 jsm 类型 SHALL 由 `@types/three`（devDep）提供
- **AND** `front/three.d.ts`（declare module 'three'）SHALL 不存在，不得遮蔽 @types/three
