package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type fakeApiDocRepo struct {
	docs map[bson.ObjectID]*model.ApiDoc
}

func newFakeApiDocRepo() *fakeApiDocRepo {
	return &fakeApiDocRepo{docs: map[bson.ObjectID]*model.ApiDoc{}}
}

func (f *fakeApiDocRepo) FindAll(ctx context.Context, params url.Values) ([]*model.ApiDoc, int64, error) {
	out := []*model.ApiDoc{}
	for _, d := range f.docs {
		copied := *d
		out = append(out, &copied)
	}
	return out, int64(len(out)), nil
}

func (f *fakeApiDocRepo) FindByID(ctx context.Context, id bson.ObjectID, params *url.Values) (*model.ApiDoc, error) {
	d, ok := f.docs[id]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	copied := *d
	return &copied, nil
}

func (f *fakeApiDocRepo) Insert(ctx context.Context, doc *model.ApiDoc) error {
	f.docs[doc.ID] = doc
	return nil
}

func (f *fakeApiDocRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	d, ok := f.docs[id]
	if !ok {
		return mongo.ErrNoDocuments
	}
	if v, ok := fields["publishStatus"]; ok {
		d.PublishStatus = v.(string)
	}
	return nil
}

func (f *fakeApiDocRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	d, ok := f.docs[id]
	if !ok {
		return mongo.ErrNoDocuments
	}
	d.DeletedAt = &deletedAt
	d.DeletedBy = &deletedBy
	return nil
}

func (f *fakeApiDocRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	if _, ok := f.docs[id]; !ok {
		return mongo.ErrNoDocuments
	}
	delete(f.docs, id)
	return nil
}

func (f *fakeApiDocRepo) FindPathsByID(ctx context.Context, id bson.ObjectID, apiDocParams, pathParams url.Values) ([]*model.ApiDocPath, int64, error) {
	d, ok := f.docs[id]
	if !ok {
		return nil, 0, mongo.ErrNoDocuments
	}
	return d.Paths, int64(len(d.Paths)), nil
}

