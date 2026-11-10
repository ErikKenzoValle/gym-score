//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// Rodada 4: modo atleta/instrutor (B4-1/B4-2), conflito de interesse (B4-3) e endereço/CEP (B4-4).

// ─── Helpers ─────────────────────────────────────────────────────────────────

// chamarCookies faz a requisição autenticada SÓ por cookies (como o navegador), com Origin opcional.
func chamarCookies(t *testing.T, srv *fiber.App, metodo, caminho, origem string, corpo any, cookies ...*http.Cookie) resp {
	t.Helper()
	var rd io.Reader
	if corpo != nil {
		b, _ := json.Marshal(corpo)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(metodo, caminho, rd)
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origem != "" {
		req.Header.Set("Origin", origem)
	}
	for _, c := range cookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw), Cookies: res.Cookies()}
	_ = json.Unmarshal(raw, &out.Body)
	if cc := res.Header.Get("Cache-Control"); cc != "" {
		out.Body["_cache_control"] = cc
	}
	return out
}

func cookieDe(r resp, nome string) *http.Cookie {
	for _, c := range r.Cookies {
		if c.Name == nome {
			return c
		}
	}
	return nil
}

func claimsDe(t *testing.T, token string) *utils.JWTClaims {
	t.Helper()
	c, err := utils.ValidarToken(token, cfgBase.JWTSecret)
	if err != nil {
		t.Fatalf("token inválido: %v", err)
	}
	return c
}

func codigoErro(r resp) string { s, _ := r.Body["codigo"].(string); return s }

func esperarCodigo(t *testing.T, r resp, status int, codigo string) {
	t.Helper()
	esperar(t, r, status)
	if codigoErro(r) != codigo {
		t.Fatalf("código esperado %q: %s", codigo, r.Raw)
	}
}

// instrutorAprovadoAutonomo: conta de atleta com pedido de instrutor aprovado e SEM vínculo (sessão
// ainda no modo atleta).
func instrutorAprovado(t *testing.T, idAcademia uint) (*user, uint) {
	t.Helper()
	u := novoUsuario(t)
	i := &models.Instrutor{IDUsuario: u.ID, CREF: "123456-G/SP", Status: models.CadastroAprovado}
	if idAcademia != 0 {
		i.IDAcademia = &idAcademia
	}
	if err := db.Create(i).Error; err != nil {
		t.Fatal(err)
	}
	return u, i.ID
}

// desafioVagaAberta: desafio gratuito com a equipe de 1 instrutor vazia (vaga aberta à candidatura;
// instrutor só percentual). Rodada 5: sem leilão.
func desafioVagaAberta(t *testing.T, c *cenario) uint {
	t.Helper()
	b := corpoDesafio(c, 0, 10, 1)
	delete(b, "id_instrutor")
	b["instrutores_vagas"], b["instrutor_modo_pagamento"], b["instrutor_pct"] = 1, "percentual", 20
	return idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, b), 201))
}

func me(t *testing.T, u *user) map[string]any {
	t.Helper()
	return esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 200).data()
}

// ─── B4-1: troca de modo, cookie e contrato ───────────────────────────────────

