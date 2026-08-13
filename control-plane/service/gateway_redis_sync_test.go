package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mocks ---

type mockGatewaySyncPublisher struct{ mock.Mock }

func (m *mockGatewaySyncPublisher) Publish(eventType string, payload any) {
	m.Called(eventType, payload)
}

// --- helpers ---

func newTestRedisSync(t *testing.T, publisher gatewaySyncPublisher) (*GatewayRedisSync, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewGatewayRedisSync(client, time.Hour, publisher), mr
}

func testGatewayService(id bson.ObjectID) *model.GatewayService {
	return &model.GatewayService{
		ID:       id,
		Name:     "svc-name",
		BasePath: "/svc",
		Enabled:  boolPtr(true),
		IsPublic: boolPtr(false),
		ResourcePaths: []*model.ResourcePath{
			{Path: "/foo", Methods: []string{"GET"}, SourceAlias: "src-1"},
		},
	}
}

// --- buildServicePayload (pure, no Redis) ---

func TestBuildServicePayload(t *testing.T) {
	id := bson.NewObjectID()

	t.Run("nil Enabled/IsPublic default to false", func(t *testing.T) {
		svc := &model.GatewayService{ID: id, Name: "n", BasePath: "/n"}
		payload := buildServicePayload(svc, nil)
		assert.False(t, payload.Enabled)
		assert.False(t, payload.IsPublic)
		assert.Nil(t, payload.PackageIDs)
	})

	t.Run("maps fields and packageIDs through", func(t *testing.T) {
		svc := testGatewayService(id)
		payload := buildServicePayload(svc, []string{"pkg-1", "pkg-2"})
		assert.Equal(t, id.Hex(), payload.ID)
		assert.Equal(t, "svc-name", payload.Name)
		assert.Equal(t, "/svc", payload.BasePath)
		assert.True(t, payload.Enabled)
		assert.False(t, payload.IsPublic)
		assert.Equal(t, []string{"pkg-1", "pkg-2"}, payload.PackageIDs)
		require.Len(t, payload.Resources, 1)
		assert.Equal(t, "/foo", payload.Resources[0].Path)
		assert.Equal(t, "src-1", payload.Resources[0].SourceAlias)
	})
}

// --- SyncService ---

func TestGatewayRedisSync_SyncService_WritesPayloadAndPublishes(t *testing.T) {
	pub := &mockGatewaySyncPublisher{}
	pub.On("Publish", SyncEventTypeReloadServices, mock.Anything).Return()
	g, mr := newTestRedisSync(t, pub)

	svc := testGatewayService(bson.NewObjectID())
	err := g.SyncService(context.Background(), svc)
	require.NoError(t, err)

	raw := mr.HGet(redisKeyGatewayServicePaths, svc.ID.Hex())
	require.NotEmpty(t, raw)
	var payload gatewayServicePayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	assert.Equal(t, "svc-name", payload.Name)
	assert.Nil(t, payload.PackageIDs)

	ttl := mr.TTL(redisKeyGatewayServicePaths)
	assert.Greater(t, ttl, time.Duration(0))
	pub.AssertCalled(t, "Publish", SyncEventTypeReloadServices, mock.Anything)
}

func TestGatewayRedisSync_SyncService_NilRedisIsNoOp(t *testing.T) {
	g := NewGatewayRedisSync(nil, time.Hour, nil)
	err := g.SyncService(context.Background(), testGatewayService(bson.NewObjectID()))
	assert.NoError(t, err)
}

// --- SyncServicePackageIDs: the regression this suite exists to guard ---

