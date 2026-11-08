package middlewares_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

type estadoAdminFake struct {
	versao int
	ativo  bool
	papel  string
	existe bool
	err    error
}

func (e estadoAdminFake) EstadoToken(uint) (int, bool, string, bool, error) {
	return e.versao, e.ativo, e.papel, e.existe, e.err
}

func appAdmin(cfg *config.Config, est middlewares.AdminEstadoReader) *fiber.App {
	app := fiber.New()
	app.Use(middlewares.AdminAuthMiddleware(cfg, est))
	app.Get("/x", func(c *fiber.Ctx) error { return c.SendString(c.Locals(middlewares.LocalAdminPapel).(string)) })
	app.Post("/super", middlewares.RequireSuperadmin(), func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func TestAdminAuthMiddleware(t *testing.T) {
	cfg := &config.Config{AppEnv: "development", JWTSecret: "segredo-usuario-0123456789abcdef0123"}
	segAdmin := cfg.SegredoAdmin()
	ok := estadoAdminFake{versao: 2, ativo: true, papel: "admin", existe: true}
	tkAdmin, _ := utils.GerarTokenAdmin(9, 2, segAdmin, time.Hour)
	tkVelho, _ := utils.GerarTokenAdmin(9, 1, segAdmin, time.Hour)
	tkUsuario, _ := utils.GerarToken(9, "a@b.c", 2, cfg.JWTSecret)
	tkUsuarioSegAdmin, _ := utils.GerarToken(9, "a@b.c", 2, segAdmin) // sem aud
	tkOutroSegredo, _ := utils.GerarTokenAdmin(9, 2, "outro-segredo-qualquer-0123456789", time.Hour)

	casos := []struct {
		nome     string
		est      estadoAdminFake
		auth     string
		cookie   string
		esperado int
	}{
		{"sem token", ok, "", "", 401},
		{"token admin no header", ok, "Bearer " + tkAdmin, "", 200},
		{"token admin no cookie admin_jwt", ok, "", tkAdmin, 200},
		{"token de usuário", ok, "Bearer " + tkUsuario, "", 401},
		{"token de usuário assinado com o segredo admin (sem aud)", ok, "Bearer " + tkUsuarioSegAdmin, "", 401},
		{"token admin com outro segredo", ok, "Bearer " + tkOutroSegredo, "", 401},
		{"versão revogada", ok, "Bearer " + tkVelho, "", 401},
		{"admin desativado", estadoAdminFake{versao: 2, ativo: false, papel: "admin", existe: true}, "Bearer " + tkAdmin, "", 401},
		{"admin inexistente", estadoAdminFake{}, "Bearer " + tkAdmin, "", 401},
		{"erro no banco", estadoAdminFake{err: errors.New("db")}, "Bearer " + tkAdmin, "", 500},
		{"Authorization sem Bearer não cai no cookie", ok, "Basic xyz", tkAdmin, 401},
	}
	for _, c := range casos {
		req := httptest.NewRequest("GET", "/x", nil)
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		if c.cookie != "" {
			req.AddCookie(&http.Cookie{Name: middlewares.CookieAdmin, Value: c.cookie})
		}
		res, err := appAdmin(cfg, c.est).Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != c.esperado {
			t.Errorf("%s: esperava %d, obteve %d", c.nome, c.esperado, res.StatusCode)
		}
	}

	// cookie `jwt` de usuário não é lido pelo painel
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "jwt", Value: tkAdmin})
	if res, _ := appAdmin(cfg, ok).Test(req); res.StatusCode != 401 {
		t.Errorf("cookie jwt não deveria autenticar o painel: %d", res.StatusCode)
	}
}

