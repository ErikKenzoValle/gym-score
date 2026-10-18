package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// versoesFake simula usuarios.token_versao.
type versoesFake map[uint]int

func (v versoesFake) VersaoToken(id uint) (int, bool, error) {
	n, ok := v[id]
	return n, ok, nil
}

func appComAuth(secret string) *fiber.App {
	app := fiber.New()
	app.Get("/p", middlewares.AuthMiddleware(&config.Config{JWTSecret: secret}, versoesFake{7: 0, 8: 3}), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"id": c.Locals("user_id")})
	})
	return app
}

func TestAuthMiddleware(t *testing.T) {
	app := appComAuth("segredo")
	token, _ := utils.GerarToken(7, "a@b.com", 0, "segredo")
	tokenOutroSegredo, _ := utils.GerarToken(7, "a@b.com", 0, "outro")

	tokenVelho, _ := utils.GerarToken(8, "x@y.com", 2, "segredo")    // versão 2, banco está na 3
	tokenAtual, _ := utils.GerarToken(8, "x@y.com", 3, "segredo")    // bate
	tokenSemUser, _ := utils.GerarToken(99, "z@y.com", 0, "segredo") // usuário inexistente

	casos := []struct {
		nome, header, cookie string
		esperado             int
	}{
		{"sem token", "", "", 401},
		{"bearer válido", "Bearer " + token, "", 200},
		{"cookie válido", "", token, 200},
		{"segredo diferente", "Bearer " + tokenOutroSegredo, "", 401},
		{"lixo", "Bearer abc", "", 401},
		{"esquema errado", "Basic " + token, "", 401},
		{"versão revogada (tv antigo)", "Bearer " + tokenVelho, "", 401},
		{"versão atual", "Bearer " + tokenAtual, "", 200},
		{"usuário inexistente", "Bearer " + tokenSemUser, "", 401},
	}
	for _, c := range casos {
		req := httptest.NewRequest("GET", "/p", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		if c.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "jwt", Value: c.cookie})
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != c.esperado {
			t.Errorf("%s: esperava %d, obteve %d", c.nome, c.esperado, res.StatusCode)
		}
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	app := fiber.New()
	app.Get("/x", middlewares.RateLimitMiddleware(3), func(c *fiber.Ctx) error { return c.SendString("ok") })
	for i := 1; i <= 5; i++ {
		res, err := app.Test(httptest.NewRequest("GET", "/x", nil))
		if err != nil {
			t.Fatal(err)
		}
		want := 200
		if i > 3 {
			want = 429
		}
		if res.StatusCode != want {
			t.Errorf("requisição %d: esperava %d, obteve %d", i, want, res.StatusCode)
		}
	}
}

func TestRateLimitMiddleware_ZeroDesativa(t *testing.T) {
	app := fiber.New()
	app.Get("/x", middlewares.RateLimitMiddleware(0), func(c *fiber.Ctx) error { return c.SendString("ok") })
	for i := 0; i < 50; i++ {
		res, _ := app.Test(httptest.NewRequest("GET", "/x", nil))
		if res.StatusCode != 200 {
			t.Fatalf("limite 0 deve desativar, obteve %d", res.StatusCode)
		}
	}
}

func TestCORSMiddleware(t *testing.T) {
	if middlewares.CORSMiddleware(&config.Config{AppEnv: "production"}) != nil {
		t.Error("produção sem CORS_ORIGINS deve desligar o CORS")
	}
	if middlewares.CORSMiddleware(&config.Config{AppEnv: "development"}) == nil {
		t.Error("desenvolvimento deve liberar CORS")
	}
	app := fiber.New()
	app.Use(middlewares.CORSMiddleware(&config.Config{AppEnv: "production", CORSOrigins: []string{"https://front.com"}}))
	app.Get("/x", func(c *fiber.Ctx) error { return c.SendString("ok") })
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://front.com")
	res, _ := app.Test(req)
	if res.Header.Get("Access-Control-Allow-Origin") != "https://front.com" {
		t.Errorf("origem permitida deveria ser ecoada: %q", res.Header.Get("Access-Control-Allow-Origin"))
	}
	req = httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://evil.com")
	res, _ = app.Test(req)
	if res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("origem não listada não pode ser liberada: %q", res.Header.Get("Access-Control-Allow-Origin"))
	}
}
