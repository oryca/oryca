package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type stubInternalServiceRepo struct {
	svcs          []*model.GatewayService
	findByIDsArgs [][]bson.ObjectID
}

func (s *stubInternalServiceRepo) FindAllActive(ctx context.Context) ([]*model.GatewayService, error) {
	return s.svcs, nil
}

func (s *stubInternalServiceRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error) {
	for _, v := range s.svcs {
		if v.ID == id {
			return v, nil
		}
	}
	return nil, nil
}

func (s *stubInternalServiceRepo) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.GatewayService, error) {
	s.findByIDsArgs = append(s.findByIDsArgs, ids)
	byID := make(map[bson.ObjectID]*model.GatewayService, len(s.svcs))
	for _, v := range s.svcs {
		byID[v.ID] = v
	}
	out := make([]*model.GatewayService, 0, len(ids))
	for _, id := range ids {
		if v, ok := byID[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

type stubInternalPackageRepo struct {
	pkgs []*model.Package
}

func (s *stubInternalPackageRepo) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.Package, error) {
	byID := make(map[bson.ObjectID]*model.Package, len(s.pkgs))
	for _, v := range s.pkgs {
		byID[v.ID] = v
	}
	out := make([]*model.Package, 0, len(ids))
	for _, id := range ids {
		if v, ok := byID[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func newInternalHandlerForResolveTest(
	userRepo internalUserRepo,
	serviceRepo internalServiceRepo,
	packageRepo internalPackageRepo,
	apiKeyRepo internalApiKeyRepo,
) *InternalHandler {
	return NewInternalHandler(InternalHandlerConfig{
		UserRepo:    userRepo,
		ServiceRepo: serviceRepo,
		PackageRepo: packageRepo,
		ApiKeyRepo:  apiKeyRepo,
	})
}

func postResolveNames(h *InternalHandler, body string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/internal/resolve-names", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.ResolveNames(c)
	return rec, err
}

func TestResolveNames_BatchesEachIDTypeOnce(t *testing.T) {
	userA := bson.NewObjectID()
	userB := bson.NewObjectID()
	svcA := bson.NewObjectID()
	pkgA := bson.NewObjectID()
	keyA := bson.NewObjectID()

	userRepo := &stubInternalUserRepo{byID: map[bson.ObjectID]*model.User{
		userA: {ID: userA, DisplayName: "Alice"},
		userB: {ID: userB, Username: "bob"}, // no DisplayName — falls back to Username
	}}
	serviceRepo := &stubInternalServiceRepo{svcs: []*model.GatewayService{
		{ID: svcA, Name: "gistda-features"},
	}}
	packageRepo := &stubInternalPackageRepo{pkgs: []*model.Package{
		{ID: pkgA, Name: "Free Tier"},
	}}
	apiKeyRepo := &stubInternalApiKeyRepo{keys: []*model.ApiKey{
		{ID: keyA, Name: "prod-key-1"},
	}}

	h := newInternalHandlerForResolveTest(userRepo, serviceRepo, packageRepo, apiKeyRepo)

	body := `{
		"userIds": ["` + userA.Hex() + `", "` + userB.Hex() + `"],
		"serviceIds": ["` + svcA.Hex() + `"],
		"packageIds": ["` + pkgA.Hex() + `"],
		"apiKeyIds": ["` + keyA.Hex() + `"]
	}`
	rec, err := postResolveNames(h, body)
	if err != nil {
		t.Fatalf("ResolveNames returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	if len(serviceRepo.findByIDsArgs) != 1 {
		t.Fatalf("expected exactly 1 FindByIDs call for services, got %d", len(serviceRepo.findByIDsArgs))
	}

	var resp ResolveNamesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Users[userA.Hex()] != "Alice" {
		t.Errorf("expected userA name Alice, got %q", resp.Users[userA.Hex()])
	}
	if resp.Users[userB.Hex()] != "bob" {
		t.Errorf("expected userB to fall back to username 'bob', got %q", resp.Users[userB.Hex()])
	}
	if resp.Services[svcA.Hex()] != "gistda-features" {
		t.Errorf("expected service name gistda-features, got %q", resp.Services[svcA.Hex()])
	}
	if resp.Packages[pkgA.Hex()] != "Free Tier" {
		t.Errorf("expected package name 'Free Tier', got %q", resp.Packages[pkgA.Hex()])
	}
	if resp.ApiKeys[keyA.Hex()] != "prod-key-1" {
		t.Errorf("expected api-key name prod-key-1, got %q", resp.ApiKeys[keyA.Hex()])
	}
}

func TestResolveNames_UnknownIDsOmittedNotError(t *testing.T) {
	h := newInternalHandlerForResolveTest(
		&stubInternalUserRepo{byID: map[bson.ObjectID]*model.User{}},
		&stubInternalServiceRepo{},
		&stubInternalPackageRepo{},
		&stubInternalApiKeyRepo{},
	)

	unknownID := bson.NewObjectID().Hex()
	rec, err := postResolveNames(h, `{"userIds": ["`+unknownID+`"]}`)
	if err != nil {
		t.Fatalf("ResolveNames returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an id that doesn't resolve to anything, got %d", rec.Code)
	}
	var resp ResolveNamesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Users) != 0 {
		t.Errorf("expected no entries for an unresolvable id, got %v", resp.Users)
	}
}

func TestResolveNames_MalformedIDSkippedNotFatal(t *testing.T) {
	userA := bson.NewObjectID()
	userRepo := &stubInternalUserRepo{byID: map[bson.ObjectID]*model.User{
		userA: {ID: userA, DisplayName: "Alice"},
	}}
	h := newInternalHandlerForResolveTest(userRepo, &stubInternalServiceRepo{}, &stubInternalPackageRepo{}, &stubInternalApiKeyRepo{})

	// "not-a-valid-object-id" must be skipped, not blow up the whole request —
	// userA must still resolve normally.
	body := `{"userIds": ["not-a-valid-object-id", "` + userA.Hex() + `"]}`
	rec, err := postResolveNames(h, body)
	if err != nil {
		t.Fatalf("ResolveNames returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp ResolveNamesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Users[userA.Hex()] != "Alice" {
		t.Errorf("expected userA to still resolve despite a malformed sibling id, got %v", resp.Users)
	}
}

func TestResolveNames_EmptyRequestReturnsEmptyResponse(t *testing.T) {
	h := newInternalHandlerForResolveTest(
		&stubInternalUserRepo{byID: map[bson.ObjectID]*model.User{}},
		&stubInternalServiceRepo{},
		&stubInternalPackageRepo{},
		&stubInternalApiKeyRepo{},
	)
	rec, err := postResolveNames(h, `{}`)
	if err != nil {
		t.Fatalf("ResolveNames returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