func TestModo_TrocaReemiteCookieEContrato(t *testing.T) {
	u := novoUsuario(t)
	tokenAtleta := u.Token
	if c := claimsDe(t, u.Token); c.Modo != "atleta" {
		t.Fatalf("login deveria entrar no modo atleta: %+v", c)
	}
	m := me(t, u)
	if m["perfil"] != "atleta" || m["modo_ativo"] != "atleta" || m["instrutor"] != nil ||
		fmt.Sprint(m["modos_disponiveis"]) != "[atleta]" {
		t.Fatalf("contrato de /me sem pedido: %v", m)
	}
	// Sem pedido / pendente / rejeitado: 403 modo_indisponivel (o modo vem do banco, nunca do corpo).
	esperarCodigo(t, api(t, "POST", "/api/sessao/modo", u.Token, map[string]any{"modo": "instrutor"}), 403, "modo_indisponivel")
	iid := idDe(esperar(t, api(t, "POST", "/api/instrutores", u.Token, map[string]any{"cref": "123456-G/SP"}), 201))
	r := esperar(t, api(t, "POST", "/api/sessao/modo", u.Token, map[string]any{"modo": "instrutor"}), 403)
	if codigoErro(r) != "modo_indisponivel" || !strings.Contains(r.Raw, "em análise") {
		t.Errorf("pendente: %s", r.Raw)
	}
	m = me(t, u)
	if inst, _ := m["instrutor"].(map[string]any); inst == nil || inst["status"] != "pendente" || fmt.Sprint(m["modos_disponiveis"]) != "[atleta]" {
		t.Errorf("/me com pedido pendente: %v", m)
	}
	esperar(t, api(t, "POST", "/api/sessao/modo", u.Token, map[string]any{"modo": "admin"}), 400)
	esperar(t, api(t, "POST", "/api/sessao/modo", u.Token, map[string]any{}), 400)

	aprovarInstrutor(t, iid)
	r = esperar(t, api(t, "POST", "/api/sessao/modo", u.Token, map[string]any{"modo": "instrutor"}), 200)
	novo := tokenDaSessao(t, r) // confere nome jwt, HttpOnly, Path=/, Lax, sem Domain, Expires futuro, fora do corpo
	c := claimsDe(t, novo)
	if c.Modo != "instrutor" || c.Versao != claimsDe(t, tokenAtleta).Versao || c.UserID != u.ID {
		t.Errorf("JWT novo: %+v", c)
	}
	if exp := cookieDe(r, "jwt").Expires; exp.Sub(c.ExpiresAt.Time).Abs() > time.Second {
		t.Errorf("cookie deveria expirar com o JWT: %v x %v", exp, c.ExpiresAt.Time)
	}
	if pref := cookieDe(r, "modo"); pref == nil || pref.Value != fmt.Sprintf("%d-instrutor", u.ID) || !pref.HttpOnly ||
		pref.SameSite != http.SameSiteLaxMode || pref.Path != "/" {
		t.Errorf("cookie de preferência de modo: %+v", pref)
	}
	sess := r.data()["usuario"].(map[string]any)
	if sess["perfil"] != "atleta" || sess["modo_ativo"] != "instrutor" || fmt.Sprint(sess["modos_disponiveis"]) != "[atleta instrutor]" {
		t.Errorf("resposta da troca: %v", sess)
	}
	// token_versao não muda: o token antigo (modo atleta) continua válido.
	esperar(t, api(t, "GET", "/api/usuarios/me", tokenAtleta, nil), 200)
	u.Token = novo
	m = me(t, u)
	if m["modo_ativo"] != "instrutor" || m["instrutor"].(map[string]any)["status"] != "aprovada" {
		t.Errorf("/me no modo instrutor: %v", m)
	}

	// Login seguinte entra no último modo usado (cookie de preferência) se ainda permitido.
	login := chamarCookies(t, servidor, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}, cookieDe(r, "modo"))
	esperar(t, login, 200)
	if c := claimsDe(t, tokenDaSessao(t, login)); c.Modo != "instrutor" {
		t.Errorf("login deveria voltar ao modo instrutor: %+v", c)
	}
	// Preferência de OUTRA conta não vale.
	outro := novoUsuario(t)
	login = chamarCookies(t, servidor, "POST", "/api/login", "", map[string]any{"email": outro.Email, "senha": outro.Senha}, cookieDe(r, "modo"))
	if c := claimsDe(t, tokenDaSessao(t, esperar(t, login, 200))); c.Modo != "atleta" {
		t.Errorf("preferência de outra conta não pode valer: %+v", c)
	}
	// Pedido revogado depois: o login cai no atleta mesmo com a preferência.
	db.Exec("UPDATE instrutores SET status = 'rejeitada', motivo_rejeicao = 'CREF suspenso' WHERE id = ?", iid)
	login = chamarCookies(t, servidor, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}, cookieDe(r, "modo"))
	esperar(t, login, 200)
	if c := claimsDe(t, tokenDaSessao(t, login)); c.Modo != "atleta" {
		t.Errorf("modo instrutor revogado não pode voltar no login: %+v", c)
	}
	us := login.data()["usuario"].(map[string]any)
	if inst := us["instrutor"].(map[string]any); inst["status"] != "rejeitada" || inst["motivo_rejeicao"] != "CREF suspenso" {
		t.Errorf("login deveria trazer o status do pedido: %v", us)
	}
}

