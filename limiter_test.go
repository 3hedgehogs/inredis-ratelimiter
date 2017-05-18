package ratelimiter

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
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

func benchmark1(wg *sync.WaitGroup, p *redis.Pool, id int) {
	defer wg.Done()
	lbt, err := New("benchmark", 10, 2, "", p, false)
	if err != nil {
		fmt.Printf("Cannot create limiter: %s\n", err)
		return
	}
	//lbt.Clean()
	//lbt.UpdatePeriod(3)
	//lbt.StopBurst = true
	for i := 0; i < 10000; i = i + 1 {
		r := lbt.TryAcquire()
		if r {
			t := time.Now().UnixNano() / 1000
			ts := strconv.FormatInt(t, 10)
			fmt.Printf("ID%02d: *OK: %s, %d\n", id, ts, lbt.Usage)
		} else {
			//fmt.Printf("ID%02d: NOK: %d\n", id, lbt.Usage)
		}
		time.Sleep(time.Duration(rand.Intn(100)) * time.Microsecond)
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

	//	var wg sync.WaitGroup
	//	var parallels = 10
	//	wg.Add(parallels)
	//	for i := 0; i < parallels; i = i + 1 {
	//		go benchmark1(&wg, p, i)
	//		//time.Sleep(10 * time.Millisecond)
	//	}
	//	wg.Wait()

}
