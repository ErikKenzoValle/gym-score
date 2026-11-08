//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/app"
	"gymScore-backend/internal/cli"
	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

// ─── Helpers do painel admin ─────────────────────────────────────────────────

const origemTeste = "http://localhost:3000" // = cfgBase.AppBaseURL

type adminT struct {
	ID     uint
	Email  string
	Senha  string
	Cookie string // valor do cookie admin_jwt
}

// novoAdminDB cria o admin direto no banco (o 1º superadmin nasce pela CLI; aqui só montamos cenário).
func novoAdminDB(t *testing.T, papel string) *adminT {
	t.Helper()
	n := seq.Add(1)
	a := &adminT{Email: fmt.Sprintf("admin%d_%d@gymscore.test", os.Getpid(), n), Senha: "senha-admin-forte-123"}
	hash, _ := bcrypt.GenerateFromPassword([]byte(a.Senha), bcrypt.MinCost)
	m := &models.Administrador{Nome: fmt.Sprintf("Admin %d", n), Email: a.Email, SenhaHash: string(hash), Papel: papel, Ativo: true}
	if err := db.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	a.ID = m.ID
	a.Cookie = loginAdmin(t, servidor, a.Email, a.Senha)
	return a
}

// chamarAdmin faz a requisição como o navegador: cookie admin_jwt + Origin da própria aplicação.
func chamarAdmin(t *testing.T, srv *fiber.App, metodo, caminho, cookie string, corpo any) (resp, *http.Response) {
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
	req.Header.Set("Origin", origemTeste)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "admin_jwt", Value: cookie})
	}
	res, err := srv.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out, res
}

func adm(t *testing.T, metodo, caminho string, a *adminT, corpo any) resp {
	t.Helper()
	r, _ := chamarAdmin(t, servidor, metodo, caminho, a.Cookie, corpo)
	return r
}

func motivo(m string) map[string]any { return map[string]any{"motivo": m} }

// loginAdmin faz o login e devolve o valor do cookie admin_jwt (confere os atributos do cookie).
func loginAdmin(t *testing.T, srv *fiber.App, email, senha string) string {
	t.Helper()
	r, res := chamarAdmin(t, srv, "POST", "/api/admin/auth/login", "", map[string]any{"email": email, "senha": senha})
	esperar(t, r, 200)
	if strings.Contains(r.Raw, "eyJ") {
		t.Errorf("o token não deveria vir no corpo: %s", r.Raw)
	}
	for _, c := range res.Cookies() {
		if c.Name == "admin_jwt" {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/admin" {
				t.Errorf("atributos do cookie admin_jwt: %+v", c)
			}
			return c.Value
		}
	}
	t.Fatalf("login sem cookie admin_jwt: %v", res.Header)
	return ""
}

func contarAuditoria(t *testing.T, acao, alvoTipo string, alvoID uint) int64 {
	t.Helper()
	var n int64
	db.Model(&models.AdminAuditoria{}).Where("acao = ? AND alvo_tipo = ? AND alvo_id = ?", acao, alvoTipo, alvoID).Count(&n)
	return n
}

// ─── Autenticação e separação de tokens ──────────────────────────────────────

