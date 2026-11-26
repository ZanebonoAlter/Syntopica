## ADDED Requirements

### Requirement: 修正画布横向滚动时标签列吸顶

修正画布 SHALL 在横向溢出时把左侧标签列钉在画布最左，SHALL NOT 让分区名随滚动移出视口。

- **溢出门控**：`vbWidth × zoom >` 容器 `clientWidth` 为假时，SHALL NOT 下发吸顶位移、SHALL NOT 绘制吸顶底色，渲染 SHALL 与无吸顶实现逐像素一致；
- **覆盖**：吸顶列 SHALL 以不透明底覆盖滑到其下方的节点与标题，并给出右缘渐隐阴影以表达层序；
- **宽度**：固定列宽度 SHALL 取日期轴起点（`LEFT_PAD` + 折叠带占位），SHALL NOT 写死为 `LEFT_PAD`。

#### Scenario: 横滚后分区名仍可见

- **WHEN** 可见天数 86、列宽取 64px 下限，画布宽约 5682px，容器可视宽 1900px
- **THEN** 横向滚动到任意位置时，「本泳道话题带 / 候选区 / 只读参照」三处分区名 SHALL 仍在视口左缘可见
- **AND** 吸顶列 SHALL 覆盖其下方滑过的节点，节点与标题 SHALL NOT 透出

#### Scenario: 折叠带存在时固定列不吞掉勾选框

- **WHEN** 存在窗外首义节点（`foldNodes` 非空），折叠带占 `x ∈ [150, 218]`
- **THEN** 固定列宽度 SHALL 扩展到日期轴起点（≈226px），折叠带与其勾选框 SHALL 常驻可见
- **AND** 固定列内的图元（标签 / 勾选框）SHALL NOT 因列宽变化重排；日期轴起点随折叠带右移 76px、日期列随之整体平移属既有行为（`axisX0` 本就参与列宽推导），SHALL NOT 被计为本 change 引入的重排
- **AND** 折叠带出现/消失时固定列宽度变化一次 SHALL 被接受，内容 SHALL NOT 重排

### Requirement: 修正画布纵向滚动时日期轴吸顶

修正画布 SHALL 在纵向溢出时把日期轴刻度钉在画布顶部，SHALL NOT 让用户滚动后不知道自己在看哪天。

- **溢出门控**：`canvasHeight × zoom >` 容器 `clientHeight` 为假时，SHALL NOT 下发吸顶位移；
- **横向跟随**：日期轴 SHALL 随横向滚动移动（刻度必须对齐日期列），SHALL NOT 固定不动；
- **裁切**：日期轴 SHALL 被裁切到标签列右侧，SHALL NOT 侵入吸顶标签列与该列文字重叠。

#### Scenario: 纵滚后日期轴仍可见

- **WHEN** 节点堆叠使 `canvasHeight × zoom` 超出容器可视高
- **THEN** 纵向滚动到任意位置时，顶部日期刻度 SHALL 仍可见且 SHALL 与下方日期列对齐
- **AND** 横向滚动时日期刻度 SHALL 随之移动，SHALL NOT 停留在吸顶标签列上方与该列文字重叠

#### Scenario: 不溢出时不出现吸顶痕迹

- **WHEN** 7 天档且成员稀少，画布宽 = 视口宽、画布高 = 容器高
- **THEN** 画布 SHALL NOT 出现固定列底色、SHALL NOT 下发吸顶 `translate`、SHALL NOT 出现右缘渐隐阴影

### Requirement: 修正画布的缩放与滚动外壳与编排画布同源

修正画布的**缩放、容器测量、滚动同步与 `Esc` 捕获** SHALL 与泳道编排画布共用同一实现，SHALL NOT 各自维护一份；**平移**（点心手势）的指针分流、位移阈值与拖后吞 `click` SHALL 两侧口径一致，手势状态机可各自持有（两个画布的手势对象不同：一边是节点拖拽、一边是落点分区）。

- 缩放范围、步进、光标锚点换算、缩放钳制 SHALL 两侧一致；
- **拖拽期间 SHALL NOT 触发滚轮缩放**；该屏蔽只针对缩放，SHALL NOT 拦截容器原生滚动（与编排画布既有行为一致）；
- `Esc` SHALL 在拖拽或平移任一进行中时取消手势，两侧一致。

#### Scenario: 拖拽节点时滚轮不缩放

- **WHEN** 用户按住节点拖动归属，期间滚动滚轮
- **THEN** 画布缩放比例 SHALL 保持不变
- **AND** 松手后滚轮 SHALL 恢复缩放能力（无需重新进入画布）

#### Scenario: 两侧缩放语义一致

- **WHEN** 在修正画布与编排画布上分别滚轮放大到 200%
- **THEN** 两侧的缩放倍率读数、可视内容范围与锚点落位 SHALL 遵循同一套换算规则
