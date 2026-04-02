## 🗺️ 整体课程大纲（暂定 6 课时）

| 课时  | 主题                                       | 核心知识点                                        |
| ----- | ------------------------------------------ | ------------------------------------------------- |
| 第1课 | Redis 入门 & 基础数据结构（String、Hash）  | 安装确认、redis-cli、set/get、hset/hget、过期时间 |
| 第2课 | 进阶数据结构（List、Set、Sorted Set）      | 队列、去重集合、排行榜原理                        |
| 第3课 | Redis 在 Go 中的基本操作（go-redis）       | 连接池、常见命令的 Go 实现、结构体映射            |
| 第4课 | 典型场景实战（缓存 + 防击穿）              | 缓存用户信息、空值缓存、singleflight 思路         |
| 第5课 | 分布式锁 & 乐观锁                          | SETNX、Lua 脚本、redlock 简析                     |
| 第6课 | 综合 Demo：一个高频访问计数器 + 排行榜 API | Gin + Redis + 原子操作 + 定时持久化               |

每节课后我会给你一个「检验任务」，你可以把代码或疑问发给我，我会逐行点评。

---

## 🔧 课前准备：确认 Redis 可用

请打开终端（或 cmd），执行：

```bash
redis-server   # 启动服务端（保持窗口运行）
```

**新开一个终端窗口**，执行：

```bash
redis-cli ping
```

如果返回 `PONG`，说明服务正常。  
如果提示 `redis-cli 不是内部命令`，说明你只是下载了但没有加到 PATH，可以进入安装目录运行 `redis-cli.exe`（Windows）或使用 `./src/redis-cli`（Linux/Mac）。

> 确保 Redis 版本 ≥ 6.0（推荐 7.x）

---

## 📘 第 1 课：Redis 是什么 & 最常用的两种数据结构

### 1.1 你为什么需要学 Redis？

大厂后端几乎必考/必用 Redis，因为：

- **快**：纯内存操作，每秒可处理 10w+ 次读写
- **数据结构丰富**：不只是 key-value，还有列表、集合、有序集合、位图、HyperLogLog 等
- **原子操作**：高并发下安全地增减、加锁
- **持久化 + 高可用**：可做缓存也可做数据库（配合集群）

学习时不要死记命令，要理解 **「用 Redis 解决什么场景问题」**。

### 1.2 核心概念：Key-Value 与数据类型

- Redis 是一个 **键值对数据库**，所有数据都通过 **key** 访问。
- **value** 可以是不同类型：String、Hash、List、Set、ZSet 等。
- 每个 key 可以设置 **过期时间**（TTL），到期自动删除。

### 1.3 第一种数据类型：String（字符串）

**命令例子**（在 `redis-cli` 中执行）：

```bash
# 设置键值
SET user:1000:name "zhangwei"
# 获取值
GET user:1000:name
# 设置过期时间（秒）
SET session:abc123 "data" EX 30
# 自增/自减（用于计数器）
SET counter 0
INCR counter
INCRBY counter 10
```

**使用场景**：
- 缓存对象（序列化后存储，如 JSON）
- 计数器（阅读量、点赞数）
- 分布式 ID 生成（`INCR` 原子性）

### 1.4 第二种数据类型：Hash（哈希）

Hash 适合存储 **一个对象的多个字段**，比如用户信息。

```bash
# 设置单个字段
HSET user:1001 name "lixiaoming" age 25
# 获取单个字段
HGET user:1001 name
# 获取所有字段
HGETALL user:1001
# 增加字段数值
HINCRBY user:1001 age 1
```

**对比 String 存 JSON**：
- 用 Hash 可以单独修改一个字段（不用整体覆盖）
- 用 String + JSON 适合读取整个对象，但不适合部分更新

### 1.5 重要概念：Key 的设计规范（大厂实践）

- 用冒号 `:` 分割层级，如 `service:module:entity:id`
- 不要包含特殊字符（空格、换行）
- 命名清晰，一眼知道用途：`cache:user:profile:10086`
- **长度适中**，太长浪费内存（Redis 做 hash 重哈希时会受影响）

