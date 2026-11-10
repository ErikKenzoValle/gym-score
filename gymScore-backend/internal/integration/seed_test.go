//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
	// rodada 6: desafio avançado, histórico (desafios encerrados + participações + pódios + treinos)
	db.Model(&models.Desafio{}).Where("titulo = ?", cli.TituloDesafioAvancadoSeed).Count(&n)
	c["desafios_avancados"] = n
	db.Model(&models.Desafio{}).Where("titulo LIKE 'Histórico de teste:%' AND status = 'encerrado'").Count(&n)
	c["historico"] = n
	db.Raw(`SELECT COUNT(*) FROM desafio_participantes p JOIN desafios d ON d.id = p.id_desafio
		WHERE d.titulo LIKE 'Histórico de teste:%'`).Scan(&n)
	c["historico_participacoes"] = n
	db.Raw(`SELECT COUNT(*) FROM podio_desafios p JOIN desafios d ON d.id = p.id_desafio
		WHERE d.titulo LIKE 'Histórico de teste:%'`).Scan(&n)
	c["historico_podios"] = n
	db.Raw("SELECT COUNT(*) FROM treino_conclusoes tc JOIN usuarios u ON u.id = tc.id_usuario WHERE u.email IN ?", contasSeed).Scan(&n)
	c["treino_conclusoes"] = n
	db.Raw(`SELECT COUNT(*) FROM desafio_premio_posicoes pp JOIN desafios d ON d.id = pp.id_desafio
		WHERE d.titulo IN ? OR d.titulo LIKE 'Histórico de teste:%'`, []string{cli.TituloDesafioSeed, cli.TituloDesafioAvancadoSeed}).Scan(&n)
	c["divisoes"] = n
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
	want := map[string]int64{"usuarios": 5, "admins": 1, "academias": 2, "instrutores": 2, "desafios": 1, "transacoes": 1,
		"desafios_avancados": 1, "historico": 6, "historico_participacoes": 8, "historico_podios": 8,
		"treino_conclusoes": foto2["treino_conclusoes"], "divisoes": 3 + 5 + 8}
	if foto2["treino_conclusoes"] < 150 {
		t.Errorf("treinos do histórico: %d", foto2["treino_conclusoes"])
	}
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
	dSeed := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", r1.IDDesafio), atleta.Token, nil), 200).data()
	// rodada 5: equipe de 2 vagas com o instrutor aprovado indicado e responsável; endereço da academia; início e fim
	if num(dSeed, "instrutores_vagas") != cli.InstrutoresVagasSeed || num(dSeed, "vagas_instrutor_abertas") != 1 ||
		len(dSeed["equipe"].([]any)) != 1 || dSeed["data_fim_evento"] == nil || sub(t, dSeed, "endereco")["cep"] != "01001000" {
		t.Errorf("desafio do seed (rodada 5): %v", dSeed)
	}
	if m := dSeed["equipe"].([]any)[0].(map[string]any); m["responsavel"] != true || m["origem"] != "indicado" {
		t.Errorf("membro do seed: %v", m)
	}

	// rodada 6: histórico dá nível alto e gráficos à atleta; desafio avançado (nível 5+, 18+, 5 premiados)
	conferirInvariante(t, db, "seed")
	est := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/estatisticas", atleta.ID), atleta.Token, nil), 200).data()
	if num(est, "nivel") < 5 || est["sem_dados"] != false || num(sub(t, est, "categorias"), "corrida") == 0 ||
		num(sub(t, est, "atributos"), "constancia") == 0 || num(sub(t, est, "categorias"), "luta") != 0 {
		t.Errorf("estatísticas da atleta do seed: %v", est)
	}
	if r1.IDDesafioAvancado == 0 || r2.IDDesafioAvancado != r1.IDDesafioAvancado {
		t.Errorf("desafio avançado: %d / %d", r1.IDDesafioAvancado, r2.IDDesafioAvancado)
	}
	dAv := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", r1.IDDesafioAvancado), atleta.Token, nil), 200).data()
	if dAv["pode_entrar"] != true || num(dAv, "nivel_min") != 5 || !igual(listaInt(dAv, "divisao_premio_bp"), []int64{3500, 2500, 2000, 1200, 800}) ||
		dAv["resumo"] == "" || dAv["regras"] == "" {
		t.Errorf("desafio avançado para a atleta: %v", dAv)
	}
	if num(dSeed, "posicoes_podio") != 3 || len(dSeed["divisao_premio_bp"].([]any)) != 3 || dSeed["regras"] == "" {
		t.Errorf("desafio do seed (rodada 6): %v", dSeed)
	}

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
	if d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", r1.IDDesafioAvancado), pend.Token, nil), 200).data(); d["pode_entrar"] != false ||
		!strings.Contains(fmt.Sprint(d["motivo_bloqueio_entrada"]), "nível 5 ou mais") {
		t.Errorf("conta pendente (nível baixo) no desafio avançado: %v / %v", d["pode_entrar"], d["motivo_bloqueio_entrada"])
	}

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