func TestModo_CookieSoComOrigemConfiavelENoStore(t *testing.T) {
	u, _ := instrutorAprovado(t, 0)
	sessao := &http.Cookie{Name: "jwt", Value: u.Token}
	esperar(t, chamarCookies(t, servidor, "POST", "/api/sessao/modo", "https://atacante.example", map[string]any{"modo": "instrutor"}, sessao), 403)
	r := esperar(t, chamarCookies(t, servidor, "POST", "/api/sessao/modo", cfgBase.AppBaseURL, map[string]any{"modo": "instrutor"}, sessao), 200)
	if r.Body["_cache_control"] != "no-store" {
		t.Errorf("troca de modo deveria ser no-store: %v", r.Body["_cache_control"])
	}
	// Sem sessão: 401.
	esperar(t, chamarCookies(t, servidor, "POST", "/api/sessao/modo", cfgBase.AppBaseURL, map[string]any{"modo": "atleta"}), 401)
	// Conta de academia não tem modos.
	a, _ := novaAcademia(t)
	esperar(t, api(t, "POST", "/api/sessao/modo", a.Token, map[string]any{"modo": "atleta"}), 403)
	if m := me(t, a); m["perfil"] != "academia" || m["modo_ativo"] != "" || fmt.Sprint(m["modos_disponiveis"]) != "[]" {
		t.Errorf("/me da academia: %v", m)
	}
}

func TestModo_CookieSeguroMesmosAtributos(t *testing.T) {
	cfg := *cfgBase
	cfg.AppBaseURL = "https://gymscore.test"
	srv := novoServidor(&cfg)
	u, _ := instrutorAprovado(t, 0)
	r := chamar(t, srv, "POST", "/api/sessao/modo", u.Token, map[string]any{"modo": "instrutor"})
	esperar(t, r, 200)
	s := cookieDe(r, "__Host-jwt")
	if s == nil || !s.Secure || !s.HttpOnly || s.Path != "/" || s.Domain != "" || s.SameSite != http.SameSiteLaxMode || s.Value == "" {
		t.Fatalf("cookie de sessão seguro: %+v", r.Cookies)
	}
	if strings.Contains(r.Raw, s.Value) {
		t.Error("JWT no corpo")
	}
	if p := cookieDe(r, "__Host-modo"); p == nil || !p.Secure || !p.HttpOnly || p.Path != "/" || p.Domain != "" {
		t.Errorf("cookie de preferência seguro: %+v", r.Cookies)
	}
	if cookieDe(r, "jwt") != nil {
		t.Error("com Secure o cookie não pode sair sem o prefixo")
	}
}

// ─── B4-2: autorização por modo ───────────────────────────────────────────────

func TestModo_AutorizacaoPorModoConfereNoBanco(t *testing.T) {
	c := novoCenario(t) // instrutor vinculado, já no modo instrutor
	id := criarDesafio(t, c, 0, 5)
	tokenInstrutor := c.I.Token
	esperar(t, api(t, "GET", "/api/instrutor/agenda", c.I.Token, nil), 200)
	esperar(t, api(t, "GET", "/api/instrutor/vagas", c.I.Token, nil), 200)
	// No modo instrutor não age como atleta.
	esperarCodigo(t, acao(t, c.I, id, "entrar", nil), 403, "modo_incorreto")

	// No modo atleta: rotas de instrutor recusadas com modo_incorreto; staff do desafio também.
	modoAtleta(t, c.I)
	esperarCodigo(t, api(t, "GET", "/api/instrutor/agenda", c.I.Token, nil), 403, "modo_incorreto")
	esperarCodigo(t, api(t, "GET", "/api/instrutor/vagas", c.I.Token, nil), 403, "modo_incorreto")
	esperar(t, acao(t, c.I, id, "iniciar", nil), 403)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), c.I.Token, map[string]any{"id_usuario": 1}), 403)

	// Pedido revogado (rejeitado) depois da troca: o token com modo=instrutor é recusado pelo banco.
	db.Exec("UPDATE instrutores SET status = 'rejeitada' WHERE id = ?", c.InstrutorID)
	esperarCodigo(t, api(t, "GET", "/api/instrutor/agenda", tokenInstrutor, nil), 403, "modo_indisponivel")
	esperarCodigo(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), tokenInstrutor, nil), 403, "modo_indisponivel")
	esperarCodigo(t, acao(t, &user{Token: tokenInstrutor}, id, "iniciar", nil), 403, "modo_indisponivel")
	tmp := &user{Token: tokenInstrutor}
	if m := me(t, tmp); m["modo_ativo"] != "instrutor" || fmt.Sprint(m["modos_disponiveis"]) != "[atleta]" {
		t.Errorf("/me deve mostrar o modo do token fora dos disponíveis (o front volta ao atleta): %v", m)
	}
	db.Exec("UPDATE instrutores SET status = 'aprovada' WHERE id = ?", c.InstrutorID)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", tokenInstrutor, nil), 200)
	// Registro de instrutor removido: idem.
	u, iid := instrutorAprovado(t, 0)
	modoInstrutor(t, u)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 200)
	if err := db.Exec("DELETE FROM instrutores WHERE id = ?", iid).Error; err != nil {
		t.Fatal(err)
	}
	esperarCodigo(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 403, "modo_indisponivel")
	esperarCodigo(t, api(t, "GET", "/api/instrutor/vagas", u.Token, nil), 403, "modo_indisponivel")
}

