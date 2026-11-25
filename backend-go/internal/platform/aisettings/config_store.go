package aisettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

const openNotebookConfigKey = "open_notebook_config"
const firecrawlConfigKey = "firecrawl_config"
const bochaConfigKey = "bocha_config"
const comtradeConfigKey = "comtrade_config"
const fredConfigKey = "fred_config"
const rsshubConfigKey = "rsshub_config"
const proxyConfigKey = "http_proxy_config"
const dailyReportTimeKey = "daily_report_time"
const defaultDailyReportTime = "21:00"
const dailyReportDeadlineKey = "daily_report_deadline"
const defaultDailyReportDeadline = "23:30"
const boardUpgradeSuggestTimeKey = "semantic_board_upgrade_suggest_time"
const defaultBoardUpgradeSuggestTime = "06:30"
const rsshubDocBaseKey = "rsshub_doc_base"
const defaultRSSHubDocBase = "https://docs.rsshub.app"
const analysisPausedKey = "analysis_paused"
const autoStartModelsKey = "auto_start_models"

// add-ai-generated-article-detection design D11：判定阈值与判定样本长度走 ai_settings
// 在线可覆盖（误报率是本功能最大的未知数，必须不改代码、不重启就能调）。三个键各自
// 存裸字符串（数值型存裸字符串，与 daily_report_time 同款），不走 JSON 对象。
const (
	aiDetectionThresholdKey  = "ai_detection_threshold"
	aiDetectionSampleHeadKey = "ai_detection_sample_head"
	aiDetectionSampleTailKey = "ai_detection_sample_tail"
)

// 判定参数默认值（design D11）：阈值 0.7、样本首 3000 runes + 尾 2000 runes。
const (
	DefaultAIDetectionThreshold  = 0.7
	DefaultAIDetectionSampleHead = 3000
	DefaultAIDetectionSampleTail = 2000
)

// DefaultRSSHubDocBase 返回 rsshub_doc_base 的默认值（design D4），供 handler 透传给前端。
func DefaultRSSHubDocBase() string { return defaultRSSHubDocBase }

var hhmmPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)

func loadConfigByKey(key string) (map[string]interface{}, *models.AISettings, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", key).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]interface{}{}, nil, nil
		}
		return nil, nil, err
	}

	config := map[string]interface{}{}
	if settings.Value == "" {
		return config, &settings, nil
	}

	if err := json.Unmarshal([]byte(settings.Value), &config); err != nil {
		return nil, nil, err
	}

	return config, &settings, nil

}

func saveConfigByKey(key string, config map[string]interface{}, description string) error {
	configJSON, err := models.ToJSONValue(config)
	if err != nil {
		return err
	}

	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", key).First(&settings).Error
	if dbErr == nil {
		settings.Value = configJSON
		if description != "" {
			settings.Description = description
		}
		return database.DB.Save(&settings).Error
	}

	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}

	settings = models.AISettings{
		Key:         key,
		Value:       configJSON,
		Description: description,
	}

	return database.DB.Create(&settings).Error
}

func LoadFirecrawlConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(firecrawlConfigKey)
}

func SaveFirecrawlConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(firecrawlConfigKey, config, description)
}

// LoadBochaConfig 读取 bocha_config（数据增强 web_search 后端的 key + endpoint + enabled）。
// 动态读取：BochaWebSearcher 每次 Search 现读，界面改即时生效（对齐 Firecrawl 每次 job 读 DB）。
func LoadBochaConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(bochaConfigKey)
}

// SaveBochaConfig 写入 bocha_config。
func SaveBochaConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(bochaConfigKey, config, description)
}

// LoadComtradeConfig 读取 comtrade_config（研究数据源 UN Comtrade 的订阅 key + enabled）。
// 动态读取：取数器/probe/目录 status 每次现读，界面改即时生效（对齐 bocha_config 语义）。
func LoadComtradeConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(comtradeConfigKey)
}

