// 静态演示：版块/新闻/计数是fixture；不访问产品API、数据库或AI。
const STORAGE_KEY='syntopica-key-event-prototype-v3';
const boards={ai:{label:'AI 与大模型',tagCount:18,lanes:['llm','agents']},housing:{label:'住房与信贷',tagCount:12,lanes:['housing','supply']}};
const lanes={llm:{board:'ai',label:'LLM 模型进展'},agents:{board:'ai',label:'Agent 工具生态'},housing:{board:'housing',label:'房贷期限政策'},supply:{board:'housing',label:'住房供应政策'}};
const fixtures=[
 {id:101,board:'ai',lane:'llm',date:'2026-10-11',title:'GPT6.1 SOL 发布',summary:'演示事件：新模型正式发布，公布能力与使用方式。具体效果仍需要后续验证。',reason:'这条泳道出现明确的产品发布节点，值得跟踪能力与应用变化。',status:'suggested',threadStatus:'emerging',evidence:'演示报道：模型发布公告；能力说明。非实际新闻。'},
 {id:102,board:'ai',lane:'llm',date:'2026-10-11',title:'媒体再次解读 GPT6.1 SOL 发布',summary:'同一发布消息的转载与评论，没有新增能力或开放条件的事实。',status:'ordinary',threadStatus:'continuing',evidence:'演示报道：同一公告的转载。',note:'重复报道 · 未再次推荐'},
 {id:104,board:'ai',lane:'llm',date:'2026-10-10',title:'此前发布的模型向开发者正式开放 API',summary:'演示事件：从发布公告进入实际可用阶段。',reason:'新增进展：从“已发布”变为“可使用”，不是再次标记发布公告。',status:'suggested',threadStatus:'continuing',evidence:'演示报道：开发者开放通知。'},
 {id:103,board:'ai',lane:'llm',date:'2026-10-09',title:'某模型调整 API 计费方式',summary:'演示事件：调整调用费用计算方式。没有AI推荐，你仍可自己标记。',status:'ordinary',threadStatus:'split',evidence:'演示报道：价格页面调整说明。'},
 {id:105,board:'ai',lane:'llm',date:'2026-10-08',title:'上一代模型的重要更新',summary:'此前人工确认的演示节点。原条目被日报重生成替换，确认快照仍保留。',status:'confirmed',detached:true,evidence:'原演示条目已失效，无可打开原文。'},
 {id:107,board:'ai',lane:'llm',date:'2026-10-07',title:'社区发布例行评测报告',summary:'演示背景动态：没有新的发布或开放事实。',status:'ordinary',threadStatus:'continuing',evidence:'演示报道：社区评测素材。'},
 {id:106,board:'ai',lane:'llm',date:'2026-10-06',title:'开发者公布常规集成指南',summary:'演示背景动态：说明已有功能的集成方式。',status:'ordinary',threadStatus:'continuing',evidence:'演示报道：常规集成指南。'},
 {id:301,board:'ai',lane:'agents',date:'2026-10-10',title:'演示工具协议支持多步骤调用',summary:'仅作演示：工具接口支持一轮多步执行，仍需验证实际能力。',status:'suggested',threadStatus:'emerging',reason:'本泳道出现具体的新工具接口能力，适合观察落地进展。',evidence:'演示报道：接口协议说明。'},
 {id:302,board:'ai',lane:'agents',date:'2026-10-09',title:'社区讨论既有工具接入经验',summary:'演示讨论：不含新的接口或发布事实。',status:'ordinary',threadStatus:'continuing',evidence:'演示讨论素材。'},
 {id:303,board:'ai',lane:'agents',date:'2026-10-07',title:'演示开源工具完成首个公开版本',summary:'演示事件：此前人工确认的工具发布，属于当前AI版块的另一条泳道。',status:'confirmed',threadStatus:'emerging',evidence:'演示报道：公开发布说明。'},
 {id:201,board:'housing',lane:'housing',date:'2026-10-11',title:'演示地区将部分房贷期限延长至 40 年',summary:'仅作演示：限定地区和人群，不代表全国政策或刺激效果已经成立。',reason:'具体期限规则发生变化，应跟踪适用范围、月供与总利息。',status:'suggested',threadStatus:'emerging',evidence:'演示地方政策说明，非真实政策。'},
 {id:202,board:'housing',lane:'housing',date:'2026-10-10',title:'市场评论讨论延长期限的影响',summary:'演示评论，没有增加新的政策执行事实。',status:'ordinary',threadStatus:'continuing',evidence:'演示评论素材。'},
 {id:401,board:'housing',lane:'supply',date:'2026-10-11',title:'演示地区公布住房供应安排',summary:'演示背景动态：供应规划的例行进展，不属于房贷期限泳道。',status:'ordinary',threadStatus:'continuing',evidence:'演示住房安排说明。'},
 {id:402,board:'housing',lane:'supply',date:'2026-10-07',title:'演示住房供应计划正式公布',summary:'这条确认只属于住房版块的供应泳道，不混入AI版块。',status:'confirmed',threadStatus:'emerging',evidence:'演示规划公告。'}
];

fixtures.push(
 {id:108,board:'ai',lane:'llm',date:'2026-10-11',title:'演示设备接口讨论被归入模型章节',summary:'用于验证：即使来源线索没有文章可展开，合法归属的标记仍是独立操作；可能跑题提示不得被标记遮掉。',status:'ordinary',threadStatus:'emerging',demoted:true,noArticles:true,evidence:'没有可打开的演示文章。'},
 {id:109,board:'ai',lane:null,date:'2026-10-11',title:'演示旁支信息尚未归属持久话题',summary:'只有日报来源、没有有效泳道归属；不给关键标记，也不自动创建话题。',status:'ordinary',threadStatus:'emerging',evidence:'演示旁支材料。'}
);
for(const item of fixtures){
 item.sectionId=item.detached?null:([101,102,108].includes(item.id)?5101:5000+item.id);
 item.clusterLabel=[101,102,108].includes(item.id)?'模型发布与开放方式出现变化（演示章节）':'当日章节：'+item.title;
 item.articleIds=item.detached||item.noArticles?[]:Array.from({length:item.id===101?2:1},(_,i)=>item.id*10+i+1);
}
const sections=[...new Set(fixtures.filter(i=>!i.detached).map(i=>i.sectionId))].map(id=>{
 const items=fixtures.filter(i=>i.sectionId===id),first=items[0];
 return {id,board:first.board,lane:first.lane,date:first.date,label:first.clusterLabel,status:first.threadStatus||'ending',threadIds:items.map(i=>i.id),articleCount:new Set(items.flatMap(i=>i.articleIds)).size};
});
