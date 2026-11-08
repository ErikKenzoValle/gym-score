//go:build integration

package integration

// Testes do QA (rodada admin): concorrência em dinheiro (reembolso admin, job × admin), separação de
// tokens e CSRF nas duas áreas, admin desativado, heartbeat × pódio e promoção × inscrição (N4).

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/jobs"
	"gymScore-backend/internal/models"
)

func TestQA_ReembolsoAdminConcorrente(t *testing.T) {
	srv, _ := servidorComJobs(t, "inscricao_saque", 200, 48*time.Hour)
	super := novoAdminDB(t, models.PapelSuperadmin)
	super2 := novoAdminDB(t, models.PapelSuperadmin)
	for rodada := 0; rodada < 5; rodada++ {
		u := usuarioComSaldo(t, 5000)
		id := saqueEmRevisao(t, srv, u, 1000)
		var wg sync.WaitGroup
		rs := make([]resp, 4)
		for i := range rs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				quem := super
				if i%2 == 1 {
					quem = super2
				}
				rs[i], _ = chamarAdmin(t, srv, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id), quem.Cookie, motivo("concorrente"))
			}(i)
		}
		wg.Wait()
		alterados := 0
		for _, r := range rs {
			if r.Status != 200 {
				t.Errorf("reembolso concorrente: %d %s", r.Status, r.Raw)
				continue
			}
			if r.data()["alterado"] == true {
				alterados++
			}
		}
		if alterados != 1 {
			t.Errorf("rodada %d: %d chamadas alteraram (esperado 1)", rodada, alterados)
		}
		if s := saldo(t, u); s != 5000 {
			t.Errorf("rodada %d: saldo %d após reembolsos concorrentes (esperado 5000)", rodada, s)
		}
		if n := contarAuditoria(t, "saque_reembolsar", "transacao", id); n != 1 {
			t.Errorf("rodada %d: auditoria %d linhas", rodada, n)
		}
		ledgerConfere(t, u)
	}
}

func TestQA_ReembolsoXMarcarPagoConcorrentes(t *testing.T) {
	srv, _ := servidorComJobs(t, "inscricao_saque", 200, 48*time.Hour)
	super := novoAdminDB(t, models.PapelSuperadmin)
	for rodada := 0; rodada < 5; rodada++ {
		u := usuarioComSaldo(t, 5000)
		id := saqueEmRevisao(t, srv, u, 1000)
		var wg sync.WaitGroup
		var rr, rp resp
		wg.Add(2)
		go func() {
			defer wg.Done()
			rr, _ = chamarAdmin(t, srv, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id), super.Cookie, motivo("reembolso concorrente"))
		}()
		go func() {
			defer wg.Done()
			rp, _ = chamarAdmin(t, srv, "POST", fmt.Sprintf("/api/admin/saques/%d/marcar-pago", id), super.Cookie, motivo("pago concorrente"))
		}()
		wg.Wait()
		ok := 0
		for _, r := range []resp{rr, rp} {
			if r.Status == 200 && r.data()["alterado"] == true {
				ok++
			} else if r.Status != 409 && r.Status != 200 {
				t.Errorf("status inesperado: %d %s", r.Status, r.Raw)
			}
		}
		if ok != 1 {
			t.Errorf("rodada %d: reembolsar=%d %s / marcar-pago=%d %s", rodada, rr.Status, rr.Raw, rp.Status, rp.Raw)
		}
		st := saqueDoBanco(t, id).Status
		s := saldo(t, u)
		if !(st == models.TransacaoFailed && s == 5000) && !(st == models.TransacaoConcluido && s == 3800) {
			t.Errorf("rodada %d: status %s com saldo %d", rodada, st, s)
		}
		ledgerConfere(t, u)
	}
}

