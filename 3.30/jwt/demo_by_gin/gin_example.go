// 鉴权中间件 (JWT) 的使用示例

// 在真正的 Web 开发中，前端每次发请求时，都会把 Token 塞进 HTTP 请求头的 Authorization 字段里。我们的中间件就是要站在大门口，搜身检查这个头。

package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 1. 定义一个极其机密的全局密钥 (在真实项目中，这个密钥绝对不能硬编码，必须用你刚学的 Viper 从 config.yaml 里读！)
var jwtKey = []byte("my_secret_key")

// 2. 定义 Payload (载荷) 结构体
// 继承 jwt.RegisteredClaims 可以自动包含过期时间(exp)、签发时间(iat)等标准字段
type CustomClaims struct {
	UserID               int64  `json:"user_id"`
	UserName             string `json:"user_name"`
	Role                 string `json:"role"`
	jwt.RegisteredClaims        // go语言里面针对继承的语法糖，匿名字段，直接嵌套一个结构体，就相当于继承了这个结构体的所有字段和方法
	/*
		官方源码里长这样(简化版)：
		type RegisteredClaims struct {
		    ExpiresAt int64 `json:"exp"` // 过期时间（必须！）
		    IssuedAt  int64 `json:"iat"` // 签发时间
		    NotBefore int64 `json:"nbf"` // 生效时间
		    Issuer    string `json:"iss"`// 签发者
		}
	*/
}

// 核心功能1:生成 Token
func GenerateToken(userID int64, userName string, role string) (string, error) {
	// 1. 创建一个 CustomClaims 实例，填充用户信息和标准字段
	claims := CustomClaims{
		UserID:   userID,
		UserName: userName,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24小时后过期
			IssuedAt:  jwt.NewNumericDate(time.Now()),                     // 当前时间作为签发时间
			Issuer:    "my_app",                                           // 签发者
		},
	}

	// 使用 HS256 算法创建一个新的 Token 对象，并将 claims 作为载荷
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 使用我们的机密密钥，进行最终的数字签名，生成真正的字符串 Token
	return token.SignedString(jwtKey)
}

// 核心功能 2：解析并校验 Token (每次请求 API 时调用)
func ParseToken(tokenString string) (*CustomClaims, error) {
	// 1. 解析 Token 字符串，得到一个 jwt.Token 对象
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 校验算法是否符合预期，是否是HMAC，防止恶意降级攻击
		// 如果攻击者把算法改成了 none，或者改成了 RSA 的算法，而我们只支持 HMAC，这里就会拒绝解析
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// 返回我们的机密密钥，供 jwt 库进行签名验证
		return jwtKey, nil
	})
	if err != nil {
		return nil, err
	}
	// 校验通过，提取出我们的 CustomClaims
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	} else {
		return nil, fmt.Errorf("invalid token")
	}
}

// Gin: JWT鉴权中间件
// 核心功能 3：编写 Gin 中间件，自动从请求头提取 Token，解析并校验，最后把用户信息放到上下文里
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 从 Authorization(请求头) 提取 Token
		// 规范的做法是前端把 Token 放在 Header 的 Authorization 字段，格式为 "Bearer 你的Token"
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未携带 Token，无权访问"})
			c.Abort()
			return
		}

		// 2. 按空格分割，获取真正的 Token 字符串
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 格式错误"})
			c.Abort()
			return
		}

		// 3. 调用我们写好的底层解析逻辑，解析并校验 Token
		claims, err := ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 无效"})
			c.Abort()
			return
		}

		// 4. 关键一步：校验通过后，把解析出的用户 ID 和角色挂载到 Gin 的 Context 中！
		// 这样后续的业务逻辑（比如发帖、看个人中心）就不需要再去查数据库，直接从 Context 拿我是谁。
		c.Set("userID", claims.UserID)
		c.Set("userName", claims.UserName)
		c.Set("role", claims.Role)

		// 5. 放行请求，继续后续的业务逻辑
		c.Next()
	}
}

func main() {
	r := gin.Default()

	// 开放路由，不需要token就能该访问的接口，（比如登录接口）
	r.POST("/login", func(ctx *gin.Context) {
		// 这里本应该去查 MySQL 验证账号密码，为了演示我们直接写死
		// 假设账号密码验证成功，我们给他颁发 Token
		token, err := GenerateToken(1001, "kaisgo", "admin")
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "Token 生成失败"})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"code": 200, "msg": "登录成功", "token": token})
	})

	// 受保护的路由组：必须经过 JWTAuthMiddleware 搜身检查
	authGroup := r.Group("/api")
	authGroup.Use(JWTAuthMiddleware())
	{
		// 测试接口：获取当前登录用户的个人信息
		authGroup.GET("/profile", func(c *gin.Context) {
			// 从 Context 中安全地取出刚才中间件放进去的数据
			userID, _ := c.Get("userID")
			username, _ := c.Get("userName")
			role, _ := c.Get("role")

			c.JSON(http.StatusOK, gin.H{
				"code": 0,
				"msg":  "成功访问机密接口",
				"data": fmt.Sprintf("你好 %s (ID:%v, 身份:%v)！你的代码非常牛逼。", username, userID, role),
			})
		})
	}
	r.Run(":8080")
}