func TestAdmin_AutenticacaoSeparadaDosUsuarios(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	u := novoUsuario(t)

	// token de usuário não vale no painel (header ou cookie jwt)
	esperar(t, api(t, "GET", "/api/admin/auth/me", u.Token, nil), 401)
	req := httptest.NewRequest("GET", "/api/admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "jwt", Value: u.Token})
	if res, _ := servidor.Test(req, -1); res.StatusCode != 401 {
		t.Errorf("cookie jwt de usuário no painel: %d", res.StatusCode)
	}
	// token admin não vale nas rotas de usuário
	esperar(t, api(t, "GET", "/api/usuarios/me", a.Cookie, nil), 401)
	esperar(t, api(t, "GET", "/api/carteira", a.Cookie, nil), 401)

	// o painel aceita o próprio cookie e o mesmo token por Bearer
	me := esperar(t, adm(t, "GET", "/api/admin/auth/me", a, nil), 200).data()
	if uint(me["id"].(float64)) != a.ID || me["papel"] != "admin" || strings.Contains(fmt.Sprint(me), "senha") {
		t.Errorf("me: %v", me)
	}
	esperar(t, api(t, "GET", "/api/admin/auth/me", a.Cookie, nil), 200)

	// login: mensagem genérica (senha errada = e-mail inexistente), auditoria do sucesso e da falha
	errado, _ := chamarAdmin(t, servidor, "POST", "/api/admin/auth/login", "", map[string]any{"email": a.Email, "senha": "x"})
	inexistente, _ := chamarAdmin(t, servidor, "POST", "/api/admin/auth/login", "", map[string]any{"email": "ninguem@gymscore.test", "senha": "x"})
	if errado.Status != 401 || inexistente.Status != 401 || errado.Body["error"] != inexistente.Body["error"] {
		t.Errorf("login deveria ser genérico: %s | %s", errado.Raw, inexistente.Raw)
	}
	if contarAuditoria(t, "login", "administrador", a.ID) != 1 || contarAuditoria(t, "login_falhou", "administrador", a.ID) != 1 {
		t.Errorf("auditoria do login ausente")
	}
	var adb models.Administrador
	db.First(&adb, a.ID)
	if adb.UltimoLoginEm == nil {
		t.Errorf("ultimo_login_em não gravado")
	}
	// login sem Origin (formulário de outro site) é recusado
	b, _ := json.Marshal(map[string]any{"email": a.Email, "senha": a.Senha})
	req = httptest.NewRequest("POST", "/api/admin/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	if res, _ := servidor.Test(req, -1); res.StatusCode != 403 {
		t.Errorf("login de outra origem: %d", res.StatusCode)
	}
	// POST autenticado por cookie sem Origin: 403
	req = httptest.NewRequest("POST", "/api/admin/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "admin_jwt", Value: a.Cookie})
	if res, _ := servidor.Test(req, -1); res.StatusCode != 403 {
		t.Errorf("POST admin por cookie sem Origin: %d", res.StatusCode)
	}
	// rota admin inexistente: 404 (não cai no grupo de usuários)
	esperar(t, adm(t, "GET", "/api/admin/nao-existe", a, nil), 404)

	// logout revoga o token
	esperar(t, adm(t, "POST", "/api/admin/auth/logout", a, nil), 200)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", a, nil), 401)
	if contarAuditoria(t, "logout", "administrador", a.ID) != 1 {
		t.Errorf("logout sem auditoria")
	}

	// troca da própria senha: cookie novo, senha antiga deixa de valer
	a.Cookie = loginAdmin(t, servidor, a.Email, a.Senha)
	esperar(t, adm(t, "POST", "/api/admin/auth/senha", a, map[string]any{"senha_atual": "errada", "nova_senha": "nova-senha-forte-456"}), 400)
	esperar(t, adm(t, "POST", "/api/admin/auth/senha", a, map[string]any{"senha_atual": a.Senha, "nova_senha": "curta"}), 400)
	_, res := chamarAdmin(t, servidor, "POST", "/api/admin/auth/senha", a.Cookie, map[string]any{"senha_atual": a.Senha, "nova_senha": "nova-senha-forte-456"})
	if res.StatusCode != 200 {
		t.Fatalf("troca de senha: %d", res.StatusCode)
	}
	esperar(t, adm(t, "GET", "/api/admin/auth/me", a, nil), 401) // token antigo revogado
	for _, c := range res.Cookies() {
		if c.Name == "admin_jwt" {
			a.Cookie = c.Value
		}
	}
	esperar(t, adm(t, "GET", "/api/admin/auth/me", a, nil), 200)
	r, _ := chamarAdmin(t, servidor, "POST", "/api/admin/auth/login", "", map[string]any{"email": a.Email, "senha": a.Senha})
	esperar(t, r, 401)
	loginAdmin(t, servidor, a.Email, "nova-senha-forte-456")
}

func TestAdmin_ComumRecebe403EmAcoesDeSuperadmin(t *testing.T) {
	comum := novoAdminDB(t, models.PapelAdmin)
	super := novoAdminDB(t, models.PapelSuperadmin)
	esperar(t, adm(t, "GET", "/api/admin/dashboard", comum, nil), 200)
	esperar(t, adm(t, "POST", "/api/admin/saques/1/reembolsar", comum, motivo("tentativa")), 403)
	esperar(t, adm(t, "POST", "/api/admin/saques/1/marcar-pago", comum, motivo("tentativa")), 403)
	esperar(t, adm(t, "GET", "/api/admin/administradores", comum, nil), 403)
	esperar(t, adm(t, "POST", "/api/admin/administradores", comum, map[string]any{
		"nome": "X", "email": "x@gymscore.test", "papel": "superadmin", "motivo": "tentativa"}), 403)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/desativar", super.ID), comum, motivo("tentativa")), 403)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/papel", comum.ID), comum,
		map[string]any{"papel": "superadmin", "motivo": "auto-promoção"}), 403)
	esperar(t, adm(t, "GET", "/api/admin/administradores", super, nil), 200)
	var n int64
	db.Model(&models.AdminAuditoria{}).Where("id_admin = ? AND acao NOT IN ?", comum.ID, []string{"login"}).Count(&n)
	if n != 0 {
		t.Errorf("ação recusada não pode gerar auditoria: %d", n)
	}
}

