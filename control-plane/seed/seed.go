// Package seed fills a fresh database from the YAML files next to this one.
//
// It runs entry by entry and skips anything that already exists, so restarting
// never overwrites what an admin changed. Nothing here updates or deletes.
package seed

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var embedded embed.FS

const (
	fileConfiguration = "configuration.yaml"
	filePackages      = "packages.yaml"
)

// Logger คือ logging hook แคบๆ เพื่อไม่ให้ package นี้ผูกกับ logger ตัวใดตัวหนึ่ง
type Logger interface {
	Info(msg string)
	Error(msg string)
}

// ConfigStore reads and writes the single Configuration document.
type ConfigStore interface {
	Get(ctx context.Context) (*model.Configuration, error)
	Upsert(ctx context.Context, doc *model.Configuration) error
}

// PackageStore looks packages up by alias and inserts new ones.
type PackageStore interface {
	FindByAlias(ctx context.Context, alias string) (*model.Package, error)
	Insert(ctx context.Context, doc *model.Package) error
}

// UserStore is what root-user seeding needs: does a root already exist, and can we add one.
type UserStore interface {
	FindByRoles(ctx context.Context, roles []string) ([]*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	Insert(ctx context.Context, doc *model.User) error
}

// Stores คือ dependency ทั้งหมดของ seeder. Repository ฝั่ง caller ใส่ให้ตรง interface ได้เลย
type Stores struct {
	Config  ConfigStore
	Package PackageStore
	User    UserStore
}

// Options tunes where the YAML comes from and who the first administrator is.
type Options struct {
	// Dir reads the YAML from a directory instead of the copy built into the
	// binary. Set ORYCA_API_SEED_DIR to edit seed data without rebuilding.
	Dir string
	// RootEmail/RootPassword create the first root account. An empty password
	// means "generate one and print it once"; an empty email skips root seeding
	// entirely.
	RootEmail    string
	RootPassword string
}

// Run applies every seed file, then the root user. Each step is independent:
// a failure is logged and the rest still run, because a half-seeded instance
// that boots is more useful than one that refuses to start.
func Run(ctx context.Context, s Stores, opts Options, log Logger) {
	seedConfiguration(ctx, s.Config, opts.Dir, log)
	seedPackages(ctx, s.Package, opts.Dir, log)
	seedRootUser(ctx, s.User, opts, log)
}

// readFile อ่านจาก dir ถ้ากำหนดไว้ ไม่งั้นใช้ไฟล์ที่ embed มากับ binary
func readFile(dir, name string) ([]byte, error) {
	if dir != "" {
		return os.ReadFile(filepath.Join(dir, name))
	}
	return fs.ReadFile(embedded, name)
}

// --- configuration ---

type configurationFile struct {
	Terms         string `yaml:"terms"`
	PrivacyPolicy string `yaml:"privacyPolicy"`
	Token         *struct {
		AccessTokenExpired  int64 `yaml:"accessTokenExpired"`
		RefreshTokenExpired int64 `yaml:"refreshTokenExpired"`
		MaxSessions         int   `yaml:"maxSessions"`
	} `yaml:"token"`
	Register *struct {
		Enabled             bool   `yaml:"enabled"`
		TrialExpiresIn      *int64 `yaml:"trialExpiresIn"`
		DefaultPackageAlias string `yaml:"defaultPackageAlias"`
	} `yaml:"register"`
}

func seedConfiguration(ctx context.Context, store ConfigStore, dir string, log Logger) {
	if store == nil {
		return
	}
	if existing, err := store.Get(ctx); err == nil && existing != nil {
		log.Info("seed: configuration already present, skipping")
		return
	}

	raw, err := readFile(dir, fileConfiguration)
	if err != nil {
		log.Error("seed: read " + fileConfiguration + ": " + err.Error())
		return
	}
	var f configurationFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		log.Error("seed: parse " + fileConfiguration + ": " + err.Error())
		return
	}

	now := tool.NowUTC()
	doc := &model.Configuration{
		Terms:         f.Terms,
		PrivacyPolicy: f.PrivacyPolicy,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	if f.Token != nil {
		doc.Token = &model.ConfigToken{
			AccessTokenExpired:  f.Token.AccessTokenExpired,
			RefreshTokenExpired: f.Token.RefreshTokenExpired,
			MaxSessions:         f.Token.MaxSessions,
		}
	}
	if f.Register != nil {
		doc.Register = &model.ConfigRegister{
			Enabled:             f.Register.Enabled,
			TrialExpiresIn:      f.Register.TrialExpiresIn,
			DefaultPackageAlias: f.Register.DefaultPackageAlias,
		}
	}
	if err := store.Upsert(ctx, doc); err != nil {
		log.Error("seed: insert configuration: " + err.Error())
		return
	}
	log.Info("seed: configuration created")
}

// --- packages ---

type packagesFile struct {
	Packages []struct {
		Alias       string `yaml:"alias"`
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Enabled     bool   `yaml:"enabled"`
		Policies    *struct {
			RateLimit *struct {
				Enabled bool `yaml:"enabled"`
				Tiers   []struct {
					Limit     int `yaml:"limit"`
					WindowSec int `yaml:"windowSec"`
				} `yaml:"tiers"`
			} `yaml:"rateLimit"`
		} `yaml:"policies"`
	} `yaml:"packages"`
}

func seedPackages(ctx context.Context, store PackageStore, dir string, log Logger) {
	if store == nil {
		return
	}
	raw, err := readFile(dir, filePackages)
	if err != nil {
		log.Error("seed: read " + filePackages + ": " + err.Error())
		return
	}
	var f packagesFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		log.Error("seed: parse " + filePackages + ": " + err.Error())
		return
	}

	created, skipped := 0, 0
	for _, p := range f.Packages {
		if p.Alias == "" {
			log.Error("seed: package with no alias, skipping")
			continue
		}
		if existing, err := store.FindByAlias(ctx, p.Alias); err == nil && existing != nil {
			skipped++
			continue
		}
		now := tool.NowUTC()
		enabled := p.Enabled
		doc := &model.Package{
			ID:          bson.NewObjectID(),
			Name:        p.Name,
			Alias:       p.Alias,
			Description: p.Description,
			Enabled:     &enabled,
			CreatedAt:   &now,
			UpdatedAt:   &now,
		}
		if p.Policies != nil && p.Policies.RateLimit != nil {
			rl := &model.PackageRateLimit{
				Enabled:   p.Policies.RateLimit.Enabled,
				Algorithm: "sliding_window",
			}
			for _, t := range p.Policies.RateLimit.Tiers {
				rl.Tiers = append(rl.Tiers, &model.RateLimitTier{Limit: t.Limit, WindowSec: t.WindowSec})
			}
			doc.Policies = &model.PackagePolicies{RateLimit: rl}
		}
		if err := store.Insert(ctx, doc); err != nil {
			log.Error("seed: insert package " + p.Alias + ": " + err.Error())
			continue
		}
		created++
	}
	log.Info(fmt.Sprintf("seed: packages created=%d skipped=%d", created, skipped))
}

