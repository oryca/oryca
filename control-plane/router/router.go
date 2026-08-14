package router

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"os"

	"github.com/oryca/oryca/control-plane/cache"
	"github.com/oryca/oryca/control-plane/handler"
	"github.com/oryca/oryca/control-plane/middleware"
	"github.com/oryca/oryca/control-plane/repository"
	"github.com/oryca/oryca/control-plane/service"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	routeProfile           = "/profile"
	routeMailServerID      = "/:mailServerId"
	routeEmailTemplateID   = "/:emailTemplateId"
	routeUserID            = "/:userId"
	routeApiKeyID          = "/:apiKeyId"
	routeSourceID          = "/:sourceId"
	routeServiceID         = "/:serviceId"
	routePackageID         = "/:packageId"
	routeServicesBase      = "/services"
	routePackageServiceID  = routeServicesBase + "/:serviceId"
	routeTransformConfigID = "/:transformConfigId"
	routeNotificationID    = "/:notificationId"
	routeApiDocBase        = "/api-docs"
	routeApiDocID          = "/:apiDocId"
)

type Services struct {
	Config                  *service.ConfigurationService
	MailServer              *service.MailServerService
	EmailTemplate           *service.EmailTemplateService
	User                    *service.UserService
	Auth                    *service.AuthService
	SetPassword             *service.SetPasswordService
	VerifyEmail             *service.VerifyEmailService
	Account                 *service.AccountService
	ApiKey                  *service.ApiKeyService
	GatewaySource           *service.GatewaySourceService
	GatewayService          *service.GatewayServiceService
	GatewaySpec             *service.GatewaySpecService
	Package                 *service.PackageService
	PackageSvcLink          *service.PackageSvcLinkService
	PackageUser             *service.PackageUserService
	TransformResponseConfig *service.TransformResponseConfigService
	Notification            *service.NotificationService
	Dashboard               *service.DashboardService
	ApiDoc                  *service.ApiDocService
}

type Infra struct {
	MongoClient     *mongo.Client
	RedisClient     *redis.Client
	SessionCache    *cache.SessionCache
	UserRepo        *repository.UserRepository
	UserSessionRepo *repository.UserSessionRepository
}

type InternalConfig struct {
	Secret                string
	SecretPrev            string
	ServiceRepo           *repository.GatewayServiceRepository
	SourceRepo            *repository.GatewaySourceRepository
	ApiKeyRepo            *repository.ApiKeyRepository
	UserRepo              *repository.UserRepository
	PackageRepo           *repository.PackageRepository
	PackageSvcRepo        *repository.PackageSvcLinkRepository
	TransformResponseRepo *repository.TransformResponseConfigRepository
	ApiHost               string
	Notifier              *service.NotificationService
	ConfigSvc             *service.ConfigurationService
}

