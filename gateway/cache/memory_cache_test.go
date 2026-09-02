package cache

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func newTestEntry(body string) UpstreamCacheEntry {
	return UpstreamCacheEntry{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"image/png"}},
		Body:       []byte(body),
		TTLSec:     60,
	}
}

func TestMemoryCacheHitAndExpiry(t *testing.T) {
	m := newMemoryCache(1 << 20)
	m.set("k1", newTestEntry("hello"), time.Minute)

	got, age, ok := m.get("k1")
	if !ok || string(got.Body) != "hello" {
		t.Fatalf("expected hit with body, got ok=%v", ok)
	}
	if age < 0 || age > 1 {
		t.Errorf("fresh entry age = %d, want ~0", age)
	}

	// entry หมดอายุต้องเป็น miss และถูกลบออก
	m.set("k2", newTestEntry("old"), -time.Second)
	if _, _, ok := m.get("k2"); ok {
		t.Error("expired entry must miss")
	}
}

func TestMemoryCacheReturnsHeaderCopy(t *testing.T) {
	m := newMemoryCache(1 << 20)
	m.set("k", newTestEntry("body"), time.Minute)

	first, _, _ := m.get("k")
	first.Headers["Injected"] = []string{"x"} // caller mutate — ต้องไม่สะท้อนกลับเข้า cache

	second, _, _ := m.get("k")
	if _, leaked := second.Headers["Injected"]; leaked {
		t.Error("header mutation leaked into cached entry")
	}
}

func TestMemoryCacheLRUEviction(t *testing.T) {
	// จุได้ ~2 entries (body 1KB + overhead 1KB ต่อ entry)
	m := newMemoryCache(4 << 10)
	big := strings.Repeat("x", 1024)
	m.set("a", newTestEntry(big), time.Minute)
	m.set("b", newTestEntry(big), time.Minute)
	m.get("a") // a กลายเป็นล่าสุด
	m.set("c", newTestEntry(big), time.Minute)

	if _, _, ok := m.get("b"); ok {
		t.Error("b (LRU) ควรถูก evict")
	}
	if _, _, ok := m.get("a"); !ok {
		t.Error("a (เพิ่งใช้) ต้องยังอยู่")
	}
	if _, _, ok := m.get("c"); !ok {
		t.Error("c (ใหม่สุด) ต้องยังอยู่")
	}
}

func TestMemoryCacheRejectsOversizeEntry(t *testing.T) {
	m := newMemoryCache(4 << 10)
	m.set("huge", newTestEntry(strings.Repeat("x", 3<<10)), time.Minute)
	if _, _, ok := m.get("huge"); ok {
		t.Error("entry ใหญ่เกินครึ่ง cap ต้องไม่ถูกเก็บ")
	}
}

func TestSyntheticETag(t *testing.T) {
	a1 := SyntheticETag([]byte("same"))
	a2 := SyntheticETag([]byte("same"))
	b := SyntheticETag([]byte("diff"))
	if a1 != a2 {
		t.Error("เนื้อหาเดิมต้องได้ ETag เดิม")
	}
	if a1 == b {
		t.Error("เนื้อหาต่างต้องได้ ETag ต่าง")
	}
	if !strings.HasPrefix(a1, `W/"gw-`) || !strings.HasSuffix(a1, `"`) {
		t.Errorf("รูปแบบ weak ETag ผิด: %s", a1)
	}
}

func TestSanitizeHeadersForStore(t *testing.T) {
	in := map[string][]string{
		"Content-Type":   {"image/png"},
		"Age":            {"99"},
		"X-Kong-Latency": {"5"},
	}
	out := sanitizeHeadersForStore(in)
	if _, ok := out["Age"]; ok {
		t.Error("Age ต้องถูกลบ")
	}
	if _, ok := out["X-Kong-Latency"]; ok {
		t.Error("X-Kong-* ต้องถูกลบ")
	}
	if !bytes.Equal([]byte(out["Content-Type"][0]), []byte("image/png")) {
		t.Error("Content-Type ต้องคงอยู่")
	}
	if _, ok := in["Age"]; !ok {
		t.Error("map ต้นทางต้องไม่ถูก mutate")
	}
}
