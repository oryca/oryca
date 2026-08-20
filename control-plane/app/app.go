// Package app wires the control plane together and runs it.
//
// It lives here rather than in package main so that the whole server can be
// embedded in another program. A distribution that adds its own endpoints, or a
// test that needs the real thing. While cmd/oryca-control-plane stays a
// three-line entry point.
package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oryca/oryca/control-plane/cache"
	"github.com/oryca/oryca/control-plane/config"
	"github.com/oryca/oryca/control-plane/connection/mongodb"
	redisconn "github.com/oryca/oryca/control-plane/connection/redis"
	"github.com/oryca/oryca/control-plane/consumer"
	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/repository"
	"github.com/oryca/oryca/control-plane/router"
	"github.com/oryca/oryca/control-plane/seed"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Run starts the server and blocks until it is asked to shut down.
func Run() {
	cfg := config.Load()
	logger.Init("oryca-control-plane")

	client := connectMongo(cfg)
	db := client.Database(cfg.DBName)

	redisClient, cacheTTL, routingTTL := connectRedis(cfg)

	// Real-time gateway sync. Soft dependency: an empty channel or unreachable Redis just
	// means Publish() calls are no-ops and gateway stays on HTTP polling alone (see
	// service.GatewayEventPublisher).
	gwPublisher := service.NewGatewayEventPublisher(redisClient, cfg.SyncChannel)

	loadJWTKeys(cfg)

	repos := buildRepos(db)
	caches := buildCaches(redisClient, cacheTTL)

	svcs := buildServices(cfg, repos, caches, redisClient, routingTTL, gwPublisher)

	// Seed before any traffic arrives. First boot only, nothing existing is overwritten (see package seed)
	runSeed(cfg, repos)

	// Separate quiet GatewayRedisSync (nil publisher) for the boot-time bulk resync
	// below. It loops over every service/source individually, and each one going
	// through the notifying instance would event-storm the gateway (one reload_services
	// per entity, dozens at once) for what's just CP re-affirming state it already had,
	// not new information. Real admin actions still notify via svcs.gatewaySync.
	quietGatewaySync := service.NewGatewayRedisSync(redisClient, routingTTL, nil)
	go startBackgroundJobs(repos, caches, svcs, quietGatewaySync)

	// Log consumer. Moves the gateway's access logs from the Redis stream into Mongo, for the dashboard
	logConsumer := startLogConsumer(cfg, repos, redisClient)

	// Daily cron for scheduled notifications (an API key about to expire, and so on). A distributed
	// lock keeps it to one pod.
	notifyCron := service.NewNotificationCron(repos.apiKey, cache.NewDistributedLock(redisClient), svcs.notification)
	if err := notifyCron.Start(); err != nil {
		logger.Error("Failed to start notification cron: " + err.Error())
	}

	e, allowOrigins := buildEcho(cfg)
	router.Setup(e, router.Infra{
		MongoClient:     client,
		RedisClient:     redisClient,
		SessionCache:    caches.session,
		UserRepo:        repos.user,
		UserSessionRepo: repos.userSession,
	}, buildRouterServices(svcs), router.InternalConfig{
		Secret:                cfg.InternalSecret,
		SecretPrev:            cfg.InternalSecretPrev,
		ServiceRepo:           repos.gatewayService,
		SourceRepo:            repos.gatewaySource,
		ApiKeyRepo:            repos.apiKey,
		UserRepo:              repos.user,
		PackageRepo:           repos.pkg,
		PackageSvcRepo:        repos.packageSvcLink,
		TransformResponseRepo: repos.transformConfig,
		ApiHost:               os.Getenv("ORYCA_API_HOST"),
		Notifier:              svcs.notification,
		ConfigSvc:             svcs.config,
	}, allowOrigins)

	go func() {
		logger.Info("Starting ORYCA Control-Plane on " + cfg.AppHost + ":" + cfg.AppPort)
		if err := e.Start(":" + cfg.AppPort); err != nil && err != http.ErrServerClosed {
			logger.Info("Server error: " + err.Error())
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutCtx); err != nil {
		logger.Info("Server shutdown error: " + err.Error())
	}

	// stop the cron and let a running job finish before any connection closes
	cronShutCtx, cronCancel := context.WithTimeout(context.Background(), 10*time.Second)
	notifyCron.Stop(cronShutCtx)
	cronCancel()

	// stop the consumer before Mongo and Redis close. A pending batch still has to be written
	if logConsumer != nil {
		consumerShutCtx, consumerCancel := context.WithTimeout(context.Background(), 15*time.Second)
		logConsumer.Stop(consumerShutCtx)
		consumerCancel()
	}

	// wait for the background notification goroutine, so no insert or publish is cut off midway
	notifyShutCtx, notifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	service.WaitForBackgroundNotifications(notifyShutCtx)
	notifyCancel()

	if err := client.Disconnect(shutCtx); err != nil {
		logger.Info("MongoDB disconnect error: " + err.Error())
	}
	if err := redisClient.Close(); err != nil {
		logger.Info("Redis close error: " + err.Error())
	}
	gwPublisher.Close()
	logger.Info("Server gracefully stopped")
}

// --- connection helpers ---

func connectMongo(cfg *config.Config) *mongo.Client {
	client, err := mongodb.Connect(cfg.DBUrl)
	if err != nil {
		logger.Info("Failed to connect to MongoDB: " + err.Error())
		os.Exit(1)
	}
	return client
}

func connectRedis(cfg *config.Config) (*redis.Client, time.Duration, time.Duration) {
	redisDB, _ := strconv.Atoi(cfg.RedisDB)
	redisPool, _ := strconv.Atoi(cfg.RedisPoolSize)
	redisMinIdle, _ := strconv.Atoi(cfg.RedisMinIdleConns)
	redisDialTimeout, _ := strconv.Atoi(cfg.RedisDialTimeout)
	redisReadTimeout, _ := strconv.Atoi(cfg.RedisReadTimeout)
	redisWriteTimeout, _ := strconv.Atoi(cfg.RedisWriteTimeout)
	redisExpires, _ := strconv.Atoi(cfg.RedisExpires)
	redisRoutingTTL, _ := strconv.Atoi(cfg.RedisRoutingTTL)

	client, err := redisconn.Connect(redisconn.Options{
		Addr:         cfg.RedisAddress,
		Password:     cfg.RedisPassword,
		DB:           redisDB,
		PoolSize:     redisPool,
		MinIdleConns: redisMinIdle,
		DialTimeout:  time.Duration(redisDialTimeout) * time.Second,
		ReadTimeout:  time.Duration(redisReadTimeout) * time.Second,
		WriteTimeout: time.Duration(redisWriteTimeout) * time.Second,
	})
	if err != nil {
		logger.Info("Failed to connect to Redis: " + err.Error())
		os.Exit(1)
	}
	return client, time.Duration(redisExpires) * time.Second, time.Duration(redisRoutingTTL) * time.Second
}

// loadJWTKeys reads the internal JWT's RSA keys once, at startup
// startLogConsumer prepares the indexes (the TTL one included) and starts the consumer. Returns nil
// when it is switched off by env.
func startLogConsumer(cfg *config.Config, r appRepos, rc *redis.Client) *consumer.LogConsumer {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	retention := time.Duration(cfg.LogRetentionDays) * 24 * time.Hour
	if err := r.accessLog.EnsureIndexes(ctx, retention); err != nil {
		logger.Error("access log indexes: " + err.Error())
	}

	if !cfg.LogConsumerEnable {
		logger.Info("log consumer: disabled by ORYCA_API_LOG_CONSUMER_ENABLED")
		return nil
	}

	// consumer names have to be unique across pods, and a container's hostname already is
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = "control-plane"
	}
	// the loop's ctx has to live as long as the process. The one above times out, and is for setup only
	lc := consumer.NewLogConsumer(rc, r.accessLog, name)
	if err := lc.Start(context.Background()); err != nil {
		logger.Error("log consumer: start failed, dashboard will have no data: " + err.Error())
		return nil
	}
	return lc
}