// ─── Saques ──────────────────────────────────────────────────────────────────

// saqueEmRevisao cria um saque ambíguo (timeout no Asaas): fica pending, em revisão, valor e tarifa debitados.
func saqueEmRevisao(t *testing.T, srv *fiber.App, u *user, valor int64) uint {
	t.Helper()
	asaas.modoTransf(modoTimeout)
	r := sacarS(t, srv, u, valor)
	asaas.modoTransf("")
	esperar(t, r, 201)
	return idDe(r)
}

func TestAdmin_ReembolsoIdempotenteEAuditado(t *testing.T) {
	srv, _ := servidorComJobs(t, "inscricao_saque", 200, 48*time.Hour)
	super := novoAdminDB(t, models.PapelSuperadmin)
	comum := novoAdminDB(t, models.PapelAdmin)
	u := usuarioComSaldo(t, 5000)
	id := saqueEmRevisao(t, srv, u, 1000)
	if saldo(t, u) != 3800 {
		t.Fatalf("saque + tarifa debitados: %d", saldo(t, u))
	}
	// listagem: em revisão, tarifa e referência do Asaas
	lista := esperar(t, adm(t, "GET", "/api/admin/saques?status=pending&em_revisao=true&limite=100", comum, nil), 200)
	achou := false
	for _, it := range itensDe(t, lista) {
		if uint(it["id"].(float64)) == id {
			achou = true
			if num(it, "tarifa_centavos") != 200 || it["referencia_asaas"] != fmt.Sprintf("saque-%d", id) || num(it, "valor_saque_centavos") != 1000 {
				t.Errorf("item do saque: %v", it)
			}
		}
	}
	if !achou {
		t.Fatalf("saque em revisão não listado: %s", lista.Raw)
	}
	// detalhe da transação traz a tarifa como derivada
	det := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/transacoes/%d", id), comum, nil), 200).data()
	if der := det["derivadas"].([]any); len(der) != 1 || der[0].(map[string]any)["tipo"] != "tarifa_saque" {
		t.Errorf("derivadas: %v", det["derivadas"])
	}
	// reconciliar sem transferência registrada: 409
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reconciliar", id), comum, motivo("conferência")), 409)
	// sem motivo: 400
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id), super, map[string]any{}), 400)

	r1 := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id), super, motivo("Asaas sem a referência")), 200).data()
	if r1["alterado"] != true || sub(t, r1, "saque")["status"] != "failed" {
		t.Errorf("reembolso: %v", r1)
	}
	if saldo(t, u) != 5000 {
		t.Errorf("valor + tarifa devolvidos: saldo %d", saldo(t, u))
	}
	// idempotente: segunda chamada não devolve de novo nem audita
	r2 := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id), super, motivo("de novo")), 200).data()
	if r2["alterado"] != false || saldo(t, u) != 5000 {
		t.Errorf("reembolso repetido: %v saldo %d", r2, saldo(t, u))
	}
	if n := contarAuditoria(t, "saque_reembolsar", "transacao", id); n != 1 {
		t.Errorf("auditoria do reembolso: %d linhas", n)
	}
	var aud models.AdminAuditoria
	db.Where("acao = 'saque_reembolsar' AND alvo_id = ?", id).First(&aud)
	if aud.IDAdmin != super.ID || aud.Motivo != "Asaas sem a referência" || aud.IP == "" {
		t.Errorf("linha de auditoria: %+v", aud)
	}
	// saque reembolsado não pode ser marcado pago
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/marcar-pago", id), super, motivo("engano")), 409)
	ledgerConfere(t, u)
	if contarNotif(t, u, models.NotifSaqueFalhou) < 1 {
		t.Errorf("usuário deveria ser avisado do reembolso")
	}
}

