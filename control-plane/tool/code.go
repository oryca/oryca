package tool

const (
	// === Request Validation ===
	CodeBodyInvalidFormat       = "BODY_INVALID_FORMAT"
	CodeBodyIsRequired          = "BODY_IS_REQUIRED"
	CodeParamRequired           = "PARAM_REQUIRED"
	CodeParamInvalidFormat      = "PARAM_INVALID_FORMAT"
	CodeQueryParamRequired      = "QUERY_PARAM_REQUIRED"
	CodeQueryParamInvalidFormat = "QUERY_PARAM_INVALID_FORMAT"

	// === Auth - General ===
	CodeIncorrectPassword  = "INCORRECT_PASSWORD"
	CodeRegisterNotEnabled = "REGISTER_NOT_ENABLED"
	CodeTrialExpired       = "TRIAL_EXPIRED"

	// === Auth - Token ===
	CodeInvalidToken          = "INVALID_TOKEN"
	CodeInvalidRefreshToken   = "INVALID_REFRESH_TOKEN"
	CodeTokenGenerationError  = "TOKEN_GENERATION_ERROR"
	CodeTokenInvalidOrExpired = "TOKEN_INVALID_OR_EXPIRED"
	CodeSessionExpired        = "SESSION_EXPIRED"
	CodeSessionNotFound       = "SESSION_NOT_FOUND"

	// === Auth - OTP ===
	CodeOtpSendError        = "OTP_SEND_ERROR"
	CodeOtpInvalid          = "OTP_INVALID"
	CodeOtpProviderNotFound = "OTP_PROVIDER_NOT_FOUND"

	// === Auth - IDP ===
	CodeIdpNotFound       = "IDP_NOT_FOUND"
	CodeIdpConfigInvalid  = "IDP_CONFIG_INVALID"
	CodeIdpRequestError   = "IDP_REQUEST_ERROR"
	CodeIdpStateInvalid   = "IDP_STATE_INVALID"
	CodeIdpEmailConflict  = "IDP_EMAIL_CONFLICT"
	CodeIdpAliasDuplicate = "IDP_ALIAS_DUPLICATE"

	// === Auth Method ===
	CodeAuthMethodNotFound      = "AUTH_METHOD_NOT_FOUND"
	CodeAuthMethodAlreadyLinked = "AUTH_METHOD_ALREADY_LINKED"

	// === User - Status ===
	CodeUserNotFound    = "USER_NOT_FOUND"
	CodeUserNotVerified = "USER_NOT_VERIFIED"
	CodeUserNotEnabled  = "USER_NOT_ENABLED"

	// === User - Duplicate ===
	CodeEmailDuplicate    = "EMAIL_DUPLICATE"
	CodePhoneDuplicate    = "PHONE_DUPLICATE"
	CodeUsernameDuplicate = "USERNAME_DUPLICATE"

	// === User - Permission ===
	CodeUnauthorizedAccess   = "UNAUTHORIZED_ACCESS"
	CodeCannotUpdateProfile  = "CANNOT_UPDATE_PROFILE"
	CodeCannotDeleteAccount  = "CANNOT_DELETE_ACCOUNT"
	CodeCannotChangePassword = "CANNOT_CHANGE_PASSWORD"
	CodeUserRoleInvalid      = "USER_ROLE_INVALID"

	// === Configuration ===
	CodeConfigNotFound    = "CONFIG_NOT_FOUND"
	CodeConfigUpdateError = "CONFIG_UPDATE_ERROR"

	// === API Key ===
	CodeApiKeyNotFound    = "API_KEY_NOT_FOUND"
	CodeApiKeyDisabled    = "API_KEY_DISABLED"
	CodeApiKeyExpired     = "API_KEY_EXPIRED"
	CodeApiKeyForbidden   = "API_KEY_FORBIDDEN"
	CodeIPNotAllowed      = "IP_NOT_ALLOWED"
	CodeRefererRequired   = "REFERER_REQUIRED"
	CodeRefererNotAllowed = "REFERER_NOT_ALLOWED"

	// === Gateway Source ===
	CodeGatewaySourceNotFound       = "GATEWAY_SOURCE_NOT_FOUND"
	CodeGatewaySourceAliasDuplicate = "GATEWAY_SOURCE_ALIAS_DUPLICATE"
	CodeGatewaySourceAliasInUse     = "GATEWAY_SOURCE_ALIAS_IN_USE"

	// === Gateway Service ===
	CodeGatewayServiceNotFound             = "GATEWAY_SERVICE_NOT_FOUND"
	CodeGatewayServiceBasePathDuplicate    = "GATEWAY_SERVICE_BASE_PATH_DUPLICATE"
	CodeGatewayServiceResourcePathConflict = "GATEWAY_SERVICE_RESOURCE_PATH_CONFLICT"
	CodeGatewayServiceSourceAliasInvalid   = "GATEWAY_SERVICE_SOURCE_ALIAS_INVALID"

	// === Gateway Spec ===
	CodeGatewaySpecNotFound = "GATEWAY_SPEC_NOT_FOUND"

	// === Package ===
	CodePackageNotFound       = "PACKAGE_NOT_FOUND"
	CodePackageAliasDuplicate = "PACKAGE_ALIAS_DUPLICATE"

	// === Package Service ===
	CodePackageServiceNotFound  = "PACKAGE_SERVICE_NOT_FOUND"
	CodePackageServiceDuplicate = "PACKAGE_SERVICE_DUPLICATE"
	CodePackagePathInvalid      = "PACKAGE_PATH_INVALID"

	// === Package Users ===
	CodePackageUserAssigned = "PACKAGE_USER_ASSIGNED"
	CodePackageUserRemoved  = "PACKAGE_USER_REMOVED"

	// === System ===
	CodeOperationFailed   = "OPERATION_FAILED"
	CodeTransactionError  = "TRANSACTION_ERROR"
	CodeHashPasswordError = "HASH_PASSWORD_ERROR"
	CodeNotFound          = "NOT_FOUND"
)
