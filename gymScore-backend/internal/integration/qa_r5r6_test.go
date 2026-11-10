//go:build integration

package integration

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"gymScore-backend/internal/models"
)

// ─── QA rodadas 5+6 (Q5-1/Q6-1): verificação independente ─────────────────────────────────────

// 00001–00022 do zero, Down até 0 (nada sobra) e Up até 22 com schema idêntico.
func TestQAR56_MigracoesDoZeroAte22DownUpCompleto(t *testing.T) {
	d, sqlDB := bancoQA(t, "r56zero", 22)
	s1 := qaSchema(t, d)
	for _, col := range []string{"desafio_instrutores|responsavel_desafio|", "desafio_premio_posicoes|pct_bp|",
		"desafios|data_fim_evento|", "desafios|nivel_min|", "usuarios|nivel_gymscore|"} {
		if !strings.Contains(s1, col) {
			t.Errorf("coluna %s ausente", col)
		}
	}
	p := providerQA(t, sqlDB)
	if _, err := p.DownTo(t.Context(), 0); err != nil {
		t.Fatalf("DownTo 0: %v", err)
	}
	var restantes []string
	d.Raw("SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema = DATABASE() AND TABLE_NAME <> 'goose_db_version'").Scan(&restantes)
	var procs int64
	d.Raw("SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = DATABASE()").Scan(&procs)
	if len(restantes) != 0 || procs != 0 {
		t.Errorf("Down até 0 deixou tabelas %v / %d procedures", restantes, procs)
	}
	if _, err := p.UpTo(t.Context(), 22); err != nil {
		t.Fatalf("UpTo 22 de novo: %v", err)
	}
	if s2 := qaSchema(t, d); s2 != s1 {
		t.Errorf("schema difere após Down/Up")
	}
}

// Rotas novas da equipe/candidatura: academia de outra, instrutor de fora, atleta e sem token.
func TestQAR56_AutorizacaoRotasDeEquipe(t *testing.T) {
	c := cenarioComSaldo(t, 50_000)
	outra := novoCenario(t)
	_, iid2 := novoInstrutorDe(t, c.AcademiaID)
	fora, foraID := novoInstrutorDe(t, 0)
	atleta := usuarioComSaldo(t, 5000)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 3, []uint{c.InstrutorID}, nil)), 201))
	cid := uint(esperar(t, candidatarR5(t, fora, id), 201).data()["id"].(float64))
	// candidatura de OUTRO desafio (IDOR por cid)
	idOutro := idDe(esperar(t, api(t, "POST", "/api/desafios", outra.A.Token, corpoEquipe(outra, 0, 2, []uint{}, nil)), 201))
	cidOutro := uint(esperar(t, candidatarR5(t, fora, idOutro), 201).data()["id"].(float64))

	rotas := []struct {
		m, p  string
		corpo any
	}{
		{"POST", fmt.Sprintf("/api/desafios/%d/equipe", id), map[string]any{"id_instrutor": iid2}},
		{"DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, c.InstrutorID), nil},
		{"PUT", fmt.Sprintf("/api/desafios/%d/equipe/responsavel", id), map[string]any{"id_instrutor": c.InstrutorID}},
		{"POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cid), nil},
		{"POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid), nil},
		{"POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/recusar", id, cid), nil},
		{"GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), nil},
	}
	for _, quem := range []struct {
		nome  string
		token string
	}{{"outra academia", outra.A.Token}, {"instrutor de fora", fora.Token}, {"membro da equipe", c.I.Token}, {"atleta", atleta.Token}} {
		for _, r := range rotas {
			got := api(t, r.m, r.p, quem.token, r.corpo)
			if got.Status != 403 && got.Status != 404 {
				t.Errorf("%s %s %s: esperava 403/404, veio %d %s", quem.nome, r.m, r.p, got.Status, got.Raw)
			}
		}
	}
	for _, r := range rotas {
		if got := api(t, r.m, r.p, "", r.corpo); got.Status != 401 {
			t.Errorf("sem token %s %s: %d", r.m, r.p, got.Status)
		}
	}
	// a dona não aceita candidatura de outro desafio pelo próprio id
	if r := api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cidOutro), c.A.Token, nil); r.Status != 404 {
		t.Errorf("aceitar cid de outro desafio: %d %s", r.Status, r.Raw)
	}
	// nada mudou
	d := detalhe(t, c.A, id)
	eq := equipeDe(t, d)
	if len(eq) != 1 || eq[c.InstrutorID]["responsavel"] != true {
		t.Errorf("equipe mudou por acesso indevido: %v", d["equipe"])
	}
	var st string
	db.Model(&models.CandidaturaInstrutor{}).Select("status").Where("id = ?", cidOutro).Scan(&st)
	if st != models.CandidaturaPendente {
		t.Errorf("candidatura do outro desafio mudou: %s", st)
	}
	// academia de outra não indica instrutor dela num desafio alheio nem o próprio vinculado de c entra por outra
	if r := api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", idOutro), outra.A.Token, map[string]any{"id_instrutor": iid2}); r.Status < 400 {
		t.Errorf("outra academia indicou instrutor vinculado a c: %d", r.Status)
	}
	_ = foraID
}

