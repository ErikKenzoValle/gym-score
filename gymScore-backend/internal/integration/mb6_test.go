//go:build integration

package integration

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gymScore-backend/internal/app"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/models"
)

// MB6: /api/denuncias/minhas traz as evidências (metadados), na mesma ordem do detalhe.
func TestMB6_MinhasDenunciasTrazEvidencias(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	idDen := idDe(esperar(t, denunciar(t, a, corpoDenuncia(b.ID, models.DenunciaCondutaAbusiva, 0)), 201))
	ev := fmt.Sprintf("/api/denuncias/%d/evidencias", idDen)
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "a.png", pngMinimo(30)), 201)
	esperar(t, enviarArquivo(t, servidor, ev, a.Token, "b.jpg", append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 20)...)), 201)

	r := esperar(t, api(t, "GET", "/api/denuncias/minhas", a.Token, nil), 200)
	itens, _ := r.data()["itens"].([]any)
	var achou map[string]any
	for _, it := range itens {
		m := it.(map[string]any)
		if uint(m["id"].(float64)) == idDen {
			achou = m
		}
	}
	if achou == nil {
		t.Fatalf("denúncia ausente em /minhas: %s", r.Raw)
	}
	evs, _ := achou["evidencias"].([]any)
	if len(evs) != 2 {
		t.Fatalf("esperava 2 evidências em /minhas: %s", r.Raw)
	}
	e1, e2 := evs[0].(map[string]any), evs[1].(map[string]any)
	if num(e1, "ordem") != 1 || e1["mime"] != "image/png" || num(e2, "ordem") != 2 || e2["mime"] != "image/jpeg" {
		t.Errorf("evidências fora de ordem/tipo: %v", evs)
	}
	if strings.Contains(r.Raw, "caminho") {
		t.Errorf("caminho interno vazou: %s", r.Raw)
	}
}

// MB6: o desafio expõe instrutor.id_usuario (id da conta), distinto de instrutor.id (instrutores.id).
func TestMB6_DesafioExpoeIDUsuarioDoInstrutor(t *testing.T) {
	c := novoCenario(t)
	d := criarDesafio(t, c, 1000, 10)
	x := usuarioComSaldo(t, 5000)
	inscrever(t, d, x)
	r := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", d), x.Token, nil), 200)
	ins, _ := r.data()["instrutor"].(map[string]any)
	if ins == nil || uint(num(ins, "id")) != c.InstrutorID || uint(num(ins, "id_usuario")) != c.I.ID {
		t.Fatalf("instrutor no desafio: %v (esperado id=%d id_usuario=%d)", ins, c.InstrutorID, c.I.ID)
	}
	// e serve para um participante denunciar o instrutor pelo desafio
	esperar(t, denunciar(t, x, corpoDenuncia(c.I.ID, models.DenunciaCondutaAbusiva, d)), 201)
}

// MB6: CSP do painel aceita blob: em img-src (evidências por object URL) e as rotas limpas das
// telas de moderação servem os HTMLs.
func TestMB6_CSPAdminBlobERotasLimpas(t *testing.T) {
	if !strings.Contains(middlewares.CSPAdmin, "img-src 'self' data: blob:;") {
		t.Fatalf("CSPAdmin sem blob: %s", middlewares.CSPAdmin)
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "admin"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "html"), 0o755)
	arqs := map[string]string{
		"admin/index.html": "<!doctype html><title>GymScore Admin</title>",
		"index.html":       "<!doctype html><title>App</title>",
	}
	rotas := map[string]string{
		"/denunciar": "denunciar.html", "/minhas-denuncias": "minhas-denuncias.html",
		"/minhas-punicoes": "minhas-punicoes.html", "/academia/denuncias": "academia-denuncias.html",
	}
	for _, f := range rotas {
		arqs["html/"+f] = "<!doctype html><title>" + f + "</title>"
	}
	for arq, conteudo := range arqs {
		if err := os.WriteFile(filepath.Join(dir, arq), []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := *cfgBase
	cfg.FrontendPath = dir
	srv := app.Construir(&cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})

	res, _ := srv.Test(httptest.NewRequest("GET", "/admin", nil), -1)
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "blob:") {
		t.Errorf("/admin sem blob: na CSP: %q", csp)
	}
	res.Body.Close()
	for rota, f := range rotas {
		res, err := srv.Test(httptest.NewRequest("GET", rota, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 200)
		n, _ := res.Body.Read(buf)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(string(buf[:n]), f) {
			t.Errorf("%s: %d %q", rota, res.StatusCode, buf[:n])
		}
	}
}
