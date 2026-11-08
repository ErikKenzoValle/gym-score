package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// setCookie executa o handler e devolve o único Set-Cookie (cru e parseado).
func setCookie(t *testing.T, h func(c *fiber.Ctx) *fiber.Cookie) (string, *http.Cookie) {
	t.Helper()
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { c.Cookie(h(c)); return c.SendStatus(204) })
	res, err := app.Test(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	cs := res.Cookies()
	if len(cs) != 1 {
		t.Fatalf("esperava 1 cookie, veio %v", res.Header.Values("Set-Cookie"))
	}
	return res.Header.Get("Set-Cookie"), cs[0]
}

// R7: nomes e atributos dos cookies de sessão em dev HTTP, com APP_BASE_URL https e em produção.
func TestCookiesDeSessao_AtributosPorAmbiente(t *testing.T) {
	segredo := "segredo-usuario-0123456789abcdef0123"
	casos := []struct {
		nome           string
		cfg            *config.Config
		seguro         bool
		usuario, admin string
	}{
		{"dev http", &config.Config{AppEnv: "development", AppBaseURL: "http://localhost:3000"}, false, "jwt", "admin_jwt"},
		{"homologação https", &config.Config{AppEnv: "development", AppBaseURL: "HTTPS://homolog.gymscore.com.br"}, true, "__Host-jwt", "__Secure-admin_jwt"},
		{"produção", &config.Config{AppEnv: "production", AppBaseURL: "http://interno"}, true, "__Host-jwt", "__Secure-admin_jwt"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			tk, _ := utils.GerarToken(1, "a@b.c", 0, segredo)
			exp := utils.ExpiracaoToken(tk, 0)
			if d := time.Until(exp); d < utils.DuracaoToken-time.Minute || d > utils.DuracaoToken {
				t.Fatalf("exp do token: %v", exp)
			}
			raw, ck := setCookie(t, func(*fiber.Ctx) *fiber.Cookie { return middlewares.CookieSessaoUsuario(c.cfg, tk) })
			if ck.Name != c.usuario || ck.Value != tk || ck.Path != "/" || ck.Domain != "" || !ck.HttpOnly ||
				ck.Secure != c.seguro || ck.SameSite != http.SameSiteLaxMode || !ck.Expires.Equal(exp.Truncate(time.Second)) {
				t.Errorf("cookie usuário: %s", raw)
			}
			raw, ck = setCookie(t, func(*fiber.Ctx) *fiber.Cookie { return middlewares.CookieSessaoUsuarioApagar(c.cfg) })
			if ck.Name != c.usuario || ck.Value != "" || ck.Path != "/" || !ck.HttpOnly || ck.Secure != c.seguro ||
				ck.SameSite != http.SameSiteLaxMode || !ck.Expires.Before(time.Now()) {
				t.Errorf("apagar cookie usuário: %s", raw)
			}

			ta, _ := utils.GerarTokenAdmin(1, 0, "segredo-admin-0123456789abcdef01234", 2*time.Hour)
			expA := utils.ExpiracaoToken(ta, 0)
			raw, ck = setCookie(t, func(*fiber.Ctx) *fiber.Cookie { return middlewares.CookieSessaoAdmin(c.cfg, ta) })
			if ck.Name != c.admin || ck.Path != "/api/admin" || !ck.HttpOnly || ck.Secure != c.seguro ||
				ck.SameSite != http.SameSiteStrictMode || !ck.Expires.Equal(expA.Truncate(time.Second)) {
				t.Errorf("cookie admin: %s", raw)
			}
			raw, ck = setCookie(t, func(*fiber.Ctx) *fiber.Cookie { return middlewares.CookieSessaoAdminApagar(c.cfg) })
			if ck.Name != c.admin || ck.Value != "" || ck.Path != "/api/admin" || ck.Secure != c.seguro ||
				ck.SameSite != http.SameSiteStrictMode || !ck.Expires.Before(time.Now()) {
				t.Errorf("apagar cookie admin: %s", raw)
			}
			if strings.Contains(strings.ToLower(raw), "domain=") {
				t.Errorf("cookie não deve ter Domain: %s", raw)
			}
		})
	}
}

// R7: com cookies Secure o middleware só aceita __Host-jwt (o `jwt` antigo/forjado em HTTP é ignorado).
func TestExtrairToken_NomeConformeAmbiente(t *testing.T) {
	for _, c := range []struct {
		cfg            *config.Config
		aceita, ignora string
	}{
		{&config.Config{AppBaseURL: "http://localhost:3000"}, "jwt", "__Host-jwt"},
		{&config.Config{AppBaseURL: "https://app.gymscore.com.br"}, "__Host-jwt", "jwt"},
	} {
		app := fiber.New()
		app.Get("/", func(ctx *fiber.Ctx) error { return ctx.SendString(middlewares.ExtrairToken(ctx, c.cfg)) })
		for nome, esperado := range map[string]string{c.aceita: "tok", c.ignora: ""} {
			req := httptest.NewRequest("GET", "/", nil)
			req.AddCookie(&http.Cookie{Name: nome, Value: "tok"})
			res, _ := app.Test(req)
			b := make([]byte, 16)
			n, _ := res.Body.Read(b)
			if string(b[:n]) != esperado {
				t.Errorf("%s com cookie %s: veio %q", c.cfg.AppBaseURL, nome, b[:n])
			}
		}
	}
}

func TestSemCache(t *testing.T) {
	app := fiber.New()
	app.Post("/login", middlewares.SemCache(), func(c *fiber.Ctx) error { return c.SendStatus(200) })
	res, _ := app.Test(httptest.NewRequest("POST", "/login", nil))
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: %q", res.Header.Get("Cache-Control"))
	}
}
