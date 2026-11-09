//go:build integration

package integration

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gymScore-backend/internal/cli"
	"gymScore-backend/internal/models"
)

// contasSeed são os e-mails de usuários (tabela usuarios) criados pelo seed-teste.
var contasSeed = []string{cli.EmailSeedAtleta, cli.EmailSeedInstrutor, cli.EmailSeedInstrutorPendente,
	cli.EmailSeedAcademia, cli.EmailSeedAcademiaPendente}

// contagensSeed fotografa as linhas que o seed cria (para conferir que rodar de novo não duplica).
func contagensSeed(t *testing.T) map[string]int64 {
	t.Helper()
	c := map[string]int64{}
	var n int64
	db.Model(&models.Usuario{}).Where("email IN ?", contasSeed).Count(&n)
	c["usuarios"] = n
	db.Model(&models.Administrador{}).Where("email = ?", cli.EmailSeedAdmin).Count(&n)
	c["admins"] = n
	db.Raw("SELECT COUNT(*) FROM academias a JOIN usuarios u ON u.id = a.id_usuario WHERE u.email IN ?", contasSeed).Scan(&n)
	c["academias"] = n
	db.Raw("SELECT COUNT(*) FROM instrutores i JOIN usuarios u ON u.id = i.id_usuario WHERE u.email IN ?", contasSeed).Scan(&n)
	c["instrutores"] = n
	db.Model(&models.Desafio{}).Where("titulo = ?", cli.TituloDesafioSeed).Count(&n)
	c["desafios"] = n
	db.Raw("SELECT COUNT(*) FROM transacoes t JOIN usuarios u ON u.id = t.id_usuario WHERE u.email IN ?", contasSeed).Scan(&n)
	c["transacoes"] = n
	return c
}

func loginSeed(t *testing.T, email string) (*user, map[string]any) {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": email, "senha": cli.SenhaContasTeste}), 200)
	u := r.data()["usuario"].(map[string]any)
	return &user{ID: uint(u["id"].(float64)), Email: email, Senha: cli.SenhaContasTeste, Token: tokenDaSessao(t, r)}, u
}

func TestSeedTeste_IdempotenteLoginsEConferencia(t *testing.T) {
	obs := novoAdminDB(t, "admin")
	antes := divergencia(t, obs)

	// recusa em produção sem tocar no banco
	fotoAntes := contagensSeed(t)
	if _, err := cli.SeedTeste("production", db, 10, nil); !errors.Is(err, cli.ErrSeedEmProducao) {
		t.Fatalf("produção: esperava recusa, veio %v", err)
	}
	if f := contagensSeed(t); fmt.Sprint(f) != fmt.Sprint(fotoAntes) {
		t.Fatalf("recusa em produção mudou o banco: %v → %v", fotoAntes, f)
	}

	var out bytes.Buffer
	r1, err := cli.SeedTeste("development", db, 10, &out)
	if err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	foto1 := contagensSeed(t)
	r2, err := cli.SeedTeste("development", db, 10, nil)
	if err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	foto2 := contagensSeed(t)
	if fmt.Sprint(foto1) != fmt.Sprint(foto2) {
		t.Fatalf("a 2ª execução duplicou linhas: %v → %v", foto1, foto2)
	}
	want := map[string]int64{"usuarios": 5, "admins": 1, "academias": 2, "instrutores": 2, "desafios": 1, "transacoes": 1}
	if fmt.Sprint(foto2) != fmt.Sprint(want) {
		t.Fatalf("contagens: %v, esperava %v", foto2, want)
	}
	if r2.IDDesafio != r1.IDDesafio || r2.DesafioCriado || r2.SaldoCreditado || r2.IDAdmin != r1.IDAdmin {
		t.Errorf("2ª execução deveria reaproveitar tudo: %+v", r2)
	}
	for _, c := range r2.Contas {
		if c.Criada {
			t.Errorf("2ª execução recriou %s", c.Email)
		}
	}
	if len(r2.Contas) != 6 {
		t.Errorf("contas: %+v", r2.Contas)
	}
	if strings.Contains(out.String(), "@") && !strings.Contains(out.String(), "gymscore.test") {
		t.Errorf("saída inesperada: %s", out.String())
	}
	var aud int64
	db.Model(&models.AdminAuditoria{}).Where("acao = 'cli_seed' AND id_admin = ?", r1.IDAdmin).Count(&aud)
	if aud < 2 {
		t.Errorf("auditoria cli_seed: %d linhas (esperava uma por execução)", aud)
	}
	if d := divergencia(t, obs); d != antes {
		t.Errorf("o seed mexeu na conferência do dashboard: divergência %d → %d", antes, d)
	}

	// atleta comum: saldo só pelo ledger, modo atleta
	atleta, ua := loginSeed(t, cli.EmailSeedAtleta)
	if ua["perfil"] != "atleta" || ua["modo_ativo"] != "atleta" {
		t.Errorf("atleta: %v", ua)
	}
	if s := saldo(t, atleta); s != cli.SaldoInicialSeedCentavos {
		t.Errorf("saldo do atleta: %d", s)
	}
	ledgerConfere(t, atleta)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", r1.IDDesafio), atleta.Token, nil), 200)

	// instrutor aprovado e vinculado: troca de modo; no modo atleta não entra no desafio da academia dele
	instr, ui := loginSeed(t, cli.EmailSeedInstrutor)
	if fmt.Sprint(ui["modos_disponiveis"]) != "[atleta instrutor]" {
		t.Errorf("instrutor: %v", ui)
	}
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/entrar", r1.IDDesafio), instr.Token, nil), 409)
	modoInstrutor(t, instr)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", instr.Token, nil), 200)

	// pedido pendente: só o modo atleta, sem afetar o atleta
	pend, up := loginSeed(t, cli.EmailSeedInstrutorPendente)
	if fmt.Sprint(up["modos_disponiveis"]) != "[atleta]" || sub(t, up, "instrutor")["status"] != "pendente" {
		t.Errorf("pendente: %v", up)
	}
	esperar(t, api(t, "POST", "/api/sessao/modo", pend.Token, map[string]any{"modo": "instrutor"}), 403)

	// academia aprovada com endereço estruturado; academia pendente barrada na gestão
	acad, _ := loginSeed(t, cli.EmailSeedAcademia)
	p := esperar(t, api(t, "GET", "/api/academia/perfil", acad.Token, nil), 200).data()
	if p["cep"] != "01001000" || p["uf"] != "SP" {
		t.Errorf("endereço da academia: %v", p)
	}
	acadPend, _ := loginSeed(t, cli.EmailSeedAcademiaPendente)
	esperar(t, api(t, "GET", "/api/academia/perfil", acadPend.Token, nil), 403)

	// admin do seed entra no painel com a senha de teste
	loginAdmin(t, servidor, cli.EmailSeedAdmin, cli.SenhaContasTeste)
}
