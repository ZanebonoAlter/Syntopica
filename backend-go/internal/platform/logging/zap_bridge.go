package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewZapLogger 返回一个写真实输出的 *zap.Logger，供仍持有 *zap.Logger 的组件
// （embedding/merge 队列 worker）使用——替换 zap.NewNop() 吞日志的历史行为
// （speed-up-tag-embedding D5）。项目日志主干是 slog；这是过渡桥，新代码应直接
// 用本包的 Infof/Warnf/Errorf。
//
// 输出目标与 slog 保持一致（fix-log-stdout-rotation）：文件日志启用时写轮转文件，
// 否则写 stdout。历史实现硬编码 os.Stdout，使队列 worker 的日志绕过轮转、无界写入
// backend-go/nohup.out。
func NewZapLogger() *zap.Logger {
	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.NameKey = "component"
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		activeWriteSyncer(),
		zapcore.InfoLevel,
	)
	return zap.New(core)
}

// activeWriteSyncer 选择与 slog 相同的主通道。
func activeWriteSyncer() zapcore.WriteSyncer {
	if w := FileWriter(); w != nil {
		return zapcore.Lock(zapcore.AddSync(w))
	}
	return zapcore.Lock(os.Stdout)
}
