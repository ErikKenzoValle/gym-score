package middlewares_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
)

// papeisFake: id -> papéis (tipo de conta + pedido de instrutor/academia).
type papeisFake map[uint]*models.PapeisUsuario

func (p papeisFake) PapeisDoUsuario(id uint) (*models.PapeisUsuario, error) { return p[id], nil }

func atleta(id uint, statusInstrutor string) *models.PapeisUsuario {
	p := &models.PapeisUsuario{IDUsuario: id, Conta: models.PerfilAtleta}
	if statusInstrutor != "" {
		p.InstrutorID, p.InstrutorStatus = 100+id, statusInstrutor
	}
	return p
}

func academia(id uint, status string) *models.PapeisUsuario {
	p := &models.PapeisUsuario{IDUsuario: id, Conta: models.PerfilAcademia}
	if status != "" {
		p.AcademiaID, p.AcademiaStatus = 200+id, status
	}
	return p
}

func TestRequirePerfil_ModoECadastroAprovado(t *testing.T) {
	repo := papeisFake{
		1: academia(1, models.CadastroAprovado), 2: academia(2, models.CadastroPendente),
		3: academia(3, models.CadastroRejeitado), 4: atleta(4, models.CadastroAprovado),
		5: atleta(5, models.CadastroPendente), 6: atleta(6, ""), 7: academia(7, ""),
		8: atleta(8, models.CadastroRejeitado),
	}
	app := fiber.New()
	// X-User = id; X-Modo = claim `modo` do token (o AuthMiddleware grava em LocalModo).
	comUsuario := func(c *fiber.Ctx) error {
		var id uint
		if _, err := fmt.Sscan(c.Get("X-User"), &id); err == nil {
			c.Locals("user_id", id)
		}
		c.Locals(middlewares.LocalModo, c.Get("X-Modo"))
		return c.Next()
	}
	ok := func(c *fiber.Ctx) error { return c.SendString("ok") }
	app.Get("/acad", comUsuario, middlewares.RequirePerfil(repo, middlewares.PapelAcademia), ok)
	app.Get("/inst", comUsuario, middlewares.RequirePerfil(repo, middlewares.PapelInstrutor), ok)
	app.Get("/atl", comUsuario, middlewares.RequirePerfil(repo, middlewares.PapelAtleta), ok)

	casos := []struct {
		rota, user, modo string
		esperado         int
		codigo           string
	}{
		{"/acad", "1", "", 200, ""}, {"/acad", "2", "", 403, ""}, {"/acad", "3", "", 403, ""}, {"/acad", "7", "", 403, ""},
		{"/acad", "4", "instrutor", 403, ""}, {"/acad", "6", "", 403, ""}, {"/acad", "", "", 401, ""}, {"/acad", "99", "", 401, ""},
		// modo instrutor: claim + pedido aprovado (no banco)
		{"/inst", "4", "instrutor", 200, ""},
		{"/inst", "4", "atleta", 403, "modo_incorreto"}, {"/inst", "4", "", 403, "modo_incorreto"},
		// claim instrutor de quem foi rejeitado/está pendente/nunca pediu: recusado pelo banco
		{"/inst", "5", "instrutor", 403, "modo_indisponivel"}, {"/inst", "8", "instrutor", 403, "modo_indisponivel"},
		{"/inst", "6", "instrutor", 403, "modo_indisponivel"},
		{"/inst", "1", "instrutor", 403, ""},
		// modo atleta: o instrutor aprovado também é atleta, mas só no modo atleta
		{"/atl", "6", "", 200, ""}, {"/atl", "6", "atleta", 200, ""}, {"/atl", "4", "atleta", 200, ""},
		{"/atl", "5", "", 200, ""}, {"/atl", "4", "instrutor", 403, "modo_incorreto"}, {"/atl", "1", "", 403, ""},
	}
	for _, c := range casos {
		req := httptest.NewRequest("GET", c.rota, nil)
		if c.user != "" {
			req.Header.Set("X-User", c.user)
		}
		req.Header.Set("X-Modo", c.modo)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != c.esperado {
			t.Errorf("%s user=%s modo=%q: esperava %d, obteve %d", c.rota, c.user, c.modo, c.esperado, res.StatusCode)
			continue
		}
		if c.codigo != "" {
			b, _ := io.ReadAll(res.Body)
			var corpo map[string]any
			_ = json.Unmarshal(b, &corpo)
			if corpo["codigo"] != c.codigo {
				t.Errorf("%s user=%s modo=%q: código esperado %s, corpo %s", c.rota, c.user, c.modo, c.codigo, b)
			}
		}
	}
}
