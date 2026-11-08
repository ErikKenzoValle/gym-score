package middlewares_test

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
)

// perfisFake: id -> (perfil, status do cadastro).
type perfisFake map[uint][2]string

func (p perfisFake) BuscarPorID(id uint) (*models.Usuario, error) {
	v, ok := p[id]
	if !ok {
		return nil, nil
	}
	return &models.Usuario{ID: id, Perfil: v[0]}, nil
}
func (p perfisFake) StatusCadastro(id uint, _ string) (string, error) { return p[id][1], nil }

func TestRequirePerfil_ExigeCadastroAprovado(t *testing.T) {
	repo := perfisFake{
		1: {"academia", models.CadastroAprovado}, 2: {"academia", models.CadastroPendente},
		3: {"academia", models.CadastroRejeitado}, 4: {"instrutor", models.CadastroAprovado},
		5: {"instrutor", models.CadastroPendente}, 6: {"atleta", ""}, 7: {"academia", ""},
	}
	app := fiber.New()
	comUsuario := func(c *fiber.Ctx) error {
		var id uint
		if _, err := fmt.Sscan(c.Get("X-User"), &id); err == nil {
			c.Locals("user_id", id)
		}
		return c.Next()
	}
	app.Get("/acad", comUsuario, middlewares.RequirePerfil(repo, "academia"), func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/inst", comUsuario, middlewares.RequirePerfil(repo, "instrutor"), func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/atl", comUsuario, middlewares.RequirePerfil(repo, "atleta"), func(c *fiber.Ctx) error { return c.SendString("ok") })

	casos := []struct {
		rota, user string
		esperado   int
	}{
		{"/acad", "1", 200}, {"/acad", "2", 403}, {"/acad", "3", 403}, {"/acad", "7", 403},
		{"/acad", "4", 403}, {"/acad", "6", 403}, {"/acad", "", 401}, {"/acad", "99", 401},
		{"/inst", "4", 200}, {"/inst", "5", 403}, {"/inst", "1", 403},
		{"/atl", "6", 200}, {"/atl", "1", 403},
	}
	for _, c := range casos {
		req := httptest.NewRequest("GET", c.rota, nil)
		if c.user != "" {
			req.Header.Set("X-User", c.user)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != c.esperado {
			t.Errorf("%s user=%s: esperava %d, obteve %d", c.rota, c.user, c.esperado, res.StatusCode)
		}
	}
}
