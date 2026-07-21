package redis

import "github.com/redis/go-redis/v9"

// slidingWindowLuaScript adalah script Lua atomic untuk algoritma Sliding Window Rate Limiter.
// KEYS[1]: Key unik rate limit (misal "distlimit:user_123")
// ARGV[1]: Timestamp saat ini (nanosecond)
// ARGV[2]: Batas window (nanosecond)
// ARGV[3]: Limit maksimum request
// ARGV[4]: Unique Member ID (now_nano + random)
// ARGV[5]: TTL Key dalam detik
const slidingWindowLuaScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member_id = ARGV[4]
local ttl = tonumber(ARGV[5])

local clear_before = now - window

-- 1. Hapus entri lama yang sudah di luar window waktu
redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

-- 2. Hitung jumlah request tersisa di dalam window saat ini
local current_requests = redis.call('ZCARD', key)

-- 3. Evaluasi Batas Limit
if current_requests < limit then
    -- Tambahkan request baru ke Sorted Set
    redis.call('ZADD', key, now, member_id)
    -- Perbarui TTL agar key otomatis terhapus jika tidak aktif
    redis.call('EXPIRE', key, ttl)
    
    local remaining = limit - current_requests - 1
    return {1, remaining, window} -- [Allowed: 1, Remaining, ResetInNs]
else
    -- Ambil item tertua untuk menghitung persis sisa waktu reset
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local reset_in = window
    if #oldest > 0 then
        reset_in = (tonumber(oldest[2]) + window) - now
        if reset_in < 0 then reset_in = 0 end
    end
    
    return {0, 0, reset_in} -- [Allowed: 0, Remaining: 0, ResetInNs]
end
`

var slidingScript = redis.NewScript(slidingWindowLuaScript)
