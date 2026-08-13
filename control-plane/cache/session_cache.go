package cache

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/redis/go-redis/v9"
)

const (
	keyPrefixSession = "session"
	keyPrefixUser    = "user"
)

func SessionKey(sessionID, userID string) string {
	return keyPrefixSession + ":" + sessionID + "," + keyPrefixUser + ":" + userID
}

type SessionCache struct {
	client *redis.Client
}

func NewSessionCache(client *redis.Client) *SessionCache {
	return &SessionCache{client: client}
}

func (c *SessionCache) Set(ctx context.Context, key string, ttl time.Duration) error {
	val := strconv.FormatInt(time.Now().UnixNano(), 10)
	return c.client.Set(ctx, key, val, ttl).Err()
}

func (c *SessionCache) Get(ctx context.Context, key string) error {
	return c.client.Get(ctx, key).Err()
}

func (c *SessionCache) Delete(ctx context.Context, keys []string) error {
	return c.client.Del(ctx, keys...).Err()
}

// ParseSessionID แยก sessionID hex จาก key รูปแบบ "session:{id},user:{uid}"
func ParseSessionID(key string) string {
	// key = "session:abc123,user:xyz"
	after, ok := strings.CutPrefix(key, keyPrefixSession+":")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(after, ",")
	return id
}

func userKey(userID string) string {
	return keyPrefixUser + ":" + userID
}

func (c *SessionCache) SetUser(ctx context.Context, user *model.User, ttl time.Duration) error {
	b, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, userKey(user.ID.Hex()), string(b), ttl).Err()
}

func (c *SessionCache) DeleteUser(ctx context.Context, userID string) error {
	return c.client.Del(ctx, userKey(userID)).Err()
}

// ScanByUser returns all active session keys belonging to a user (TTL > 0 only),
// sorted oldest-first so callers can evict in FIFO order.
func (c *SessionCache) ScanByUser(ctx context.Context, userID string) ([]string, error) {
	pattern := keyPrefixSession + ":*," + keyPrefixUser + ":" + userID
	var scanned []string
	iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		scanned = append(scanned, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	if len(scanned) == 0 {
		return nil, nil
	}

	pipe := c.client.Pipeline()
	ttlCmds := make([]*redis.DurationCmd, len(scanned))
	valCmds := make([]*redis.StringCmd, len(scanned))
	for i, key := range scanned {
		ttlCmds[i] = pipe.TTL(ctx, key)
		valCmds[i] = pipe.Get(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	type entry struct {
		key string
		ts  int64
	}
	var entries []entry
	for i, cmd := range ttlCmds {
		if ttl, err := cmd.Result(); err == nil && ttl > 0 {
			ts, _ := strconv.ParseInt(valCmds[i].Val(), 10, 64)
			entries = append(entries, entry{key: scanned[i], ts: ts})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ts < entries[j].ts
	})

	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = e.key
	}
	return keys, nil
}