// SaveComtradeConfig 写入 comtrade_config。
func SaveComtradeConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(comtradeConfigKey, config, description)
}

// LoadFredConfig 读取 fred_config（研究数据源 FRED 的 API key + enabled）。
// 动态读取：取数器/probe/目录 status 每次现读，界面改即时生效（comtrade 同款）。
func LoadFredConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(fredConfigKey)
}

// SaveFredConfig 写入 fred_config。
func SaveFredConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(fredConfigKey, config, description)
}

func LoadOpenNotebookConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(openNotebookConfigKey)
}

func SaveOpenNotebookConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(openNotebookConfigKey, config, description)
}

// LoadRSSHubConfig 读取 rsshub_config（RSSHub 实例地址等，design E）。
func LoadRSSHubConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(rsshubConfigKey)
}

// SaveRSSHubConfig 写入 rsshub_config。
func SaveRSSHubConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(rsshubConfigKey, config, description)
}

// LoadProxyConfig 读取 http_proxy_config（feed 抓取等所有外部请求的全局出站代理地址）。
func LoadProxyConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(proxyConfigKey)
}

// SaveProxyConfig 写入 http_proxy_config。
func SaveProxyConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(proxyConfigKey, config, description)
}

// LoadDailyReportTimeConfig loads the daily_report_time setting from ai_settings.
// Returns the HH:MM string. If the key is missing or invalid, returns default "21:00".
func LoadDailyReportTimeConfig() (string, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", dailyReportTimeKey).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultDailyReportTime, nil
		}
		return "", err
	}

	value := strings.TrimSpace(settings.Value)
	if !hhmmPattern.MatchString(value) {
		logging.Warnf("Invalid daily_report_time value %q, falling back to default %s", value, defaultDailyReportTime)
		return defaultDailyReportTime, nil
	}
	return value, nil
}

// SaveDailyReportTimeConfig saves the daily_report_time setting.
// Validates HH:MM format (00:00–23:59). Returns error for invalid values.
func SaveDailyReportTimeConfig(value string) error {
	value = strings.TrimSpace(value)
	if !hhmmPattern.MatchString(value) {
		return fmt.Errorf("invalid daily_report_time format %q: expected HH:MM (00:00–23:59)", value)
	}

	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", dailyReportTimeKey).First(&settings).Error
	if dbErr == nil {
		settings.Value = value
		return database.DB.Save(&settings).Error
	}
	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}

	return database.DB.Create(&models.AISettings{
		Key:         dailyReportTimeKey,
		Value:       value,
		Description: "日报生成时刻（HH:MM）",
	}).Error
}

// LoadDailyReportDeadlineConfig loads the daily_report_deadline setting from
// ai_settings (night-window-alignment D5). Returns the HH:MM string, or
// default "23:30" when the key is missing or invalid (mirrors
// LoadDailyReportTimeConfig).
func LoadDailyReportDeadlineConfig() (string, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", dailyReportDeadlineKey).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultDailyReportDeadline, nil
		}
		return "", err
	}

	value := strings.TrimSpace(settings.Value)
	if !hhmmPattern.MatchString(value) {
		logging.Warnf("Invalid daily_report_deadline value %q, falling back to default %s", value, defaultDailyReportDeadline)
		return defaultDailyReportDeadline, nil
	}
	return value, nil
}

// SaveDailyReportDeadlineConfig saves the daily_report_deadline setting.
// Validates HH:MM format (00:00–23:59). Returns error for invalid values.
func SaveDailyReportDeadlineConfig(value string) error {
	value = strings.TrimSpace(value)
	if !hhmmPattern.MatchString(value) {
		return fmt.Errorf("invalid daily_report_deadline format %q: expected HH:MM (00:00–23:59)", value)
	}

	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", dailyReportDeadlineKey).First(&settings).Error
	if dbErr == nil {
		settings.Value = value
		return database.DB.Save(&settings).Error
	}
	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}

	return database.DB.Create(&models.AISettings{
		Key:         dailyReportDeadlineKey,
		Value:       value,
		Description: "日报兜底强制生成时刻（HH:MM），不得早于 daily_report_time",
	}).Error
}

