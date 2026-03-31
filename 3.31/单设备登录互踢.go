// 白名单模式实现“单设备登录互踢”
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

var (
	ctx    = context.Background()
	jwtKey = []byte("my_secret_key")
	rdb    *redis.Client // 全局 Redis 客户端连接池，这里声明的是一个空指针，真正的连接池会在 initRedis() 里初始化
)

// 初始化 Redis
func initRedis() {
	// 初始化 Redis 客户端连接池
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Redis 服务器地址
		// 这里可以设置密码，如果 Redis 没有设置密码，这个字段可以省略
		// Password: "your_redis_password",
		DB: 0, // 使用默认数据库，Redis 默认有 16 个数据库，编号从 0 到 15，默认使用 0 号数据库
		// 这里可以进行一些有关于连接池的配置，比如连接池大小、空闲连接数等，默认已经有合理的设置了，不需要特别调整
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(fmt.Sprintf("连接 Redis 失败: %v", err))
	}
	fmt.Println("连接 Redis 成功")
}

// JWT 载荷定义与生成
type CustomClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int64) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			// 24小时后过期
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// 安保中间件：JWT 签名校验 + Redis 单设备互踢校验
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从 Authorization 头部提取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未携带 Token"})
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 格式错误"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 2. 解析并校验 Token
		// 验证 JWT 自身签名和过期时间（防止伪造）
		token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 无效或已过期"})
			c.Abort()
			return
		}
		claims := token.Claims.(*CustomClaims)

		// 去 Redis 校验是否是当前最新登录的设备 (白名单比对)
		redisKey := fmt.Sprintf("user_token:%d", claims.UserID)
		validTokenInRedis, err := rdb.Get(ctx, redisKey).Result()

		if err == redis.Nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "登录已失效，请重新登录"})
			c.Abort()
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "服务器开小差啦"})
			c.Abort()
			return
		}

		// 如果前端传来的 Token 和 Redis 里存的最新 Token 不一致，说明在别处登录了！
		if tokenString != validTokenInRedis {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "您的账号已在其他设备登录，您已被迫下线！"})
			c.Abort()
			return
		}

		// 步骤 C：所有安检通过，放行！
		c.Set("userID", claims.UserID)
		c.Next()
	}
}

func main() {
	initRedis() // 启动时初始化 Redis 连接池

	r := gin.Default()

	// 模拟登录接口
	r.POST("/login", func(c *gin.Context) {
		userID := int64(1001) // 模拟一个用户 ID，实际开发中应该根据用户名密码验证后拿到用户 ID
		// 1.生成 JWT Token
		token, _ := GenerateToken(userID)

		// 2. 将 Token 存入 Redis，覆盖之前的 Token（单设备登录互踢核心），实现踢人！TTL 设置和 JWT 过期时间一致（24小时）
		redisKey := fmt.Sprintf("user_token:%d", userID)
		rdb.Set(ctx, redisKey, token, 24*time.Hour) // 设置过期时间和 JWT 一致

		c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "登录成功", "token": token})
	})

	// 模拟机密接口
	authGroup := r.Group("/api")
	authGroup.Use(JWTAuthMiddleware())
	{
		authGroup.GET("/profile", func(c *gin.Context) {
			userID, _ := c.Get("userID")
			c.JSON(http.StatusOK, gin.H{"code": 200, "msg": fmt.Sprintf("欢迎回来，尊贵的用户 %v", userID)})
		})
	}

	r.Run(":8080")
}
