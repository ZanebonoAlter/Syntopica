package preferences

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/app/runtimeinfo"
)

type stubPreferenceScheduler struct {
	called bool
}

func (s *stubPreferenceScheduler) TriggerNow() map[string]interface{} {
	s.called = true
	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  "Preference update triggered",
	}
}

func TestTriggerPreferenceUpdateUsesRuntimeSchedulerWhenAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := runtimeinfo.PreferenceUpdateSchedulerInterface
	stub := &stubPreferenceScheduler{}
	runtimeinfo.PreferenceUpdateSchedulerInterface = stub
	defer func() {
		runtimeinfo.PreferenceUpdateSchedulerInterface = original
	}()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	TriggerPreferenceUpdate(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, stub.called)

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
	require.Equal(t, "Preference update triggered", body["message"])
}