func Setup(e *echo.Echo, infra Infra, svcs Services, internalCfg InternalConfig, allowedOrigins []string) {
	jwtMW := middleware.NewJWTMiddleware(infra.SessionCache, infra.UserRepo)
	apiKeyMW := middleware.NewApiKeyMiddleware(svcs.ApiKey, infra.UserRepo)

	cp := e.Group("/control-plane")
	v1 := cp.Group("/api/v1", middleware.AccessLog())

	healthHandler := handler.NewHealthHandler(infra.MongoClient, infra.RedisClient)
	v1.GET("/", healthHandler.Check)
	v1.GET("/health", healthHandler.Check)

	// JWKS — register both paths: with and without /control-plane prefix (depends on ingress strip-prefix config)
	jwksHandler := func(c echo.Context) error {
		keyPath := os.Getenv("ORYCA_API_JWT_PUBLIC_KEY")
		pemBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "cannot read public key"})
		}

		block, _ := pem.Decode(pemBytes)
		if block == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "invalid PEM"})
		}

		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "cannot parse public key"})
		}

		der, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "cannot marshal public key"})
		}

		return c.JSON(http.StatusOK, map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"use": "sig",
					"alg": "RS256",
					"kid": "oryca-default",
					"x5c": []string{base64.StdEncoding.EncodeToString(der)},
				},
			},
		})
	}

	internalMW := middleware.InternalKeyMiddleware(internalCfg.Secret, internalCfg.SecretPrev)
	v1.GET("/.well-known/jwks.json", jwksHandler, internalMW)

	// Internal — X-Internal-Key
	internalHandler := handler.NewInternalHandler(handler.InternalHandlerConfig{
		ServiceRepo:    internalCfg.ServiceRepo,
		SourceRepo:     internalCfg.SourceRepo,
		ApiKeyRepo:     internalCfg.ApiKeyRepo,
		UserRepo:       internalCfg.UserRepo,
		PackageRepo:    internalCfg.PackageRepo,
		PackageSvcRepo: internalCfg.PackageSvcRepo,
		TransformRepo:  internalCfg.TransformResponseRepo,
	})

	internal := v1.Group("/internal", internalMW)
	internal.GET("/services", internalHandler.GetServices)
	internal.GET("/sources", internalHandler.GetSources)
	internal.GET("/api-keys", internalHandler.GetApiKeys)
	internal.GET("/response-transforms", internalHandler.GetTransformConfigs)
	internal.POST("/resolve-names", internalHandler.ResolveNames)
	internal.GET("/users/:id", internalHandler.GetUser)

	// Auth — public
	authHandler := handler.NewAuthHandler(handler.AuthHandlerDeps{
		AuthSvc:        svcs.Auth,
		SetPasswordSvc: svcs.SetPassword,
		VerifyEmailSvc: svcs.VerifyEmail,
		AllowedOrigins: allowedOrigins,
	})
	auth := v1.Group("/auth")
	auth.POST("/login", authHandler.Login)
	auth.POST("/register", authHandler.Register)
	auth.POST("/token", authHandler.RefreshToken)
	auth.GET("/verify-email", authHandler.VerifyEmail)
	auth.POST("/forgot-password", authHandler.ForgotPassword)
	auth.GET("/set-password", authHandler.VerifySetPasswordToken)
	auth.POST("/set-password", authHandler.SetPassword)
	auth.POST("/verify-email", authHandler.ResendVerifyEmail)
	auth.POST("/setting-password", authHandler.SettingPassword)

	auth.POST("/logout", authHandler.Logout, jwtMW.Authenticate)

	// Account — GET profile also allows API key auth
	accountHandler := handler.NewAccountHandler(svcs.Account)
	v1.GET("/account"+routeProfile, accountHandler.GetProfile, middleware.AllowJWTOrApiKey(jwtMW, apiKeyMW))
	account := v1.Group("/account", jwtMW.Authenticate)
	account.POST("/change-password", accountHandler.ChangePassword)
	account.PUT(routeProfile, accountHandler.UpdateProfile)
	account.DELETE(routeProfile, accountHandler.DeleteProfile)
	account.GET("/sessions", accountHandler.GetSessions)
	account.GET("/sessions/history", accountHandler.GetSessionHistory)
	account.DELETE("/sessions/:sessionId", accountHandler.DeleteSession)
	account.DELETE("/sessions", accountHandler.DeleteAllSessions)

	// Configuration — GET public (optional auth), PATCH root only
	configurationHandler := handler.NewConfigurationHandler(svcs.Config)
	v1.GET("/configuration", configurationHandler.GetConfiguration, jwtMW.OptionalAuthenticate)

	// Mail Servers
	mailServerHandler := handler.NewMailServerHandler(svcs.MailServer)
	mailServers := v1.Group("/mail-servers", jwtMW.Authenticate)
	mailServers.GET("", mailServerHandler.GetMailServers)
	mailServers.GET(routeMailServerID, mailServerHandler.GetMailServer)
	mailServers.POST("", mailServerHandler.CreateMailServer)
	mailServers.PUT(routeMailServerID, mailServerHandler.UpdateMailServer)
	mailServers.DELETE(routeMailServerID, mailServerHandler.DeleteMailServer)

	// Email Templates
	emailTemplateHandler := handler.NewEmailTemplateHandler(svcs.EmailTemplate)
	emailTemplates := v1.Group("/email-templates", jwtMW.Authenticate)
	emailTemplates.GET("", emailTemplateHandler.GetEmailTemplates)
	emailTemplates.GET(routeEmailTemplateID, emailTemplateHandler.GetEmailTemplate)

	// Users
	userHandler := handler.NewUserHandler(svcs.User)
	users := v1.Group("/users", jwtMW.Authenticate)
	users.GET("", userHandler.GetUsers)
	users.GET(routeUserID, userHandler.GetUser)
	users.GET(routeUserID+"/public", userHandler.GetUserPublic)
	users.POST("", userHandler.CreateUser)
	users.PUT(routeUserID, userHandler.UpdateUser)
	users.DELETE(routeUserID, userHandler.DeleteUser)
	users.DELETE("", userHandler.BulkDeleteUsers)
	// API Keys — GET by ID also allows API key auth
	apiKeyHandler := handler.NewApiKeyHandler(svcs.ApiKey, svcs.User)
	apiKeys := v1.Group("/api-keys", jwtMW.Authenticate)
	apiKeys.GET("", apiKeyHandler.GetApiKeys)
	apiKeys.POST("", apiKeyHandler.CreateApiKey)
	apiKeys.PUT(routeApiKeyID, apiKeyHandler.UpdateApiKey)
	apiKeys.POST(routeApiKeyID+"/regenerate", apiKeyHandler.RegenerateApiKey)
	apiKeys.DELETE(routeApiKeyID, apiKeyHandler.DeleteApiKey)
	apiKeys.DELETE("", apiKeyHandler.BulkDeleteApiKeys)

	// GET by ID — allow both JWT and API key auth
	v1.GET("/api-keys"+routeApiKeyID, apiKeyHandler.GetApiKey, middleware.AllowJWTOrApiKey(jwtMW, apiKeyMW))

	// Notifications — Stream ใช้ one-time ticket แทน JWT header (EventSource แนบ custom header ไม่ได้)
	notificationHandler := handler.NewNotificationHandler(svcs.Notification)
	notifications := v1.Group("/notifications", jwtMW.Authenticate)
	notifications.GET("", notificationHandler.GetNotifications)
	notifications.GET("/unread-count", notificationHandler.GetUnreadCount)
	notifications.PUT(routeNotificationID+"/read", notificationHandler.MarkAsRead)
	notifications.PUT("/read-all", notificationHandler.MarkAllAsRead)
	notifications.DELETE(routeNotificationID, notificationHandler.DeleteNotification)
	notifications.POST("/stream-ticket", notificationHandler.IssueStreamTicket)

	// Stream — auth ผ่าน ticket query param เท่านั้น ไม่ผ่าน jwtMW
	v1.GET("/notifications/stream", notificationHandler.Stream)

	// Dashboard — every signed-in user sees their own traffic; root/admin see everything
	dashboardHandler := handler.NewDashboardHandler(svcs.Dashboard)
	dashboard := v1.Group("/dashboard", jwtMW.Authenticate)
	dashboard.GET("/stats", dashboardHandler.GetStats)
	dashboard.GET("/summary", dashboardHandler.GetSummary)
	dashboard.GET("/logs", dashboardHandler.GetLogs)

	// Gateway Sources
	gatewaySourceHandler := handler.NewGatewaySourceHandler(svcs.GatewaySource)
	sources := v1.Group("/sources", jwtMW.Authenticate)
	sources.GET("", gatewaySourceHandler.GetSources)
	sources.GET(routeSourceID, gatewaySourceHandler.GetSource)
	sources.POST("", gatewaySourceHandler.CreateSource)
	sources.PUT(routeSourceID, gatewaySourceHandler.UpdateSource)
	sources.DELETE(routeSourceID, gatewaySourceHandler.DeleteSource)

	// Gateway Services
	gatewayServiceHandler := handler.NewGatewayServiceHandler(svcs.GatewayService)
	gatewaySpecHandler := handler.NewGatewaySpecHandler(svcs.GatewaySpec, svcs.GatewayService)
	services := v1.Group(routeServicesBase, jwtMW.Authenticate)
	services.GET("", gatewayServiceHandler.GetServices)
	services.GET(routeServiceID, gatewayServiceHandler.GetService)
	services.POST("", gatewayServiceHandler.CreateService)
	services.POST("/check-paths", gatewayServiceHandler.CheckResourcePaths)
	services.PUT(routeServiceID, gatewayServiceHandler.UpdateService)
	services.DELETE(routeServiceID, gatewayServiceHandler.DeleteService)
	services.GET(routeServiceID+"/spec", gatewaySpecHandler.GetSpec)
	services.PUT(routeServiceID+"/spec", gatewaySpecHandler.UpsertSpec)
	services.DELETE(routeServiceID+"/spec", gatewaySpecHandler.DeleteSpec)

	// GET /spec/openapi — allow both JWT and API key (future: called by data-plane)
	v1.GET(routeServicesBase+routeServiceID+"/spec/openapi", gatewaySpecHandler.GetOpenAPI, middleware.AllowJWTOrApiKey(jwtMW, apiKeyMW))

	// Packages
	packageHandler := handler.NewPackageHandler(svcs.Package, svcs.PackageSvcLink, svcs.PackageUser)
	packages := v1.Group("/packages", jwtMW.Authenticate)
	packages.GET("", packageHandler.GetPackages)
	packages.GET(routePackageID, packageHandler.GetPackage)
	packages.POST("", packageHandler.CreatePackage)
	packages.PUT(routePackageID, packageHandler.UpdatePackage)
	packages.DELETE(routePackageID, packageHandler.DeletePackage)
	packages.GET(routePackageID+routeServicesBase, packageHandler.GetPackageServices)
	packages.GET(routePackageID+routePackageServiceID, packageHandler.GetPackageService)
	packages.POST(routePackageID+routeServicesBase, packageHandler.CreatePackageService)
	packages.POST(routePackageID+routeServicesBase+"/check-paths", packageHandler.CheckPackageServicePaths)
	packages.PUT(routePackageID+routePackageServiceID, packageHandler.UpdatePackageService)
	packages.DELETE(routePackageID+routePackageServiceID, packageHandler.DeletePackageService)
	packages.GET(routePackageID+"/users", packageHandler.GetPackageUsers)
	packages.POST(routePackageID+"/users", packageHandler.AssignPackageUsers)
	packages.DELETE(routePackageID+"/users", packageHandler.RemovePackageUsers)

	// Response Transforms
	transformConfigHandler := handler.NewTransformConfigHandler(svcs.TransformResponseConfig, svcs.GatewayService, internalCfg.SourceRepo)
	transformConfigs := v1.Group("/response-transforms", jwtMW.Authenticate)
	transformConfigs.GET("", transformConfigHandler.List)
	transformConfigs.GET("/variables", transformConfigHandler.Variables)
	transformConfigs.GET("/presets", transformConfigHandler.Presets)
	transformConfigs.POST("/presets/:presetName/apply", transformConfigHandler.ApplyPreset)
	transformConfigs.GET(routeTransformConfigID, transformConfigHandler.Get)
	transformConfigs.POST("", transformConfigHandler.Create)
	transformConfigs.PUT(routeTransformConfigID, transformConfigHandler.Update)
	transformConfigs.DELETE(routeTransformConfigID, transformConfigHandler.Delete)

	// Response ApiDocs
	apiDocHandler := handler.NewApiDocHandler(svcs.ApiDoc, internalCfg.ApiHost)
	apiDocs := v1.Group(routeApiDocBase)
	apiDocs.GET("", apiDocHandler.GetApiDocs, jwtMW.Authenticate)
	apiDocs.GET(routeApiDocID, apiDocHandler.GetApiDoc, jwtMW.Authenticate)
	apiDocs.POST("", apiDocHandler.CreateApiDoc, jwtMW.Authenticate)
	apiDocs.PUT(routeApiDocID, apiDocHandler.UpdateApiDoc, jwtMW.Authenticate)
	apiDocs.DELETE(routeApiDocID, apiDocHandler.DeleteApiDoc, jwtMW.Authenticate)
	apiDocs.GET(routeApiDocID+"/paths", apiDocHandler.GetApiDocPaths, jwtMW.Authenticate)

}
