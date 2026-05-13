package tagging

import (
	"fmt"
)

type AbstractionLevel struct {
	Level             int      `json:"level"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	IsLeaf            bool     `json:"is_leaf"`
	MaxChildren       int      `json:"max_children"`
	ForbiddenPatterns []string `json:"forbidden_patterns"`
}

type LevelConstraints struct {
	IsLeaf      bool     `json:"is_leaf"`
	MaxChildren int      `json:"max_children"`
	Forbidden   []string `json:"forbidden"`
}

type CategoryHierarchyTemplate struct {
	Category string             `json:"category"`
	SubType  string             `json:"sub_type"`
	MaxLevel int                `json:"max_level"`
	Levels   []AbstractionLevel `json:"levels"`
}

func (t *CategoryHierarchyTemplate) TemplateKey() string {
	if t.SubType != "" {
		return t.Category + ":" + t.SubType
	}
	return t.Category
}

func (t *CategoryHierarchyTemplate) IsLeafLevel(level int) bool {
	if level <= 0 || level > len(t.Levels) {
		return false
	}
	return t.Levels[level-1].IsLeaf
}

func (t *CategoryHierarchyTemplate) GetLevelName(level int) string {
	if level <= 0 || level > len(t.Levels) {
		return fmt.Sprintf("L%d", level)
	}
	return t.Levels[level-1].Name
}

func (t *CategoryHierarchyTemplate) GetLeafLevel() int {
	for _, l := range t.Levels {
		if l.IsLeaf {
			return l.Level
		}
	}
	return t.MaxLevel
}

func getMaxDepthForCategory(category string) int {
	mgr := GetHierarchyManager()
	tmpl := mgr.GetTemplate(category, "")
	if tmpl == nil {
		return 3
	}
	return tmpl.MaxLevel - 1
}

func buildDefaultEventTemplate() *CategoryHierarchyTemplate {
	return &CategoryHierarchyTemplate{
		Category: "event",
		SubType:  "",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "事件类型", Description: "事件的大类，如产品发布、融资并购、政策法规", IsLeaf: false, MaxChildren: 20},
			{Level: 2, Name: "事件主体", Description: "事件关联的核心实体，如公司、产品、组织", IsLeaf: false, MaxChildren: 50},
			{Level: 3, Name: "具体事件", Description: "具体的新闻事件实例", IsLeaf: true, MaxChildren: 0},
		},
	}
}

func buildDefaultPersonTemplate() *CategoryHierarchyTemplate {
	return &CategoryHierarchyTemplate{
		Category: "person",
		SubType:  "",
		MaxLevel: 2,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "人物群组", Description: "共享领域/角色/机构的人物群体", IsLeaf: false, MaxChildren: 30},
			{Level: 2, Name: "具体人物", Description: "具体的人名", IsLeaf: true, MaxChildren: 0},
		},
	}
}

func buildDefaultKeywordTemplate() *CategoryHierarchyTemplate {
	return &CategoryHierarchyTemplate{
		Category: "keyword",
		SubType:  "",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "主题领域", Description: "关键词的大领域，如AI、金融、生物技术", IsLeaf: false, MaxChildren: 20},
			{Level: 2, Name: "主题子域", Description: "细分方向，如大语言模型、新能源电池", IsLeaf: false, MaxChildren: 50},
			{Level: 3, Name: "具体概念/术语", Description: "具体的技术、产品、概念、术语", IsLeaf: true, MaxChildren: 0},
		},
	}
}

func BuildAllDefaultTemplates() []*CategoryHierarchyTemplate {
	return []*CategoryHierarchyTemplate{
		buildDefaultEventTemplate(),
		buildDefaultPersonTemplate(),
		buildDefaultKeywordTemplate(),
	}
}