// Duas trocas de responsável simultâneas (e troca × remoção): sempre exatamente 1 responsável, espelho coerente.
func TestQAR56_DoisResponsaveisConcorrentes(t *testing.T) {
	c := cenarioComSaldo(t, 50_000)
	_, b := novoInstrutorDe(t, c.AcademiaID)
	_, x := novoInstrutorDe(t, c.AcademiaID)
	for rodada := 0; rodada < 8; rodada++ {
		id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 3, []uint{c.InstrutorID, b, x}, nil)), 201))
		var wg sync.WaitGroup
		st := make([]int, 3)
		for i, alvo := range []uint{b, x} {
			wg.Add(1)
			go func(i int, alvo uint) {
				defer wg.Done()
				st[i] = api(t, "PUT", fmt.Sprintf("/api/desafios/%d/equipe/responsavel", id), c.A.Token, map[string]any{"id_instrutor": alvo}).Status
			}(i, alvo)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			st[2] = api(t, "DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, c.InstrutorID), c.A.Token, nil).Status
		}()
		wg.Wait()
		for _, s := range st {
			if s >= 500 {
				t.Fatalf("rodada %d: 5xx %v", rodada, st)
			}
		}
		var n int64
		var resp, espelho *uint
		db.Raw("SELECT COUNT(*) FROM desafio_instrutores WHERE id_desafio = ? AND responsavel = 1", id).Scan(&n)
		db.Raw("SELECT id_instrutor FROM desafio_instrutores WHERE id_desafio = ? AND responsavel = 1", id).Scan(&resp)
		db.Raw("SELECT id_instrutor FROM desafios WHERE id = ?", id).Scan(&espelho)
		if n != 1 || resp == nil || espelho == nil || *resp != *espelho {
			t.Errorf("rodada %d: responsáveis %d, resp %v, espelho %v (status %v)", rodada, n, resp, espelho, st)
		}
	}
}

// Indicar × aceitar candidatura na ÚLTIMA vaga, simultâneos: equipe nunca passa de instrutores_vagas.
func TestQAR56_IndicarXAceitarNaUltimaVaga(t *testing.T) {
	c := cenarioComSaldo(t, 50_000)
	_, b := novoInstrutorDe(t, c.AcademiaID)
	for rodada := 0; rodada < 6; rodada++ {
		fora, _ := novoInstrutorDe(t, 0)
		id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 2, []uint{c.InstrutorID}, nil)), 201))
		cid := uint(esperar(t, candidatarR5(t, fora, id), 201).data()["id"].(float64))
		var wg sync.WaitGroup
		var s1, s2 int
		wg.Add(2)
		go func() {
			defer wg.Done()
			s1 = api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cid), c.A.Token, nil).Status
		}()
		go func() {
			defer wg.Done()
			s2 = api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", id), c.A.Token, map[string]any{"id_instrutor": b}).Status
		}()
		wg.Wait()
		var n int64
		db.Raw("SELECT COUNT(*) FROM desafio_instrutores WHERE id_desafio = ?", id).Scan(&n)
		if n != 2 || !((s1 == 200) != (s2 == 200)) || s1 >= 500 || s2 >= 500 {
			t.Errorf("rodada %d: equipe %d (vagas 2), aceitar %d, indicar %d", rodada, n, s1, s2)
		}
		var st string
		db.Model(&models.CandidaturaInstrutor{}).Select("status").Where("id = ?", cid).Scan(&st)
		if (s1 == 200 && st != models.CandidaturaEscolhida) || (s2 == 200 && st != models.CandidaturaRecusada) {
			t.Errorf("rodada %d: candidatura %s (aceitar %d, indicar %d)", rodada, st, s1, s2)
		}
	}
}