func TestQA_CancelamentoJobXAdminConcorrentes(t *testing.T) {
	srv, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	adminC := novoAdminDB(t, models.PapelAdmin)
	for rodada := 0; rodada < 4; rodada++ {
		c := cenarioComSaldo(t, 10000)
		corpo := corpoFin(c, 1000, 4, 2, map[string]any{"premio_fixo_centavos": 3000})
		id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpo), 201))
		as := atletas(t, 2, 3000)
		inscrever(t, id, as...)
		esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
		db.Exec("UPDATE desafios SET data_evento = ? WHERE id = ?", time.Now().Add(-72*time.Hour), id)
		var wg sync.WaitGroup
		var ra resp
		wg.Add(3)
		go func() { defer wg.Done(); jobs.ExecutarAgora(tarefas...) }()
		go func() { defer wg.Done(); jobs.ExecutarAgora(tarefas...) }()
		go func() {
			defer wg.Done()
			ra, _ = chamarAdmin(t, srv, "POST", fmt.Sprintf("/api/admin/desafios/%d/cancelar", id), adminC.Cookie, motivo("vencido"))
		}()
		wg.Wait()
		if ra.Status != 200 && ra.Status != 409 {
			t.Errorf("cancelar admin: %d %s", ra.Status, ra.Raw)
		}
		if s := statusDesafio(t, c.A, id); s != "cancelado" {
			t.Errorf("status: %s", s)
		}
		if s := saldo(t, c.A); s != 10000 {
			t.Errorf("rodada %d: reserva devolvida %d (esperado 10000)", rodada, s)
		}
		for _, a := range as {
			if s := saldo(t, a); s != 3000 {
				t.Errorf("rodada %d: entrada devolvida %d (esperado 3000)", rodada, s)
			}
		}
		var n int64
		db.Model(&models.Transacao{}).Where("id_desafio = ? AND tipo IN ?", id, []string{"estorno_desafio", "estorno_reserva"}).Count(&n)
		if n != 3 {
			t.Errorf("rodada %d: %d estornos (esperado 3)", rodada, n)
		}
		n = contarAuditoria(t, "desafio_cancelar", "desafio", id)
		if (ra.Status == 200) != (n == 1) {
			t.Errorf("rodada %d: auditoria %d com status admin %d", rodada, n, ra.Status)
		}
		ledgerConfere(t, append(as, c.A)...)
	}
}