// LoadDailyReportWindowConfig loads the daily report wall-clock time and
// fallback deadline as a pair (night-window-alignment D4/D5): both keys
// self-fall-back to defaults on missing/invalid values (warn included), and a
// deadline earlier than the wall-clock time falls back to the default 23:30
// with a warning (spec: 兜底不得早于墙钟)。A database error returns empty
// strings + err; the caller applies full defaults.
func LoadDailyReportWindowConfig() (timeStr, deadlineStr string, err error) {
	timeStr, err = LoadDailyReportTimeConfig()
	if err != nil {
		return "", "", err
	}
	deadlineStr, err = LoadDailyReportDeadlineConfig()
	if err != nil {
		return "", "", err
	}
	if hhmmMinutes(deadlineStr) < hhmmMinutes(timeStr) {
		logging.Warnf("daily_report_deadline %s is earlier than daily_report_time %s, falling back to default %s",
			deadlineStr, timeStr, defaultDailyReportDeadline)
		deadlineStr = defaultDailyReportDeadline
	}
	return timeStr, deadlineStr, nil
}

// hhmmMinutes converts a pre-validated HH:MM string to minutes since midnight.
func hhmmMinutes(hhmm string) int {
	var h, m int
	_, _ = fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	return h*60 + m
}

// LoadBoardUpgradeSuggestTimeConfig loads the semantic_board_upgrade_suggest_time
// setting from ai_settings. Returns the HH:MM string, or default "06:30" when
// the key is missing or invalid (mirrors LoadDailyReportTimeConfig).
func LoadBoardUpgradeSuggestTimeConfig() (string, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", boardUpgradeSuggestTimeKey).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultBoardUpgradeSuggestTime, nil
		}
		return "", err
	}

	value := strings.TrimSpace(settings.Value)
	if !hhmmPattern.MatchString(value) {
		logging.Warnf("Invalid semantic_board_upgrade_suggest_time value %q, falling back to default %s", value, defaultBoardUpgradeSuggestTime)
		return defaultBoardUpgradeSuggestTime, nil
	}
	return value, nil
}

// SaveBoardUpgradeSuggestTimeConfig saves the semantic_board_upgrade_suggest_time
// setting. Validates HH:MM format (00:00–23:59). Returns error for invalid values.
func SaveBoardUpgradeSuggestTimeConfig(value string) error {
	value = strings.TrimSpace(value)
	if !hhmmPattern.MatchString(value) {
		return fmt.Errorf("invalid semantic_board_upgrade_suggest_time format %q: expected HH:MM (00:00–23:59)", value)
	}

	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", boardUpgradeSuggestTimeKey).First(&settings).Error
	if dbErr == nil {
		settings.Value = value
		return database.DB.Save(&settings).Error
	}
	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}

	return database.DB.Create(&models.AISettings{
		Key:         boardUpgradeSuggestTimeKey,
		Value:       value,
		Description: "版块升级建议生成时刻（HH:MM）",
	}).Error
}

// isValidHTTPURL reports whether s is an http/https URL with a non-empty host.
// Used to guard rsshub_doc_base against junk / non-reachable values.
func isValidHTTPURL(s string) bool {
	parsed, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// LoadRSSHubDocBaseConfig loads the rsshub_doc_base setting from ai_settings.
// Returns the doc base URL. If the key is missing or invalid (not an http/https
// URL), returns the default "https://docs.rsshub.app" (design D4).
func LoadRSSHubDocBaseConfig() (string, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", rsshubDocBaseKey).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultRSSHubDocBase, nil
		}
		return "", err
	}

	value := strings.TrimSpace(settings.Value)
	if !isValidHTTPURL(value) {
		logging.Warnf("Invalid rsshub_doc_base value %q, falling back to default %s", value, defaultRSSHubDocBase)
		return defaultRSSHubDocBase, nil
	}
	return value, nil
}