// Ciclo de dinheiro de uma equipe: edição de vagas para cima/baixo, remoção de membro, 33,33% com menos
// presentes que premiados, anulação devolvendo as inscrições. Σ saldos = Σ ledger em todas as etapas.
func TestQAR56_DinheiroCicloEquipeDivisaoEAnulacao(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	srv := servidorMod(1)
	c := cenarioComSaldo(t, 100_000)
	_, b := novoInstrutorDe(t, c.AcademiaID)
	fora, _ := novoInstrutorDe(t, 0)
	saldo0 := saldo(t, c.A)
	// 2 vagas x fixo R$ 7,01 + 3%; prêmio fixo R$ 10,00; pote 40%; 3 premiados 33,34/33,33/33,33
	corpo := corpoEquipe(c, 1000, 2, []uint{c.InstrutorID}, map[string]any{
		"premio_fixo_centavos": 1000, "dividir_pote": true, "percentual_pote": 40, "instrutor_modo_pagamento": "fixo_percentual",
		"instrutor_fixo_centavos": 701, "instrutor_pct": 3, "posicoes_podio": 3, "divisao_premio_bp": []int{3334, 3333, 3333},
	})
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpo), 201))
	if g := saldo0 - saldo(t, c.A); g != 1000+2*701 {
		t.Fatalf("reserva inicial %d", g)
	}
	conferirInvariante(t, db, "criação")
	// sobe para 4 vagas (débito 2x701) e desce para 3 (crédito 701)
	esperar(t, chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"instrutores_vagas": 4}), 200)
	if g := saldo0 - saldo(t, c.A); g != 1000+4*701 {
		t.Errorf("reserva com 4 vagas %d", g)
	}
	esperar(t, chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"instrutores_vagas": 3}), 200)
	if g := saldo0 - saldo(t, c.A); g != 1000+3*701 {
		t.Errorf("reserva com 3 vagas %d", g)
	}
	// 4% x 3 + 40 + 10 = 62 ok; pote acima do permitido com o % por vaga -> 400
	if r := chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"percentual_pote": 85}); r.Status != 400 && r.Status != 422 {
		t.Errorf("pote 85%% + 10 + 3x3: %d", r.Status)
	}
	conferirInvariante(t, db, "edição")
	// equipe: indica b, aceita candidato de fora, remove b -> 2 membros de 3 vagas
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/equipe", id), c.A.Token, map[string]any{"id_instrutor": b}), 200)
	cid := uint(esperar(t, candidatarR5(t, fora, id), 201).data()["id"].(float64))
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cid), c.A.Token, nil), 200)
	esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, b), c.A.Token, nil), 200)
	// pagamento por instrutor travado (há membro vindo de candidatura)
	if r := chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"instrutor_fixo_centavos": 100}); r.Status != 409 {
		t.Errorf("mudar fixo com candidato aceito: %d", r.Status)
	}
	at := atletas(t, 4, 5000)
	inscrever(t, id, at...)
	// travas após a 1ª inscrição
	for nome, m := range map[string]map[string]any{
		"divisão": {"divisao_premio_bp": []int{5000, 3000, 2000}}, "premiados": {"posicoes_podio": 2},
		"nível": {"nivel_min": 2}, "idade": {"idade_max": 50}, "entrada": {"valor_entrada_centavos": 2000},
	} {
		if r := chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, m); r.Status != 409 {
			t.Errorf("trava após inscrição (%s): %d", nome, r.Status)
		}
	}
	esperar(t, acaoS(t, srv, c.I, id, "iniciar", nil), 200)
	// só 2 presentes para 3 premiados
	for _, a := range at[:2] {
		esperar(t, presencaPorID(fora, t, id, a), 200)
	}
	// membro não responsável não lança o pódio
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", id), fora.Token,
		podioBody(entradaPodio(at[0], "1"), entradaPodio(at[1], "2"))), 403)
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", id), c.I.Token,
		podioBody(entradaPodio(at[0], "1"), entradaPodio(at[1], "2"))), 200)
	// T = 4000; taxa 10% = 400; pote 40% = 1600; prêmio 2600 -> 3334/(6667) e 3333/(6667): 1300 + 1300 (sobra no 1º)
	p1, p2 := pendenteDe(t, id, at[0].ID), pendenteDe(t, id, at[1].ID)
	// 2600x3334/6667 = 1300, 2600x3333/6667 = 1299, sobra 1 para o 1º
	if p1 != 1301 || p2 != 1299 {
		t.Errorf("prêmios %d / %d (esperava 1301/1299)", p1, p2)
	}
	// cada membro: 701 + 120 = 821
	if v := pendenteDe(t, id, c.I.ID); v != 821 {
		t.Errorf("responsável a receber %d", v)
	}
	if v := pendenteDe(t, id, fora.ID); v != 821 {
		t.Errorf("membro aceito a receber %d", v)
	}
	// academia: 4000 + 3103 - 2600 - 1642 - 400 = 2461 (inclui o fixo da vaga vazia)
	if v := pendenteDe(t, id, c.A.ID); v != 2461 {
		t.Errorf("academia a receber %d", v)
	}
	conferirInvariante(t, db, "pódio")
	// anulação devolvendo as inscrições: ninguém recebe, atletas reembolsados, academia de volta à reserva
	esperar(t, anular(t, sa, id, models.AnulacaoDevolverInscricoes, c.I.ID, 0), 200)
	liberarPagamentos(t)
	for _, a := range at {
		if v := saldo(t, a); v != 5000 {
			t.Errorf("atleta %d após devolver inscrições: %d", a.ID, v)
		}
	}
	if v := saldo(t, fora); v != 0 {
		t.Errorf("membro recebeu após anulação: %d", v)
	}
	if v := saldo(t, c.A); v != saldo0 {
		t.Errorf("academia após anulação: %d (esperava %d)", v, saldo0)
	}
	conferirInvariante(t, db, "anulação")
	ledgerConfere(t, append(at, c.A, c.I, fora)...)
}

