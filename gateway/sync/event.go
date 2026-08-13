package sync

import (
	"context"
	"encoding/json"
)

// EventType identifies what changed in a real-time sync Event published by control-plane.
type EventType string

const (
	// EventTypeReloadServices is a thin signal (no payload) telling the gateway to
	// re-pull services/sources/transform-configs via the existing HTTP+ETag
	// SyncProvider path instead of carrying the data itself — those are large
	// collections and fanning the full payload out to every pod would multiply
	// network traffic by pod count for no benefit (ETag makes the re-pull cheap).
	EventTypeReloadServices EventType = "reload_services"
	// EventTypeApiKey carries a full ApiKeyPayload — small and entity-scoped, safe to fan out whole.
	EventTypeApiKey EventType = "apikey"
	// EventTypeUser carries a full ApiKeyOwnerPayload (see its doc comment) — single-user
	// authorization-field changes (Update/Create/Verify/Delete on control-plane).
	EventTypeUser EventType = "user"
	// EventTypeUserInvalidate carries a UserInvalidatePayload (ids only) — bulk operations
	// (AssignUsers/RemoveUsers/BulkDelete) that touch many users in one control-plane call.
	EventTypeUserInvalidate EventType = "user_invalidate"
)

// Event is the wire contract published by control-plane on the sync pub/sub channel.
// Payload is empty for EventTypeReloadServices; for EventTypeApiKey it's the
// JSON-encoded ApiKeyPayload.
// Version is UnixMilli(updated_at) from control-plane's DB — listeners use it to drop
// out-of-order deliveries (an older event arriving after a newer one for the same entity).
type Event struct {
	Type    EventType       `json:"type"`
	Version int64           `json:"version"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// EventListener is the adapter boundary for real-time sync push — today implemented by
// RedisListener. Swapping to gRPC streaming or an internal Go package later means
// writing a new implementation of this interface; callers never change.
type EventListener interface {
	// Listen starts consuming in the background and returns immediately. Connection
	// failures — including the very first connect attempt — are never fatal: they're
	// logged and retried, so a listener with no reachable broker simply never sends
	// anything and the gateway keeps running on HTTP polling alone.
	Listen(ctx context.Context) <-chan Event
}
