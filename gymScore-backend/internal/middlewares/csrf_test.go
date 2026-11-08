package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"

	"github.com/gofiber/fiber/v2"
)

func TestCSRFMiddleware(t *testing.T) {
	cfg := &config.Config{AppBaseURL: "https://app.gymscore.com.br", CORSOrigins: []string{"https://front.gymscore.com.br"}}
	app := fiber.New()
	app.Use(middlewares.CSRFMiddleware(cfg))
	app.All("/x", func(c *fiber.Ctx) error { return c.SendString("ok") })

	casos := []struct {
		nome, metodo, origin, referer, auth string
		cookie                              bool
		esperado                            int
	}{
		{"GET com cookie de outra origem", "GET", "https://evil.com", "", "", true, 200},
		{"POST cookie sem Origin/Referer", "POST", "", "", "", true, 403},
		{"POST cookie de outra origem", "POST", "https://evil.com", "", "", true, 403},
		{"POST cookie origem 'null'", "POST", "null", "", "", true, 403},
		{"POST cookie subdomínio parecido", "POST", "https://app.gymscore.com.br.evil.com", "", "", true, 403},
		{"POST cookie APP_BASE_URL", "POST", "https://app.gymscore.com.br", "", "", true, 200},
		{"PATCH cookie CORS_ORIGINS", "PATCH", "https://front.gymscore.com.br", "", "", true, 200},
		{"DELETE cookie mesma origem (Host)", "DELETE", "http://example.com", "", "", true, 200},
		{"POST cookie Referer permitido", "POST", "", "https://app.gymscore.com.br/carteira", "", true, 200},
		{"POST cookie Referer de outra origem", "POST", "", "https://evil.com/x", "", true, 403},
		{"POST com Bearer (sem cookie)", "POST", "https://evil.com", "", "Bearer abc", false, 200},
		{"POST com Bearer e cookie", "POST", "https://evil.com", "", "Bearer abc", true, 200},
		{"POST sem credencial", "POST", "https://evil.com", "", "", false, 200},
	}
	for _, c := range casos {
		req := httptest.NewRequest(c.metodo, "http://example.com/x", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if c.referer != "" {
			req.Header.Set("Referer", c.referer)
		}
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		if c.cookie {
			req.AddCookie(&http.Cookie{Name: middlewares.NomeCookieUsuario(cfg), Value: "tok"})
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
