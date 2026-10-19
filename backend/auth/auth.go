// Package auth: login form + HMAC-signed cookie. No session table — the cookie
// carries role|branch|expiry and its signature. Production would use NP's SSO.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/backend/config"
	"novapost/contract"
)

const (
	CookieName = "session"
	ttl        = 12 * time.Hour

	maxFailedLogins = 10
	failedWindow    = 15 * time.Minute
)

// failures counts failed logins per client IP in a sliding window.
// ponytail: in-process, resets on restart; enough against password guessing on one demo box.
type failures struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (f *failures) recent(ip string, now time.Time) []time.Time {
	kept := f.hits[ip][:0]
	for _, t := range f.hits[ip] {
		if now.Sub(t) < failedWindow {
			kept = append(kept, t)
		}
	}
	f.hits[ip] = kept
	return kept
}

func (f *failures) blocked(ip string, now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.recent(ip, now)) >= maxFailedLogins
}

func (f *failures) add(ip string, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[ip] = append(f.recent(ip, now), now)
}

// clientIP: Caddy replaces any client-sent X-Forwarded-For with the real address,
// and the backend is not exposed directly, so the first value is trustworthy here.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

type Session struct {
	Role      string    `json:"role"`
	BranchID  string    `json:"branchId,omitempty"`
	ExpiresAt time.Time `json:"-"`
}

var errBadSession = errors.New("bad session")

func sign(payload, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

func Encode(s Session, secret string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(s.Role + "|" + s.BranchID + "|" + strconv.FormatInt(s.ExpiresAt.Unix(), 10)))
	return payload + "." + sign(payload, secret)
}

func Decode(v, secret string, now time.Time) (Session, error) {
	payload, sig, ok := strings.Cut(v, ".")
	if !ok || !hmac.Equal([]byte(sign(payload, secret)), []byte(sig)) {
		return Session{}, errBadSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Session{}, errBadSession
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return Session{}, errBadSession
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.Unix() >= exp {
		return Session{}, errBadSession
	}
	return Session{Role: parts[0], BranchID: parts[1], ExpiresAt: time.Unix(exp, 0)}, nil
}

func fromRequest(c *echo.Context, secret string) (Session, bool) {
	ck, err := c.Cookie(CookieName)
	if err != nil {
		return Session{}, false
	}
	s, err := Decode(ck.Value, secret, time.Now())
	return s, err == nil
}

// Middleware requires a valid session with one of roles (401 without, 403 wrong role).
func Middleware(secret string, roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			s, ok := fromRequest(c, secret)
			if !ok {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			for _, r := range roles {
				if r == s.Role {
					c.Set("session", s)
					return next(c)
				}
			}
			return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
		}
	}
}

func FromContext(c *echo.Context) Session {
	s, _ := c.Get("session").(Session)
	return s
}

func Handlers(cfg config.Config) (login, logout, me echo.HandlerFunc) {
	passwords := map[string]string{"admin": cfg.AdminPassword, "senior": cfg.SeniorPassword, "operator": cfg.OperatorPassword}
	failed := &failures{hits: map[string][]time.Time{}}

	login = func(c *echo.Context) error {
		ip := clientIP(c.Request())
		if failed.blocked(ip, time.Now()) {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Забагато невдалих спроб. Спробуйте за 15 хвилин."})
		}
		var req struct {
			Role     string `json:"role"`
			BranchID string `json:"branchId"`
			Password string `json:"password"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
		}
		want, ok := passwords[req.Role]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown role")
		}
		if req.Role == "operator" {
			if l, ok := contract.LocationByID(req.BranchID); !ok || l.Kind != "branch" {
				return echo.NewHTTPError(http.StatusBadRequest, "branch required")
			}
		} else {
			req.BranchID = ""
		}
		if subtle.ConstantTimeCompare([]byte(req.Password), []byte(want)) != 1 {
			failed.add(ip, time.Now())
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Невірний пароль"})
		}
		s := Session{Role: req.Role, BranchID: req.BranchID, ExpiresAt: time.Now().Add(ttl)}
		c.SetCookie(&http.Cookie{
			Name: CookieName, Value: Encode(s, cfg.AppSecret), Path: "/", Expires: s.ExpiresAt,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: c.Request().Header.Get("X-Forwarded-Proto") == "https",
		})
		return c.JSON(http.StatusOK, s)
	}

	logout = func(c *echo.Context) error {
		c.SetCookie(&http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		return c.NoContent(http.StatusNoContent)
	}

	me = func(c *echo.Context) error {
		s, ok := fromRequest(c, cfg.AppSecret)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		return c.JSON(http.StatusOK, s)
	}
	return login, logout, me
}
