package trie

import (
	"github.com/oryca/oryca/gateway/model"
	"testing"
)

func svc(id, basePath string, paths ...string) *model.GatewayService {
	s := &model.GatewayService{ID: id, BasePath: basePath}
	for _, p := range paths {
		s.ResourcePaths = append(s.ResourcePaths, &model.ResourcePath{Path: p, Methods: []string{"GET"}})
	}
	return s
}

func TestFindBestMatch_ExactPath(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/api", "/users"))

	m := pt.FindBestMatch("/api/users")
	if m == nil || m.Service.ID != "svc1" {
		t.Fatalf("expected svc1, got %v", m)
	}
}

func TestFindBestMatch_PathParam(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/api", "/{id}"))

	m := pt.FindBestMatch("/api/123")
	if m == nil || m.Service.ID != "svc1" {
		t.Fatalf("expected svc1, got %v", m)
	}
	if m.PathParams["id"] != "123" {
		t.Errorf("expected id=123, got %v", m.PathParams)
	}
}

func TestFindBestMatch_SuffixWildcard(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/api", "/collections/*"))

	m := pt.FindBestMatch("/api/collections/abc/items")
	if m == nil || m.Service.ID != "svc1" {
		t.Fatalf("expected svc1, got %v", m)
	}
	if m.Remaining != "abc/items" {
		t.Errorf("expected remaining=abc/items, got %q", m.Remaining)
	}
}

func TestFindBestMatch_ExactBeatsWildcard(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc-exact", "/api", "/collections/special"))
	pt.Insert(svc("svc-wild", "/api", "/collections/*"))

	m := pt.FindBestMatch("/api/collections/special")
	if m == nil || m.Service.ID != "svc-exact" {
		t.Fatalf("exact should win over wildcard, got %v", m)
	}
}

// TestFindBestMatch_ExactAndWildcardShareParentSegment. Regression test: "/items" กับ
// "/items/*" ต้องผ่าน node "items" เดียวกันเสมอ (ต่างจาก TestFindBestMatch_ExactBeatsWildcard
// ที่ exact/wildcard อยู่คนละ node กันเลย) เคยเป็นบั๊กจริง: insertResource เขียน exact-match
// ทับ wildcard (หรือกลับกัน) ที่ node เดียวกัน ตัวที่ insert ทีหลังชนะเสมอไม่ว่าจะถูกหรือผิด —
// เจอจากการทดสอบจริงที่ /items (ราคา 2) ถูกหักราคาของ /items/* (ราคา 60) แทน
func TestFindBestMatch_ExactAndWildcardShareParentSegment(t *testing.T) {
	t.Run("exact inserted before wildcard", func(t *testing.T) {
		pt := New()
		pt.Insert(svc("svc-exact", "/api", "/items"))
		pt.Insert(svc("svc-wild", "/api", "/items/*"))

		m := pt.FindBestMatch("/api/items")
		if m == nil || m.Service.ID != "svc-exact" {
			t.Fatalf("bare /items must resolve to the exact resource regardless of insert order, got %v", m)
		}
	})

	t.Run("wildcard inserted before exact", func(t *testing.T) {
		pt := New()
		pt.Insert(svc("svc-wild", "/api", "/items/*"))
		pt.Insert(svc("svc-exact", "/api", "/items"))

		m := pt.FindBestMatch("/api/items")
		if m == nil || m.Service.ID != "svc-exact" {
			t.Fatalf("bare /items must resolve to the exact resource regardless of insert order, got %v", m)
		}
	})

	t.Run("deeper path still falls through to wildcard", func(t *testing.T) {
		pt := New()
		pt.Insert(svc("svc-exact", "/api", "/items"))
		pt.Insert(svc("svc-wild", "/api", "/items/*"))

		m := pt.FindBestMatch("/api/items/foo/bar")
		if m == nil || m.Service.ID != "svc-wild" {
			t.Fatalf("nested path must still resolve to the wildcard resource, got %v", m)
		}
		if m.Remaining != "foo/bar" {
			t.Errorf("expected remaining=foo/bar, got %q", m.Remaining)
		}
	})

	// resource ที่กำหนดเป็น "/*" ล้วนๆ (ไม่มี path อื่นแยก) ต้อง match bare parent path ได้ด้วย —
	// เจอจริงกับ service "SensorThing V2" (basePath=/streaming/v2/test, resourcePath เดียวคือ "/*")
	// การแก้ครั้งแรกเข้าใจผิดว่า wildcard ต้องมีอย่างน้อย 1 segment ตามหลังเสมอ ทำให้ resource
	// ที่มีแค่ "/*" ตัวเดียวเรียกไม่ได้เลยทั้งที่ไม่มี exact resource มาแข่งที่ node เดียวกัน
	t.Run("wildcard-only resource still matches its own bare parent path", func(t *testing.T) {
		pt := New()
		pt.Insert(svc("svc-wild", "/api", "/items/*"))

		m := pt.FindBestMatch("/api/items")
		if m == nil || m.Service.ID != "svc-wild" {
			t.Fatalf("bare /items must resolve to the wildcard resource when no exact resource competes for the same node, got %v", m)
		}
		if m.Remaining != "" {
			t.Errorf("expected empty remaining for the bare parent path, got %q", m.Remaining)
		}
	})
}

func TestFindBestMatch_NoMatch(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/api", "/users"))

	m := pt.FindBestMatch("/api/orders")
	if m != nil {
		t.Fatalf("expected no match, got %v", m.Service.ID)
	}
}

func TestFindBestMatch_RootPath(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/gistda", "/"))

	m := pt.FindBestMatch("/gistda/")
	if m == nil || m.Service.ID != "svc1" {
		t.Fatalf("expected svc1 for root path, got %v", m)
	}
}

func TestFindBestMatch_MultipleServices(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc-a", "/service-a", "/data"))
	pt.Insert(svc("svc-b", "/service-b", "/data"))

	ma := pt.FindBestMatch("/service-a/data")
	mb := pt.FindBestMatch("/service-b/data")

	if ma == nil || ma.Service.ID != "svc-a" {
		t.Errorf("expected svc-a, got %v", ma)
	}
	if mb == nil || mb.Service.ID != "svc-b" {
		t.Errorf("expected svc-b, got %v", mb)
	}
}

func TestLoadBulk_ReplacesExisting(t *testing.T) {
	pt := New()
	pt.Insert(svc("old-svc", "/old", "/path"))

	pt.LoadBulk([]*model.GatewayService{
		svc("new-svc", "/new", "/path"),
	})

	if m := pt.FindBestMatch("/old/path"); m != nil {
		t.Error("old service should be gone after LoadBulk")
	}
	if m := pt.FindBestMatch("/new/path"); m == nil || m.Service.ID != "new-svc" {
		t.Error("new service should be found after LoadBulk")
	}
}

func TestDelete_RemovesService(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/api", "/users"))

	pt.Delete("svc1")

	if m := pt.FindBestMatch("/api/users"); m != nil {
		t.Error("service should be deleted")
	}
}

func TestFindBestMatch_DeepNested(t *testing.T) {
	pt := New()
	pt.Insert(svc("svc1", "/gistda", "/collections/{col}/items/{id}"))

	m := pt.FindBestMatch("/gistda/collections/viirs/items/42")
	if m == nil || m.Service.ID != "svc1" {
		t.Fatalf("expected svc1, got %v", m)
	}
	if m.PathParams["col"] != "viirs" || m.PathParams["id"] != "42" {
		t.Errorf("wrong path params: %v", m.PathParams)
	}
}