// seedLogger adapts the shared logger to the narrow interface package seed asks for
type seedLogger struct{}

func (seedLogger) Info(msg string)  { logger.Info(msg) }
func (seedLogger) Error(msg string) { logger.Error(msg) }

// runSeed loads the starting data from the YAML files. Idempotent, so it is safe on every boot.
func runSeed(cfg *config.Config, r appRepos) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seed.Run(ctx, seed.Stores{
		Config:        r.config,
		EmailTemplate: r.emailTemplate,
		Package:       r.pkg,
		User:          r.user,
	}, seed.Options{
		Dir:          cfg.SeedDir,
		RootEmail:    cfg.RootEmail,
		RootPassword: cfg.RootPassword,
	}, seedLogger{})
}

func loadJWTKeys(cfg *config.Config) {
	if err := tool.InitAPIJWTKeys(cfg.JWTPrivateKey, cfg.JWTPublicKey); err != nil {
		logger.Error("Failed to load API JWT keys: " + err.Error())
		os.Exit(1)
	}
}

// --- wiring structs ---

type appRepos struct {
	config          *repository.ConfigurationRepository
	mailServer      *repository.MailServerRepository
	emailTemplate   *repository.EmailTemplateRepository
	user            *repository.UserRepository
	userSession     *repository.UserSessionRepository
	apiKey          *repository.ApiKeyRepository
	gatewaySource   *repository.GatewaySourceRepository
	gatewayService  *repository.GatewayServiceRepository
	gatewaySpec     *repository.GatewaySpecRepository
	pkg             *repository.PackageRepository
	packageSvcLink  *repository.PackageSvcLinkRepository
	transformConfig *repository.TransformResponseConfigRepository
	notification    *repository.NotificationRepository
	apiDoc          *repository.ApiDocRepository
	accessLog       *repository.AccessLogRepository
}

