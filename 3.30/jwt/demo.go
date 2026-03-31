// 编写 JWT 签发与解析核心逻辑

/*
一个标准的 JWT 长得像一长串乱码，由三个部分组成，中间用 . 隔开：
Header（头部）：记录加密算法（比如 HS256）。
Payload（载荷）：存放不敏感的用户信息（比如用户 ID、过期时间）。
Signature（签名）：核心防伪标志，用服务器的私钥生成的。
*/

package main

import (
	"fmt"
	"time"

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

// 关于算法的解释：
// HMAC 是算法家族（大类）
// HS256 是这个家族里最常用的具体算法（小类）

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

func main() {
	// 测试环节
	fmt.Println("1. 模拟用户登录成功，颁发Token")
	token, err := GenerateToken(1001, "kaisgo", "admin")
	if err != nil {
		panic(err)
	}
	fmt.Printf("生成的 Token 是:\n%s\n\n", token)

	fmt.Println("2. 模拟前端带着 Token 来请求 API，验证解析")
	claims, err := ParseToken(token)
	if err != nil {
		fmt.Printf("解析 Token 失败: %v\n", err)
	} else {
		fmt.Printf("解析成功，Token 中的用户信息是:\nUserID: %d\nUserName: %s\nRole: %s\n", claims.UserID, claims.UserName, claims.Role)
	}

}
