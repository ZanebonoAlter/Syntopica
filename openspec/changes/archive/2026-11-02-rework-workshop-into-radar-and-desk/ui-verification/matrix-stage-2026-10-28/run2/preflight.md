# 第二次静态版本的复验前置事实

- 入口：http://localhost:5100（LAN同为http://192.168.5.24:5100）。host80旧入口不在本轮范围。
- 新index SHA：`b86179cb4318a06ff104dcc72ccbfc7fabe9c2951b4e225d2384c9989551db5c`，chunk`/_nuxt/CSxw8iJQ.js`。构建exit0/100s，服务端index/chunk与磁盘一致；backendPID1061075未重启。
- 日志：`.pi/subagents/validation/matrix-static-20261028-055414-run2/build.log`，source-fingerprint-before/after逐文件一致，652文件，构建期间无源码变动。
- 当前安全回退备份：`.pi/subagents/deploy-backups/matrix-before-20261028-055425.tar.gz`，完整SHA `d4bc92e5546526977a6e9adb0ad26b9f3d6b30540bdd24d3bf85081aed4e756b`，与02:55包相同，恢复旧index=b2a61d733968037f0d6c06c8fa826e0ef694505e5766f4ce12199dbe18e64715。

## 构建报告的源码delta校正

构建者报告“相对首轮111路径内容变”不准确。父发现两个manifest条目顺序不同，不能按行diff推内容变；按文件路径为key、SHA为value重新比较：**645→652；实际21个已有文件内容变、7个新增、0个删除**。已逐项确认均为R34–40本期授权源码/tests：console/G1观察窗与useBoardConsole、AppDialog/ArticlePreviewModal opt-in；新增restoreScroll与测宽/命中/settled/workface小模块。FeedLayout.css、KaTeX、.dockerignore等误列条目SHA前后相同，不是本次新增外域漂移。

原`.pi/.../source-delta-vs-prev-run.txt`保留不覆盖；本校正以两个source-fingerprint文件的path→SHA映射为证据。构建期间前后零变动结论成立，不能将“111”写入最终范围归因。

## 复验范围与口径

- 首轮原证据不动；父核主25步是21PASS/2FAIL(15,25)/2PARTIAL(22,24)，不是原误计23PASS。新证据写本run2目录。
- 定向实页复验失败/受影响项：冷首屏SVG晚到热区、7/14日端判定/偏移/单日/边界/cancel，慢详情深滚动与pending用户取消，缓存365/14切档、30/31/32列几何与日期清晰、原文语义初焦/返回、业务大小写；宽窄双主题做受影响默认布局回归。
- 未动主链PASS可注明继承首轮实际证据，不虚称本轮重跑；纯缓存容量/迟到等按规范关联已有最便宜层用例与实际命令，不往真库造样本。最终综合故事状态与“浏览器本轮覆盖/模拟/未验”必须分栏。
- 真实生成DB/WS时序/真机触摸仍如实未验；不拦掉旧根53/1/2或他域7类型债。
- 所有非GET业务请求仍需浏览器级拦写；恢复当前static验证不是读旧b2a站点。发现新阻塞先留证/关闭会话报父，不自改代码或后台。
