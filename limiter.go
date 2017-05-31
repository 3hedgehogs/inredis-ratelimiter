// Package ratelimiter provides a fast and simlpe library
// to set and check limits using redis backend.
package ratelimiter

import (
	"errors"
	"github.com/garyburd/redigo/redis"
	"log"
	"time"
)

const maxexpirationtime = 31536000
const maxperiod = 31536000

const script = `
redis.replicate_commands()
redis.set_repl(redis.REPL_NONE);

local period     = tonumber(ARGV[1])
local limit      = tonumber(ARGV[2])
local expiretime = tonumber(ARGV[3])
local mindiff    = tonumber(ARGV[4])
local reserv     = tonumber(ARGV[5])

local time_string = ""
local redistime = redis.call("TIME")
for _,tp in ipairs(redistime) do
   while string.len(tp) < 6 do
       tp = "0" .. tp
   end
   time_string = time_string .. tp
end

local ts = tonumber(time_string)
local startwindow = ts - period * 1000000
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", startwindow)

local usage = tonumber(redis.call("ZCOUNT", KEYS[1], 1, ts))

if usage >= limit then
   return -usage
end

if reserv == 1 then
   local min = ts - mindiff
   local max = ts
   local n = tonumber(redis.call("ZCOUNT", KEYS[1], min, max))
   if n == 0 then
      redis.call("ZADD", KEYS[1], ts, ts)
      redis.call("EXPIRE", KEYS[1], expiretime)
      usage = tonumber(redis.call("ZCOUNT", KEYS[1], 0, ts))
      if usage > limit then
         return -usage
      end
   else
      return redis.error_reply("too fast requests")
   end
end

return usage
`

// Limiter structure
type Limiter struct {
	perPeriod  int64         // Time window size [now - PerPeriod ,now], seconds
	expKey     int           // Time of life for Zset (key) in Redis, seconds (default is perPeriod * 2)
	key        string        // Name of the Limiter
	limit      int           // Maximal Rate Usage/Limit
	Usage      int           // Last seen Counter/Usage
	redisKey   string        // Name of the key in Redis (sorted set)
	redisPool  *redis.Pool   // Redis Pool (github.com/garyburd/redigo/redis)
	StopBurst  bool          // Switch of the burst part (default is off/false)
	burstQuant int64         // Minimal time distant between 2 "events" in Zset, used in the burst control
	lastID     int64         // Time of the last successfully insertion to the Zset (localtime, microseconds)
	script     *redis.Script // Redigo script
	debug      bool          // Print debug information
}

// New Limiter creation
// 'rediskey' can be empty, 'debug' parameter is optional
func New(key string, limit int, period int, rediskey string, pool *redis.Pool, debug ...bool) (*Limiter, error) {

	debugFlag := false
	if len(debug) > 0 {
		debugFlag = debug[0]
	}

	if key == "" {
		return nil, errors.New("ratelimiter: Key name is empty")
	}
	if period < 1 || period > maxperiod {
		return nil, errors.New("ratelimiter: invalid 'perPeriod' value for the key: " + key)
	}
	if limit < 1 || limit > 1e6 {
		return nil, errors.New("ratelimiter: invalid 'limit' value for the key: " + key)
	}
	if rediskey == "" {
		rediskey = key + "-ratelimit:rk"
	}
	expKey := period * 2

	// Load script
	c := pool.Get()
	defer c.Close()
	s := redis.NewScript(1, script)
	err := s.Load(c)
	if err != nil {
		return nil, errors.New("ratelimiter: could not store the script in Redis: " + err.Error())
	}

	t := time.Now().UnixNano() / 1000
	burstQuant, err := makeQuant(int64(period), int64(limit))
	if err != nil {
		return nil, err
	}

	return &Limiter{
			perPeriod:  int64(period),
			expKey:     expKey,
			key:        key,
			limit:      limit,
			Usage:      0,
			redisKey:   rediskey,
			redisPool:  pool,
			StopBurst:  false,
			burstQuant: burstQuant,
			lastID:     t,
			script:     s,
			debug:      debugFlag},
		nil
}

