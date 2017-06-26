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


func benchmark2(wg *sync.WaitGroup, p *redis.Pool, id int, N int) {
	defer wg.Done()
	lbt, err := New("benchmark", 10, 2, "", p, false)
	if err != nil {
		fmt.Printf("Cannot create limiter: %s\n", err)
		return
	}
	//lbt.Clean()
	//lbt.UpdatePeriod(3)
	//lbt.StopBurst = true
	for i := 0; i < N; i = i + 1 {
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

func BenchmarkRateLimiter(b *testing.B) {

	var p = NewPool(redisServer, redisAuth, redisDB)
	var c = p.Get()
	defer c.Close()
	defer p.Close()
	pong, err := c.Do("PING")
	assert.Nil(b, err)
	assert.Equal(b, "PONG", pong)

	var wg sync.WaitGroup
	var parallels = 10
	var N = 100000
	b.Logf("Benchmark with %d goroutines and loop size = %d\n",parallels, N)
	wg.Add(parallels)
	for i := 0; i < parallels; i = i + 1 {
		go benchmark2(&wg, p, i, N)
		//time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()

}
