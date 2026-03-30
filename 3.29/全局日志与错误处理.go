/*
全局日志与错误处理

在以往的写法中含有一些问题：

1.使用了标准库的 log 和 fmt： 大厂的日志是需要被 ELK（Elasticsearch, Logstash, Kibana）等日志系统收集分析的。标准库的日志缺乏结构化（JSON 格式化），且性能较差。大厂几乎清一色使用高性能的结构化日志库，比如 Uber 开源的 zap 或 logrus。
2.日志文件没有切割（Log Rotation）： 直接用 os.Create("gin.log") 把日志全塞进一个文件里。在生产环境中，这个文件几天就会膨胀到几十个 G，导致服务器磁盘打满挂掉。真实的玩法必须按天或按文件大小对日志进行切割。
3.过度依赖 gin.Default()： gin.Default() 会默认带上 Gin 官方的 Logger 和 Recovery 中间件。官方的日志格式很难看，且 Recovery 遇到 Panic 时虽然能防止程序崩溃，但打出来的堆栈信息不易被收集系统解析。大厂通常用 gin.New() 创建一个干净的引擎，然后注入自己写的基于 zap 的日志和恢复中间件。

进阶/优化建议：
直接引入 Uber 的 zap 日志库，替换掉 Gin 简陋的默认日志，并建立一套规范的 API 响应体系。

第一步：安装必备依赖
在你的终端（项目目录下）运行以下命令，拉取 zap 库：
go get -u go.uber.org/zap
*/

package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 1.定义规范的API的统一响应结构
type Response struct {
	Code int         `json:"code"` // 业务状态码
	Msg  string      `json:"msg"`  // 提示信息
	Data interface{} `json:"data"` // 核心数据
}

// 统一成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: data,
	})
}

// 统一错误响应
func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{ // 注意：HTTP状态码通常保持200，具体错误看业务Code
		Code: code,
		Msg:  msg,
		Data: nil,
	})
}

// 2. 初始化 Zap 高性能日志
func InitLogger() *zap.Logger {
	// 在生产环境中，我们会配置输出到文件并切割，这里为了演示直观，先输出到控制台(带颜色)
	config := zap.NewDevelopmentConfig()
	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}
	return logger
}

// 3.自定义 gin 中间件，使用 zap 记录请求日志
func ZapLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		// 执行后续的Handler
		c.Next() // 处理请求

		cost := time.Since(start)
		// 结构化记录请求日志
		logger.Info("HTTP Request",
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.Duration("cost", cost),
		)
	}
}

// 4.自定义Recovery中间件，使用zap记录Panic信息
func ZapRecovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录 Panic 的严重错误和堆栈信息
				logger.Error("Application Panic Recovery",
					zap.Any("error", err),
					zap.Stack("stack"), // 自动捕获堆栈追踪
				)
				// 给前端返回友好的 500 错误，而不是白屏
				c.AbortWithStatusJSON(http.StatusInternalServerError, Response{
					Code: 500,
					Msg:  "服务器内部开小差啦，请稍后再试",
				})
			}
		}()
		c.Next()
	}
}

// 核心路由与测试
func test_main() {
	// 初始化日志
	logger := InitLogger()
	// 确保所有日志在程序退出前刷入磁盘
	defer logger.Sync()

	// 创建 Gin 引擎
	r := gin.New() // 不使用默认的 Logger 和 Recovery 中间件

	// 注入自定义的 Zap 日志和 Recovery 中间件
	r.Use(ZapLogger(logger), ZapRecovery(logger))

	// 测试1：正常请求
	r.GET("/user", func(ctx *gin.Context) {
		Success(ctx, gin.H{
			"username": "kaisgo",
			"role":     "admin",
		})
	})

	// 测试2：业务逻辑错误 (例如参数校验失败)
	r.GET("/login", func(ctx *gin.Context) {
		// 模拟密码错误的业务逻辑
		Fail(ctx, 10001, "用户名或密码错误")
	})

	// 测试3：致命错误 Panic 拦截测试
	r.GET("/panic", func(ctx *gin.Context) {
		// 模拟代码写了 Bug，导致切片越界或空指针等 Panic
		var slice []int
		fmt.Println(slice[10]) // 这里会触发 Panic！
	})
}
