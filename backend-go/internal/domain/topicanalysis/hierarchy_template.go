package topicanalysis

import (
	"fmt"

	"my-robot-backend/internal/domain/models"
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

func ResolveLevelFromDepth(category string, depth int) int {
	mgr := GetHierarchyManager()
	tmpl := mgr.GetTemplate(category, "")
	if tmpl == nil {
		if depth == 0 {
			return 1
		}
		if depth > 4 {
			return 4
		}
		return depth + 1
	}

	level := depth + 1
	if level > tmpl.MaxLevel {
		level = tmpl.MaxLevel
	}
	return level
}

func GetTagLevel(tag *models.TopicTag) int {
	depth := getTagDepthFromRoot(tag.ID)
	return ResolveLevelFromDepth(tag.Category, depth)
}

func GetTagLevelByID(tagID uint, category string) int {
	depth := getTagDepthFromRoot(tagID)
	return ResolveLevelFromDepth(category, depth)
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

func buildDefaultTechnologyTemplate() *CategoryHierarchyTemplate {
	return &CategoryHierarchyTemplate{
		Category: "keyword",
		SubType:  "technology",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "一级领域", Description: "技术大领域，如AI、半导体、生物技术", IsLeaf: false, MaxChildren: 15},
			{Level: 2, Name: "二级子域", Description: "细分技术方向，如大语言模型、GPU架构", IsLeaf: false, MaxChildren: 30},
			{Level: 3, Name: "技术概念/产品", Description: "具体技术、产品、框架、协议", IsLeaf: true, MaxChildren: 0},
		},
	}
}

func buildDefaultCompanyBusinessTemplate() *CategoryHierarchyTemplate {
	return &CategoryHierarchyTemplate{
		Category: "keyword",
		SubType:  "company_business",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "产业赛道", Description: "产业大类，如云计算、新能源汽车", IsLeaf: false, MaxChildren: 15},
			{Level: 2, Name: "细分市场/业务线", Description: "具体市场或业务方向", IsLeaf: false, MaxChildren: 30},
			{Level: 3, Name: "公司/业务新闻", Description: "具体公司或业务新闻条目", IsLeaf: true, MaxChildren: 0},
		},
	}
}

func buildDefaultConceptTemplate() *CategoryHierarchyTemplate {
	return &CategoryHierarchyTemplate{
		Category: "keyword",
		SubType:  "concept",
		MaxLevel: 3,
		Levels: []AbstractionLevel{
			{Level: 1, Name: "一级概念域", Description: "宏观概念领域，如经济学、科学方法论", IsLeaf: false, MaxChildren: 15},
			{Level: 2, Name: "二级概念群", Description: "具体理论框架、学派、方法论", IsLeaf: false, MaxChildren: 30},
			{Level: 3, Name: "具体概念/理论", Description: "具体概念、理论、现象、术语", IsLeaf: true, MaxChildren: 0},
		},
	}
}

func BuildAllDefaultTemplates() []*CategoryHierarchyTemplate {
	return []*CategoryHierarchyTemplate{
		buildDefaultEventTemplate(),
		buildDefaultPersonTemplate(),
		buildDefaultTechnologyTemplate(),
		buildDefaultCompanyBusinessTemplate(),
		buildDefaultConceptTemplate(),
	}
}
