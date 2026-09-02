package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// stubInternalApiKeyRepo/stubInternalUserRepo let the test assert exactly how many
// times each repo method is called. The whole point of the fix being tested is
// that owners are fetched in one FindByIDs call instead of one FindByID per key.
type stubInternalApiKeyRepo struct {
	keys []*model.ApiKey
}

func (s *stubInternalApiKeyRepo) FindAllActive(ctx context.Context) ([]*model.ApiKey, error) {
	return s.keys, nil
}

func (s *stubInternalApiKeyRepo) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.ApiKey, error) {
	byID := make(map[bson.ObjectID]*model.ApiKey, len(s.keys))
	for _, k := range s.keys {
		byID[k.ID] = k
	}
	out := make([]*model.ApiKey, 0, len(ids))
	for _, id := range ids {
		if k, ok := byID[id]; ok {
			out = append(out, k)
		}
	}
	return out, nil
}

type stubInternalUserRepo struct {
	byID          map[bson.ObjectID]*model.User
	findByIDCalls int
	findByIDsArgs [][]bson.ObjectID
}

func (s *stubInternalUserRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	s.findByIDCalls++
	return s.byID[id], nil
}

func (s *stubInternalUserRepo) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.User, error) {
	s.findByIDsArgs = append(s.findByIDsArgs, ids)
	out := make([]*model.User, 0, len(ids))
	for _, id := range ids {
		if u, ok := s.byID[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func newInternalHandlerForApiKeyTest(apiKeyRepo internalApiKeyRepo, userRepo internalUserRepo) *InternalHandler {
	return NewInternalHandler(InternalHandlerConfig{
		ApiKeyRepo: apiKeyRepo,
		UserRepo:   userRepo,
	})
}

func newInternalGetApiKeysContext() (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/internal/api-keys", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestGetApiKeys_BatchesOwnerLookup(t *testing.T) {
	ownerA := bson.NewObjectID()
	ownerB := bson.NewObjectID()
	pkgA := bson.NewObjectID()

	// three keys, two share the same owner. Dedup should mean FindByIDs is
	// called with exactly 2 unique ids, not 3.
	keys := []*model.ApiKey{
		{ID: bson.NewObjectID(), ApiKey: "key-1", OwnerBy: &ownerA},
		{ID: bson.NewObjectID(), ApiKey: "key-2", OwnerBy: &ownerA},
		{ID: bson.NewObjectID(), ApiKey: "key-3", OwnerBy: &ownerB},
	}
	users := map[bson.ObjectID]*model.User{
		ownerA: {ID: ownerA, Role: "user", PackageID: &pkgA},
		ownerB: {ID: ownerB, Role: "admin"},
	}

	apiKeyRepo := &stubInternalApiKeyRepo{keys: keys}
	userRepo := &stubInternalUserRepo{byID: users}
	h := newInternalHandlerForApiKeyTest(apiKeyRepo, userRepo)

	c, rec := newInternalGetApiKeysContext()
	if err := h.GetApiKeys(c); err != nil {
		t.Fatalf("GetApiKeys returned error: %v", err)
	}

	if userRepo.findByIDCalls != 0 {
		t.Errorf("expected FindByID to never be called (N+1 pattern), got %d calls", userRepo.findByIDCalls)
	}
	if len(userRepo.findByIDsArgs) != 1 {
		t.Fatalf("expected exactly 1 FindByIDs call, got %d", len(userRepo.findByIDsArgs))
	}
	if got := len(userRepo.findByIDsArgs[0]); got != 2 {
		t.Errorf("expected FindByIDs called with 2 unique owner ids (deduped), got %d: %v", got, userRepo.findByIDsArgs[0])
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var result []*model.ApiKeyCache
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 api-keys in response, got %d", len(result))
	}
	if result[0].Owner == nil || result[0].Owner.Role != "user" {
		t.Errorf("key-1 owner not populated correctly: %+v", result[0].Owner)
	}
	if result[1].Owner == nil || result[1].Owner.Role != "user" {
		t.Errorf("key-2 (shares owner with key-1) not populated correctly: %+v", result[1].Owner)
	}
	if result[2].Owner == nil || result[2].Owner.Role != "admin" {
		t.Errorf("key-3 owner not populated correctly: %+v", result[2].Owner)
	}
}

func TestGetApiKeys_MissingOwnerLeavesOwnerNil(t *testing.T) {
	orphanOwner := bson.NewObjectID() // no matching user — simulates a dangling OwnerBy reference
	keys := []*model.ApiKey{
		{ID: bson.NewObjectID(), ApiKey: "key-1", OwnerBy: &orphanOwner},
		{ID: bson.NewObjectID(), ApiKey: "key-2", OwnerBy: nil},
	}
	apiKeyRepo := &stubInternalApiKeyRepo{keys: keys}
	userRepo := &stubInternalUserRepo{byID: map[bson.ObjectID]*model.User{}}
	h := newInternalHandlerForApiKeyTest(apiKeyRepo, userRepo)

	c, rec := newInternalGetApiKeysContext()
	if err := h.GetApiKeys(c); err != nil {
		t.Fatalf("GetApiKeys returned error: %v", err)
	}

	var result []*model.ApiKeyCache
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 api-keys in response, got %d", len(result))
	}
	if result[0].Owner != nil {
		t.Errorf("expected nil Owner for dangling OwnerBy reference, got %+v", result[0].Owner)
	}
	if result[1].Owner != nil {
		t.Errorf("expected nil Owner for key with no OwnerBy, got %+v", result[1].Owner)
	}
}

func TestGetApiKeys_NoKeysSkipsOwnerLookup(t *testing.T) {
	apiKeyRepo := &stubInternalApiKeyRepo{keys: nil}
	userRepo := &stubInternalUserRepo{byID: map[bson.ObjectID]*model.User{}}
	h := newInternalHandlerForApiKeyTest(apiKeyRepo, userRepo)

	c, rec := newInternalGetApiKeysContext()
	if err := h.GetApiKeys(c); err != nil {
		t.Fatalf("GetApiKeys returned error: %v", err)
	}
	if len(userRepo.findByIDsArgs) != 1 || len(userRepo.findByIDsArgs[0]) != 0 {
		t.Errorf("expected FindByIDs called once with an empty slice, got %v", userRepo.findByIDsArgs)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