type appCaches struct {
	config      *cache.ConfigurationCache
	mailServer  *cache.MailServerCache
	apiKey      *cache.ApiKeyCache
	setPassword *cache.SetPasswordCache
	session     *cache.SessionCache
	verifyEmail *cache.VerifyEmailCache
}

type appServices struct {
	config          *service.ConfigurationService
	mailServer      *service.MailServerService
	emailTemplate   *service.EmailTemplateService
	user            *service.UserService
	auth            *service.AuthService
	setPassword     *service.SetPasswordService
	verifyEmail     *service.VerifyEmailService
	account         *service.AccountService
	apiKey          *service.ApiKeyService
	gatewaySync     *service.GatewayRedisSync
	gatewaySource   *service.GatewaySourceService
	gatewayService  *service.GatewayServiceService
	gatewaySpec     *service.GatewaySpecService
	pkg             *service.PackageService
	packageSvcLink  *service.PackageSvcLinkService
	packageUser     *service.PackageUserService
	transformConfig *service.TransformResponseConfigService
	notification    *service.NotificationService
	apiDoc          *service.ApiDocService
	dashboard       *service.DashboardService
}

func buildRepos(db interface { /* *mongo.Database */
}) appRepos {
	d := db.(*mongo.Database)
	return appRepos{
		config:          repository.NewConfigurationRepository(d),
		mailServer:      repository.NewMailServerRepository(d),
		emailTemplate:   repository.NewEmailTemplateRepository(d),
		user:            repository.NewUserRepository(d),
		userSession:     repository.NewUserSessionRepository(d),
		apiKey:          repository.NewApiKeyRepository(d),
		gatewaySource:   repository.NewGatewaySourceRepository(d),
		gatewayService:  repository.NewGatewayServiceRepository(d),
		gatewaySpec:     repository.NewGatewaySpecRepository(d),
		pkg:             repository.NewPackageRepository(d),
		packageSvcLink:  repository.NewPackageSvcLinkRepository(d),
		transformConfig: repository.NewTransformConfigRepository(d),
		notification:    repository.NewNotificationRepository(d),
		apiDoc:          repository.NewApiDocRepository(d),
		accessLog:       repository.NewAccessLogRepository(d),
	}
}

func buildCaches(rc *redis.Client, ttl time.Duration) appCaches {
	return appCaches{
		config:      cache.NewConfigurationCache(rc, ttl),
		mailServer:  cache.NewMailServerCache(rc, ttl),
		apiKey:      cache.NewApiKeyCache(rc, ttl),
		setPassword: cache.NewSetPasswordCache(rc),
		session:     cache.NewSessionCache(rc),
		verifyEmail: cache.NewVerifyEmailCache(rc),
	}
}