func TestApiDocService_GetByID(t *testing.T) {
	repo := newFakeApiDocRepo()
	svc := NewApiDocService(repo)
	id := bson.NewObjectID()
	repo.docs[id] = &model.ApiDoc{ID: id, Info: model.ApiDocInfo{Title: "Doc"}}

	t.Run("พบเอกสาร ต้องได้ข้อมูลกลับมา", func(t *testing.T) {
		doc, err := svc.GetByID(context.Background(), id, url.Values{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.Info.Title != "Doc" {
			t.Fatalf("unexpected title: %s", doc.Info.Title)
		}
	})

	t.Run("ไม่พบเอกสาร ต้องได้ ErrApiDocNotFound", func(t *testing.T) {
		_, err := svc.GetByID(context.Background(), bson.NewObjectID(), url.Values{})
		if err != ErrApiDocNotFound {
			t.Fatalf("expected ErrApiDocNotFound, got %v", err)
		}
	})
}

func TestApiDocService_Create(t *testing.T) {
	repo := newFakeApiDocRepo()
	svc := NewApiDocService(repo)
	ctxUser := &model.User{ID: bson.NewObjectID()}

	body := &model.ApiDocCreate{
		OpenAPI:        "3.0.4",
		AuthenRequired: model.ApiDocAuthenRequiredNone,
		PublishStatus:  model.ApiDocPublishStatusDraft,
		Info:           model.ApiDocInfo{Title: "New Doc", Version: "1.0.0"},
	}

	doc, err := svc.Create(context.Background(), body, ctxUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.CreatedBy == nil || *doc.CreatedBy != ctxUser.ID {
		t.Fatalf("expected CreatedBy to be set to ctxUser.ID")
	}
	if _, ok := repo.docs[doc.ID]; !ok {
		t.Fatalf("expected doc to be persisted in repo")
	}
}

func TestApiDocService_Update(t *testing.T) {
	repo := newFakeApiDocRepo()
	svc := NewApiDocService(repo)
	id := bson.NewObjectID()
	repo.docs[id] = &model.ApiDoc{ID: id, PublishStatus: model.ApiDocPublishStatusDraft}
	ctxUser := &model.User{ID: bson.NewObjectID()}

	t.Run("อัปเดตเอกสารที่มีอยู่ ต้องสำเร็จ", func(t *testing.T) {
		body := &model.ApiDocUpdate{PublishStatus: model.ApiDocPublishStatusPublished}
		doc, err := svc.Update(context.Background(), id, body, ctxUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if doc.PublishStatus != model.ApiDocPublishStatusPublished {
			t.Fatalf("expected publishStatus updated, got %s", doc.PublishStatus)
		}
	})

	t.Run("อัปเดตเอกสารที่ไม่มี ต้องได้ ErrApiDocNotFound", func(t *testing.T) {
		body := &model.ApiDocUpdate{PublishStatus: model.ApiDocPublishStatusPublished}
		_, err := svc.Update(context.Background(), bson.NewObjectID(), body, ctxUser)
		if err != ErrApiDocNotFound {
			t.Fatalf("expected ErrApiDocNotFound, got %v", err)
		}
	})
}

func TestApiDocService_Delete(t *testing.T) {
	ctxUser := &model.User{ID: bson.NewObjectID()}

	t.Run("soft delete เอกสารที่มีอยู่ ต้อง set deletedAt", func(t *testing.T) {
		repo := newFakeApiDocRepo()
		svc := NewApiDocService(repo)
		id := bson.NewObjectID()
		repo.docs[id] = &model.ApiDoc{ID: id}

		if err := svc.Delete(context.Background(), id, false, ctxUser); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.docs[id].DeletedAt == nil {
			t.Fatalf("expected DeletedAt to be set")
		}
	})

	t.Run("hard delete เอกสารที่มีอยู่ ต้องลบออกจาก repo", func(t *testing.T) {
		repo := newFakeApiDocRepo()
		svc := NewApiDocService(repo)
		id := bson.NewObjectID()
		repo.docs[id] = &model.ApiDoc{ID: id}

		if err := svc.Delete(context.Background(), id, true, ctxUser); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := repo.docs[id]; ok {
			t.Fatalf("expected doc to be removed from repo")
		}
	})

	t.Run("ลบเอกสารที่ไม่มี ต้องได้ ErrApiDocNotFound", func(t *testing.T) {
		repo := newFakeApiDocRepo()
		svc := NewApiDocService(repo)
		if err := svc.Delete(context.Background(), bson.NewObjectID(), false, ctxUser); err != ErrApiDocNotFound {
			t.Fatalf("expected ErrApiDocNotFound, got %v", err)
		}
	})
}

func TestApiDocService_GetPathsByID(t *testing.T) {
	repo := newFakeApiDocRepo()
	svc := NewApiDocService(repo)
	id := bson.NewObjectID()
	repo.docs[id] = &model.ApiDoc{
		ID:    id,
		Paths: []*model.ApiDocPath{{Path: "/foo", Method: "GET"}},
	}

	t.Run("มี apiDoc ต้องได้ paths กลับมา", func(t *testing.T) {
		paths, total, err := svc.GetPathsByID(context.Background(), id, url.Values{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 1 || len(paths) != 1 {
			t.Fatalf("expected 1 path, got total=%d len=%d", total, len(paths))
		}
	})

	t.Run("ไม่มี apiDoc ต้องได้ ErrApiDocNotFound", func(t *testing.T) {
		_, _, err := svc.GetPathsByID(context.Background(), bson.NewObjectID(), url.Values{})
		if err != ErrApiDocNotFound {
			t.Fatalf("expected ErrApiDocNotFound, got %v", err)
		}
	})
}
