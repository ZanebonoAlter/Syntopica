# S9 返修源码：父定向复核与第二次构建准入

## 准入结论

前两轮独立源码复核中R35的手动deferred红例、R36短窗分支、R38默认兼容语义初焦、R39业务大小写，以及R40最近端/grabOffset逻辑已通过。最后两个明确生产接线断点经原作者修正、父直接定向读实际代码确认，现准入**第二次构建暂铺与真实目标K3**；不再重复审已通过代码。这仍不是S9绿/正式验收。

## 最后两处证据

1. `usePlotWidth.ts`：新增watch(plotRef,{flush:'post'})，异步SVG出现/替换/消失时measure并disconnect旧RO/观察新SVG；mounted/nextTick/resize路径保留，unmount清理。真实生产不再仅首挂null时测一次。新增初始bounds/range=null后晚到的Controls命中层用例，以及width0→375→0 helper用例。
2. `useConsoleReturnContext.ts`：scheduleScrollRestore先建pending即attachUserIntentListeners，再maybeStart；stopRestore同时清pending/布局cancel/窗口监听；布局阶段复用监听，自然onDone清理。悬挂详情期间已add wheel证明pending已建立，wheel取消后详情resolve不再强拉；无用户意图仍settled后302→382。

父通过定向工具读源码与用例锚核对，没有复跑作者测试。作者实际命令证据：本轮仅3目标文件69测试绿、TS eslint0；typecheck新增自身0、旧SignalReportView.test.ts7条exit1。未把它叫全仓绿、未重复181/308或无关旧网络测试。

## 后续实页必须验证

- 全新首载不切归档：数据晚到后真实24px hit层存在；375/390短窗左右可见端中点正确、外缘小移不跳、单日与池端点/cancel。
- 真实1440 stage/1280与390 window深滚动；GET详情延迟>3秒仍恢复；pending期间用户滚动后不得强拉。
- 缓存365/14切档UI默认范围/轴与URL一致；外部四参深链仍稳定。
- 30/31单屏sw==cw，32+才内横滚；真实日期与数字可辨。
- matrix原文role/初焦/关闭回焦、Agent业务标签computed transform原样。

当前网站还是b2a61d73回退版，新源码未构建；不能拿它验本修复。第二次构建需再备份当前安全版、记录新入口SHA与源指纹，后端不重启。首轮21PASS/2FAIL/2PARTIAL及PNG/请求证据原样留存，复验另目录。旧根53/1/2、他域7类型债、真实生成DB/WS时序未实测继续留账；不勾8B.3/8B.4。