---

## 💻 第 1 课 Go 实践：使用 go-redis 操作 String 和 Hash

### 1.5 安装库

推荐官方主流库：`go-redis/redis`（比 `garyburd/redigo` 更易用）

```bash
go get github.com/redis/go-redis/v9
```

### 1.6 建立连接（带连接池）

创建 `main.go`：

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

func initRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr:         "localhost:6379", // Redis 地址
		Password:     "",               // 没有密码就留空
		DB:           0,                // 使用默认 DB 0
		PoolSize:     10,               // 连接池大小
		MinIdleConns: 5,                // 最小空闲连接
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("连接 Redis 失败: %v", err)
	}
	fmt.Println("✅ Redis 连接成功")
}

func main() {
	initRedis()
	ctx := context.Background()

	// ========== String 操作示例 ==========
	// 设置 key，过期时间 10 秒
	err := rdb.Set(ctx, "go:user:1001:name", "zhangwei", 10*time.Second).Err()
	if err != nil {
		panic(err)
	}

	// 获取值
	val, err := rdb.Get(ctx, "go:user:1001:name").Result()
	if err == redis.Nil {
		fmt.Println("key 不存在或已过期")
	} else if err != nil {
		panic(err)
	} else {
		fmt.Printf("GET 结果: %s\n", val)
	}

	// 原子自增（模拟点赞数）
	rdb.Set(ctx, "post:10086:likes", 0, 0)
	rdb.Incr(ctx, "post:10086:likes")
	newLikes, _ := rdb.Get(ctx, "post:10086:likes").Int()
	fmt.Printf("点赞数: %d\n", newLikes)

	// ========== Hash 操作示例 ==========
	// 存储用户信息（三个字段）
	userInfo := map[string]interface{}{
		"name": "lixiaoming",
		"age":  25,
		"city": "Beijing",
	}
	err = rdb.HSet(ctx, "go:user:1002", userInfo).Err()
	if err != nil {
		panic(err)
	}

	// 获取单个字段
	age, err := rdb.HGet(ctx, "go:user:1002", "age").Result()
	fmt.Printf("年龄: %s\n", age)

	// 获取全部字段
	allFields, err := rdb.HGetAll(ctx, "go:user:1002").Result()
	fmt.Printf("完整用户信息: %v\n", allFields)

	// 给年龄 +1
	rdb.HIncrBy(ctx, "go:user:1002", "age", 1)
	newAge, _ := rdb.HGet(ctx, "go:user:1002", "age").Int()
	fmt.Printf("增加后年龄: %d\n", newAge)
}
```

运行：

```bash
go run main.go
```

你应该看到类似的输出（过期后 GET 可能返回不存在）：

```
✅ Redis 连接成功
GET 结果: zhangwei
点赞数: 1
年龄: 25
完整用户信息: map[name:lixiaoming age:25 city:Beijing]
增加后年龄: 26
```

---

## 📝 第 1 课课后任务（务必完成）

1. **手动命令练习**：打开 `redis-cli`，创建 3 个不同的 key（一个 String，一个 Hash，一个带 EX 过期时间的 key），然后用 `TTL key` 查看剩余时间。
2. **Go 代码修改**：基于上面的例子，写一个函数 `CacheUser(id int, name string, age int)`，用 Hash 存储用户信息，并且设置该 key 在 60 秒后自动过期（注意：Hash 本身不能整体过期，但可以用 `Expire` 命令对 key 设置过期）。
3. **思考题**：为什么 Redis 的 String 类型也适合做计数器（INCR）？如果并发 1000 个请求同时 `INCR`，会不会出现丢失计数的情况？

---

你可以把任务 2 的代码发给我，我会帮你 review。如果对某个概念不清楚（比如 `context.Context` 是什么），直接问我，我们慢下来讲透。下一课我们会学习 List 和 Set 并用它们实现「消息队列」和「标签系统」。

准备好了就继续，我们按你的节奏来。