// CheckLimit only checks if the limit was reached or not
// without slot reservation
func (l *Limiter) CheckLimit() bool {
	return l.doAcquire(false)
}

// TryAcquire is checking the existing limit and reserves one slot if it possible.
// return false in cases the limit is reached or some internal errors.
func (l *Limiter) TryAcquire() bool {
	return l.doAcquire(true)
}

// do the job.
func (l *Limiter) doAcquire(reserv bool) bool {

	c := l.redisPool.Get()
	defer c.Close()

	t := time.Now().UnixNano() / 1000
	localStartWindow := t - l.perPeriod*1e6

	// "Local" anti.burst
	last := l.lastID
	fastgo := l.burstQuant / 1000
	if fastgo <= 0 {
		fastgo = 10
	}
	if last < (localStartWindow - 1) {
		last = localStartWindow + 1
	}
	if (t-last) < fastgo && reserv {
		if l.debug {
			log.Printf("ratelimiter: (local) too fast, at:  %d\n", t)
		}
		return false
	}

	if l.StopBurst && reserv {
		// "Local" anti.burst
		if (t - last) < l.burstQuant {
			if l.debug {
				log.Printf("ratelimiter: (local) too fast, at: %d\n", t)
			}
			return false
		}
	}

	var rUsage int
	var err error
	if !l.StopBurst {
		rUsage, err = redis.Int(l.script.Do(c, l.redisKey, l.perPeriod, l.limit, l.expKey, fastgo, reserv))
	} else {
		rUsage, err = redis.Int(l.script.Do(c, l.redisKey, l.perPeriod, l.limit, l.expKey, l.burstQuant, reserv))
	}
	if err != nil {
		if l.debug {
			log.Printf("ratelimiter: exec SCRIPT error: %s.\n", err)
		}
		return false
	}
	if reserv {
		l.lastID = t
	}
	if rUsage < 0 {
		l.Usage = -rUsage
		if l.debug {
			log.Printf("ratelimiter: LIMIT is reached\n")
		}
		if reserv {
			return false
		}
	} else {
		l.Usage = rUsage
		if l.debug && rUsage >= l.limit {
			log.Printf("ratelimiter: LIMIT is reached\n")
		}
	}
	if l.debug {
		log.Printf("ratelimiter: current usage: %d\n", l.Usage)
	}
	return true
}

// NoBurst sets the flag to use anti.burst algorythm
func (l *Limiter) NoBurst() {
	l.StopBurst = true
}

// AllowBurst sets the flag to swith off anti.burst algorythm
func (l *Limiter) AllowBurst() {
	l.StopBurst = false
}

// UpdatePeriod updates/sets used perPeriod value to the new one.
func (l *Limiter) UpdatePeriod(newPeriod int) error {
	if newPeriod < 1 || newPeriod > maxperiod {
		return errors.New("ratelimiter: invalid 'perPeriod' value for the key: " + l.key)
	}

	burstQuant, err := makeQuant(l.perPeriod, int64(l.limit))
	if err != nil {
		return err
	}

	l.burstQuant = burstQuant
	l.perPeriod = int64(newPeriod)
	l.expKey = newPeriod * 2

	c := l.redisPool.Get()
	defer c.Close()

	_, err = redis.Int(c.Do("EXPIRE", l.redisKey, l.expKey))
	if err != nil {
		return err
	}
	return nil
}

// Reset cleans in Redis used data and resets Limiter
func (l *Limiter) Reset() error {
	c := l.redisPool.Get()
	defer c.Close()

	_, err := redis.Int(c.Do("DEL", l.redisKey))
	if err != nil {
		return err
	}

	l.lastID = time.Now().UnixNano()/1000 - int64(l.perPeriod)*1e6
	l.Usage = 0

	return nil
}

func makeQuant(period int64, limit int64) (int64, error) {
	burstQuant := period * 850000 / limit
	if burstQuant <= 100 {
		return 0, errors.New("ratelimiter: too high ratelimit, cannot handle it")
	}
	return burstQuant, nil
}
