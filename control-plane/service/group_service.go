package service

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrGroupNotFound       = errors.New("group not found")
	ErrGroupAliasDuplicate = errors.New("group alias already exists")
	ErrGroupForbidden      = errors.New("no permission to manage groups")
)

type groupRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.Group, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.Group, error)
	FindByAlias(ctx context.Context, alias string) (*model.Group, error)
	Insert(ctx context.Context, doc *model.Group) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
}

type groupUserRepo interface {
	FindByGroup(ctx context.Context, groupID bson.ObjectID, params url.Values) ([]*model.GroupUserDetail, int64, error)
	FindByUserAndGroup(ctx context.Context, userID, groupID bson.ObjectID) (*model.GroupUser, error)
	CountByGroup(ctx context.Context, groupID bson.ObjectID) (int, error)
	InsertMany(ctx context.Context, docs []interface{}) error
	SoftDeleteByGroupAndUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDeleteByGroupAndUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID) error
	SoftDeleteAllByGroup(ctx context.Context, groupID bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDeleteAllByGroup(ctx context.Context, groupID bson.ObjectID) error
}

type groupNotifier interface {
	Create(ctx context.Context, in *model.NotificationCreate) error
}

type GroupService struct {
	repo          groupRepo
	groupUserRepo groupUserRepo
	notifier      groupNotifier
}

func NewGroupService(repo groupRepo, groupUserRepo groupUserRepo, notifier groupNotifier) *GroupService {
	return &GroupService{repo: repo, groupUserRepo: groupUserRepo, notifier: notifier}
}

func (s *GroupService) List(ctx context.Context, params url.Values) ([]*model.Group, int64, error) {
	groups, total, err := s.repo.FindAll(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	for _, g := range groups {
		count, _ := s.groupUserRepo.CountByGroup(ctx, g.ID)
		g.UserCount = count
	}

	return groups, total, nil
}

func (s *GroupService) GetByID(ctx context.Context, id bson.ObjectID) (*model.Group, error) {
	g, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrGroupNotFound
	}

	count, _ := s.groupUserRepo.CountByGroup(ctx, g.ID)
	g.UserCount = count

	return g, nil
}

func (s *GroupService) Create(ctx context.Context, body *model.GroupCreate, ctxUser *model.User) (*model.Group, error) {
	alias := body.Alias
	if alias == "" {
		for {
			token, err := tool.GenerateToken(8)
			if err != nil {
				return nil, err
			}
			existing, err := s.repo.FindByAlias(ctx, token)
			if err != nil || existing == nil {
				alias = token
				break
			}
		}
	} else {
		if existing, err := s.repo.FindByAlias(ctx, alias); err == nil && existing != nil {
			return nil, ErrGroupAliasDuplicate
		}
	}

	now := tool.NowUTC()
	doc := &model.Group{
		ID:          bson.NewObjectID(),
		Alias:       alias,
		Name:        body.Name,
		Description: body.Description,
		Properties:  body.Properties,
		ParentID:    body.ParentID,
		CreatedAt:   &now,
		CreatedBy:   &ctxUser.ID,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	return doc, nil
}

func (s *GroupService) Update(ctx context.Context, id bson.ObjectID, body *model.GroupUpdate, ctxUser *model.User) (*model.Group, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrGroupNotFound
	}

	if body.Alias != "" && body.Alias != existing.Alias {
		if dup, err := s.repo.FindByAlias(ctx, body.Alias); err == nil && dup != nil {
			return nil, ErrGroupAliasDuplicate
		}
	}

	now := tool.NowUTC()
	fields := bson.M{
		"updatedAt":   now,
		"updatedBy":   ctxUser.ID,
		"alias":       body.Alias,
		"name":        body.Name,
		"description": body.Description,
		"properties":  body.Properties,
		"parentId":    body.ParentID,
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}

	return s.repo.FindByID(ctx, id)
}

func (s *GroupService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return ErrGroupNotFound
	}

	now := tool.NowUTC()
	if forever {
		if err := s.groupUserRepo.HardDeleteAllByGroup(ctx, id); err != nil {
			return err
		}
		return s.repo.HardDelete(ctx, id)
	}

	if err := s.groupUserRepo.SoftDeleteAllByGroup(ctx, id, now, ctxUser.ID); err != nil {
		return err
	}
	return s.repo.SoftDelete(ctx, id, now, ctxUser.ID)
}

func (s *GroupService) GetGroupUsers(ctx context.Context, groupID bson.ObjectID, params url.Values) ([]*model.GroupUserDetail, int64, error) {
	if _, err := s.repo.FindByID(ctx, groupID); err != nil {
		return nil, 0, ErrGroupNotFound
	}

	return s.groupUserRepo.FindByGroup(ctx, groupID, params)
}

func (s *GroupService) AddGroupUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID, ctxUser *model.User) (*model.GroupUserAddResult, error) {
	group, err := s.repo.FindByID(ctx, groupID)
	if err != nil {
		return nil, ErrGroupNotFound
	}

	now := tool.NowUTC()
	var docs []interface{}
	var addedUserIDs []bson.ObjectID
	skipped := 0
	for _, uid := range userIDs {
		if existing, err := s.groupUserRepo.FindByUserAndGroup(ctx, uid, groupID); err == nil && existing != nil {
			skipped++
			continue
		}
		docs = append(docs, &model.GroupUser{
			ID:        bson.NewObjectID(),
			User:      model.GroupUserRef{ID: uid},
			Group:     model.GroupUserRef{ID: groupID},
			CreatedAt: &now,
			CreatedBy: &ctxUser.ID,
		})
		addedUserIDs = append(addedUserIDs, uid)
	}

	if len(docs) > 0 {
		if err := s.groupUserRepo.InsertMany(ctx, docs); err != nil {
			return nil, err
		}
	}

	if s.notifier != nil && len(addedUserIDs) > 0 {
		runNotifyBackground(ctx, 60*time.Second, func(bgCtx context.Context) {
			s.notifyGroupUsersAdded(bgCtx, group, addedUserIDs, ctxUser.ID)
		})
	}

	return &model.GroupUserAddResult{
		Added:   len(docs),
		Skipped: skipped,
	}, nil
}

func (s *GroupService) notifyGroupUsersAdded(ctx context.Context, group *model.Group, userIDs []bson.ObjectID, actorID bson.ObjectID) {
	for _, uid := range userIDs {
		if err := s.notifier.Create(ctx, &model.NotificationCreate{
			UserID:  uid,
			Topic:   model.NotificationTopicGroupAdded,
			Level:   model.NotificationLevelInfo,
			Title:   "Added to a New Group",
			Message: "You were added to the group \"" + group.Name + "\"",
			Detail: map[string]any{
				"groupId":   group.ID.Hex(),
				"groupName": group.Name,
			},
			CreatedBy: actorID,
		}); err != nil {
			logger.Error("group_added notification failed userId=" + uid.Hex() + " err=" + err.Error())
		}
	}
}

func (s *GroupService) RemoveGroupUsers(ctx context.Context, groupID bson.ObjectID, body *model.GroupUserRemove, ctxUser *model.User) error {
	if _, err := s.repo.FindByID(ctx, groupID); err != nil {
		return ErrGroupNotFound
	}

	now := tool.NowUTC()
	if body.Forever {
		return s.groupUserRepo.HardDeleteByGroupAndUsers(ctx, groupID, body.UserIDs)
	}
	return s.groupUserRepo.SoftDeleteByGroupAndUsers(ctx, groupID, body.UserIDs, now, ctxUser.ID)
}