// B5-6: o seed automático do boot é o mesmo seed (idempotente), nunca roda em produção, respeita
// SEED_TESTE_AUTO=false e serializa instâncias subindo juntas com GET_LOCK.
func TestSeedTesteAuto_BootIdempotenteConcorrenteENuncaEmProducao(t *testing.T) {
	ctx := context.Background()
	if _, err := cli.SeedTesteAuto(ctx, " Development ", true, db, 10, nil); err != nil {
		t.Fatalf("seed automático: %v", err)
	}
	foto := contagensSeed(t)
	// "subir de novo" = mesmo estado
	r, err := cli.SeedTesteAuto(ctx, "test", true, db, 10, nil)
	if err != nil || !r.Executado || r.Seed.DesafioCriado || r.Seed.SaldoCreditado {
		t.Fatalf("2ª subida: %+v %v", r, err)
	}
	if f := contagensSeed(t); fmt.Sprint(f) != fmt.Sprint(foto) {
		t.Fatalf("2ª subida mudou o estado: %v → %v", foto, f)
	}
	// produção e desligado: não executa e não mexe no banco
	for nome, caso := range map[string]struct {
		env string
		hab bool
	}{"produção": {"production", true}, "prod": {"prod", true}, "desligado": {"development", false},
		"sem APP_ENV": {"", true}, "staging": {"staging", true}, "homolog": {"homolog", true}, "brancos": {"   ", true}} {
		r, err := cli.SeedTesteAuto(ctx, caso.env, caso.hab, db, 10, nil)
		if err != nil || r.Executado {
			t.Errorf("%s: não deveria executar: %+v %v", nome, r, err)
		}
	}
	if f := contagensSeed(t); fmt.Sprint(f) != fmt.Sprint(foto) {
		t.Fatalf("produção/desligado mudaram o estado: %v → %v", foto, f)
	}
	// 3 instâncias subindo juntas: nenhuma falha, estado igual (a trava serializa; quem espera semeia de novo
	// sem duplicar)
	var wg sync.WaitGroup
	erros := make([]error, 3)
	for i := range erros {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, erros[i] = cli.SeedTesteAuto(ctx, "development", true, db, 10, nil)
		}(i)
	}
	wg.Wait()
	for i, err := range erros {
		if err != nil {
			t.Errorf("instância %d: %v", i, err)
		}
	}
	if f := contagensSeed(t); fmt.Sprint(f) != fmt.Sprint(foto) {
		t.Fatalf("subidas concorrentes mudaram o estado: %v → %v", foto, f)
	}
	// trava ocupada por outra sessão além do limite: pula com ErrSeedTravado (o boot só loga)
	sqlDB, _ := db.DB()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var ok int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 5)", cli.LockSeedNome).Scan(&ok); err != nil || ok != 1 {
		t.Fatalf("pegar a trava no teste: %v %d", err, ok)
	}
	defer conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", cli.LockSeedNome)
	ctxCurto, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := cli.SeedTesteAuto(ctxCurto, "development", true, db, 10, nil); !errors.Is(err, cli.ErrSeedTravado) {
		t.Errorf("trava ocupada: esperava ErrSeedTravado, veio %v", err)
	}
}
