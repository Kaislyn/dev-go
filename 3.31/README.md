# JWT & Redis 结合场景
## 前言
JWT 本身可以不依赖 Redis（无状态，服务端只验证签名）。
但真实项目里 JWT + Redis 几乎总是成对出现，目的就是为了解决纯 JWT 的几个致命痛点。

## 一、纯 JWT 的问题（不用 Redis 时）

假设你只发 JWT，不存 Redis：

1. **无法主动登出**  
   用户点击“退出登录”，你只能删掉客户端 token，但 token 只要没过期，服务端依然认它。如果有人窃取了 token，你毫无办法。

2. **无法强制踢人下线**  
   管理员想封禁某个用户，让他立刻不能访问。纯 JWT 做不到，只能等 token 自然过期。

3. **无法控制同一账号多设备登录**  
   比如你想限制一个账号最多在 2 台设备登录。纯 JWT 不知道哪些 token 还在用。

4. **修改密码后，旧 token 依然有效**  
   用户改了密码，但用旧 token 还能调接口，这显然不合理。

> 一句话：**纯 JWT 是“签发后即失控”的**，服务端没有对已签发 token 的撤销能力。

## 二、Redis 如何解决？—— 白名单 / 黑名单机制

常用的**两种模式**，可以按需选择：

### 模式 1：Token 黑名单（最常用）

- **正常登录**：生成 JWT，**不存 Redis**（保持无状态）
- **需要撤销时**：把该 token 的唯一标识（比如 `jti` 或用户ID+过期时间）存入 Redis 黑名单，设置过期时间 = token 剩余有效期
- **每次请求**：先验证 JWT 签名，再检查 Redis 黑名单，如果在里面就拒绝

**适用场景**：大部分 token 不会被撤销，只有少数需要拉黑（登出、改密、踢人）

### 模式 2：Token 白名单（更严格）

- **登录成功**：生成 JWT，把 token 存入 Redis（key = `token:用户ID:设备ID`，value = token）
- **每次请求**：验证 JWT 签名后，还要从 Redis 取出来比对是否一致
- **登出/踢人**：直接删 Redis 中的 token

**适用场景**：严格控制并发登录、服务端希望完全掌控每个有效 token

> 实际项目中 **黑名单模式** 更常见，性能更好，因为绝大多数请求不用查 Redis。

## 三、具体能解决什么问题？

下面这是**真实开发每天都会遇到的需求**：

| 需求                             | 纯 JWT                 | JWT + Redis                     |
| -------------------------------- | ---------------------- | ------------------------------- |
| 用户登出                         | ❌ 只能客户端删         | ✅ 加黑名单，token 立即失效      |
| 修改密码后踢掉旧设备             | ❌ 旧 token 仍可用      | ✅ 将该用户所有 token 加入黑名单 |
| 管理员封禁用户                   | ❌ 无法即时生效         | ✅ 黑名单 + 用户状态校验         |
| 限制单设备登录（后登录踢前一个） | ❌ 做不到               | ✅ 白名单模式，存最新 token      |
| 刷新 token 时防止并发重复刷新    | ❌ 可能多个请求同时刷新 | ✅ 用 Redis 分布式锁             |

**一句话总结关联**：  
> Redis 给了 JWT **可控的生命周期管理能力**，让无状态的 token 变得“可撤销、可监控、可限制”。

## 四、Go + Gin + JWT + Redis 典型代码结构（黑名单模式）

给你一个极简的集成思路（不是完整代码，但能帮你理解流程）：

### 1. 登录生成 JWT
```go
func Login(c *gin.Context) {
    // 验证用户名密码...
    token := jwt.GenerateToken(userID) // 标准 JWT，包含 exp, jti
    c.JSON(200, gin.H{"token": token})
}
```

### 2. 登出 → 加入黑名单
```go
func Logout(c *gin.Context) {
    token := c.GetHeader("Authorization")
    // 解析 token 拿到 jti 和 剩余过期时间（秒）
    claims := jwt.Parse(token)
    ttl := claims.ExpiresAt - time.Now().Unix()
    // 存入 Redis，key=blacklist:jti，值随意，过期时间=ttl
    rdb.SetEx(ctx, "blacklist:"+claims.Id, "1", time.Duration(ttl)*time.Second)
    c.JSON(200, "登出成功")
}
```

### 3. 认证中间件（验证 JWT + 查黑名单）
```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := extractToken(c)
        claims, err := jwt.Verify(token) // 验证签名、过期
        if err != nil {
            c.AbortWithStatus(401)
            return
        }
        // 检查 Redis 黑名单
        _, err = rdb.Get(ctx, "blacklist:"+claims.Id).Result()
        if err == nil {
            c.AbortWithStatus(401) // token 已被拉黑
            return
        }
        c.Set("userID", claims.UserID)
        c.Next()
    }
}
```

### 4. 修改密码时，拉黑该用户所有 token（需要提前记录用户关联的 jti）
```go
func ChangePassword(c *gin.Context) {
    userID := c.GetInt64("userID")
    // 修改密码...
    // 将该用户所有 jti 加入黑名单（你需要额外存一个 user:jti 的集合）
    jtis := rdb.SMembers(ctx, fmt.Sprintf("user:%d:jtis", userID)).Val()
    for _, jti := range jtis {
        // 获取该 jti 对应的剩余时间（可以从原 token 结构来，或存一个临时值）
        rdb.SetEx(ctx, "blacklist:"+jti, "1", remainingTime)
    }
    // 清空 user:jtis 集合
}
```

> 实际项目中为了简化，也可以直接**暴力一点**：修改密码/封禁用户时，把 `user:block:用户ID` 存入 Redis，中间件里额外校验这个标志。但这会让所有请求都多查一次 Redis，不过通常可以接受。

## 五、你接下来的学习建议

你现在学完 Gin、GORM、Redis、JWT，但不会组合，这是**正常阶段**。建议你做一个小项目来打通：

**推荐项目：用户认证系统 + 文章管理（或简单的 todo）**

要求自己实现：
1. 注册/登录 → 返回 JWT
2. 登出 → 将 token 加入 Redis 黑名单
3. 修改密码 → 清空该用户所有 token（黑名单批量加入）
4. 刷新 token 接口（用 Redis 存 refresh token）
5. 限制单设备登录：用 Redis 记录最新 token，旧 token 请求时比对失效

做完这个，你就会彻底明白 JWT 和 Redis 为什么是“黄金搭档”。

---

如果你愿意，我可以给你一个 **最小可运行的 Gin + JWT + Redis 黑名单示例项目** 的完整代码结构（main.go + 中间件 + handler），这样你跑起来就能看到效果。