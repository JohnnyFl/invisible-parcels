package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/backend/config"
)

func TestDecode_RejectsTamperedAndExpired(t *testing.T) {
	now := time.Now()
	v := Encode(Session{Role: "operator", BranchID: "branch-kyiv-12", ExpiresAt: now.Add(time.Hour)}, "k")
	if s, err := Decode(v, "k", now); err != nil || s.BranchID != "branch-kyiv-12" || s.Role != "operator" {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := Decode(v, "other", now); err == nil {
		t.Fatal("wrong secret accepted")
	}
	forged := Encode(Session{Role: "admin", ExpiresAt: now.Add(time.Hour)}, "x")
	if _, err := Decode(strings.Split(forged, ".")[0]+"."+strings.Split(v, ".")[1], "k", now); err == nil {
		t.Fatal("forged payload accepted")
	}
	if _, err := Decode(v, "k", now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired accepted")
	}
	for _, bad := range []string{"", "abc", "a.b.c", "!!!.00"} {
		if _, err := Decode(bad, "k", now); err == nil {
			t.Fatalf("garbage %q accepted", bad)
		}
	}
}

func get(e *echo.Echo, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func cookie(role, branch string) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: Encode(Session{Role: role, BranchID: branch, ExpiresAt: time.Now().Add(time.Hour)}, "k")}
}

func TestMiddleware_RoleChecks(t *testing.T) {
	e := echo.New()
	e.GET("/x", func(c *echo.Context) error { return c.String(200, FromContext(c).Role) }, Middleware("k", "senior"))
	if rec := get(e, "/x", nil); rec.Code != 401 {
		t.Fatalf("no cookie: %d", rec.Code)
	}
	if rec := get(e, "/x", &http.Cookie{Name: CookieName, Value: "junk"}); rec.Code != 401 {
		t.Fatalf("junk cookie: %d", rec.Code)
	}
	if rec := get(e, "/x", cookie("operator", "branch-kyiv-12")); rec.Code != 403 {
		t.Fatalf("operator: %d", rec.Code)
	}
	if rec := get(e, "/x", cookie("senior", "")); rec.Code != 200 || rec.Body.String() != "senior" {
		t.Fatalf("senior: %d %s", rec.Code, rec.Body)
	}
}

func login(e *echo.Echo, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLoginLogoutMe(t *testing.T) {
	cfg := config.Config{AppSecret: "k", AdminPassword: "a", SeniorPassword: "s", OperatorPassword: "o"}
	e := echo.New()
	in, out, me := Handlers(cfg)
	e.POST("/api/login", in)
	e.POST("/api/logout", out)
	e.GET("/api/me", me)

	if rec := login(e, `{"role":"admin","password":"wrong"}`); rec.Code != 401 {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	if rec := login(e, `{"role":"admin","password":"o"}`); rec.Code != 401 {
		t.Fatalf("other role's password: %d", rec.Code)
	}
	if rec := login(e, `{"role":"operator","password":"o"}`); rec.Code != 400 {
		t.Fatalf("operator without branch: %d", rec.Code)
	}
	if rec := login(e, `{"role":"operator","branchId":"hub-kyiv","password":"o"}`); rec.Code != 400 {
		t.Fatalf("operator at hub: %d", rec.Code)
	}
	if rec := login(e, `{"role":"root","password":"a"}`); rec.Code != 400 {
		t.Fatalf("unknown role: %d", rec.Code)
	}
	rec := login(e, `{"role":"operator","branchId":"branch-kyiv-12","password":"o"}`)
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	c := rec.Result().Cookies()[0]
	if c.Name != CookieName || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie %+v", c)
	}
	if rec := get(e, "/api/me", c); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"branchId":"branch-kyiv-12"`) {
		t.Fatalf("me: %d %s", rec.Code, rec.Body)
	}
	if rec := get(e, "/api/me", nil); rec.Code != 401 {
		t.Fatalf("me without cookie: %d", rec.Code)
	}
	req := httptest.NewRequest("POST", "/api/logout", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if cs := rec.Result().Cookies(); len(cs) != 1 || cs[0].MaxAge >= 0 {
		t.Fatalf("logout cookie %+v", cs)
	}
}