func TestRequireSuperadmin_PapelVemDoBanco(t *testing.T) {
	cfg := &config.Config{AppEnv: "development"}
	tk, _ := utils.GerarTokenAdmin(1, 0, cfg.SegredoAdmin(), time.Hour)
	for papel, esperado := range map[string]int{"admin": 403, "superadmin": 200} {
		req := httptest.NewRequest("POST", "/super", nil)
		req.Header.Set("Authorization", "Bearer "+tk)
		res, _ := appAdmin(cfg, estadoAdminFake{ativo: true, papel: papel, existe: true}).Test(req)
		if res.StatusCode != esperado {
			t.Errorf("papel %s: esperava %d, obteve %d", papel, esperado, res.StatusCode)
		}
	}
}

func TestAdminCSRFMiddleware(t *testing.T) {
	cfg := &config.Config{AppBaseURL: "https://app.gymscore.com.br"}
	app := fiber.New()
	app.Use(middlewares.AdminCSRFMiddleware(cfg))
	app.All("/x", func(c *fiber.Ctx) error { return c.SendString("ok") })
	casos := []struct {
		nome, metodo, origin, auth string
		esperado                   int
	}{
		{"GET sem origem", "GET", "", "", 200},
		{"POST sem origem (login por formulário alheio)", "POST", "", "", 403},
		{"POST de outra origem", "POST", "https://evil.com", "", 403},
		{"POST da APP_BASE_URL", "POST", "https://app.gymscore.com.br", "", 200},
		{"POST com Bearer", "POST", "https://evil.com", "Bearer x", 200},
	}
	for _, c := range casos {
		req := httptest.NewRequest(c.metodo, "http://example.com/x", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		res, _ := app.Test(req)
		if res.StatusCode != c.esperado {
			t.Errorf("%s: esperava %d, obteve %d", c.nome, c.esperado, res.StatusCode)
		}
	}
}

func TestAdminPaginasHeaders(t *testing.T) {
	app := fiber.New()
	app.Use("/admin", middlewares.AdminPaginasHeaders())
	app.Get("/admin", func(c *fiber.Ctx) error { return c.SendString("ok") })
	res, _ := app.Test(httptest.NewRequest("GET", "/admin", nil))
	csp := res.Header.Get("Content-Security-Policy")
	if csp != middlewares.CSPAdmin || res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("headers: %v", res.Header)
	}
	if contains(csp, "unsafe-inline") || !contains(csp, "script-src 'self'") || !contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP fraca: %s", csp)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// R1: os headers do painel valem para qualquer forma de caminho que chegue a /admin.
func TestEhCaminhoAdmin(t *testing.T) {
	sim := []string{"/admin", "/admin/", "/ADMIN/", "/Admin/index.html", "/%61dmin/index.html", "/%2561dmin/login.html",
		"/admin/../admin/index.html", "//admin//js/x.js", "/./admin", "/x/../admin/css/a.css", "/%41DMIN", "\\admin\\index.html"}
	nao := []string{"/", "/administrador", "/api/admin/auth/me", "/adminx/index.html", "/login", "/admin/../menu", "/%61dminx"}
	for _, c := range sim {
		if !middlewares.EhCaminhoAdmin(c) {
			t.Errorf("%q deveria ser caminho do admin", c)
		}
	}
	for _, c := range nao {
		if middlewares.EhCaminhoAdmin(c) {
			t.Errorf("%q não é caminho do admin", c)
		}
	}
}

func TestAdminPaginasHeadersPorCaminho(t *testing.T) {
	app := fiber.New()
	app.Use(middlewares.AdminPaginasHeadersPorCaminho())
	app.Get("/*", func(c *fiber.Ctx) error { return c.SendString("ok") })
	for _, c := range []string{"/%61dmin/index.html", "/ADMIN/", "/admin/../admin/index.html", "/admin/js/app.js"} {
		res, _ := app.Test(httptest.NewRequest("GET", c, nil))
		if res.Header.Get("Content-Security-Policy") != middlewares.CSPAdmin || res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s sem headers do admin: %v", c, res.Header)
		}
	}
	res, _ := app.Test(httptest.NewRequest("GET", "/menu", nil))
	if res.Header.Get("Content-Security-Policy") != "" || res.Header.Get("X-Frame-Options") != "" {
		t.Errorf("/menu não deveria receber os headers do admin: %v", res.Header)
	}
}
