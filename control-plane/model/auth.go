package model

import "time"

type Auth struct {
	User         *User      `json:"user"`
	AccessToken  string     `json:"accessToken"`
	TokenType    string     `json:"tokenType"`
	ExpiresIn    int64      `json:"expiresIn"`
	ExpiredAt    *time.Time `json:"expiredAt"`
	RefreshToken string     `json:"refreshToken"`
}

type LoginRequest struct {
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
}

type RegisterRequest struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	Phone       string `json:"phone"`
	CountryCode string `json:"countryCode"`
	Password    string `json:"password"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	DisplayName string `json:"displayName"` // optional — ไม่บังคับ, ไม่ตั้งก็ fallback เป็น FirstName+LastName ตอนแสดงผล
	CallbackUrl string `json:"callbackUrl"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" form:"refresh_token"`
}

type ForgotPasswordRequest struct {
	Email       string `json:"email"`
	CallbackUrl string `json:"callbackUrl"`
}

type SendEmailRequest struct {
	Email       string `json:"email"`
	CallbackUrl string `json:"callbackUrl"`
}

type SendEmailResult struct {
	Email     string    `json:"email"`
	ExpiredAt time.Time `json:"expiredAt"`
}

// --- IDP OAuth Flow ---

type IdpLoginRequest struct {
	IdpToken string `json:"idpToken"`
}

type IdpStateData struct {
	Alias            string `json:"alias"`
	CallbackURI      string `json:"callbackUri"`
	OauthRedirectUrl string `json:"oauthRedirectUrl"`
	// OpenerOrigin คือ origin ของหน้าที่เปิด popup (มาจาก header Origin/Referer ตอนเรียก /authorize)
	// ใช้เป็น targetOrigin ตอน postMessage กลับที่หน้า /redirect กัน token รั่วไปยัง origin ที่ไม่ตั้งใจ
	OpenerOrigin string `json:"openerOrigin,omitempty"`
}

type IdpCallbackData struct {
	Alias string `json:"alias"`
	Code  string `json:"code"`
}

type IdpTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IdToken      string `json:"id_token,omitempty"`
}