func buildServices(cfg *config.Config, r appRepos, c appCaches, rc *redis.Client, routingTTL time.Duration, gwPublisher *service.GatewayEventPublisher) appServices {
	mailSvc := service.NewMailService(cfg.SMTP)
	configSvc := service.NewConfigurationService(r.config, c.config)
	setPasswordSvc := service.NewSetPasswordService(c.setPassword, r.user, mailSvc, time.Duration(cfg.MailResetTTLMin)*time.Minute)
	verifyEmailSvc := service.NewVerifyEmailService(c.verifyEmail, r.user, mailSvc, gwPublisher, time.Duration(cfg.MailVerifyTTLMin)*time.Minute)
	gatewaySync := service.NewGatewayRedisSync(rc, routingTTL, gwPublisher)

	notificationSvc := service.NewNotificationService(r.notification, rc)

	return appServices{
		config:          configSvc,
		mailServer:      service.NewMailServerService(r.mailServer, c.mailServer),
		emailTemplate:   service.NewEmailTemplateService(r.emailTemplate),
		user:            service.NewUserService(r.user, setPasswordSvc, gwPublisher),
		auth:            service.NewAuthService(configSvc, r.user, r.pkg, c.session, r.userSession, c.session, verifyEmailSvc),
		setPassword:     setPasswordSvc,
		verifyEmail:     verifyEmailSvc,
		account:         service.NewAccountService(r.user, c.session, r.userSession),
		apiKey:          service.NewApiKeyService(r.apiKey, c.apiKey, r.user, notificationSvc, gwPublisher),
		gatewaySync:     gatewaySync,
		gatewaySource:   service.NewGatewaySourceService(r.gatewaySource, r.gatewayService, gatewaySync),
		gatewayService:  service.NewGatewayServiceService(r.gatewayService, r.gatewaySource, gatewaySync),
		gatewaySpec:     service.NewGatewaySpecService(r.gatewaySpec),
		pkg:             service.NewPackageService(r.pkg),
		packageSvcLink:  service.NewPackageSvcLinkService(r.packageSvcLink, r.gatewayService, r.pkg, r.pkg, gatewaySync, r.user, notificationSvc),
		packageUser:     service.NewPackageUserService(r.user, r.pkg, r.pkg, notificationSvc, gwPublisher),
		transformConfig: service.NewTransformConfigService(r.transformConfig, gwPublisher),
		notification:    notificationSvc,
		apiDoc:          service.NewApiDocService(r.apiDoc),
		dashboard:       service.NewDashboardService(r.accessLog, r.gatewayService),
	}
}

func buildRouterServices(s appServices) router.Services {
	return router.Services{
		Config:                  s.config,
		MailServer:              s.mailServer,
		EmailTemplate:           s.emailTemplate,
		User:                    s.user,
		Auth:                    s.auth,
		SetPassword:             s.setPassword,
		VerifyEmail:             s.verifyEmail,
		Account:                 s.account,
		ApiKey:                  s.apiKey,
		GatewaySource:           s.gatewaySource,
		GatewayService:          s.gatewayService,
		GatewaySpec:             s.gatewaySpec,
		Package:                 s.pkg,
		PackageSvcLink:          s.packageSvcLink,
		PackageUser:             s.packageUser,
		TransformResponseConfig: s.transformConfig,
		Notification:            s.notification,
		Dashboard:               s.dashboard,
		ApiDoc:                  s.apiDoc,
	}
}

func buildEcho(cfg *config.Config) (*echo.Echo, []string) {
	e := echo.New()
	e.HideBanner = true
	// e.IPExtractor is left unset, so Echo's default applies and X-Forwarded-For is trusted. The IP is
	// only written to the log today, and nothing is decided from it. Should anything ever decide from
	// it, set an IPExtractor that matches the real topology first.
	// The timeouts stop a slow client from holding a connection open.
	e.Server.ReadHeaderTimeout = envSeconds(cfg.ReadHeaderTimeout, 10)
	e.Server.ReadTimeout = envSeconds(cfg.ReadTimeout, 60)
	e.Server.IdleTimeout = envSeconds(cfg.IdleTimeout, 120)

	// cap the request body
	e.Use(middleware.BodyLimit(cfg.BodyLimit))
	allowOrigins := strings.Split(cfg.AllowOrigin, ",")
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowOrigins,
		AllowMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin, echo.HeaderAccept,
			echo.HeaderContentType, echo.HeaderAuthorization,
		},
	}))
	return e, allowOrigins
}

// envSeconds reads an env value in seconds as a Duration, falling back to the default
func envSeconds(val string, defaultSec int) time.Duration {
	if sec, err := strconv.Atoi(val); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return time.Duration(defaultSec) * time.Second
}

// envTrue reads an env value as a bool, accepting "1", "TRUE" and "True" as well. A safety switch
// should not fall silent over capitalisation. Anything it cannot parse is false (observe mode),
// which is safer than enforcing something nobody asked for.
func envTrue(val string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(val))
	return err == nil && b
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// startBackgroundJobs runs all startup background tasks. Called in a single goroutine.
func startBackgroundJobs(r appRepos, c appCaches, s appServices, quietGatewaySync *service.GatewayRedisSync) {
	ctx := context.Background()

	go syncPackageUserCounts(ctx, r)
	go syncPackageServiceCounts(ctx, r)
	go syncApiKeysToRedis(ctx, r, c)
	go syncGatewayDataToRedis(ctx, r, s, quietGatewaySync)
	go syncNotificationIndexes(ctx, r)
}

