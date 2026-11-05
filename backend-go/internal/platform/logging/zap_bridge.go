package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewZapLogger 返回一个写真实输出的 *zap.Logger（stdout console 流，与 slog
// 默认 logger 同一去处），供仍持有 *zap.Logger 的组件（embedding/merge 队列
// worker）使用——替换 zap.NewNop() 吞日志的历史行为（speed-up-tag-embedding D5）。
// 项目日志主干是 slog；这是过渡桥，新代码应直接用本包的 Infof/Warnf/Errorf。
func NewZapLogger() *zap.Logger {
	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.NameKey = "component"
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.Lock(os.Stdout),
		zapcore.InfoLevel,
	)
	return zap.New(core)
}
