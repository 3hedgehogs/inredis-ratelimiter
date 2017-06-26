package ratelimiter

import (
	"fmt"
	"time"

	"testing"

	"github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/assert"
)

var maxConnections = 1000
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

func TestRateLimiter(t *testing.T) {

	var p = NewPool(redisServer, redisAuth, redisDB)
	var c = p.Get()
	defer c.Close()
	defer p.Close()
	pong, err := c.Do("PING")
	assert.Nil(t, err)
	assert.Equal(t, "PONG", pong)

	l, err := New("key", 10, 2, "", p, true)

	assert.Nil(t, err)
	assert.NotNil(t, l)
	assert.Equal(t, 0, l.Usage)

	err = l.Reset()
	assert.Nil(t, err)

	r := l.TryAcquire()
	assert.Equal(t, true, r)
	assert.Equal(t, 1, l.Usage)
	r = l.CheckLimit()
	assert.Equal(t, true, r)
	assert.Equal(t, 1, l.Usage)

	a := l.UpdatePeriod(-1)
	assert.NotNil(t, a)
	a = l.UpdatePeriod(2)
	assert.Nil(t, a)

	for i := 0; i < 20; i = i + 1 {
		time.Sleep(time.Duration(200) * time.Millisecond)
		_ = l.TryAcquire()
	}

	r = l.TryAcquire()
	assert.Equal(t, false, r)
	assert.Equal(t, 10, l.Usage)

	r = l.TryAcquire()
	assert.Equal(t, false, r)
	assert.Equal(t, 10, l.Usage)

	time.Sleep(2 * time.Second)
	r = l.TryAcquire()
	assert.Equal(t, true, r)
	assert.Equal(t, 1, l.Usage)
}