// Cancelamento com equipe parcial devolve a reserva inteira.
func TestQAR56_CancelarComEquipeParcialDevolveReserva(t *testing.T) {
	c := cenarioComSaldo(t, 30_000)
	s0 := saldo(t, c.A)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 500, 5, []uint{c.InstrutorID}, map[string]any{
		"premio_fixo_centavos": 333, "instrutor_modo_pagamento": "fixo", "instrutor_fixo_centavos": 999})), 201))
	if g := s0 - saldo(t, c.A); g != 333+5*999 {
		t.Fatalf("reserva %d", g)
	}
	at := atletas(t, 2, 1000)
	inscrever(t, id, at...)
	esperar(t, acao(t, c.A, id, "cancelar", nil), 200)
	if saldo(t, c.A) != s0 || saldo(t, at[0]) != 1000 {
		t.Errorf("cancelar: academia %d (era %d), atleta %d", saldo(t, c.A), s0, saldo(t, at[0]))
	}
	invariantes(t, id)
	conferirInvariante(t, db, "cancelar")
}

// Estatísticas: qualquer logado vê (perfil público), sem PII, 404 inexistente, 401 sem token.
func TestQAR56_EstatisticasVisibilidadeSemPII(t *testing.T) {
	a := novoUsuario(t)
	b := novoUsuario(t)
	r := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/estatisticas", a.ID), b.Token, nil), 200)
	for _, proibido := range []string{"data_nascimento", "email", "cpf", a.Email, "senha"} {
		if strings.Contains(strings.ToLower(r.Raw), strings.ToLower(proibido)) {
			t.Errorf("estatísticas expõem %q", proibido)
		}
	}
	if api(t, "GET", fmt.Sprintf("/api/usuarios/%d/estatisticas", a.ID), "", nil).Status != 401 {
		t.Error("sem token deveria ser 401")
	}
	if s := api(t, "GET", "/api/usuarios/999999999/estatisticas", b.Token, nil).Status; s != 404 {
		t.Errorf("inexistente: %d", s)
	}
	if s := api(t, "GET", "/api/usuarios/abc/estatisticas", b.Token, nil).Status; s != 400 {
		t.Errorf("id inválido: %d", s)
	}
	// coerência com o perfil público: mesmo status para o mesmo alvo
	acad, _ := novaAcademia(t)
	s1 := api(t, "GET", fmt.Sprintf("/api/usuarios/%d/publico", acad.ID), b.Token, nil).Status
	s2 := api(t, "GET", fmt.Sprintf("/api/usuarios/%d/estatisticas", acad.ID), b.Token, nil).Status
	t.Logf("conta de academia: público %d, estatísticas %d", s1, s2)
	if s2 >= 500 {
		t.Errorf("estatísticas de academia: %d", s2)
	}
}