// SaveRSSHubDocBaseConfig saves the rsshub_doc_base setting. Validates that
// value is an http/https URL with a non-empty host. Returns error for invalid
// values.
func SaveRSSHubDocBaseConfig(value string) error {
	value = strings.TrimSpace(value)
	if !isValidHTTPURL(value) {
		return fmt.Errorf("invalid rsshub_doc_base value %q: expected http/https URL", value)
	}

	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", rsshubDocBaseKey).First(&settings).Error
	if dbErr == nil {
		settings.Value = value
		return database.DB.Save(&settings).Error
	}
	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}

	return database.DB.Create(&models.AISettings{
		Key:         rsshubDocBaseKey,
		Value:       value,
		Description: "RSSHub 官方文档基址（用于生成参数文档链接）",
	}).Error
}

// LoadAnalysisPausedConfig reads the analysis_paused flag from ai_settings.
// Returns (paused=false, pausedAt=nil) when the key is missing so the default
// is "not paused" (fail-open). The value is stored as a JSON object
// {"paused":bool,"paused_at":RFC3339}.
func LoadAnalysisPausedConfig() (paused bool, pausedAt *time.Time, err error) {
	config, _, err := loadConfigByKey(analysisPausedKey)
	if err != nil {
		return false, nil, err
	}
	if v, ok := config["paused"].(bool); ok {
		paused = v
	}
	if v, ok := config["paused_at"].(string); ok && v != "" {
		if t, parseErr := time.Parse(time.RFC3339, v); parseErr == nil {
			pausedAt = &t
		}
	}
	return paused, pausedAt, nil
}