func TestAdmin_MarcarPagoEReconciliar(t *testing.T) {
	srv, _ := servidorComJobs(t, "inscricao_saque", 200, 48*time.Hour)
	super := novoAdminDB(t, models.PapelSuperadmin)
	u := usuarioComSaldo(t, 5000)
	id := saqueEmRevisao(t, srv, u, 1000)
	r := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/marcar-pago", id), super, motivo("pago no painel do Asaas")), 200).data()
	if r["alterado"] != true || sub(t, r, "saque")["status"] != "concluido" || num(sub(t, r, "saque"), "tarifa_centavos") != 200 {
		t.Errorf("marcar pago: %v", r)
	}
	r = esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/marcar-pago", id), super, motivo("de novo")), 200).data()
	if r["alterado"] != false {
		t.Errorf("marcar pago repetido: %v", r)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id), super, motivo("engano")), 409)
	if saldo(t, u) != 3800 || contarAuditoria(t, "saque_marcar_pago", "transacao", id) != 1 {
		t.Errorf("saldo %d / auditoria", saldo(t, u))
	}
	ledgerConfere(t, u)

	// reconciliar um saque com transferência: o Asaas diz FAILED -> reembolsa e audita
	u2 := usuarioComSaldo(t, 3000)
	s := esperar(t, sacarS(t, srv, u2, 1000), 201)
	id2 := idDe(s)
	var tr models.Transacao
	db.First(&tr, id2)
	asaas.definirTransf(*tr.AsaasTransferID, "FAILED")
	comum := novoAdminDB(t, models.PapelAdmin)
	rec := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reconciliar", id2), comum, motivo("usuário reclamou")), 200).data()
	if rec["status_asaas"] != "FAILED" || rec["alterado"] != true || saldo(t, u2) != 3000 {
		t.Errorf("reconciliar: %v saldo %d", rec, saldo(t, u2))
	}
	if contarAuditoria(t, "saque_reconciliar", "transacao", id2) != 1 {
		t.Errorf("reconciliar sem auditoria")
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reconciliar", id2), comum, motivo("de novo")), 409) // não está mais pending
	// reembolsar com o Asaas dizendo DONE é recusado
	u3 := usuarioComSaldo(t, 3000)
	id3 := idDe(esperar(t, sacarS(t, srv, u3, 1000), 201))
	var tr3 models.Transacao
	db.First(&tr3, id3)
	asaas.definirTransf(*tr3.AsaasTransferID, "DONE")
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/saques/%d/reembolsar", id3), super, motivo("engano")), 409)
	ledgerConfere(t, u2, u3)
}

// ─── Administradores ─────────────────────────────────────────────────────────

