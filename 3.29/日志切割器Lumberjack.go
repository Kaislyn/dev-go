/*
引入日志切割神器：Lumberjack
在 Go 生态中，配合 Zap 做日志切割最经典的库是 lumberjack。

第一步：在终端下载依赖
go get -u gopkg.in/natefinch/lumberjack.v2
*/
package main

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func InitLogger2() *zap.Logger {
	// 1. 配置日志切割规则 (Lumberjack)
	logWriter := &lumberjack.Logger{
		Filename:   "./logs/app.log", // 日志文件存放路径
		MaxSize:    10,               // 每个日志文件最大 10 MB
		MaxBackups: 5,                // 最多保留 5 个旧的日志文件
		MaxAge:     30,               // 最多保留 30 天的日志
		Compress:   true,             // 是否对旧日志进行 gzip 压缩
	}

	// 2. 配置日志的输出格式 (JSON 格式更适合机器收集，Console 格式适合人类阅读)
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder   // 时间格式：2026-03-29T...
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 级别大写：INFO, ERROR

	// 这里为了方便你本地调试，我们依然用 Console 格式。生产环境通常换成 zapcore.NewJSONEncoder(encoderConfig)
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	// 3. 将日志同时输出到“文件”和“控制台”
	// zapcore.AddSync 包装了 lumberjack
	fileSyncer := zapcore.AddSync(logWriter)
	consoleSyncer := zapcore.AddSync(os.Stdout)

	// 使用 MultiWriteSyncer 实现双写
	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(fileSyncer, consoleSyncer),
		zap.DebugLevel, // 记录 Debug 及以上级别的日志
	)

	// 4. 生成 Logger，并开启调用栈抓取 (AddCaller)
	logger := zap.New(core, zap.AddCaller())
	return logger
}