func TestModo_PunicaoValeNosDoisModos(t *testing.T) {
	u, _ := instrutorAprovado(t, 0)
	tokenAtleta := u.Token
	modoInstrutor(t, u)
	// Suspensão: ações de participação/gestão bloqueadas nos dois modos.
	db.Exec("UPDATE usuarios SET suspenso_ate = ? WHERE id = ?", time.Now().Add(time.Hour), u.ID)
	c := novoCenario(t)
	id := desafioVagaAberta(t, c)
	esperarCodigo(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), u.Token, map[string]any{"lance_centavos": 0}), 403, "conta_suspensa")
	esperarCodigo(t, acao(t, &user{Token: tokenAtleta}, id, "entrar", nil), 403, "conta_suspensa")
	// Banimento: nenhuma rota em nenhum modo, inclusive a troca de modo.
	db.Exec("UPDATE usuarios SET suspenso_ate = NULL, banido_em = NOW(3) WHERE id = ?", u.ID)
	esperarCodigo(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 403, "conta_banida")
	esperarCodigo(t, api(t, "POST", "/api/sessao/modo", tokenAtleta, map[string]any{"modo": "instrutor"}), 403, "conta_banida")
}

func TestModo_PedidoPendenteNaoAfetaAtleta(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 0, 5)
	id2 := criarDesafio(t, c, 0, 5)
	u := novoUsuario(t)
	inscrever(t, id, u)
	// N4 antiga removida: o pedido de instrutor com inscrição ativa é aceito e nada muda para o atleta.
	esperar(t, api(t, "POST", "/api/instrutores", u.Token, map[string]any{"cref": "123456-G/SP"}), 201)
	var perfil string
	db.Raw("SELECT perfil FROM usuarios WHERE id = ?", u.ID).Scan(&perfil)
	if perfil != "atleta" {
		t.Errorf("o pedido não muda usuarios.perfil: %s", perfil)
	}
	inscrever(t, id2, u)
	esperar(t, acao(t, u, id2, "sair", nil), 200)
	// Presença do atleta com pedido pendente continua valendo.
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), c.I.Token, map[string]any{"id_usuario": u.ID}), 200)
	// Rejeitado: idem.
	db.Exec("UPDATE instrutores SET status = 'rejeitada' WHERE id_usuario = ?", u.ID)
	inscrever(t, id2, u)
}

// ─── B4-3: conflito de interesse ──────────────────────────────────────────────