func TestAdmin_UltimoSuperadminProtegido(t *testing.T) {
	s1 := novoAdminDB(t, models.PapelSuperadmin)
	// isola o cenário: os demais superadmins (de outros testes) ficam inativos
	db.Exec("UPDATE administradores SET ativo = 0 WHERE papel = 'superadmin' AND id <> ?", s1.ID)

	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/desativar", s1.ID), s1, motivo("sair")), 409)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/papel", s1.ID), s1,
		map[string]any{"papel": "admin", "motivo": "rebaixar"}), 409)

	// cria um admin comum pela tela: senha temporária devolvida uma vez e login funciona
	c := esperar(t, adm(t, "POST", "/api/admin/administradores", s1, map[string]any{
		"nome": "Operador", "email": fmt.Sprintf("op%d@gymscore.test", seq.Add(1)), "papel": "admin", "motivo": "novo operador"}), 201).data()
	novo := sub(t, c, "admin")
	tmp, _ := c["senha_temporaria"].(string)
	if len(tmp) < 12 || novo["papel"] != "admin" {
		t.Fatalf("criação: %v", c)
	}
	idNovo := uint(novo["id"].(float64))
	op := &adminT{ID: idNovo, Email: novo["email"].(string), Senha: tmp}
	op.Cookie = loginAdmin(t, servidor, op.Email, op.Senha)
	esperar(t, adm(t, "POST", "/api/admin/administradores", s1, map[string]any{
		"nome": "Dup", "email": op.Email, "papel": "admin", "motivo": "duplicado"}), 409)

	// promove a superadmin: a sessão antiga cai (token_versao), e agora s1 pode se rebaixar
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/papel", idNovo), s1,
		map[string]any{"papel": "superadmin", "motivo": "promoção"}), 200)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", op, nil), 401)
	op.Cookie = loginAdmin(t, servidor, op.Email, op.Senha)
	esperar(t, adm(t, "GET", "/api/admin/administradores", op, nil), 200)

	// desativar: derruba a sessão; reativar; idempotente
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/desativar", idNovo), s1, motivo("férias")), 200)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", op, nil), 401)
	r, _ := chamarAdmin(t, servidor, "POST", "/api/admin/auth/login", "", map[string]any{"email": op.Email, "senha": op.Senha})
	esperar(t, r, 401) // desativado não loga (mensagem genérica)
	if d := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/desativar", idNovo), s1, motivo("de novo")), 200).data(); d["alterado"] != false {
		t.Errorf("desativar repetido: %v", d)
	}
	// com o outro desativado, s1 volta a ser o último
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/papel", s1.ID), s1,
		map[string]any{"papel": "admin", "motivo": "rebaixar"}), 409)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/reativar", idNovo), s1, motivo("voltou")), 200)
	op.Cookie = loginAdmin(t, servidor, op.Email, op.Senha)

	// resetar senha: a antiga para de valer e a nova funciona
	rs := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/resetar-senha", idNovo), s1, motivo("esqueceu")), 200).data()
	nova, _ := rs["senha_temporaria"].(string)
	esperar(t, adm(t, "GET", "/api/admin/auth/me", op, nil), 401)
	r, _ = chamarAdmin(t, servidor, "POST", "/api/admin/auth/login", "", map[string]any{"email": op.Email, "senha": op.Senha})
	esperar(t, r, 401)
	op.Cookie = loginAdmin(t, servidor, op.Email, nova)

	// com 2 superadmins ativos, s1 pode se rebaixar; aí o outro vira o último e não pode se rebaixar
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/papel", s1.ID), s1,
		map[string]any{"papel": "admin", "motivo": "rebaixar"}), 200)
	esperar(t, adm(t, "GET", "/api/admin/administradores", s1, nil), 401) // sessão caiu com a troca de papel
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/papel", idNovo), op,
		map[string]any{"papel": "admin", "motivo": "rebaixar"}), 409)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/administradores/%d/desativar", idNovo), op, motivo("sair")), 409)

	for _, acao := range []string{"admin_criar", "admin_trocar_papel", "admin_desativar", "admin_reativar", "admin_resetar_senha"} {
		if contarAuditoria(t, acao, "administrador", idNovo) == 0 {
			t.Errorf("auditoria ausente: %s", acao)
		}
	}
	// auditoria pela API, com filtros
	aud := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/auditoria?alvo_tipo=administrador&alvo_id=%d&acao=admin_criar", idNovo), op, nil), 200)
	if its := itensDe(t, aud); len(its) != 1 || its[0]["motivo"] != "novo operador" || uint(its[0]["id_admin"].(float64)) != s1.ID || its[0]["admin_email"] != s1.Email {
		t.Errorf("auditoria filtrada: %s", aud.Raw)
	}
	// restaura os superadmins dos outros testes
	db.Exec("UPDATE administradores SET ativo = 1 WHERE papel = 'superadmin'")
}

