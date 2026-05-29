package jobs

import (
	"os"
	"strings"
	"testing"
)

func TestTriggerNowStatusCode(t *testing.T) {
	t.Run("firecrawl uses http status constant", func(t *testing.T) {
		assertTriggerNowStatusCodeConstant(t, "firecrawl.go")
	})

	t.Run("content completion uses http status constant", func(t *testing.T) {
		assertTriggerNowStatusCodeConstant(t, "content_completion.go")
	})

	t.Run("preference update uses http status constant", func(t *testing.T) {
		assertTriggerNowStatusCodeConstant(t, "preference_update.go")
	})
}

func assertTriggerNowStatusCodeConstant(t *testing.T, fileName string) {
	t.Helper()

	path := fileName
	content, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("read %s: %v", fileName, err)
	}

	source := string(content)
	if !strings.Contains(source, "\"status_code\": http.StatusConflict") {
		t.Fatalf("%s should use http.StatusConflict", fileName)
	}
	if strings.Contains(source, "\"status_code\": 409") {
		t.Fatalf("%s should not hardcode 409", fileName)
	}
	if !strings.Contains(source, "\"reason\":      \"already_running\"") {
		t.Fatalf("%s should still return already_running", fileName)
	}
	if !strings.Contains(source, "\"accepted\":    false") {
		t.Fatalf("%s should still reject duplicate execution", fileName)
	}
}