func TestConflito_InstrutorCompeteExcetoOndeTrabalha(t *testing.T) {
	c := novoCenario(t) // I vinculado à academia A
	idA := criarDesafio(t, c, 0, 5)
	b := novoCenario(t)
	idB := criarDesafio(t, b, 0, 5)
	modoAtleta(t, c.I)
	// Academia do vínculo: recusado; outra academia: aceito (instrutor continua competindo).
	esperar(t, acao(t, c.I, idA, "entrar", nil), 409)
	inscrever(t, idB, c.I)

	// Instrutor designado (vaga aberta escolhida) em desafio de academia sem vínculo não se inscreve.
	auto, _ := instrutorAprovado(t, 0)
	idV := desafioVagaAberta(t, b)
	modoInstrutor(t, auto)
	cid := uint(esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", idV), auto.Token, map[string]any{"lance_centavos": 0}), 201).data()["id"].(float64))
	modoAtleta(t, auto)
	esperar(t, acao(t, auto, idV, "entrar", nil), 409) // candidatura pendente
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", idV, cid), b.A.Token, nil), 200)
	esperar(t, acao(t, auto, idV, "entrar", nil), 409) // designado

	// Direção inversa: inscrito como atleta não se candidata nem é escolhido/designado.
	outro, _ := instrutorAprovado(t, 0)
	idV2 := desafioVagaAberta(t, b)
	inscrever(t, idV2, outro)
	modoInstrutor(t, outro)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", idV2), outro.Token, map[string]any{"lance_centavos": 0}), 409)
	// Indicação pela academia (rodada 5: POST /equipe) de instrutor vinculado e inscrito (estado legado).
	vinc, vincID := instrutorAprovado(t, b.AcademiaID)
	db.Exec("UPDATE instrutores SET id_academia = NULL WHERE id = ?", vincID)
	inscrever(t, idB, vinc)
	db.Exec("UPDATE instrutores SET id_academia = ? WHERE id = ?", b.AcademiaID, vincID)
	esperar(t, api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", idB), b.A.Token, map[string]any{"instrutores_vagas": 2}), 200)
	rEq := esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", idB), b.A.Token, map[string]any{"id_instrutor": vincID}), 409)
	if !strings.Contains(rEq.Raw, "inscrito como atleta") {
		t.Errorf("409 deveria ser o conflito de inscrito: %s", rEq.Raw)
	}
	// O instrutor de outra academia é elegível a presença/pódio em B; o inscrito "de dentro" não.
	esperar(t, acao(t, b.I, idB, "iniciar", nil), 200)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/presenca", idB), b.I.Token, map[string]any{"id_usuario": c.I.ID}), 200)
	r := api(t, "POST", fmt.Sprintf("/api/desafios/%d/presenca", idB), b.I.Token, map[string]any{"id_usuario": vinc.ID})
	if r.Status == 200 {
		t.Errorf("instrutor vinculado à academia não pode ter presença como atleta: %s", r.Raw)
	}
}

func TestConflito_AceiteDeConviteComInscricaoAtiva(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 0, 5)
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/instrutores", u.Token, map[string]any{"cref": "123456-G/SP"}), 201)
	inscrever(t, id, u) // pendente: inscreve normalmente
	esperar(t, api(t, "POST", "/api/academia/instrutores", c.A.Token, map[string]any{"email": u.Email}), 201)
	esperar(t, api(t, "POST", "/api/instrutor/convite/aceitar", u.Token, nil), 409)
	esperar(t, acao(t, u, id, "sair", nil), 200)
	esperar(t, api(t, "POST", "/api/instrutor/convite/aceitar", u.Token, nil), 200)
}

// Corrida inscrição × aceite de convite da mesma academia: nunca as duas vencem (trava de linha do
// instrutor nas duas direções), e nenhuma termina em erro interno.
func TestConflito_InscricaoXAceiteConcorrentes(t *testing.T) {
	var ambos, rodadas atomic.Int32
	for k := 0; k < 12; k++ {
		c := novoCenario(t)
		id := criarDesafio(t, c, 0, 5)
		u, iid := instrutorAprovado(t, 0)
		esperar(t, api(t, "POST", "/api/academia/instrutores", c.A.Token, map[string]any{"email": u.Email}), 201)
		var wg sync.WaitGroup
		var stEntrar, stAceitar int
		wg.Add(2)
		go func() {
			defer wg.Done()
			stEntrar = api(t, "POST", fmt.Sprintf("/api/desafios/%d/entrar", id), u.Token, nil).Status
		}()
		go func() {
			defer wg.Done()
			stAceitar = api(t, "POST", "/api/instrutor/convite/aceitar", u.Token, nil).Status
		}()
		wg.Wait()
		rodadas.Add(1)
		if stEntrar >= 500 || stAceitar >= 500 {
			t.Fatalf("erro interno: entrar=%d aceitar=%d", stEntrar, stAceitar)
		}
		var inscrito int64
		db.Raw("SELECT COUNT(*) FROM desafio_participantes WHERE id_desafio = ? AND id_usuario = ?", id, u.ID).Scan(&inscrito)
		var vinc *uint
		db.Raw("SELECT id_academia FROM instrutores WHERE id = ?", iid).Scan(&vinc)
		if inscrito > 0 && vinc != nil {
			ambos.Add(1)
			t.Errorf("rodada %d: inscrito E vinculado (entrar=%d aceitar=%d)", k, stEntrar, stAceitar)
		}
		if (stEntrar == 200) != (inscrito > 0) || (stAceitar == 200) != (vinc != nil) {
			t.Errorf("status x banco: entrar=%d/%d aceitar=%d/%v", stEntrar, inscrito, stAceitar, vinc)
		}
	}
	if ambos.Load() != 0 {
		t.Fatalf("%d de %d rodadas com conflito", ambos.Load(), rodadas.Load())
	}
}

// Corrida inscrição (token modo atleta) × candidatura (token modo instrutor) da MESMA conta no mesmo
// desafio: as duas sessões valem ao mesmo tempo (mesma token_versao), mas só uma ação vence.
func TestConflito_InscricaoXCandidaturaConcorrentes(t *testing.T) {
	b := novoCenario(t)
	for k := 0; k < 12; k++ {
		id := desafioVagaAberta(t, b)
		u, iid := instrutorAprovado(t, 0)
		atleta := &user{Token: u.Token}
		modoInstrutor(t, u)
		var wg sync.WaitGroup
		var stEntrar, stCand int
		wg.Add(2)
		go func() {
			defer wg.Done()
			stEntrar = api(t, "POST", fmt.Sprintf("/api/desafios/%d/entrar", id), atleta.Token, nil).Status
		}()
		go func() {
			defer wg.Done()
			stCand = api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), u.Token, map[string]any{"lance_centavos": 0}).Status
		}()
		wg.Wait()
		if stEntrar >= 500 || stCand >= 500 {
			t.Fatalf("erro interno: entrar=%d candidatar=%d", stEntrar, stCand)
		}
		var inscrito, cands int64
		db.Raw("SELECT COUNT(*) FROM desafio_participantes WHERE id_desafio = ? AND id_usuario = ?", id, u.ID).Scan(&inscrito)
		db.Raw("SELECT COUNT(*) FROM desafio_candidaturas_instrutor WHERE id_desafio = ? AND id_instrutor = ? AND status = 'pendente'", id, iid).Scan(&cands)
		if inscrito > 0 && cands > 0 {
			t.Fatalf("rodada %d: inscrito E candidato (entrar=%d candidatar=%d)", k, stEntrar, stCand)
		}
		if inscrito+cands != 1 {
			t.Errorf("rodada %d: exatamente uma das ações deveria vencer: entrar=%d candidatar=%d", k, stEntrar, stCand)
		}
	}
}