// ─── Usuários, cadastros e desafios ──────────────────────────────────────────

func TestAdmin_UsuariosBloqueioEConsulta(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	u := usuarioComSaldo(t, 2500)
	l := esperar(t, adm(t, "GET", "/api/admin/usuarios?q="+url.QueryEscape(u.Email), a, nil), 200)
	its := itensDe(t, l)
	if len(its) != 1 || !strings.HasPrefix(fmt.Sprint(its[0]["cpf_mascarado"]), "***.") || strings.Contains(l.Raw, "senha") {
		t.Fatalf("busca de usuário: %s", l.Raw)
	}
	d := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/usuarios/%d", u.ID), a, nil), 200).data()
	if d["ledger_confere"] != true || num(sub(t, d, "usuario"), "saldo_centavos") != 2500 || num(sub(t, d, "ledger"), "total") < 1 {
		t.Errorf("detalhe do usuário: %v", d)
	}
	esperar(t, adm(t, "GET", "/api/admin/usuarios/99999999", a, nil), 404)

	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), a, motivo("x")), 400) // motivo curto
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), a, motivo("fraude no PIX")), 200)
	esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 401) // token revogado
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 403)
	if r := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/bloquear", u.ID), a, motivo("de novo")), 200).data(); r["alterado"] != false {
		t.Errorf("bloqueio repetido: %v", r)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/usuarios/%d/desbloquear", u.ID), a, motivo("resolvido")), 200)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	if contarAuditoria(t, "usuario_bloquear", "usuario", u.ID) != 1 || contarAuditoria(t, "usuario_desbloquear", "usuario", u.ID) != 1 {
		t.Errorf("auditoria do bloqueio")
	}
}

func TestAdmin_AprovacaoDeAcademiaEInstrutor(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	n := seq.Add(1)
	email := fmt.Sprintf("qa+acadpend%d@gymscore.test", n)
	r := esperar(t, api(t, "POST", "/api/academias", "", map[string]any{
		"nome": "Pendente", "email": email, "senha": senhaPadrao, "razao_social": "Academia Pendente LTDA",
		"nome_fantasia": fmt.Sprintf("Pendente %d", n), "cnpj": cnpjUnico(),
	}), 201)
	acad := idDe(r)
	lista := esperar(t, adm(t, "GET", "/api/admin/academias?limite=100", a, nil), 200)
	if !idsDe(itensDe(t, lista), "id")[acad] {
		t.Errorf("academia pendente fora da fila")
	}
	login := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": email, "senha": senhaPadrao}), 200)
	tk := tokenDaSessao(t, login)
	esperar(t, api(t, "GET", "/api/academia/dashboard", tk, nil), 403)

	ap := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/academias/%d/aprovar", acad), a, motivo("documentos ok")), 200).data()
	if ap["status"] != "aprovada" {
		t.Errorf("aprovação: %v", ap)
	}
	esperar(t, api(t, "GET", "/api/academia/dashboard", tk, nil), 200)
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/academias/%d/rejeitar", acad), a, motivo("mudou de ideia")), 409)
	esperar(t, adm(t, "POST", "/api/admin/academias/99999999/aprovar", a, motivo("x y z")), 404)
	if contarAuditoria(t, "academia_aprovar", "academia", acad) != 1 {
		t.Errorf("auditoria da aprovação")
	}

	// instrutor: rejeição com motivo
	u := novoUsuario(t)
	iid := idDe(esperar(t, api(t, "POST", "/api/instrutores", u.Token, map[string]any{"cref": "123456-G/SP"}), 201))
	rj := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/instrutores/%d/rejeitar", iid), a, motivo("CREF inválido")), 200).data()
	if rj["status"] != "rejeitada" || rj["motivo_rejeicao"] != "CREF inválido" {
		t.Errorf("rejeição: %v", rj)
	}
	if contarNotif(t, u, models.NotifCadastroRejeitado) != 1 {
		t.Errorf("instrutor deveria ser avisado da rejeição")
	}
	todos := esperar(t, adm(t, "GET", "/api/admin/instrutores?status=rejeitada&limite=100", a, nil), 200)
	if !idsDe(itensDe(t, todos), "id")[iid] {
		t.Errorf("filtro rejeitada")
	}
	esperar(t, adm(t, "GET", "/api/admin/instrutores?status=xpto", a, nil), 400)
}

