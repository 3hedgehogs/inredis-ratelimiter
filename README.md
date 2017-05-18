
inredis-ratelimiter
===================

Simple go ratelimiter library with redis backend.
Supports simple burst prevention algorythm.
Requires Redis version >= 3.2.0

## Installation

```sh
go get github.com/3hedgehogs/inredis-ratelimiter
```

## Example

```go
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

    // New RateLimiter: it allows max 10 events per 2 seconds
    l, err := ratelimiter.New("testkey", 10, 2, "", p)
    l.Reset()

    _ = l.TryAcquire()
    _ = l.CheckLimit()
    fmt.Printf("Current usage: %d\n", l.Usage);

    l.Reset()
}

```

## Idea
The original idea is from here:
https://engineering.classdojo.com/blog/2015/02/06/rolling-rate-limiter/

But moved to use Redis lua scripting due to possible "time" problems.
If clients are not synced with ntp servers limiting will not work correctly
without it. But I still hope the code is small and easy.
It uses sliding window that can be used in situations that reqire ratelimit changes without 
recreating of the current ratelimiter and loosing data in Redis.


> sp