// --- root user ---

// seedRootUser creates the first administrator. The password comes from the
// environment or, when that is empty, is generated and printed once. A fresh
// clone must never boot with a password that is published in this repository.
func seedRootUser(ctx context.Context, store UserStore, opts Options, log Logger) {
	if store == nil {
		return
	}
	if opts.RootEmail == "" {
		log.Info("seed: ORYCA_API_ROOT_EMAIL is not set, no root account created")
		return
	}
	if roots, err := store.FindByRoles(ctx, []string{"root"}); err == nil && len(roots) > 0 {
		log.Info("seed: root user already present, skipping")
		return
	}
	if existing, err := store.FindByEmail(ctx, opts.RootEmail); err == nil && existing != nil {
		log.Info("seed: a user with the root email already exists, skipping")
		return
	}

	password := opts.RootPassword
	generated := password == ""
	if generated {
		// 12 bytes → 24 hex chars: no ambiguous characters to mistype out of a log
		p, err := tool.GenerateToken(12)
		if err != nil {
			log.Error("seed: generate root password: " + err.Error())
			return
		}
		password = p
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Error("seed: hash root password: " + err.Error())
		return
	}

	now := tool.NowUTC()
	verified := true
	enabled := true
	doc := &model.User{
		ID:          bson.NewObjectID(),
		AccountType: "email",
		Email:       opts.RootEmail,
		Username:    opts.RootEmail,
		Password:    string(hashed),
		FirstName:   "Root",
		DisplayName: "Root",
		Role:        "root",
		Verified:    &verified,
		Enabled:     &enabled,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	if err := store.Insert(ctx, doc); err != nil {
		log.Error("seed: insert root user: " + err.Error())
		return
	}

	if generated {
		printRootBanner(opts.RootEmail, password, log)
		return
	}
	log.Info("seed: root user created email=" + opts.RootEmail)
}

// printRootBanner prints the generated password once, boxed so it is findable in
// a busy compose log. It is never stored in plain text and never printed again.
func printRootBanner(email, password string, log Logger) {
	line := "═══════════════════════════════════════════════════════════════"
	log.Info(line)
	log.Info("  ROOT ACCOUNT CREATED. This password is shown ONCE.")
	log.Info("    email:    " + email)
	log.Info("    password: " + password)
	log.Info("  Sign in and change it, or set ORYCA_API_ROOT_PASSWORD yourself.")
	log.Info(line)
}