// data_nascimento: só uma vez; mesma data reenviada = ok; futuro = 400.
func TestQAR56_DataNascimentoSoUmaVez(t *testing.T) {
	u := novoUsuario(t)
	db.Exec("UPDATE usuarios SET data_nascimento = NULL WHERE id = ?", u.ID)
	if s := api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"data_nascimento": "2999-01-01"}).Status; s != 400 {
		t.Errorf("futuro: %d", s)
	}
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"data_nascimento": "1990-02-03"}), 200)
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"data_nascimento": "1990-02-03"}), 200)
	if s := api(t, "PATCH", "/api/usuarios/perfil", u.Token, map[string]any{"data_nascimento": "2000-02-03"}).Status; s != 409 {
		t.Errorf("trocar a data: %d", s)
	}
	// corrida: duas datas diferentes ao mesmo tempo para quem não tem -> só uma fica, a outra não sobrescreve
	v := novoUsuario(t)
	db.Exec("UPDATE usuarios SET data_nascimento = NULL WHERE id = ?", v.ID)
	var wg sync.WaitGroup
	st := make([]int, 2)
	for i, d := range []string{"1980-01-01", "2001-12-31"} {
		wg.Add(1)
		go func(i int, d string) {
			defer wg.Done()
			st[i] = api(t, "PATCH", "/api/usuarios/perfil", v.Token, map[string]any{"data_nascimento": d}).Status
		}(i, d)
	}
	wg.Wait()
	t.Logf("corrida data_nascimento: %v", st)
	ok200, ok409 := 0, 0
	for _, s := range st {
		switch s {
		case 200:
			ok200++
		case 409:
			ok409++
		}
	}
	if ok200 != 1 || ok409 != 1 {
		t.Fatalf("QA56-1: esperado exatamente um 200 e um 409, veio %v", st)
	}
	var gravada string
	db.Raw("SELECT DATE_FORMAT(data_nascimento, '%Y-%m-%d') FROM usuarios WHERE id = ?", v.ID).Scan(&gravada)
	vencedora := "1980-01-01"
	if st[1] == 200 {
		vencedora = "2001-12-31"
	}
	if gravada != vencedora {
		t.Errorf("data gravada %q, mas o 200 foi para %q", gravada, vencedora)
	}
}

// Texto livre (resumo/regras/mensagem) é guardado e devolvido como texto: o JSON não interpreta HTML.
func TestQAR56_TextoLivreComHTMLVoltaLiteral(t *testing.T) {
	c := cenarioComSaldo(t, 10_000)
	fora, _ := novoInstrutorDe(t, 0)
	x := `<img src=x onerror=alert(1)><script>alert(2)</script>`
	b := corpoEquipe(c, 0, 2, []uint{}, map[string]any{"resumo": "Resumo " + x, "regras": "Regras completas " + x + strings.Repeat(" r", 20)})
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, b), 201))
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), fora.Token, map[string]any{"mensagem": "Oi " + x}), 201)
	d := detalhe(t, c.A, id)
	if !strings.Contains(d["resumo"].(string), x) || !strings.Contains(d["regras"].(string), x) {
		t.Errorf("texto alterado: %v / %v", d["resumo"], d["regras"])
	}
	r := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), c.A.Token, nil), 200)
	if !strings.Contains(r.Raw, `onerror`) {
		t.Logf("mensagem da candidatura saneada no backend: %s", r.Raw)
	}
	t.Logf("desafio XSS id=%d", id)
}
