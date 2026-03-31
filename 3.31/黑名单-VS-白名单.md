这正是从“理论学习”向“工程实战”跨越的必经之路。你现在的情况就好比手里已经有了高性能发动机（Gin）、顶级防伪车钥匙（JWT）和氮气加速系统（Redis），但还没把它们组装成一辆能在赛道上狂飙的跑车。

今天，我们就把它们组装起来。

### 答疑：黑名单 vs 白名单，哪个更常用？

大厂面试中，这是一个非常经典的系统设计问题。答案是：**都要掌握，但大厂实际业务中，基于“白名单”思想的演进方案用得更多！**

* **黑名单模式（只存坏人）：**
    * *原理：* 只有用户点击“退出登录”时，才把他的 Token 塞进 Redis（设为黑名单）。中间件每次都去查这个 Token 在不在黑名单里。
    * *优点：* 完美保留了 JWT 的“无状态”特性，Redis 里数据很少，极其节省内存。
    * *缺点：* 只能处理“主动退出”。如果遇到“账号被盗，想强制踢下线”或者“只允许一台设备在线”这种需求，黑名单很难搞。
* **白名单模式（变种 Session，存当前真身）：**
    * *原理：* 登录时，把 Token 存入 Redis `Key = user_token:1001`。每次请求，不但要校验 JWT 自身签名，还要去 Redis 里对比，看看请求头里的 Token 和 Redis 里存的是不是完全一样。
    * *大厂现状：* **大厂绝大多数核心业务为了绝对的安全控制，会采用这种模式（或者它的变种：存储 Token 版本号）。** 因为对大厂来说，“账号安全和风控拦截”远比“省那点 Redis 内存”重要得多。

---

### 实战场景演示：大厂必备的“单设备登录互踢”

为了让你感受最真实的业务场景，我们今天用 **白名单模式** 来实现一个几乎所有 App 都有的功能：**单设备互踢**（你在手机上登录了，如果你又在电脑上登录，手机就会被强制挤下线）。



#### 核心运转逻辑（仔细看这三步）：
1.  **用户 A 在 Mac 登录：** 生成 `Token_A`。将 `Key: user_token:1001`, `Value: Token_A` 存入 Redis。返回给 Mac。
2.  **用户 A 又在手机登录：** 生成 `Token_B`。存入 Redis 时，**直接覆盖**掉了原来的记录！此时 Redis 里 `user_token:1001` 的值变成了 `Token_B`。
3.  **Mac 再次发请求：** Mac 带着旧的 `Token_A` 来请求 API。中间件解析出用户是 1001，去 Redis 一查，发现 1001 当前合法的 Token 是 `Token_B`！两者不匹配！中间件直接无情拦截：“您的账号已在其他设备登录，请重新登录”。

---

### 硬核代码组装 (Gin + JWT + Redis)

把下面的代码完整复制到你 Mac 的项目中，这就已经是一个可以写进简历里的微型架构了：

```go
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
	ctx      = context.Background()
	jwtKey   = []byte("Kaisgo_Super_Secret_Key")
	rdb      *redis.Client
)

// --- 1. 初始化 Redis ---
func initRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic("Redis 连接失败")
	}
	fmt.Println("✅ Redis 就绪")
}

// --- 2. JWT 载荷定义与生成 ---
type CustomClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int64) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24小时过期
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// --- 3. 核心大厂级安保中间件：JWT 签名校验 + Redis 单设备互踢校验 ---
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		// 步骤 A：验证 JWT 自身签名和过期时间（防止伪造）
		token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 无效或已过期"})
			c.Abort()
			return
		}

		claims := token.Claims.(*CustomClaims)

		// 步骤 B：绝杀！去 Redis 校验是否是当前最新登录的设备 (白名单比对)
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
	initRedis()
	r := gin.Default()

	// 模拟登录接口
	r.POST("/login", func(c *gin.Context) {
		userID := int64(1001) // 假设数据库校验账号密码成功，用户ID为1001

		// 1. 生成新 Token
		token, _ := GenerateToken(userID)

		// 2. 存入 Redis，直接覆盖旧 Token，实现踢人！TTL 设置和 JWT 过期时间一致（24小时）
		redisKey := fmt.Sprintf("user_token:%d", userID)
		rdb.Set(ctx, redisKey, token, 24*time.Hour)

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
```

### 你接下来的测试任务（极具戏剧性）：

把这段代码跑起来，打开你的 Apifox，亲自导演这场“篡权夺位”的大戏：

1.  **模拟第一台设备登录：** 请求 `/login`，拿到 `Token_A`。
2.  **验证第一台设备：** 带着 `Token_A` 去请求 `/api/profile`，你会看到“欢迎回来”。
3.  **模拟第二台设备登录（关键）：** 再次请求 `/login`，你会拿到一个全新的 `Token_B`。此时，Redis 里的记录已经被 `Token_B` 覆盖了。
4.  **旧设备惨遭踢出：** 你再次拿着旧的 `Token_A` 去请求 `/api/profile`，你会立刻吃到一个惊叹号级别的报错：`"您的账号已在其他设备登录，您已被迫下线！"`。

这个实战场景，不仅让你彻底搞懂了 JWT 和 Redis 的联合应用，更是你找实习时和面试官对答如流的绝对资本。要不要马上用 Apifox 试一把被“踢下线”的快感？