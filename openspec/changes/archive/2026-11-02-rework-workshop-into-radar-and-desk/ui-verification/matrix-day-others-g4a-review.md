# G4a 父准入复核与G4b前置

父已读useConsoleReading、otherTopicSelection关键实现与返回接口。实际独立执行useConsoleReading.test.ts+useConsoleReturnContext.test.ts → 2 files/24 passed，exit0；不冒父重跑56/94。旧独立26/189等也不混算。

## 入场小回归：旧日期重试不能抢写新日期

retryReport在await force index之后只核board，随后ensureReportDay(payload.date,force)；直到写完旧日期才比较currentSelectedDate并返回applied=false。若此期间用户已切B且B阅读已发布，旧A重试会再次触发G1 reportSeq、覆盖reportDay为A；即使返回false，B被取消且标题gate可能永pending。

G4b被明确授权先补真实回归并最小修：索引await后、任何旧date读调用前复核当前board/date/上下文；过时直接停止，不能靠末尾applied=false补救已写store。消费结果还应核section身份是否仍为payload，不能把旧section核对提示展示给新选条。

## 选择合法性与UI接线

selectOther目前仅核当日详情存在，未核G1 otherTopics.rows（正式/休眠可能也在详情）。G4b入场补精确用例并仅接受当前其他组合法行，旧正常选择/观察节点/缺分区不默清保持。这里不是新身份算法，直接复用G1视图成员。

## 返回与重试的集成义务

- 接受G4a「URL lane不同则显式优先，同lane往返可恢复other」的兼容判断，避免原URL残留的lane把刚保存other一刀压掉；other阅读key绝不塞legacy lane实体/URL字段。
- 现返回hook仍只导出saveContext/openReport/openResearch，尚无同实例归档→矩阵恢复触发方法。G4b必须在实际view切换中补本次所需接口/调用（不改归档页面），不能拿重建hook的测试冒同实例返回完成。
- G2提示当前无独立retry事件，不能把索引/详情重试简单接旧load(A)就称可用。G4b加最小显式入口/透传，最终到force索引→force报告；也可用正确的全刷新编排，但不得仍只刷A。
- 生成/显式刷新除reportDay还需更新已挂载lane.current（复用已刷新缓存重建工作面即可，避免第二遍force index）；失败时全局stale提示可见，不静默把旧缓存说成新成功。
- 只框架接线，不添date=、日历、生命周期动作，不改其他页面/foreign/后端。Shell约494/15045B、Shell测试1095/44663B，G4b必须内聚拆小且保38旧用例；返回hook15301B也不得继续堆过规范。

8C.5A初版目标绿历史保留；其上述入场角由下一G4b线程先补测收口再勾A。完整8C.5/UI验收仍未完成，未部署。