// ─── B4-4: endereço da academia e CEP ─────────────────────────────────────────

func enderecoOK() map[string]any {
	return map[string]any{"cep": "01310-100", "logradouro": " Av.  Paulista ", "numero": "1000", "complemento": "Sala 5",
		"bairro": "Bela Vista", "cidade": "São Paulo", "uf": "sp"}
}

func TestEndereco_CadastroValidaEMontaTexto(t *testing.T) {
	base := func(extra map[string]any) map[string]any {
		n := seq.Add(1)
		b := map[string]any{"nome": "Acad", "email": fmt.Sprintf("qa+end%d@gymscore.test", n), "senha": senhaPadrao,
			"razao_social": "Academia End LTDA", "nome_fantasia": fmt.Sprintf("End %d", n), "cnpj": cnpjUnico(),
			"endereco": "ignorado"}
		for k, v := range enderecoOK() {
			b[k] = v
		}
		for k, v := range extra {
			if v == nil {
				delete(b, k)
			} else {
				b[k] = v
			}
		}
		return b
	}
	for nome, extra := range map[string]map[string]any{
		"sem número": {"numero": nil}, "número em branco": {"numero": "  "}, "CEP 7 dígitos": {"cep": "1310100"},
		"CEP com letras": {"cep": "0131O-100"}, "UF inexistente": {"uf": "XX"}, "sem cidade": {"cidade": nil},
		"sem CEP": {"cep": nil},
	} {
		if r := api(t, "POST", "/api/academias", "", base(extra)); r.Status != 400 {
			t.Errorf("%s: esperava 400, veio %d %s", nome, r.Status, r.Raw)
		}
	}
	r := esperar(t, api(t, "POST", "/api/academias", "", base(nil)), 201)
	d := r.data()
	if d["cep"] != "01310100" || d["uf"] != "SP" || d["logradouro"] != "Av. Paulista" || d["complemento"] != "Sala 5" ||
		d["endereco"] != "Av. Paulista, 1000 - Sala 5 - Bela Vista, São Paulo - SP, CEP 01310-100" {
		t.Errorf("endereço gravado: %v", d)
	}
	// Tamanho máximo de todos os campos cabe no VARCHAR(500).
	longo := base(map[string]any{"logradouro": strings.Repeat("L", 150), "numero": strings.Repeat("9", 20),
		"complemento": strings.Repeat("C", 100), "bairro": strings.Repeat("B", 100), "cidade": strings.Repeat("ç", 100)})
	if r := esperar(t, api(t, "POST", "/api/academias", "", longo), 201); len([]rune(r.data()["endereco"].(string))) > 500 {
		t.Errorf("endereco passou de 500")
	}
}

