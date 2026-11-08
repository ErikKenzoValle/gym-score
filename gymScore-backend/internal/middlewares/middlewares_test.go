package middlewares_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/models"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// versoesFake simula usuarios.token_versao; id 9 = banido, 10 = suspenso (vigente), 11 = suspensão vencida.
type versoesFake map[uint]int

var (
	banidoEmFake   = time.Now().Add(-time.Hour)
	suspensoFake   = time.Now().Add(48 * time.Hour)
	suspensoVencid = time.Now().Add(-time.Minute)
)

func (v versoesFake) EstadoSessao(id uint) (models.EstadoSessao, error) {
	n, ok := v[id]
	e := models.EstadoSessao{Versao: n, Existe: ok}
	switch id {
	case 9:
		e.BanidoEm = &banidoEmFake
	case 10:
		e.SuspensoAte = &suspensoFake
	case 11:
		e.SuspensoAte = &suspensoVencid
	}
	return e, nil
}

// Q2: suspenso segue autenticado (leitura), mas as rotas com BloquearSuspenso devolvem 403 conta_suspensa;
// banido recebe 403 conta_banida em qualquer rota; suspensão vencida não bloqueia (tempo real, sem job).
func TestAuthMiddleware_BanidoESuspenso(t *testing.T) {
	app := fiber.New()
	cfg := &config.Config{JWTSecret: "segredo"}
	auth := middlewares.AuthMiddleware(cfg, versoesFake{7: 0, 9: 0, 10: 0, 11: 0})
	ok := func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) }
	app.Get("/ler", auth, ok)
	app.Post("/participar", auth, middlewares.BloquearSuspenso(), ok)
	chamar := func(metodo, rota string, id uint) (int, map[string]any) {
		tk, _ := utils.GerarToken(id, "a@b.com", 0, "segredo")
		req := httptest.NewRequest(metodo, rota, nil)
		req.Header.Set("Authorization", "Bearer "+tk)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return res.StatusCode, m
	}
	casos := []struct {
		metodo, rota string
		id           uint
		status       int
		codigo       string
	}{
		{"GET", "/ler", 7, 200, ""},
		{"POST", "/participar", 7, 200, ""},
		{"GET", "/ler", 9, 403, "conta_banida"},
		{"POST", "/participar", 9, 403, "conta_banida"},
		{"GET", "/ler", 10, 200, ""},
		{"POST", "/participar", 10, 403, "conta_suspensa"},
		{"POST", "/participar", 11, 200, ""},
	}
	for _, c := range casos {
		st, corpo := chamar(c.metodo, c.rota, c.id)
		if st != c.status || (c.codigo != "" && corpo["codigo"] != c.codigo) {
			t.Errorf("%s %s (usuário %d): status %d corpo %v; esperava %d %s", c.metodo, c.rota, c.id, st, corpo, c.status, c.codigo)
		}
		if c.codigo == "conta_suspensa" {
			if d, _ := corpo["data"].(map[string]any); d == nil || d["suspenso_ate"] == nil {
				t.Errorf("403 conta_suspensa sem suspenso_ate: %v", corpo)
			}
		}
	}
}

func appComAuth(secret string) *fiber.App {
	app := fiber.New()
	app.Get("/p", middlewares.AuthMiddleware(&config.Config{JWTSecret: secret}, versoesFake{7: 0, 8: 3, 9: 0}), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"id": c.Locals("user_id")})
	})
	return app
}

func TestAuthMiddleware(t *testing.T) {
	app := appComAuth("segredo")
	token, _ := utils.GerarToken(7, "a@b.com", 0, "segredo")
	tokenOutroSegredo, _ := utils.GerarToken(7, "a@b.com", 0, "outro")

	tokenVelho, _ := utils.GerarToken(8, "x@y.com", 2, "segredo")     // versão 2, banco está na 3
	tokenAtual, _ := utils.GerarToken(8, "x@y.com", 3, "segredo")     // bate
	tokenSemUser, _ := utils.GerarToken(99, "z@y.com", 0, "segredo")  // usuário inexistente
	tokenBloqueado, _ := utils.GerarToken(9, "b@y.com", 0, "segredo") // conta bloqueada

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
		{"conta bloqueada", "Bearer " + tokenBloqueado, "", 403},
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

func TestLimiteCorpo_ExcecaoSoNaRotaDeUpload(t *testing.T) {
	app := fiber.New(fiber.Config{BodyLimit: 4096})
	app.Use(middlewares.LimiteCorpo(100, func(c *fiber.Ctx) bool { return c.Path() == "/upload" }))
	ok := func(c *fiber.Ctx) error { return c.SendStatus(200) }
	app.Post("/json", ok)
	app.Post("/upload", ok)
	enviar := func(rota string, n int) int {
		req := httptest.NewRequest("POST", rota, strings.NewReader(strings.Repeat("a", n)))
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	if s := enviar("/json", 100); s != 200 {
		t.Errorf("no limite: %d", s)
	}
	if s := enviar("/json", 101); s != 413 {
		t.Errorf("acima do limite geral: %d", s)
	}
	if s := enviar("/upload", 2000); s != 200 {
		t.Errorf("upload com limite próprio: %d", s)
	}
}