func TestGatewayRedisSync_SyncServicePackageIDs_WorksWithoutExistingRedisEntry(t *testing.T) {
	// Regression test: SyncServicePackageIDs used to HGet the existing entry before writing,
	// so a service missing from Redis (e.g. skipped by boot-time pagination) made it fail
	// with redis.Nil and silently skip the sync notify. It must now succeed from svc alone.
	pub := &mockGatewaySyncPublisher{}
	pub.On("Publish", SyncEventTypeReloadServices, mock.Anything).Return()
	g, mr := newTestRedisSync(t, pub)

	svc := testGatewayService(bson.NewObjectID())

	require.Empty(t, mr.HGet(redisKeyGatewayServicePaths, svc.ID.Hex()))

	err := g.SyncServicePackageIDs(context.Background(), svc, []string{"pkg-1"})
	require.NoError(t, err)

	raw := mr.HGet(redisKeyGatewayServicePaths, svc.ID.Hex())
	require.NotEmpty(t, raw)
	var payload gatewayServicePayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	assert.Equal(t, []string{"pkg-1"}, payload.PackageIDs)
	assert.Equal(t, "svc-name", payload.Name)
	pub.AssertCalled(t, "Publish", SyncEventTypeReloadServices, mock.Anything)
}

func TestGatewayRedisSync_SyncServicePackageIDs_OverwritesStaleEntry(t *testing.T) {
	g, mr := newTestRedisSync(t, nil)
	svc := testGatewayService(bson.NewObjectID())

	mr.HSet(redisKeyGatewayServicePaths, svc.ID.Hex(), `{"id":"stale","name":"stale-name"}`)

	err := g.SyncServicePackageIDs(context.Background(), svc, []string{"pkg-2"})
	require.NoError(t, err)

	raw := mr.HGet(redisKeyGatewayServicePaths, svc.ID.Hex())
	require.NotEmpty(t, raw)
	var payload gatewayServicePayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	assert.Equal(t, "svc-name", payload.Name)
	assert.Equal(t, []string{"pkg-2"}, payload.PackageIDs)
}

func TestGatewayRedisSync_SyncServicePackageIDs_NilRedisIsNoOp(t *testing.T) {
	g := NewGatewayRedisSync(nil, time.Hour, nil)
	err := g.SyncServicePackageIDs(context.Background(), testGatewayService(bson.NewObjectID()), []string{"pkg-1"})
	assert.NoError(t, err)
}

// --- DeleteService ---

func TestGatewayRedisSync_DeleteService_RemovesEntryAndPublishes(t *testing.T) {
	pub := &mockGatewaySyncPublisher{}
	pub.On("Publish", SyncEventTypeReloadServices, mock.Anything).Return()
	g, mr := newTestRedisSync(t, pub)

	svc := testGatewayService(bson.NewObjectID())
	require.NoError(t, g.SyncService(context.Background(), svc))

	err := g.DeleteService(context.Background(), svc.ID.Hex())
	require.NoError(t, err)

	assert.Empty(t, mr.HGet(redisKeyGatewayServicePaths, svc.ID.Hex()))
	pub.AssertNumberOfCalls(t, "Publish", 2)
}

// --- SyncSource / DeleteSource ---

func TestGatewayRedisSync_SyncSource_WritesPayload(t *testing.T) {
	g, mr := newTestRedisSync(t, nil)
	src := &model.GatewaySource{Alias: "src-1", Type: "REST", URL: "http://upstream"}

	err := g.SyncSource(context.Background(), src)
	require.NoError(t, err)

	raw := mr.HGet(redisKeySourceRegistry, "src-1")
	require.NotEmpty(t, raw)
	var payload gatewaySourcePayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	assert.Equal(t, "REST", payload.Type)
}

func TestGatewayRedisSync_DeleteSource_RemovesEntry(t *testing.T) {
	g, mr := newTestRedisSync(t, nil)
	src := &model.GatewaySource{Alias: "src-1", Type: "REST"}
	require.NoError(t, g.SyncSource(context.Background(), src))

	err := g.DeleteSource(context.Background(), "src-1")
	require.NoError(t, err)
	assert.Empty(t, mr.HGet(redisKeySourceRegistry, "src-1"))
}

// --- publish: thin-signal fan-out ---

func TestGatewayRedisSync_Publish_SkipsPublisherWhenNil(t *testing.T) {
	g, _ := newTestRedisSync(t, nil)
	err := g.SyncService(context.Background(), testGatewayService(bson.NewObjectID()))
	require.NoError(t, err)
	// no publisher set — nothing to assert beyond "did not panic and Redis write succeeded"
}