func TestEndereco_EdicaoPelaAcademia(t *testing.T) {
	a, aid := novaAcademia(t)
	// Academia antiga (só texto livre).
	db.Exec("UPDATE academias SET cep = NULL, logradouro = NULL, numero = NULL, bairro = NULL, cidade = NULL, uf = NULL, endereco = 'Rua velha 1' WHERE id = ?", aid)
	d := esperar(t, api(t, "GET", "/api/academia/perfil", a.Token, nil), 200).data()
	if d["endereco"] != "Rua velha 1" || d["cep"] != nil {
		t.Errorf("academia antiga: %v", d)
	}
	b := enderecoOK()
	b["numero"], b["complemento"], b["telefone"] = "S/N", "", "11999990000"
	d = esperar(t, api(t, "PATCH", "/api/academia/perfil", a.Token, b), 200).data()
	if d["numero"] != "S/N" || d["complemento"] != nil || d["telefone"] != "11999990000" ||
		d["endereco"] != "Av. Paulista, S/N - Bela Vista, São Paulo - SP, CEP 01310-100" {
		t.Errorf("edição: %v", d)
	}
	b["uf"] = "ZZ"
	esperar(t, api(t, "PATCH", "/api/academia/perfil", a.Token, b), 400)
	// Atleta não edita academia.
	esperar(t, api(t, "PATCH", "/api/academia/perfil", novoUsuario(t).Token, enderecoOK()), 403)
}

// academiaRepoForcaCheck repassa ao repositório real um UPDATE com UF fora do CHECK (simula um valor que
// escapou da validação) para provar o mapeamento 3819 → 422.
type academiaRepoForcaCheck struct {
	repositories.AcademiaRepository
}

func (r academiaRepoForcaCheck) Atualizar(id uint, campos map[string]any) error {
	campos["uf"] = "sp"
	return r.AcademiaRepository.Atualizar(id, campos)
}

func TestEndereco_CheckDoBancoVira422(t *testing.T) {
	a, _ := novaAcademia(t)
	repo := repositories.NovoAcademiaRepository(db)
	svc := services.NovoAcademiaGestaoService(db, academiaRepoForcaCheck{repo}, repositories.NovoDesafioRepository(db),
		repositories.NovoPainelRepository(db), repositories.NovoVinculoInstrutorRepository(db), nil)
	req := &models.AtualizarAcademiaRequest{EnderecoAcademiaRequest: models.EnderecoAcademiaRequest{
		CEP: "01310100", Logradouro: "Rua", Numero: "1", Bairro: "B", Cidade: "C", UF: "SP"}}
	_, err := svc.AtualizarPerfil(a.ID, req)
	e, ok := services.AsErroNegocio(err)
	if !ok || e.Status != 422 || !strings.Contains(e.Mensagem, "CEP") {
		t.Fatalf("3819 deveria virar 422 com mensagem clara: %v", err)
	}
	if !repositories.EhViolacaoCheck(repo.Atualizar(0, map[string]any{"cep": "123"})) {
		// id 0 não existe, mas o CHECK é avaliado só em linha afetada: confere numa linha real.
		var id uint
		db.Raw("SELECT id FROM academias WHERE id_usuario = ?", a.ID).Scan(&id)
		if !repositories.EhViolacaoCheck(repo.Atualizar(id, map[string]any{"cep": "123"})) {
			t.Error("CEP de 3 dígitos deveria violar chk_academias_cep (3819)")
		}
	}
}

