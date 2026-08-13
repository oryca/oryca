package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/repository"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrDashboardRangeInvalid = errors.New("dashboard: 'to' must be after 'from'")

const (
	dashboardDefaultRange = 24 * time.Hour
	dashboardMaxRange     = 90 * 24 * time.Hour
	dashboardLogsMaxLimit = 100
	dashboardTopServices  = 5
)

type accessLogStore interface {
	Stats(ctx context.Context, q model.AccessLogQuery, interval string) ([]*model.DashboardBucket, error)
	Summary(ctx context.Context, q model.AccessLogQuery) (*model.DashboardSummary, error)
	TopServices(ctx context.Context, q model.AccessLogQuery, limit int) ([]*model.DashboardServiceUsage, error)
	FindLogs(ctx context.Context, q model.AccessLogQuery) ([]*model.AccessLog, error)
}

type dashboardServiceNamer interface {
	FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.GatewayService, error)
}

type DashboardService struct {
	logs        accessLogStore
	serviceRepo dashboardServiceNamer
}

func NewDashboardService(logs accessLogStore, serviceRepo dashboardServiceNamer) *DashboardService {
	return &DashboardService{logs: logs, serviceRepo: serviceRepo}
}

// scope limits what a caller can see. A plain user's userId is replaced, not
// merged, so no filter can widen it. Root and admin see everything.
func scope(q model.AccessLogQuery, actor *model.User) model.AccessLogQuery {
	if actor == nil {
		q.UserID = "-" // ไม่มี actor = ไม่เห็นอะไรเลย (กันพลาดถ้า middleware หลุด)
		return q
	}
	if actor.Role != "root" && actor.Role != "admin" {
		q.UserID = actor.ID.Hex()
	}
	return q
}

// normalizeRange fills in a default window and rejects an inverted or absurdly
// long one — an unbounded range would scan the whole collection.
func normalizeRange(q model.AccessLogQuery) (model.AccessLogQuery, error) {
	now := time.Now().UTC()
	if q.To.IsZero() {
		q.To = now
	}
	if q.From.IsZero() {
		q.From = q.To.Add(-dashboardDefaultRange)
	}
	if !q.To.After(q.From) {
		return q, ErrDashboardRangeInvalid
	}
	if q.To.Sub(q.From) > dashboardMaxRange {
		q.From = q.To.Add(-dashboardMaxRange)
	}
	return q, nil
}

// pickInterval keeps the chart readable: about 60–360 points whatever the range.
func pickInterval(from, to time.Time) string {
	switch d := to.Sub(from); {
	case d <= 6*time.Hour:
		return repository.IntervalMinute
	case d <= 7*24*time.Hour:
		return repository.IntervalHour
	default:
		return repository.IntervalDay
	}
}

func (s *DashboardService) Stats(ctx context.Context, q model.AccessLogQuery, actor *model.User) (*model.DashboardStats, error) {
	q, err := normalizeRange(scope(q, actor))
	if err != nil {
		return nil, err
	}
	interval := pickInterval(q.From, q.To)
	buckets, err := s.logs.Stats(ctx, q, interval)
	if err != nil {
		return nil, err
	}
	return &model.DashboardStats{Interval: interval, Buckets: buckets}, nil
}

func (s *DashboardService) Summary(ctx context.Context, q model.AccessLogQuery, actor *model.User) (*model.DashboardSummary, error) {
	q, err := normalizeRange(scope(q, actor))
	if err != nil {
		return nil, err
	}
	summary, err := s.logs.Summary(ctx, q)
	if err != nil {
		return nil, err
	}
	top, err := s.logs.TopServices(ctx, q, dashboardTopServices)
	if err != nil {
		return nil, err
	}
	s.fillServiceNames(ctx, top)
	summary.TopServices = top
	return summary, nil
}

// fillServiceNames resolves ids to names in one query — best-effort, a deleted
// service just keeps its id.
func (s *DashboardService) fillServiceNames(ctx context.Context, rows []*model.DashboardServiceUsage) {
	if len(rows) == 0 || s.serviceRepo == nil {
		return
	}
	ids := make([]bson.ObjectID, 0, len(rows))
	for _, row := range rows {
		if id, err := bson.ObjectIDFromHex(row.ServiceID); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	services, err := s.serviceRepo.FindByIDs(ctx, ids)
	if err != nil {
		return
	}
	names := make(map[string]string, len(services))
	for _, svc := range services {
		names[svc.ID.Hex()] = svc.Name
	}
	for _, row := range rows {
		row.Name = names[row.ServiceID]
	}
}

func (s *DashboardService) Logs(ctx context.Context, q model.AccessLogQuery, actor *model.User) (*model.AccessLogPage, error) {
	q, err := normalizeRange(scope(q, actor))
	if err != nil {
		return nil, err
	}
	if q.Limit <= 0 || q.Limit > dashboardLogsMaxLimit {
		q.Limit = dashboardLogsMaxLimit
	}

	// ขอเกินมา 1 แถวเพื่อรู้ว่ามีหน้าถัดไปไหม โดยไม่ต้อง count ทั้ง collection
	q.Limit++
	logs, err := s.logs.FindLogs(ctx, q)
	if err != nil {
		return nil, err
	}

	page := &model.AccessLogPage{Data: logs}
	if len(logs) == q.Limit {
		last := logs[len(logs)-2]
		page.Data = logs[:len(logs)-1]
		page.NextCursor = EncodeLogCursor(last.Time, last.ID)
	}
	if page.Data == nil {
		page.Data = []*model.AccessLog{}
	}
	return page, nil
}

// EncodeLogCursor/DecodeLogCursor carry the (time, _id) pair the next page
// starts after. Format is "<unixMilli>_<objectId>" — opaque to clients but
// readable in a log when debugging.
func EncodeLogCursor(t time.Time, id bson.ObjectID) string {
	return strconv.FormatInt(t.UTC().UnixMilli(), 10) + "_" + id.Hex()
}

func DecodeLogCursor(raw string) (*time.Time, *bson.ObjectID, bool) {
	if raw == "" {
		return nil, nil, false
	}
	rawTime, rawID, ok := strings.Cut(raw, "_")
	if !ok {
		return nil, nil, false
	}
	ms, err := strconv.ParseInt(rawTime, 10, 64)
	if err != nil {
		return nil, nil, false
	}
	id, err := bson.ObjectIDFromHex(rawID)
	if err != nil {
		return nil, nil, false
	}
	t := time.UnixMilli(ms).UTC()
	return &t, &id, true
}
