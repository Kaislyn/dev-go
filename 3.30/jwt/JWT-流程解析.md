# JWT （令牌签发 + 令牌解析校验）
适用场景：Go 语言基于 `golang-jwt/jwt/v5` 库实现 JWT 身份认证核心逻辑
专业术语规范：**令牌(Token)**、**载荷(Claims)**、**签名算法**、**数字签名**、**密钥(SecretKey)**、**算法校验**、**令牌验证**

---

## 一、GenerateToken 令牌签发函数
### 1. 函数定义
```go
func GenerateToken(userID int64, userName string, role string) (string, error)
```

### 2. 核心作用
接收用户业务身份信息，构建**自定义载荷(CustomClaims)**，通过 **HMAC-SHA256 对称签名算法**（注意：JWT 的作用是防篡改，而非加密防窥探，任何人都可以解码看到载荷内容）生成携带数字签名的 JWT 令牌，完成身份凭证的签发。

### 3. 入参/出参（专业定义）
| 类型 | 参数名   | 数据类型 | 含义                      |
| ---- | -------- | -------- | ------------------------- |
| 入参 | userID   | int64    | 业务用户唯一标识          |
| 入参 | userName | string   | 用户名                    |
| 入参 | role     | string   | 用户角色权限              |
| 出参 | string   | -        | 签发完成的完整 JWT 字符串 |
| 出参 | error    | -        | 签发过程中的异常信息      |

### 4. 代码逐行专业解析
```go
// 1. 初始化自定义载荷结构体
// 合并：业务自定义字段 + JWT 标准声明字段(RegisteredClaims)
claims := CustomClaims{
    UserID:   userID,
    UserName: userName,
    Role:     role,
    // 注入JWT标准声明：过期时间、签发时间、签发方
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 令牌过期时间（强制失效）
        IssuedAt:  jwt.NewNumericDate(time.Now()),                     // 令牌签发时间
        Issuer:    "my_app",                                           // 令牌签发方标识
    },
}

// 2. 创建令牌对象
// 指定签名算法：HS256(HMAC-SHA256)；绑定自定义载荷
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

// 3. 生成数字签名 + 拼接完整JWT
// 用服务器密钥对 Header+Payload 进行签名，返回最终JWT字符串
return token.SignedString(jwtKey)
```

### 5. 核心原理
1. **载荷构建**：将业务数据与 JWT 标准安全字段（过期、签发时间）合并为完整载荷；
2. **算法绑定**：指定 `HS256` 作为签名算法，写入 JWT 头部；
3. **数字签名**：使用服务器密钥对 `Header.Payload` 进行哈希加密，生成不可伪造的签名；
4. **令牌拼接**：`Header.Payload.Signature` 组合为最终 JWT 令牌。

### 6. 关键注意事项
1. 密钥为**对称密钥**，服务端独占，严禁硬编码、泄露；
2. 必须设置 `ExpiresAt`，实现令牌时效性控制；
3. 载荷仅存储非敏感业务数据，不支持加密存储。

### 关于 JWT 令牌是否会重复的说明
#### Q：会出现两个相同的令牌吗？
没有任何两个用户的 JWT 是相同的，甚至同一个用户连续登录两次，生成的令牌也不一样。

#### 核心原因
令牌由三部分决定：
- Header（算法）：固定
- Payload（载荷）：动态、唯一、用户专属
- Signature（签名）：基于前两部分 + 密钥生成，载荷变化则签名必然变化

在当前代码中，Payload 内部均为动态唯一值：

```go
claims := CustomClaims{
    UserID:   userID,    // 每个用户ID唯一（1001、1002...）
    UserName: userName,  // 用户名唯一
    Role:     role,      // 角色不同
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: ...,  // 过期时间：每次登录都不同
        IssuedAt:  ...,  // 签发时间：精确到毫秒，绝对唯一
    },
}
```

---

## 二、ParseToken 令牌解析校验函数
### 1. 函数定义
```go
func ParseToken(tokenString string) (*CustomClaims, error)
```

### 2. 核心作用
对客户端传入的 JWT 字符串进行**解析、拆分、签名验证、算法校验、合法性校验**，验证通过后提取载荷数据；验证失败则返回异常，拒绝非法请求。

### 3. 入参/出参（专业定义）
| 类型 | 参数名        | 数据类型 | 含义                        |
| ---- | ------------- | -------- | --------------------------- |
| 入参 | tokenString   | string   | 客户端携带的 JWT 令牌字符串 |
| 出参 | *CustomClaims | -        | 解析成功的自定义载荷指针    |
| 出参 | error         | -        | 解析/校验失败的异常信息     |

### 4. 代码逐行专业解析
```go
// 1. 核心解析函数：解析JWT，自动完成签名验证
// 入参：令牌字符串、自定义载荷结构体、密钥回调函数
token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
    // 2. 算法合法性校验（防御算法降级攻击）
    // 验证令牌头部声明的算法，是否为服务端指定的HMAC系列算法
    if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
        return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
    }
    // 3. 返回服务端密钥，供库自动执行【重新签名+签名比对】
    return jwtKey, nil
})

// 4. 基础解析异常：格式错误、签名非法、过期等
if err != nil {
    return nil, err
}

// 5. 最终合法性校验：断言载荷类型 + 验证令牌有效性
// token.Valid = 签名一致 + 未过期 + 格式合法
if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
    return claims, nil
} else {
    return nil, fmt.Errorf("invalid token")
}
```

### 5. 核心原理（JWT 安全校验核心）
1. **JWT 拆分**：库自动将令牌拆分为 `Header`/`Payload`/`Signature` 三部分；
2. **算法校验**：校验客户端传入的算法是否与服务端一致，防止恶意篡改算法；
3. **重算签名**：使用服务端密钥 + 令牌头部算法，**重新对 Header.Payload 生成签名**；
4. **签名比对**：将新生成的签名与令牌原签名对比，验证令牌是否被篡改/伪造；
5. **合法性校验**：验证令牌是否过期、格式是否完整，最终返回载荷。

### 6. 关键注意事项
1. 算法校验为**安全刚需**，可防御「算法 none 攻击」「非预期算法攻击」；
2. 签名比对由库底层自动实现，无需手动编码；
3. `token.Valid` 是令牌最终合法的唯一判断标准；
4. 校验失败直接返回错误，拒绝后续业务处理。

---

## 三、双函数核心关联总结
1. **签发**：`GenerateToken` → 生成**带签名的合法 JWT**（服务端颁发身份凭证）；
2. **解析**：`ParseToken` → 验证**签名有效性** + 提取身份数据（服务端校验身份）；
3. 安全核心：**对称密钥 + 签名算法** 实现令牌防篡改、防伪；
4. 标准规范：基于 JWT RFC7519 标准，适配分布式/微服务无状态认证。