// requisição crua (sem helpers) para combinar cabeçalhos/cookies à vontade
func cru(t *testing.T, metodo, caminho string, hdr map[string]string, cookies map[string]string) (int, string) {
	t.Helper()
	var body io.Reader
	if metodo != "GET" {
		body = strings.NewReader(`{"motivo":"teste de seguranca"}`)
	}
	req := httptest.NewRequest(metodo, caminho, body)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	res, err := servidor.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestQA_SeparacaoDeTokensCSRFEAdminDesativado(t *testing.T) {
	u := novoUsuario(t)
	super := novoAdminDB(t, models.PapelSuperadmin)
	comum := novoAdminDB(t, models.PapelAdmin)
	o := map[string]string{"Origin": origemTeste}
	casos := []struct {
		nome    string
		metodo  string
		caminho string
		hdr     map[string]string
		cookies map[string]string
		quer    int
	}{
		{"user bearer em /api/admin", "GET", "/api/admin/auth/me", map[string]string{"Authorization": "Bearer " + u.Token}, nil, 401},
		{"user token no cookie admin_jwt", "GET", "/api/admin/auth/me", nil, map[string]string{"admin_jwt": u.Token}, 401},
		{"user cookie jwt em /api/admin", "GET", "/api/admin/dashboard", nil, map[string]string{"jwt": u.Token}, 401},
		{"admin bearer em /api/usuarios/me", "GET", "/api/usuarios/me", map[string]string{"Authorization": "Bearer " + super.Cookie}, nil, 401},
		{"admin token no cookie jwt", "GET", "/api/usuarios/me", nil, map[string]string{"jwt": super.Cookie}, 401},
		{"admin POST origem forjada", "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), map[string]string{"Origin": "https://evil.example"}, map[string]string{"admin_jwt": super.Cookie}, 403},
		{"admin POST sem Origin/Referer", "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), nil, map[string]string{"admin_jwt": super.Cookie}, 403},
		{"admin POST Referer forjado", "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), map[string]string{"Referer": "https://evil.example/x"}, map[string]string{"admin_jwt": super.Cookie}, 403},
		{"admin POST origem prefixo enganoso", "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), map[string]string{"Origin": origemTeste + ".evil.example"}, map[string]string{"admin_jwt": super.Cookie}, 403},
		{"login admin origem forjada", "POST", "/api/admin/auth/login", map[string]string{"Origin": "https://evil.example"}, nil, 403},
		{"user POST cookie origem forjada", "POST", "/api/logout", map[string]string{"Origin": "https://evil.example"}, map[string]string{"jwt": u.Token}, 403},
		{"user POST cookie sem origem", "POST", "/api/logout", nil, map[string]string{"jwt": u.Token}, 403},
		{"user PATCH cookie origem forjada", "PATCH", "/api/usuarios/ultima-vez", map[string]string{"Origin": "https://evil.example"}, map[string]string{"jwt": u.Token}, 403},
		{"admin comum em rota S", "GET", "/api/admin/administradores", o, map[string]string{"admin_jwt": comum.Cookie}, 403},
		{"admin sem sessão", "GET", "/api/admin/dashboard", o, nil, 401},
	}
	for _, c := range casos {
		st, body := cru(t, c.metodo, c.caminho, c.hdr, c.cookies)
		if st != c.quer {
			t.Errorf("%s: %d (esperado %d) %s", c.nome, st, c.quer, body)
		}
	}
	// nada foi bloqueado pelas tentativas forjadas
	var bloq bool
	db.Raw("SELECT banido_em IS NOT NULL FROM usuarios WHERE id=?", u.ID).Scan(&bloq)
	if bloq {
		t.Errorf("usuário bloqueado por requisição forjada")
	}
	// admin desativado perde acesso imediatamente (mesmo token)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", comum, nil), 200)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/desativar", comum.ID), super, motivo("saiu")), 200)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", comum, nil), 401)
	esperar(t, adm(t, "GET", "/api/admin/dashboard", comum, nil), 401)
	r, _ := chamarAdmin(t, servidor, "POST", "/api/admin/auth/login", "", map[string]any{"email": comum.Email, "senha": comum.Senha})
	if r.Status != 401 {
		t.Errorf("login de admin desativado: %d %s", r.Status, r.Raw)
	}
	// reativado: o token antigo continua morto (token_versao subiu)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/reativar", comum.ID), super, motivo("voltou")), 200)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", comum, nil), 401)
}

func TestQA_HeartbeatNaoRevertePontosDoPodio(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 0, 4, 1)
	a := novoUsuario(t)
	inscrever(t, id, a)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, presencaPorID(c.I, t, id, a), 200)
	antes := pontos(t, a)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); api(t, "PATCH", "/api/usuarios/ultima-vez", a.Token, nil) }()
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(a, "1 min"))), 200)
	wg.Wait()
	esperar(t, api(t, "PATCH", "/api/usuarios/ultima-vez", a.Token, nil), 200)
	if depois := pontos(t, a); depois <= antes {
		t.Errorf("pontos do pódio revertidos/ausentes: antes %d depois %d", antes, depois)
	}
}

// N4: promoção a instrutor concorrente com a inscrição. Nunca pode sobrar um instrutor inscrito como atleta.
func TestQA_PromocaoXEntrarConcorrentes(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 500, 50, 3)
	inscritos, promovidos := 0, 0
	for i := 0; i < 15; i++ {
		u := usuarioComSaldo(t, 1000)
		var wg sync.WaitGroup
		var re, rp resp
		wg.Add(2)
		go func() { defer wg.Done(); re = acao(t, u, id, "entrar", nil) }()
		go func() {
			defer wg.Done()
			rp = api(t, "POST", "/api/instrutores", u.Token, map[string]any{"cref": "123456-G/SP", "especialidade": "x"})
		}()
		wg.Wait()
		if re.Status == 200 {
			inscritos++
		}
		if rp.Status == 201 {
			promovidos++
		}
		if re.Status == 200 && rp.Status == 201 {
			t.Errorf("rodada %d: entrou e foi promovido ao mesmo tempo", i)
		}
		var n int64
		db.Raw(`SELECT COUNT(*) FROM desafio_participantes dp JOIN usuarios u ON u.id = dp.id_usuario
			WHERE dp.id_desafio = ? AND u.id = ? AND u.perfil <> 'atleta'`, id, u.ID).Scan(&n)
		if n != 0 {
			t.Errorf("rodada %d: instrutor inscrito como atleta", i)
		}
		ledgerConfere(t, u)
	}
	t.Logf("inscritos=%d promovidos=%d", inscritos, promovidos)
}