func syncNotificationIndexes(ctx context.Context, r appRepos) {
	if err := r.notification.EnsureIndexes(ctx); err != nil {
		logger.Error("notification indexes: " + err.Error())
	}
}

func syncPackageUserCounts(ctx context.Context, r appRepos) {
	if err := r.pkg.SyncUserCounts(ctx); err != nil {
		logger.Error("package userCount sync: " + err.Error())
	}
}

func syncPackageServiceCounts(ctx context.Context, r appRepos) {
	if err := r.pkg.SyncServiceCounts(ctx); err != nil {
		logger.Error("package serviceCount sync: " + err.Error())
	}
}

func syncApiKeysToRedis(ctx context.Context, r appRepos, c appCaches) {
	keys, err := r.apiKey.FindAllActive(ctx)
	if err != nil {
		logger.Error("apikey sync: " + err.Error())
		return
	}

	// fetch every owner in one query, rather than a FindByID per api-key (N+1)
	seen := make(map[bson.ObjectID]struct{})
	ownerIDs := make([]bson.ObjectID, 0, len(keys))
	for _, ak := range keys {
		if ak.OwnerBy == nil {
			continue
		}
		if _, dup := seen[*ak.OwnerBy]; dup {
			continue
		}
		seen[*ak.OwnerBy] = struct{}{}
		ownerIDs = append(ownerIDs, *ak.OwnerBy)
	}
	owners, err := r.user.FindByIDs(ctx, ownerIDs)
	if err != nil {
		logger.Error("apikey sync: fetch owners failed: " + err.Error())
		return
	}
	ownerByID := make(map[bson.ObjectID]*model.User, len(owners))
	for _, o := range owners {
		ownerByID[o.ID] = o
	}

	// write to Redis in pipelined chunks instead of one key at a time, taking the round trips from
	// O(n) to O(n/500)
	entries := make([]cache.ApiKeyEntry, len(keys))
	for i, ak := range keys {
		var owner *model.User
		if ak.OwnerBy != nil {
			owner = ownerByID[*ak.OwnerBy]
		}
		entries[i] = cache.ApiKeyEntry{ApiKey: ak, Owner: owner}
	}
	failed, err := c.apiKey.SetBatch(ctx, entries)
	if err != nil {
		logger.Error("apikey sync: set batch failed: " + err.Error())
	}
	if failed > 0 {
		logger.Error("apikey sync: done with failed=" + strconv.Itoa(failed))
	} else {
		logger.Info("apikey sync: synced count=" + strconv.Itoa(len(keys)))
	}
}

func syncGatewayDataToRedis(ctx context.Context, r appRepos, s appServices, quietGatewaySync *service.GatewayRedisSync) {
	syncGatewayServices(ctx, r, s, quietGatewaySync)
	syncGatewaySources(ctx, s, quietGatewaySync)
}

func syncGatewayServices(ctx context.Context, r appRepos, s appServices, quietGatewaySync *service.GatewayRedisSync) {
	svcs, err := s.gatewayService.ListAllActive(ctx)
	if err != nil {
		logger.Error("gateway sync: list services failed: " + err.Error())
		return
	}
	failed := 0
	for _, svc := range svcs {
		if err := quietGatewaySync.SyncService(ctx, svc); err != nil {
			failed++
		}
	}
	if failed > 0 {
		logger.Error("gateway sync: services done, failed=" + strconv.Itoa(failed))
	} else {
		logger.Info("gateway sync: services synced count=" + strconv.Itoa(len(svcs)))
	}
	for _, svc := range svcs {
		ids, err := r.packageSvcLink.FindPackageIDsByServiceID(ctx, svc.ID)
		if err != nil {
			continue
		}
		_ = quietGatewaySync.SyncServicePackageIDs(ctx, svc, ids)
	}
	logger.Info("gateway sync: package IDs synced for services count=" + strconv.Itoa(len(svcs)))
}

func syncGatewaySources(ctx context.Context, s appServices, quietGatewaySync *service.GatewayRedisSync) {
	srcs, err := s.gatewaySource.ListAllActive(ctx)
	if err != nil {
		logger.Error("gateway sync: list sources failed: " + err.Error())
		return
	}
	failed := 0
	for _, src := range srcs {
		if err := quietGatewaySync.SyncSource(ctx, src); err != nil {
			failed++
		}
	}
	if failed > 0 {
		logger.Error("gateway sync: sources done, failed=" + strconv.Itoa(failed))
	} else {
		logger.Info("gateway sync: sources synced count=" + strconv.Itoa(len(srcs)))
	}
}