// viaCEPFake é o ViaCEP de teste (httptest): conta as chamadas e responde por CEP.
func viaCEPFake(t *testing.T, chamadas *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ws/01310100/json/":
			_, _ = w.Write([]byte(`{"cep":"01310-100","logradouro":"Avenida Paulista","complemento":"de 612 a 1510 - lado par","bairro":"Bela Vista","localidade":"São Paulo","uf":"SP"}`))
		case "/ws/99999999/json/":
			_, _ = w.Write([]byte(`{"erro": true}`))
		case "/ws/88888888/json/":
			_, _ = w.Write([]byte(`{"erro": "true"}`))
		case "/ws/77777777/json/":
			w.WriteHeader(http.StatusInternalServerError)
		case "/ws/66666666/json/":
			time.Sleep(4 * time.Second) // passa do timeout de 3s
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCEP_ProxyCacheErrosERateLimit(t *testing.T) {
	var chamadas atomic.Int32
	fake := viaCEPFake(t, &chamadas)
	cfg := *cfgBase
	cfg.ViaCEPURL = fake.URL + "/ws"
	cfg.CEPRateLimitMax = 1000
	srv := novoServidor(&cfg)

	r := esperar(t, chamar(t, srv, "GET", "/api/cep/01310-100", "", nil), 200)
	d := r.data()
	if d["cep"] != "01310100" || d["logradouro"] != "Avenida Paulista" || d["bairro"] != "Bela Vista" ||
		d["cidade"] != "São Paulo" || d["uf"] != "SP" {
		t.Errorf("CEP encontrado: %v", d)
	}
	esperar(t, chamar(t, srv, "GET", "/api/cep/01310100", "", nil), 200)
	if chamadas.Load() != 1 {
		t.Errorf("a segunda consulta deveria vir do cache: %d chamadas", chamadas.Load())
	}
	esperar(t, chamar(t, srv, "GET", "/api/cep/99999999", "", nil), 404)
	esperar(t, chamar(t, srv, "GET", "/api/cep/99999999", "", nil), 404)
	esperar(t, chamar(t, srv, "GET", "/api/cep/88888888", "", nil), 404)
	if chamadas.Load() != 3 {
		t.Errorf("não encontrado também vai para o cache: %d chamadas", chamadas.Load())
	}
	esperar(t, chamar(t, srv, "GET", "/api/cep/1234", "", nil), 400)
	esperar(t, chamar(t, srv, "GET", "/api/cep/abcdefgh", "", nil), 400)
	antes := chamadas.Load()
	esperarCodigo(t, chamar(t, srv, "GET", "/api/cep/77777777", "", nil), 503, "cep_indisponivel")
	esperarCodigo(t, chamar(t, srv, "GET", "/api/cep/77777777", "", nil), 503, "cep_indisponivel")
	if chamadas.Load() != antes+2 {
		t.Errorf("indisponibilidade não vai para o cache")
	}
	ini := time.Now()
	esperarCodigo(t, chamar(t, srv, "GET", "/api/cep/66666666", "", nil), 503, "cep_indisponivel")
	if dur := time.Since(ini); dur > 3500*time.Millisecond {
		t.Errorf("timeout curto não respeitado: %v", dur)
	}
	// ViaCEP fora do ar não bloqueia o cadastro (o cadastro não consulta o CEP).
	n := seq.Add(1)
	b := enderecoOK()
	b["cep"] = "77777-777"
	for k, v := range map[string]any{"nome": "Acad", "email": fmt.Sprintf("qa+cep%d@gymscore.test", n), "senha": senhaPadrao,
		"razao_social": "Academia CEP LTDA", "nome_fantasia": fmt.Sprintf("CEP %d", n), "cnpj": cnpjUnico()} {
		b[k] = v
	}
	esperar(t, chamar(t, srv, "POST", "/api/academias", "", b), 201)

	// Rate limit por IP (nunca desligado).
	cfg2 := *cfgBase
	cfg2.ViaCEPURL = fake.URL + "/ws"
	cfg2.CEPRateLimitMax = 3
	srv2 := novoServidor(&cfg2)
	for i := 0; i < 3; i++ {
		esperar(t, chamar(t, srv2, "GET", "/api/cep/01310100", "", nil), 200)
	}
	esperar(t, chamar(t, srv2, "GET", "/api/cep/01310100", "", nil), 429)
}
