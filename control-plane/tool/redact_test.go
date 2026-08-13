package tool

import "testing"

// fakeSecret ค่า dummy สำหรับเทสต์เท่านั้น ไม่ใช่ secret จริง
const fakeSecret = "not-a-real-secret"

func TestRedactDSN(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"mongodb url", "mongodb://admin:" + fakeSecret + "@localhost:27017/agricultural", "mongodb://***@localhost:27017/agricultural"},
		{"postgres url", "postgres://user:" + fakeSecret + "@db:5432/app?sslmode=disable", "postgres://***@db:5432/app?sslmode=disable"},
		{"url no credentials", "mongodb://localhost:27017/db", "mongodb://localhost:27017/db"},
		{"sqlserver kv", "server=db;user id=sa;password=" + fakeSecret + ";database=app", "server=db;user id=***;password=***;database=app"},
		{"kv pwd variant", "Server=db;Uid=sa;Pwd=" + fakeSecret, "Server=db;Uid=***;Pwd=***"},
		{"query secret param", "https://api.example.com/data?token=abc123&x=1", "https://api.example.com/data?token=***&x=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RedactDSN(c.in); got != c.want {
				t.Errorf("RedactDSN(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
