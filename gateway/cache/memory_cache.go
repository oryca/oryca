package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

// memoryCache คือ L0 ของ upstream cache. LRU ใน RAM ต่อ pod จำกัดด้วยขนาดรวม (bytes)
// hit ที่นี่ไม่แตะ Redis/S3 เลย; entry หมดอายุตาม TTL เดียวกับชั้นล่างเสมอ ไม่ stale เกินของเดิม
type memoryCache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	ll       *list.List // front = ใช้ล่าสุด
	items    map[string]*list.Element
}

type memItem struct {
	key      string
	entry    UpstreamCacheEntry // Body ครบใน RAM, BodyRef ว่างเสมอ
	size     int64
	expireAt time.Time
}

func newMemoryCache(maxBytes int64) *memoryCache {
	return &memoryCache{
		maxBytes: maxBytes,
		ll:       list.New(),
		items:    make(map[string]*list.Element),
	}
}

// get คืน (entry copy, age วินาที, hit). Copy Headers เสมอเพราะ caller mutate ได้ (strip hop-by-hop)
func (m *memoryCache) get(key string) (*UpstreamCacheEntry, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	el, ok := m.items[key]
	if !ok {
		return nil, 0, false
	}
	it := el.Value.(*memItem)
	if time.Now().After(it.expireAt) {
		m.removeLocked(el)
		return nil, 0, false
	}
	m.ll.MoveToFront(el)

	cp := it.entry
	cp.Headers = make(map[string][]string, len(it.entry.Headers))
	for k, v := range it.entry.Headers {
		cp.Headers[k] = v
	}
	age := it.entry.TTLSec - int(time.Until(it.expireAt).Seconds())
	if age < 0 {
		age = 0
	}
	return &cp, age, true
}

// set เก็บ entry ด้วยอายุที่เหลือ remaining. ของใหญ่เกินครึ่ง cap ไม่เก็บ (กันก้อนเดียวไล่ของร้อนทั้งหมดออก)
func (m *memoryCache) set(key string, entry UpstreamCacheEntry, remaining time.Duration) {
	if remaining <= 0 {
		return
	}
	size := int64(len(entry.Body)) + 1024 // เผื่อ headers/โครงสร้างคร่าวๆ
	if size > m.maxBytes/2 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if el, ok := m.items[key]; ok {
		m.removeLocked(el)
	}
	el := m.ll.PushFront(&memItem{key: key, entry: entry, size: size, expireAt: time.Now().Add(remaining)})
	m.items[key] = el
	m.curBytes += size

	for m.curBytes > m.maxBytes {
		oldest := m.ll.Back()
		if oldest == nil {
			break
		}
		m.removeLocked(oldest)
	}
}

func (m *memoryCache) removeLocked(el *list.Element) {
	it := el.Value.(*memItem)
	m.ll.Remove(el)
	delete(m.items, it.key)
	m.curBytes -= it.size
}

// sanitizeHeadersForStore ลบ headers ที่ GW จัดการเอง/ข้อมูล internal ก่อนเก็บทุกชั้น
func sanitizeHeadersForStore(h map[string][]string) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	for _, rm := range headersToStripOnStore {
		delete(out, rm)
	}
	for k := range out {
		if strings.HasPrefix(k, "X-Kong-") {
			delete(out, k)
		}
	}
	return out
}