// SaveAnalysisPausedConfig writes the analysis_paused flag. When engaging
// (paused=true), paused_at is stamped to now (UTC); when releasing (false),
// paused_at is cleared.
func SaveAnalysisPausedConfig(paused bool) error {
	config := map[string]interface{}{"paused": paused}
	if paused {
		config["paused_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	return saveConfigByKey(analysisPausedKey, config, "分析暂停总闸开关（paused + paused_at）")
}

// LoadAutoStartModelsConfig reads the auto_start_models flag from ai_settings.
// Returns false when the key is missing or the value cannot be parsed so the
// default is "do not auto-launch local model processes" (safe default). The
// value is stored as a JSON object {"enabled":bool}.
func LoadAutoStartModelsConfig() (bool, error) {
	config, _, err := loadConfigByKey(autoStartModelsKey)
	if err != nil {
		return false, err
	}
	if v, ok := config["enabled"].(bool); ok {
		return v, nil
	}
	return false, nil
}

// SaveAutoStartModelsConfig writes the auto_start_models flag (the local-model
// auto-launch master switch for aihealth.RunStartupProbe).
func SaveAutoStartModelsConfig(enabled bool) error {
	config := map[string]interface{}{"enabled": enabled}
	return saveConfigByKey(autoStartModelsKey, config, "本地模型自动拉起总开关（enabled）")
}

// AIDetectionConfig 是「疑似 AI 生成」判定参数（add-ai-generated-article-detection design D11）。
//
// Threshold：置信分不低于它即标记为疑似（0-1）。SampleHead/SampleTail：判定样本取正文
// 首 N runes + 尾 M runes。三个字段都经 ai_settings 在线可覆盖，改后立即对后续判定生效
// （判定服务每次判定现读，不缓存）。
type AIDetectionConfig struct {
	Threshold  float64
	SampleHead int
	SampleTail int
}

// DefaultAIDetectionConfig 返回判定参数默认值（阈值 0.7、样本首 3000 + 尾 2000 runes）。
func DefaultAIDetectionConfig() AIDetectionConfig {
	return AIDetectionConfig{
		Threshold:  DefaultAIDetectionThreshold,
		SampleHead: DefaultAIDetectionSampleHead,
		SampleTail: DefaultAIDetectionSampleTail,
	}
}

// LoadAIDetectionConfig 读取判定参数（阈值 + 样本长度）。
// 键缺失或值非法时回落到默认值并记警告（fail-safe：配置坏了也不能把判定链路卡死）；
// 数据库错误直接返回（调用方按失败处理，不静默用旧值）。
func LoadAIDetectionConfig() (AIDetectionConfig, error) {
	cfg := DefaultAIDetectionConfig()

	raw, ok, err := loadScalarSetting(aiDetectionThresholdKey)
	if err != nil {
		return cfg, err
	}
	if ok {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil || value <= 0 || value > 1 {
			logging.Warnf("Invalid %s value %q, falling back to default %v", aiDetectionThresholdKey, raw, cfg.Threshold)
		} else {
			cfg.Threshold = value
		}
	}

	if raw, ok, err = loadScalarSetting(aiDetectionSampleHeadKey); err != nil {
		return cfg, err
	} else if ok {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 0 {
			logging.Warnf("Invalid %s value %q, falling back to default %d", aiDetectionSampleHeadKey, raw, cfg.SampleHead)
		} else {
			cfg.SampleHead = value
		}
	}

	if raw, ok, err = loadScalarSetting(aiDetectionSampleTailKey); err != nil {
		return cfg, err
	} else if ok {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 0 {
			logging.Warnf("Invalid %s value %q, falling back to default %d", aiDetectionSampleTailKey, raw, cfg.SampleTail)
		} else {
			cfg.SampleTail = value
		}
	}

	return cfg, nil
}

// SaveAIDetectionConfig 写入判定参数（阈值 + 样本长度），改后立即对后续判定生效。
// 校验：阈值必须在 (0, 1]；样本长度不得为负。
func SaveAIDetectionConfig(cfg AIDetectionConfig) error {
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		return fmt.Errorf("invalid %s %v: expected (0, 1]", aiDetectionThresholdKey, cfg.Threshold)
	}
	if cfg.SampleHead < 0 || cfg.SampleTail < 0 {
		return fmt.Errorf("invalid ai detection sample lengths: head=%d tail=%d, must be >= 0", cfg.SampleHead, cfg.SampleTail)
	}
	if err := saveScalarSetting(aiDetectionThresholdKey, strconv.FormatFloat(cfg.Threshold, 'f', -1, 64),
		"疑似 AI 判定置信分阈值（0-1，达到即标记疑似）"); err != nil {
		return err
	}
	if err := saveScalarSetting(aiDetectionSampleHeadKey, strconv.Itoa(cfg.SampleHead),
		"疑似 AI 判定样本首段长度（runes）"); err != nil {
		return err
	}
	return saveScalarSetting(aiDetectionSampleTailKey, strconv.Itoa(cfg.SampleTail),
		"疑似 AI 判定样本尾段长度（runes）")
}

// loadScalarSetting 读取一个存裸字符串的 ai_settings 键。
// 缺失 → (空, false, nil)；DB 错误 → 错误；值为空 → (空, false, nil)。
func loadScalarSetting(key string) (string, bool, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", key).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	value := strings.TrimSpace(settings.Value)
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

// saveScalarSetting 写入一个存裸字符串的 ai_settings 键（存在则更新，不存在则创建）。
func saveScalarSetting(key, value, description string) error {
	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", key).First(&settings).Error
	if dbErr == nil {
		settings.Value = value
		if description != "" {
			settings.Description = description
		}
		return database.DB.Save(&settings).Error
	}
	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}
	return database.DB.Create(&models.AISettings{
		Key:         key,
		Value:       value,
		Description: description,
	}).Error
}
