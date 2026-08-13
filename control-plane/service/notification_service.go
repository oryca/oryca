package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/repository"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var ErrNotificationNotFound = errors.New("notification not found")

const (
	notificationStreamTicketPrefix = "notification:stream-ticket:"
	notificationStreamTicketTTL    = 10 * time.Second
	notificationPubSubChanPrefix   = "notifications:user:"
)

type notificationRepo interface {
	Insert(ctx context.Context, doc *model.Notification) error
	FindAllByUser(ctx context.Context, userID bson.ObjectID, params url.Values) ([]*model.Notification, int64, error)
	CountUnread(ctx context.Context, userID bson.ObjectID) (int64, error)
	MarkAsRead(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error
	MarkAllAsRead(ctx context.Context, userID bson.ObjectID) error
	Delete(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error
}

type NotificationService struct {
	repo  notificationRepo
	redis *redis.Client
}

func NewNotificationService(repo notificationRepo, redisClient *redis.Client) *NotificationService {
	return &NotificationService{repo: repo, redis: redisClient}
}

// Create บันทึกแจ้งเตือนแล้ว publish ต่อ Redis — ชน dedupKey ถือว่าสำเร็จเงียบๆ (idempotent)
func (s *NotificationService) Create(ctx context.Context, in *model.NotificationCreate) error {
	createdBy := in.CreatedBy
	if createdBy.IsZero() {
		createdBy = model.SystemObjectID
	}

	doc := &model.Notification{
		ID:        bson.NewObjectID(),
		UserID:    in.UserID,
		Topic:     in.Topic,
		Level:     in.Level,
		Title:     in.Title,
		Message:   in.Message,
		Read:      false,
		DedupKey:  in.DedupKey,
		Detail:    in.Detail,
		CreatedAt: tool.NowUTC(),
		CreatedBy: createdBy,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		if repository.IsDuplicateKeyError(err) {
			logger.Info("notification dedup skip: " + in.DedupKey)
			return nil
		}
		return err
	}

	s.publish(ctx, doc)
	return nil
}

// CreateBackground เหมือน Create แต่รันแบบ background ไม่ block ผู้เรียก — pattern เดียวกับ trigger อื่น
func (s *NotificationService) CreateBackground(ctx context.Context, in *model.NotificationCreate) {
	runNotifyBackground(ctx, 30*time.Second, func(bgCtx context.Context) {
		if err := s.Create(bgCtx, in); err != nil {
			logger.Error(in.Topic + " notification failed userId=" + in.UserID.Hex() + " err=" + err.Error())
		}
	})
}

// publish ส่งเข้า Redis Pub/Sub แบบ best-effort
func (s *NotificationService) publish(ctx context.Context, doc *model.Notification) {
	payload, err := json.Marshal(doc)
	if err != nil {
		logger.Error("notification publish marshal error: " + err.Error())
		return
	}
	chan_ := notificationPubSubChanPrefix + doc.UserID.Hex()
	if err := s.redis.Publish(ctx, chan_, payload).Err(); err != nil {
		logger.Error("notification publish error: " + err.Error())
	}
}

func (s *NotificationService) List(ctx context.Context, userID bson.ObjectID, params url.Values) ([]*model.Notification, int64, error) {
	return s.repo.FindAllByUser(ctx, userID, params)
}

func (s *NotificationService) UnreadCount(ctx context.Context, userID bson.ObjectID) (int64, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *NotificationService) MarkAsRead(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error {
	if err := s.repo.MarkAsRead(ctx, id, userID); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrNotificationNotFound
		}
		return err
	}
	return nil
}

func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID bson.ObjectID) error {
	return s.repo.MarkAllAsRead(ctx, userID)
}

func (s *NotificationService) Delete(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrNotificationNotFound
		}
		return err
	}
	return nil
}

// IssueStreamTicket ออกตั๋วครั้งเดียวสำหรับ SSE auth (EventSource ส่ง custom header ไม่ได้)
func (s *NotificationService) IssueStreamTicket(ctx context.Context, userID bson.ObjectID) (string, error) {
	ticket := uuid.NewString()
	key := notificationStreamTicketPrefix + ticket
	if err := s.redis.Set(ctx, key, userID.Hex(), notificationStreamTicketTTL).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

var ErrInvalidStreamTicket = errors.New("invalid or expired ticket")

// VerifyStreamTicket อ่าน+ลบตั๋วแบบ atomic (GetDel) กัน replay
func (s *NotificationService) VerifyStreamTicket(ctx context.Context, ticket string) (bson.ObjectID, error) {
	key := notificationStreamTicketPrefix + ticket
	userIDStr, err := s.redis.GetDel(ctx, key).Result()
	if err != nil || userIDStr == "" {
		return bson.ObjectID{}, ErrInvalidStreamTicket
	}
	userID, err := bson.ObjectIDFromHex(userIDStr)
	if err != nil {
		return bson.ObjectID{}, ErrInvalidStreamTicket
	}
	return userID, nil
}

// SubscribeUser คืน PubSub subscription ของ userID นี้ — ผู้เรียกต้อง Close() เอง
func (s *NotificationService) SubscribeUser(ctx context.Context, userID bson.ObjectID) *redis.PubSub {
	return s.redis.Subscribe(ctx, notificationPubSubChanPrefix+userID.Hex())
}
