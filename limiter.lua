-- Lua script exstracted from limiter.go
-- KEYS[1] - zSet name
-- ARGV[n == 5] - Period (seconds), counter limit, TTL of zSet (seconds), mindiff, to reserv a Slot or not

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
      return redis.error_reply("to fast requests")
   end
end

return usage
