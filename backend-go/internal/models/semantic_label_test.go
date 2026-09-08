package models

import (
	"reflect"
	"strings"
	"testing"
)

func TestSemanticLabelModelShape(t *testing.T) {
	if got := (SemanticLabel{}).TableName(); got != "semantic_labels" {
		t.Fatalf("SemanticLabel table = %q, want semantic_labels", got)
	}

	typ := reflect.TypeOf(SemanticLabel{})
	mustHaveGORMTag(t, typ, "Embedding", "type:vector;")
	mustHaveGORMTag(t, typ, "Embedding", "column:embedding")
	mustHaveGORMTag(t, typ, "MergeEmbedding", "type:vector;")
	mustHaveGORMTag(t, typ, "MergeEmbedding", "column:merge_embedding")
	mustHaveFieldType(t, typ, "MergeEmbedding", reflect.TypeOf((*string)(nil)))
	mustHaveGORMTag(t, typ, "Aliases", "type:jsonb")
	mustHaveGORMTag(t, typ, "Aliases", "serializer:json")
	mustHaveGORMTag(t, typ, "Slug", "uniqueIndex:idx_semantic_labels_slug")
	mustHaveGORMTag(t, typ, "LabelType", "index:idx_semantic_labels_label_type")
	mustHaveGORMTag(t, typ, "Status", "index:idx_semantic_labels_status")
}

func TestSemanticLabelAssociationModelShapes(t *testing.T) {
	if got := (TopicTagSemanticLabel{}).TableName(); got != "topic_tag_semantic_labels" {
		t.Fatalf("TopicTagSemanticLabel table = %q", got)
	}
	if got := (TopicTagBoardLabel{}).TableName(); got != "topic_tag_board_labels" {
		t.Fatalf("TopicTagBoardLabel table = %q", got)
	}
	if got := (BoardComposition{}).TableName(); got != "board_composition" {
		t.Fatalf("BoardComposition table = %q", got)
	}

	mustHaveGORMTag(t, reflect.TypeOf(TopicTagSemanticLabel{}), "TopicTagID", "primaryKey")
	mustHaveGORMTag(t, reflect.TypeOf(TopicTagSemanticLabel{}), "SemanticLabelID", "primaryKey")
	mustHaveGORMTag(t, reflect.TypeOf(TopicTagBoardLabel{}), "SemanticBoardID", "primaryKey")
	mustHaveGORMTag(t, reflect.TypeOf(BoardComposition{}), "BoardID", "primaryKey")
	mustHaveGORMTag(t, reflect.TypeOf(BoardComposition{}), "AuxiliaryLabelID", "primaryKey")
}

func TestCompositeComponentModelShape(t *testing.T) {
	if got := (CompositeComponent{}).TableName(); got != "composite_components" {
		t.Fatalf("CompositeComponent table = %q, want composite_components", got)
	}

	typ := reflect.TypeOf(CompositeComponent{})
	mustHaveGORMTag(t, typ, "CompositeID", "primaryKey")
	mustHaveGORMTag(t, typ, "ComponentLabelID", "primaryKey")
	mustHaveFieldType(t, typ, "Position", reflect.TypeOf(int(0)))
	mustHaveGORMTag(t, typ, "Composite", "foreignKey:CompositeID")
	mustHaveGORMTag(t, typ, "Composite", "constraint:OnDelete:CASCADE")
	mustHaveGORMTag(t, typ, "ComponentLabel", "foreignKey:ComponentLabelID")
	mustHaveGORMTag(t, typ, "ComponentLabel", "constraint:OnDelete:CASCADE")
}

// LabelType "composite" is a plain string column value (no enum CHECK blocks
// it); the model-level contract is documented on SemanticLabel.LabelType.
func TestSemanticLabelLabelTypeCompositeValue(t *testing.T) {
	label := SemanticLabel{Label: "美债收益率", LabelType: "composite"}
	if label.LabelType != "composite" {
		t.Fatalf("LabelType = %q, want composite", label.LabelType)
	}
}

func mustHaveGORMTag(t *testing.T, typ reflect.Type, fieldName string, want string) {
	t.Helper()
	field, ok := typ.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s.%s field not found", typ.Name(), fieldName)
	}
	if tag := field.Tag.Get("gorm"); !strings.Contains(tag, want) {
		t.Fatalf("%s.%s gorm tag = %q, want it to contain %q", typ.Name(), fieldName, tag, want)
	}
}

func mustHaveFieldType(t *testing.T, typ reflect.Type, fieldName string, want reflect.Type) {
	t.Helper()
	field, ok := typ.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s.%s field not found", typ.Name(), fieldName)
	}
	if field.Type != want {
		t.Fatalf("%s.%s type = %v, want %v", typ.Name(), fieldName, field.Type, want)
	}
}
