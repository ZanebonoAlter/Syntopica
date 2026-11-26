# retire-person-extraction

打标 prompt 停止提取 person 类标签：辅助标签已覆盖人物叫法（Top10 person 100% 有 auxiliary 对应），person 仅占 4.9% 且与 event 同数组挤压模型注意力（184183 因 event_person_tags 返回 object 而永久打标失败）。存量 person 标签与 story_fingerprint 的 person: subject_key 前缀不受影响。
