package tool

import "regexp"

// reURLCredentials matches the userinfo segment of a URL, e.g. "://user:pass@" —
// covers postgres://, mongodb://, sqlserver://, https:// with basic auth, etc.
var reURLCredentials = regexp.MustCompile(`://[^/\s@]+@`)

// reURLSecretParam matches common secret query-string params (api_key, token,
// password, ...) so their values never end up in logs — e.g. Go's *url.Error
// embeds the full request URL (including query string) in its Error() text.
var reURLSecretParam = regexp.MustCompile(`(?i)([?&](?:api[_-]?key|apikey|token|access[_-]?token|password|passwd|pwd|secret|auth)=)[^&\s"']+`)

// RedactSecrets scrubs credentials/secrets that third-party drivers or Go's net/http
// error wrapping may embed directly in an error message (DSN userinfo, API keys in
// URL query strings) before the message is logged or otherwise persisted.
func RedactSecrets(s string) string {
	s = reURLCredentials.ReplaceAllString(s, "://***@")
	s = reURLSecretParam.ReplaceAllString(s, "${1}***")
	return s
}

// reKVCredential จับ user/password ใน DSN แบบ key=value; เช่น ADO-style ของ SQL Server
var reKVCredential = regexp.MustCompile(`(?i)\b(password|passwd|pwd|user id|uid|username|user)\s*=\s*[^;&\s]+`)

// RedactDSN ปิดบัง credentials ใน DSN ทั้งแบบ URL และแบบ key=value; สำหรับส่งให้ FE แสดงผล
func RedactDSN(dsn string) string {
	dsn = RedactSecrets(dsn)
	dsn = reKVCredential.ReplaceAllString(dsn, "${1}=***")
	return dsn
}
