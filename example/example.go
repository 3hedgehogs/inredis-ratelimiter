package main

import (
	"fmt"
	"time"

	"github.com/3hedgehogs/inredis-ratelimiter"
	"github.com/garyburd/redigo/redis"
)

var maxConnections = 10
var redisServer = "localhost:6379"
var redisAuth = ""
var redisDB = 0

func NewPool(server string, password string, db int) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     maxConnections,
		IdleTimeout: 20 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				fmt.Println("Could not connect to redis server:", server, err.Error())
				return nil, err
			}
			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					fmt.Printf("Could not connect to redis server: %s using AUTH command, error: %s\n", server, err.Error())
					return nil, err
				}
			}
			if _, err := c.Do("select", db); err != nil {
				c.Close()
				fmt.Printf("Could not select DB: %d, redis server: %s, error: %s\n", db, server, err.Error())
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

func main() {

	var p = NewPool(redisServer, redisAuth, redisDB)
	var c = p.Get()
	defer c.Close()
	defer p.Close()

	l, _ := ratelimiter.New("testkey", 10, 2, "", p, true)
	l.Reset()

	_ = l.TryAcquire()
	_ = l.CheckLimit()
	fmt.Printf("Current usage: %d\n", l.Usage)

	//for i := 0; i < 15; i = i + 1 {
	//	_ = l.TryAcquire()
	//	time.Sleep(100 * time.Millisecond)
	//}

	l.Reset()
}
