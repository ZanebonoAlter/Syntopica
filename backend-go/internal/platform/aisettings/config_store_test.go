package aisettings

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupOpenNotebookConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:aisettings_opennotebook_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&models.AISettings{}))
	database.DB = db

	return db
}

func TestLoadOpenNotebookConfig_MissingReturnsEmpty(t *testing.T) {
	setupOpenNotebookConfigTestDB(t)

	config, settings, err := LoadOpenNotebookConfig()

	require.NoError(t, err)
	require.Nil(t, settings)
	require.Equal(t, map[string]interface{}{}, config)
}

func TestSaveOpenNotebookConfig_CreatesAndUpdatesRow(t *testing.T) {
	db := setupOpenNotebookConfigTestDB(t)

	err := SaveOpenNotebookConfig(map[string]interface{}{
		"enabled":         true,
		"base_url":        "https://open-notebook.example/api",
		"target_notebook": "digest-lab",
	}, "open notebook config")
	require.NoError(t, err)

	config, settings, err := LoadOpenNotebookConfig()
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, true, config["enabled"])
	require.Equal(t, "https://open-notebook.example/api", config["base_url"])
	require.Equal(t, "digest-lab", config["target_notebook"])
	require.Equal(t, "open notebook config", settings.Description)

	err = SaveOpenNotebookConfig(map[string]interface{}{
		"enabled":  false,
		"base_url": "https://open-notebook.example/v2",
	}, "updated open notebook config")
	require.NoError(t, err)

	config, settings, err = LoadOpenNotebookConfig()
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, false, config["enabled"])
	require.Equal(t, "https://open-notebook.example/v2", config["base_url"])
	require.Equal(t, "updated open notebook config", settings.Description)

	var count int64
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", openNotebookConfigKey).Count(&count).Error)
	require.EqualValues(t, 1, count)

	var summaryCount int64
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", summaryConfigKey).Count(&summaryCount).Error)
	require.EqualValues(t, 0, summaryCount)
}