func TestAdmin_CancelarDesafioReusaOCancelamento(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	at := atletas(t, 2, 3000)
	inscrever(t, id, at...)
	saldoAcad := saldo(t, c.A)

	// detalhe antes: 2 participantes, ledger com as entradas
	d := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/desafios/%d", id), a, nil), 200).data()
	if len(d["participantes"].([]any)) != 2 || num(sub(t, sub(t, d, "financeiro"), "totais_por_tipo"), "entrada_desafio") != -2000 {
		t.Errorf("detalhe do desafio: %v", d)
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/desafios/%d/cancelar", id), a, map[string]any{}), 400)
	r := esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/desafios/%d/cancelar", id), a, motivo("denúncia de fraude")), 200).data()
	if sub(t, r, "desafio")["status"] != "cancelado" {
		t.Errorf("cancelamento: %v", r)
	}
	for _, u := range at {
		if saldo(t, u) != 3000 {
			t.Errorf("entrada não devolvida: %d", saldo(t, u))
		}
		if contarNotif(t, u, models.NotifDesafioCancel) != 1 {
			t.Errorf("atleta não avisado")
		}
	}
	if saldo(t, c.A) < saldoAcad {
		t.Errorf("reserva da academia não pode diminuir")
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/desafios/%d/cancelar", id), a, motivo("de novo")), 409)
	if contarAuditoria(t, "desafio_cancelar", "desafio", id) != 1 {
		t.Errorf("auditoria do cancelamento")
	}
	ledgerConfere(t, append(at, c.A)...)
	l := esperar(t, adm(t, "GET", "/api/admin/desafios?status=cancelado&limite=100", a, nil), 200)
	if !idsDe(itensDe(t, l), "id")[id] {
		t.Errorf("listagem por status")
	}
}

func TestAdmin_DashboardEListagens(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	d := esperar(t, adm(t, "GET", "/api/admin/dashboard?de=2020-01-01&ate=2099-12-31", a, nil), 200).data()
	for _, k := range []string{"financeiro", "conferencia", "receita", "pendencias", "desafios_por_status"} {
		if _, ok := d[k]; !ok {
			t.Errorf("dashboard sem %s: %v", k, d)
		}
	}
	conf := sub(t, d, "conferencia")
	if num(conf, "divergencia_centavos") != num(conf, "lado_aplicacoes_centavos")-num(conf, "lado_origens_centavos") {
		t.Errorf("conferência incoerente: %v", conf)
	}
	esperar(t, adm(t, "GET", "/api/admin/dashboard?de=ontem", a, nil), 400)
	esperar(t, adm(t, "GET", "/api/admin/transacoes?tipo=deposito&limite=5", a, nil), 200)
	esperar(t, adm(t, "GET", "/api/admin/transacoes?tipo=xpto", a, nil), 400)
	esperar(t, adm(t, "GET", "/api/admin/receitas", a, nil), 200)
	esperar(t, adm(t, "GET", "/api/admin/reservas", a, nil), 200)
	esperar(t, adm(t, "GET", "/api/admin/desafios?vencidos=true", a, nil), 200)
	esperar(t, adm(t, "GET", "/api/admin/auditoria?de=2020-01-01", a, nil), 200)
}

// ─── Páginas /admin (P12) ────────────────────────────────────────────────────

func TestAdmin_PaginasComCSP(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "admin", "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"login.html", "index.html", "js/admin.js"} {
		if err := os.WriteFile(filepath.Join(dir, "admin", f), []byte("<!doctype html><title>x</title>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := *cfgBase
	cfg.FrontendPath = dir
	srv := app.Construir(&cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
	for _, caminho := range []string{"/admin/login", "/admin", "/admin/js/admin.js"} {
		res, err := srv.Test(httptest.NewRequest("GET", caminho, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		csp := res.Header.Get("Content-Security-Policy")
		if res.StatusCode != 200 || !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") ||
			res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d %v", caminho, res.StatusCode, res.Header)
		}
	}
}

// ─── CLI ─────────────────────────────────────────────────────────────────────

func TestAdmin_CLICriaSuperadminEResetaSenha(t *testing.T) {
	email := fmt.Sprintf("cli%d@gymscore.test", seq.Add(1))
	var out bytes.Buffer
	senha := "senha-da-cli-forte-1"
	err := cli.Executar([]string{"criar-superadmin", "--email", email, "--nome", "Dona da Plataforma"}, db,
		cli.LeitorDeTexto(strings.NewReader(senha+"\n"+senha+"\n")), &out)
	if err != nil {
		t.Fatalf("CLI: %v", err)
	}
	if strings.Contains(out.String(), senha) {
		t.Errorf("a CLI não pode ecoar a senha: %s", out.String())
	}
	var a models.Administrador
	if err := db.Where("email = ?", email).First(&a).Error; err != nil || a.Papel != "superadmin" || !a.Ativo {
		t.Fatalf("superadmin criado: %+v %v", a, err)
	}
	if contarAuditoria(t, "cli_criar_superadmin", "administrador", a.ID) != 1 {
		t.Errorf("auditoria da CLI")
	}
	ck := loginAdmin(t, servidor, email, senha)

	// e-mail repetido, senhas diferentes, senha curta e senha por argumento: recusados
	if err := cli.Executar([]string{"criar-superadmin", "--email", email, "--nome", "Outra"}, db,
		cli.LeitorDeTexto(strings.NewReader(senha+"\n"+senha+"\n")), io.Discard); err == nil {
		t.Error("e-mail repetido aceito")
	}
	outro := fmt.Sprintf("cli%d@gymscore.test", seq.Add(1))
	if err := cli.Executar([]string{"criar-superadmin", "--email", outro, "--nome", "Outra"}, db,
		cli.LeitorDeTexto(strings.NewReader("senha-da-cli-forte-1\nsenha-da-cli-forte-2\n")), io.Discard); err == nil {
		t.Error("confirmação diferente aceita")
	}
	if err := cli.Executar([]string{"criar-superadmin", "--email", outro, "--nome", "Outra", "--senha", senha}, db,
		cli.LeitorDeTexto(strings.NewReader("")), io.Discard); err == nil {
		t.Error("senha por argumento aceita")
	}
	var n int64
	db.Model(&models.Administrador{}).Where("email = ?", outro).Count(&n)
	if n != 0 {
		t.Error("admin criado apesar do erro")
	}

	// resetar-senha: revoga a sessão e troca a senha
	nova := "nova-senha-da-cli-22"
	if err := cli.Executar([]string{"resetar-senha", "--email", email}, db,
		cli.LeitorDeTexto(strings.NewReader(nova+"\n"+nova+"\n")), io.Discard); err != nil {
		t.Fatal(err)
	}
	r, _ := chamarAdmin(t, servidor, "GET", "/api/admin/auth/me", ck, nil)
	esperar(t, r, 401)
	loginAdmin(t, servidor, email, nova)
}
