// 所有的现代 Go 数据库驱动（包括刚才的 GORM 和现在的 Redis）都强制要求传入 Context，用来实现高并发下的超时控制和请求取消。

// 手写一段经典逻辑：手机验证码存取与过期销毁

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// 声明一个全局的 Context，在实际 Web 开发中，这个 ctx 会从 Gin 的 c.Request.Context() 里拿
var ctx = context.Background()

func main() {
	// 1. 初始化 Redis 客户端连接池
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // 本地启动的 Redis 地址
		Password: "",               // 没有设置密码
		DB:       0,                // Redis 默认自带 16 个数据库（0-15），默认用 0 号
		PoolSize: 10,               // 细节：设置连接池大小，避免高并发时频繁创建连接
	})
	defer rdb.Close() // 程序结束时关闭连接池

	// 验证是否连通
	// ping pong 是 Redis 的一个命令，客户端发送 PING 命令后，Redis 会回复 PONG，表示连接正常
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Println("连接 Redis 失败:", err)
		return
	}
	fmt.Println("连接 Redis 成功:", pong)

	// 2. 存储验证码，并设置 5 秒后自动过期 (TTL)
	key := "verify_code:13800138000" // 大厂规范：Key的命名必须带有冒号分割的命名空间
	val := "8888"
	// Set(ctx, key, value, expiration)
	err = rdb.Set(ctx, key, val, 5*time.Second).Err() // 设置键值对，并指定过期时间
	if err != nil {
		panic(err)
	}
	fmt.Println("\n验证码 [8888] 已存入 Redis，倒计时 5 秒后自动销毁！")

	// 3. 立即读取验证码
	getVal, err := rdb.Get(ctx, key).Result() // 获取键对应的值
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("第一次读取：成功拿到验证码 -> %s\n", getVal)
	}

	// 4. 体验内存级的高速自动清理
	fmt.Println("\n⏳ 等待 6 秒，验证码应该已经过期了...")
	time.Sleep(6 * time.Second) // 等待 6 秒，超过设置的过期时间

	// 再次尝试获取
	_, err = rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		fmt.Printf("第二次读取：验证码已过期，无法获取！\n")
	} else if err != nil {
		panic(err)
	}
}
