//go:build integration

package integration

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/models"
)

// Rodada 6: premiação configurável (U1), resumo/regras (U2), Nível GymScore (U3), filtros de nível e idade
// (U4), estatísticas (U5) e a pendência herdada da rodada 5 (editar horário com inscritos).

func estatisticas(t *testing.T, quem *user, alvo uint) map[string]any {
	t.Helper()
	return esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/estatisticas", alvo), quem.Token, nil), 200).data()
}

func nivelNoBanco(t *testing.T, id uint) (pontuacao, nivel int, calculado bool) {
	t.Helper()
	var u struct {
		PontuacaoGymscore   int
		NivelGymscore       int
		GymscoreCalculadoEm *time.Time
	}
	db.Raw("SELECT pontuacao_gymscore, nivel_gymscore, gymscore_calculado_em FROM usuarios WHERE id = ?", id).Scan(&u)
	return u.PontuacaoGymscore, u.NivelGymscore, u.GymscoreCalculadoEm != nil
}

func divisaoNoBanco(t *testing.T, id uint) []int64 {
	t.Helper()
	var bp []int64
	db.Raw("SELECT pct_bp FROM desafio_premio_posicoes WHERE id_desafio = ? ORDER BY posicao", id).Scan(&bp)
	return bp
}

// treinosEmDias grava exercícios concluídos em n dias distintos (o mais recente ontem), direto no banco.
func treinosEmDias(t *testing.T, u *user, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		db.Exec("INSERT INTO treino_conclusoes (id_usuario, id_treino, grupo_muscular, concluido_em) VALUES (?, 0, 'pernas', NOW() - INTERVAL ? DAY)", u.ID, i)
	}
}

func TestR6_CriarValidaResumoRegrasDivisaoEFiltros(t *testing.T) {
	c := novoCenario(t)
	criar := func(extra map[string]any, apagar ...string) resp {
		b := corpoFin(c, 1000, 10, 3, extra)
		for _, k := range apagar {
			delete(b, k)
		}
		return api(t, "POST", "/api/desafios", c.A.Token, b)
	}
	casos := []struct {
		nome   string
		extra  map[string]any
		apagar []string
		erro   string
	}{
		{"sem resumo", nil, []string{"resumo"}, "resumo"},
		{"resumo curto", map[string]any{"resumo": "curto demais"}, nil, "resumo"},
		{"resumo longo", map[string]any{"resumo": strings.Repeat("a", 281)}, nil, "resumo"},
		{"sem regras", nil, []string{"regras"}, "regras"},
		{"regras curtas", map[string]any{"regras": "só isso"}, nil, "regras"},
		{"regras só espaços", map[string]any{"regras": strings.Repeat(" ", 60)}, nil, "regras"},
		{"divisão soma 9999", map[string]any{"divisao_premio_bp": []int{6000, 3000, 999}}, nil, "10000"},
		{"divisão crescente", map[string]any{"divisao_premio_bp": []int{3000, 6000, 1000}}, nil, "não pode receber mais"},
		{"divisão com zero", map[string]any{"divisao_premio_bp": []int{10000, 0, 0}}, nil, "mais que 0"},
		{"divisão tamanho errado", map[string]any{"divisao_premio_bp": []int{7000, 3000}}, nil, "exatamente 3"},
		{"nível min > max", map[string]any{"nivel_min": 6, "nivel_max": 3}, nil, "nivel_min"},
		{"nível 11", map[string]any{"nivel_max": 11}, nil, ""},
		{"idade min > max", map[string]any{"idade_min": 40, "idade_max": 18}, nil, "idade_min"},
		{"101 premiados", map[string]any{"posicoes_podio": 101, "vagas": 200}, nil, ""},
		{"mais premiados que vagas", map[string]any{"posicoes_podio": 11}, nil, "vagas"},
	}
	for _, cs := range casos {
		r := esperar(t, criar(cs.extra, cs.apagar...), 400)
		if cs.erro != "" && !strings.Contains(r.Raw, cs.erro) {
			t.Errorf("%s: mensagem sem %q: %s", cs.nome, cs.erro, r.Raw)
		}
	}

	// válido: 33,34/33,33/33,33 + filtros; resumo/regras aparados; texto com HTML devolvido como veio (texto puro)
	r := esperar(t, criar(map[string]any{"divisao_premio_bp": []int{3334, 3333, 3333}, "nivel_min": 2, "nivel_max": 0,
		"idade_min": 18, "resumo": "  Resumo <b>com</b> espaços nas pontas  "}), 201).data()
	id := uint(r["id"].(float64))
	if !igual(divisaoNoBanco(t, id), []int64{3334, 3333, 3333}) || !igual(listaInt(r, "divisao_premio_bp"), []int64{3334, 3333, 3333}) {
		t.Errorf("divisão gravada %v / resposta %v", divisaoNoBanco(t, id), r["divisao_premio_bp"])
	}
	if r["resumo"] != "Resumo <b>com</b> espaços nas pontas" || r["regras"] != regrasTeste || r["descricao"] != "Descrição do desafio" {
		t.Errorf("textos: resumo %q regras %q descricao %q", r["resumo"], r["regras"], r["descricao"])
	}
	if r["nivel_min"] != float64(2) || r["nivel_max"] != nil || r["idade_min"] != float64(18) || r["idade_max"] != nil {
		t.Errorf("filtros: %v %v %v %v", r["nivel_min"], r["nivel_max"], r["idade_min"], r["idade_max"])
	}
	if p := sub(t, r, "premiacao"); len(p["divisao_bp"].([]any)) != 3 {
		t.Errorf("premiacao.divisao_bp: %v", p)
	}

	// sem divisão: padrão do tamanho; 8 premiados (acima da tabela antiga de 5)
	r8 := esperar(t, criar(map[string]any{"posicoes_podio": 8}), 201).data()
	bp8 := listaInt(r8, "divisao_premio_bp")
	var soma int64
	for i, v := range bp8 {
		soma += v
		if v <= 0 || (i > 0 && v > bp8[i-1]) {
			t.Errorf("divisão padrão de 8 inválida: %v", bp8)
		}
	}
	if len(bp8) != 8 || soma != 10000 || len(listaInt(r8, "premiacao_prevista_centavos")) != 8 {
		t.Errorf("divisão padrão de 8: %v (soma %d)", bp8, soma)
	}
	d3 := esperar(t, criar(nil), 201).data()
	if !igual(listaInt(d3, "divisao_premio_bp"), []int64{6000, 3000, 1000}) {
		t.Errorf("padrão de 3 = 60/30/10: %v", d3["divisao_premio_bp"])
	}

	// rota de sugestão
	s := esperar(t, api(t, "GET", "/api/desafios/divisao-padrao?posicoes=6", c.A.Token, nil), 200).data()
	if len(s["divisao_premio_bp"].([]any)) != 6 {
		t.Errorf("divisão padrão de 6: %v", s)
	}
	esperar(t, api(t, "GET", "/api/desafios/divisao-padrao?posicoes=0", c.A.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/desafios/divisao-padrao?posicoes=101", c.A.Token, nil), 400)

	// admin vê resumo/regras/divisão/filtros
	sa := novoAdminDB(t, models.PapelAdmin)
	ad := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/desafios/%d", id), sa, nil), 200).data()
	if ad["regras"] != regrasTeste || len(ad["divisao_premio_bp"].([]any)) != 3 || ad["idade_min"] != float64(18) {
		t.Errorf("admin: %v", ad)
	}
}

// Dinheiro com a divisão da academia: 4 premiados (40/30/20/10), prêmio fixo ímpar, 3 presentes
// (renormaliza entre as ocupadas, sobra para o 1º); Σ saldos = Σ ledger e invariantes do desafio.
func TestR6_PremiacaoConfiguravelDinheiro(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 20000)
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 10, 4, map[string]any{
		"premio_fixo_centavos": 1001, "dividir_pote": true, "percentual_pote": 50,
		"divisao_premio_bp": []int{4000, 3000, 2000, 1000},
	})), 201))
	us := atletas(t, 5, 2000)
	inscrever(t, id, us...)
	acionar(t, srv, c.I, id, "iniciar", nil, 200)
	for _, u := range us[:3] {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	d := detalheS(t, srv, c.A, id)
	// T = 5000: taxa 500, pote 50% = 2500, prêmio total = 1001 + 2500 = 3501
	if p := sub(t, d, "premiacao"); num(p, "premio_total_centavos") != 3501 {
		t.Fatalf("prêmio total previsto: %v", p)
	}
	// prevista (4 posições): 3501 x 40/30/20/10 = 1400/1050/700/350 (=3500) + 1 -> 1401
	if got := listaInt(d, "premiacao_prevista_centavos"); !igual(got, []int64{1401, 1050, 700, 350}) {
		t.Errorf("prevista: %v", got)
	}
	esperar(t, acaoS(t, srv, c.I, id, "podio", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(us[2], "3"))), 200)
	aposPodio(t, id)
	// 3 presentes: 40/30/20 sobre 90 -> 3501*40/90 = 1556, *30/90 = 1167, *20/90 = 778 (=3501): resto 0
	var premios []int64
	db.Raw("SELECT premio_centavos FROM podio_desafios WHERE id_desafio = ? ORDER BY posicao", id).Scan(&premios)
	if !igual(premios, []int64{1556, 1167, 778}) {
		t.Errorf("prêmios do pódio: %v", premios)
	}
	if v := saldo(t, us[0]); v != 1000+1556 {
		t.Errorf("1º lugar: saldo %d", v)
	}
	invariantes(t, id)
	ledgerConfere(t, append(us, c.A, c.I)...)
	conferirInvariante(t, db, "premiação configurável")

	// 33,34/33,33/33,33 com total que não divide: soma preservada, sobra no 1º
	c2 := cenarioComSaldo(t, 20000)
	id2 := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c2.A.Token, corpoFin(c2, 0, 5, 3, map[string]any{
		"premio_fixo_centavos": 1000, "dividir_pote": false, "divisao_premio_bp": []int{3334, 3333, 3333},
	})), 201))
	us2 := []*user{novoUsuario(t), novoUsuario(t), novoUsuario(t)}
	inscrever(t, id2, us2...)
	acionar(t, srv, c2.I, id2, "iniciar", nil, 200)
	for _, u := range us2 {
		esperar(t, presencaPorID(c2.I, t, id2, u), 200)
	}
	esperar(t, acaoS(t, srv, c2.I, id2, "podio", podioBody(entradaPodio(us2[0], "1"), entradaPodio(us2[1], "2"), entradaPodio(us2[2], "3"))), 200)
	aposPodio(t, id2)
	premios = nil
	db.Raw("SELECT premio_centavos FROM podio_desafios WHERE id_desafio = ? ORDER BY posicao", id2).Scan(&premios)
	if !igual(premios, []int64{334, 333, 333}) {
		t.Errorf("33/33/33 de 1000: %v", premios)
	}
	invariantes(t, id2)
	ledgerConfere(t, append(us2, c2.A, c2.I)...)
	conferirInvariante(t, db, "33/33/33")
}

// 7 premiados configurados, 7 presentes: pódio aceita 7 posições (antes o teto era 5).
func TestR6_PodioComMaisDeCincoPosicoes(t *testing.T) {
	c := cenarioComSaldo(t, 10000)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoFin(c, 0, 10, 7, map[string]any{
		"premio_fixo_centavos": 7000, "dividir_pote": false,
	})), 201))
	var us []*user
	for i := 0; i < 7; i++ {
		us = append(us, novoUsuario(t))
	}
	inscrever(t, id, us...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	var entradas []map[string]any
	for i, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
		entradas = append(entradas, entradaPodio(u, fmt.Sprint(i+1)))
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradas[:6]...)), 400)
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradas...)), 200)
	var soma, n int64
	db.Raw("SELECT COALESCE(SUM(premio_centavos),0), COUNT(*) FROM podio_desafios WHERE id_desafio = ?", id).Row().Scan(&soma, &n)
	if soma != 7000 || n != 7 {
		t.Errorf("7 posições: n %d, soma %d", n, soma)
	}
	invariantes(t, id)
	ledgerConfere(t, append(us, c.A, c.I)...)
}

func TestR6_EditarPremiacaoEFiltrosSoAntesDaPrimeiraInscricao(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 0, 10, 3)
	patch := func(corpo map[string]any) resp {
		return api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, corpo)
	}
	esperar(t, patch(map[string]any{"divisao_premio_bp": []int{5000, 5000, 1000}}), 400)
	d := esperar(t, patch(map[string]any{"divisao_premio_bp": []int{5000, 3000, 2000}, "nivel_max": 8, "idade_min": 16}), 200).data()
	if !igual(listaInt(d, "divisao_premio_bp"), []int64{5000, 3000, 2000}) || d["nivel_max"] != float64(8) || d["pode_editar_premiacao"] != true {
		t.Errorf("edição antes da inscrição: %v", d)
	}
	// muda o tamanho sem mandar a divisão: padrão do novo tamanho
	d = esperar(t, patch(map[string]any{"posicoes_podio": 2}), 200).data()
	if !igual(listaInt(d, "divisao_premio_bp"), []int64{7000, 3000}) || !igual(divisaoNoBanco(t, id), []int64{7000, 3000}) {
		t.Errorf("tamanho 2: %v", d["divisao_premio_bp"])
	}
	// resumo/regras: mesmas regras da criação
	esperar(t, patch(map[string]any{"resumo": "curto"}), 400)
	esperar(t, patch(map[string]any{"regras": ""}), 400)
	d = esperar(t, patch(map[string]any{"resumo": "Novo resumo do desafio editado", "idade_min": 0}), 200).data()
	if d["resumo"] != "Novo resumo do desafio editado" || d["idade_min"] != nil {
		t.Errorf("edição de texto/filtro removido: %v %v", d["resumo"], d["idade_min"])
	}

	inscrever(t, id, novoUsuario(t))
	if detalhe(t, c.A, id)["pode_editar_premiacao"] != false {
		t.Error("com inscrito pode_editar_premiacao deveria ser false")
	}
	esperar(t, patch(map[string]any{"divisao_premio_bp": []int{6000, 4000}}), 409)
	esperar(t, patch(map[string]any{"posicoes_podio": 3}), 409)
	esperar(t, patch(map[string]any{"nivel_min": 2}), 409)
	esperar(t, patch(map[string]any{"idade_max": 60}), 409)
	// reenviar o que já está não é mudança
	esperar(t, patch(map[string]any{"divisao_premio_bp": []int{7000, 3000}, "posicoes_podio": 2, "nivel_max": 8, "titulo": "Título novo"}), 200)
	esperar(t, patch(map[string]any{"resumo": "Resumo que ainda pode mudar depois da inscrição"}), 200)
}

func TestR6_FiltrosDeNivelEIdadeNaInscricao(t *testing.T) {
	c := novoCenario(t)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoFin(c, 0, 10, 3, map[string]any{"nivel_min": 2, "nivel_max": 4})), 201))
	a := novoUsuario(t)
	msg := "Este desafio é para nível 2 a 4; você está no nível 1"
	if d := detalhe(t, a, id); d["pode_entrar"] != false || d["motivo_bloqueio_entrada"] != msg {
		t.Errorf("DTO nível baixo: %v / %v", d["pode_entrar"], d["motivo_bloqueio_entrada"])
	}
	if f := feedItem(t, api(t, "GET", fmt.Sprintf("/api/desafios?id_academia=%d&limite=100", c.AcademiaID), a.Token, nil), id); f["motivo_bloqueio_entrada"] != msg {
		t.Errorf("feed: %v", f["motivo_bloqueio_entrada"])
	}
	r := esperar(t, acao(t, a, id, "entrar", nil), 409)
	if !strings.Contains(r.Raw, msg) {
		t.Errorf("409 do entrar: %s", r.Raw)
	}
	// 20 dias de treino = 20 pontos = nível 2 (o Entrar recalcula na hora, mesmo com o cache velho)
	treinosEmDias(t, a, 20)
	esperar(t, acao(t, a, id, "entrar", nil), 200)
	if p, n, ok := nivelNoBanco(t, a.ID); p != 20 || n != 2 || !ok {
		t.Errorf("cache do nível depois da inscrição: %d/%d/%v", p, n, ok)
	}
	// nível acima do máximo
	alto := novoUsuario(t)
	treinosEmDias(t, alto, 100) // 100 pontos = nível 4
	esperar(t, acao(t, alto, id, "entrar", nil), 200)
	maisAlto := novoUsuario(t)
	treinosEmDias(t, maisAlto, 175) // nível 5
	r = esperar(t, acao(t, maisAlto, id, "entrar", nil), 409)
	if !strings.Contains(r.Raw, "nível 2 a 4; você está no nível 5") {
		t.Errorf("nível alto: %s", r.Raw)
	}

	// idade (na data do evento): 18+
	idIdade := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoFin(c, 0, 10, 3, map[string]any{"idade_min": 18})), 201))
	jovem := novoUsuario(t)
	db.Exec("UPDATE usuarios SET data_nascimento = CURDATE() - INTERVAL 17 YEAR WHERE id = ?", jovem.ID)
	r = esperar(t, acao(t, jovem, idIdade, "entrar", nil), 409)
	if !strings.Contains(r.Raw, "idade mínima de 18 anos") {
		t.Errorf("idade: %s", r.Raw)
	}
	semData := novoUsuario(t)
	db.Exec("UPDATE usuarios SET data_nascimento = NULL WHERE id = ?", semData.ID)
	if d := detalhe(t, semData, idIdade); d["motivo_bloqueio_entrada"] != "Informe sua data de nascimento no perfil para entrar neste desafio" {
		t.Errorf("sem data de nascimento: %v", d["motivo_bloqueio_entrada"])
	}
	esperar(t, acao(t, semData, idIdade, "entrar", nil), 409)
	// sem filtro de idade, a falta da data não bloqueia
	esperar(t, acao(t, semData, criarDesafio(t, c, 0, 5), "entrar", nil), 200)
	// informa a data no perfil (só quem não tem); depois não troca
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", semData.Token, map[string]any{"data_nascimento": "1990-05-20"}), 200)
	esperar(t, acao(t, semData, idIdade, "entrar", nil), 200)
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", semData.Token, map[string]any{"data_nascimento": "1980-05-20"}), 409)
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", semData.Token, map[string]any{"data_nascimento": "1990-05-20"}), 200)
	esperar(t, api(t, "PATCH", "/api/usuarios/perfil", jovem.Token, map[string]any{"data_nascimento": "2999-01-01"}), 400)

	// mudança de nível depois da inscrição não expulsa
	db.Exec("DELETE FROM treino_conclusoes WHERE id_usuario = ?", a.ID)
	if d := detalhe(t, a, id); d["voce_participa"] != true {
		t.Error("inscrição deveria continuar")
	}
}

// Pendência da rodada 5: mudar início/fim de desafio com inscritos recusa (409) se criar choque de horário
// para algum inscrito em outro desafio da mesma academia.
func TestR6_EditarHorarioComInscritosRecusaChoque(t *testing.T) {
	c := novoCenario(t)
	base := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Hour)
	criarEm := func(inicio time.Time) uint {
		b := corpoDesafio(c, 0, 10, 3)
		b["data_evento"], b["data_fim_evento"] = inicio.Format(time.RFC3339), inicio.Add(2*time.Hour).Format(time.RFC3339)
		return idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, b), 201))
	}
	d1 := criarEm(base)                    // 0h–2h
	d2 := criarEm(base.Add(5 * time.Hour)) // 5h–7h
	d3 := criarEm(base.Add(5 * time.Hour)) // 5h–7h, sem inscritos em comum
	a := novoUsuario(t)
	inscrever(t, d1, a)
	inscrever(t, d2, a)
	inscrever(t, d3, novoUsuario(t))
	patch := func(id uint, inicio time.Time) resp {
		return api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{
			"data_evento": inicio.Format(time.RFC3339), "data_fim_evento": inicio.Add(2 * time.Hour).Format(time.RFC3339)})
	}
	r := esperar(t, patch(d2, base.Add(time.Hour)), 409) // 1h–3h choca com d1 (0h–2h)
	var titulo string
	db.Raw("SELECT titulo FROM desafios WHERE id = ?", d1).Scan(&titulo)
	if !strings.Contains(r.Raw, "1 atleta(s)") || !strings.Contains(r.Raw, titulo) {
		t.Errorf("mensagem do choque: %s", r.Raw)
	}
	// só o fim invadindo também choca
	esperar(t, api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", d1), c.A.Token, map[string]any{
		"data_fim_evento": base.Add(6 * time.Hour).Format(time.RFC3339)}), 409)
	esperar(t, patch(d2, base.Add(2*time.Hour)), 200) // 2h–4h encosta, não sobrepõe
	esperar(t, patch(d3, base.Add(time.Hour)), 200)   // inscrito de d3 não está em d1
	// desafio cancelado não conta
	esperar(t, acao(t, c.A, d1, "cancelar", nil), 200)
	esperar(t, patch(d2, base), 200)
}

// Nível GymScore: pódio + presença contam no encerramento; treino 1/dia; a anulação retira; o job reconcilia.
func TestR6_NivelGymScoreEventosAnulacaoEJob(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	p := lancarPodioMod(t, servidorMod(1), 3) // us[0] 1º, us[1] 2º, us[2] presente fora do pódio
	for i, want := range []int{60, 45, 10} {
		if pts, _, ok := nivelNoBanco(t, p.us[i].ID); pts != want || !ok {
			t.Errorf("atleta %d: pontuação %d (calculada %v), esperava %d", i, pts, ok, want)
		}
	}
	e := estatisticas(t, p.us[2], p.us[0].ID) // outro usuário vê (perfil público)
	if num(e, "pontuacao") != 60 || num(e, "nivel") != 3 || num(e, "nivel_inicio_em") != 50 || num(e, "proximo_nivel_em") != 100 ||
		num(e, "faltam_pontos") != 40 || e["sem_dados"] != false || e["base_calculo"] == "" {
		t.Errorf("estatísticas: %v", e)
	}
	comp := sub(t, e, "composicao")
	if num(comp, "primeiros") != 1 || num(comp, "presencas") != 1 || num(comp, "pontos_podios") != 50 {
		t.Errorf("composição: %v", comp)
	}
	atr, cat := sub(t, e, "atributos"), sub(t, e, "categorias")
	if len(atr) != 6 || len(cat) != 8 || num(cat, "corrida") <= 0 || num(cat, "luta") != 0 || num(atr, "velocidade") <= 0 {
		t.Errorf("eixos: %v / %v", atr, cat)
	}
	eixos := sub(t, e, "eixos")
	if len(eixos["atributos"].([]any)) != 6 || len(eixos["categorias"].([]any)) != 8 {
		t.Errorf("eixos: %v", eixos)
	}

	// treino: 1 ponto por dia, mesmo com vários exercícios
	for i := 0; i < 3; i++ {
		esperar(t, api(t, "POST", "/api/treinos/concluir", p.us[1].Token, map[string]any{"grupo_muscular": "peito"}), 200)
	}
	if pts, _, _ := nivelNoBanco(t, p.us[1].ID); pts != 46 {
		t.Errorf("treino de hoje: pontuação %d, esperava 46", pts)
	}

	// anulação (devolver inscrições): o resultado deixa de contar
	esperar(t, anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0), 200)
	for i, want := range []int{0, 1, 0} {
		if pts, _, _ := nivelNoBanco(t, p.us[i].ID); pts != want {
			t.Errorf("depois da anulação, atleta %d: %d, esperava %d", i, pts, want)
		}
	}
	if e := estatisticas(t, p.us[0], p.us[0].ID); num(e, "pontuacao") != 0 || e["sem_dados"] != true || e["mensagem"] == "" {
		t.Errorf("estatísticas sem dados: %v", e)
	}
	ledgerConfere(t, todos(p)...)
	conferirInvariante(t, db, "anulação")

	// relancar_podio: pontos saem na anulação e voltam para quem ficou no novo pódio
	p2 := lancarPodioMod(t, servidorMod(1), 3)
	esperar(t, anular(t, sa, p2.id, models.AnulacaoRelancarPodio, p2.c.I.ID, 0), 200)
	if pts, _, _ := nivelNoBanco(t, p2.us[0].ID); pts != 0 {
		t.Errorf("anulado para relançar: %d", pts)
	}
	esperar(t, chamar(t, servidorMod(0), "POST", fmt.Sprintf("/api/desafios/%d/podio", p2.id), p2.c.A.Token,
		podioBody(entradaPodio(p2.us[2], "1"), entradaPodio(p2.us[0], "2"))), 200)
	for i, want := range []int{45, 10, 60} {
		if pts, _, _ := nivelNoBanco(t, p2.us[i].ID); pts != want {
			t.Errorf("relançado, atleta %d: %d, esperava %d", i, pts, want)
		}
	}

	// 404 / id inválido
	esperar(t, api(t, "GET", "/api/usuarios/999999999/estatisticas", p.us[0].Token, nil), 404)
	esperar(t, api(t, "GET", "/api/usuarios/abc/estatisticas", p.us[0].Token, nil), 400)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/estatisticas", p.us[0].ID), "", nil), 401)

	// job: cache vazio/errado é corrigido
	db.Exec("UPDATE usuarios SET pontuacao_gymscore = 999, nivel_gymscore = 10, gymscore_calculado_em = NULL WHERE id = ?", p2.us[2].ID)
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	achou := false
	for _, tf := range tarefas {
		if tf.Nome == "reconciliar-nivel" {
			achou = true
			for i := 0; i < 50; i++ { // o banco de teste tem muitos usuários: lotes de 100
				if err := tf.Executar(); err != nil {
					t.Fatal(err)
				}
				if _, _, ok := nivelNoBanco(t, p2.us[2].ID); ok {
					break
				}
			}
		}
	}
	if pts, n, ok := nivelNoBanco(t, p2.us[2].ID); !achou || pts != 60 || n != 3 || !ok {
		t.Errorf("job reconciliar-nivel (achou %v): %d/%d/%v", achou, pts, n, ok)
	}
}

// Corrida: o atleta se inscreve em d2 enquanto a academia move d2 para cima de d1 (onde ele já está).
// A edição trava os inscritos e o Entrar trava o usuário: nunca terminam os dois com sucesso (choque).
func TestR6_CorridaEditarHorarioXEntrar(t *testing.T) {
	c := novoCenario(t)
	for rodada := 0; rodada < 4; rodada++ {
		base := time.Now().Add(time.Duration(40+rodada*3) * 24 * time.Hour).Truncate(time.Hour)
		criarEm := func(inicio time.Time) uint {
			b := corpoDesafio(c, 0, 10, 3)
			b["data_evento"], b["data_fim_evento"] = inicio.Format(time.RFC3339), inicio.Add(2*time.Hour).Format(time.RFC3339)
			return idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, b), 201))
		}
		d1, d2 := criarEm(base), criarEm(base.Add(5*time.Hour))
		a := novoUsuario(t)
		inscrever(t, d1, a)
		var wg sync.WaitGroup
		var re, rp resp
		wg.Add(2)
		go func() { defer wg.Done(); re = api(t, "POST", fmt.Sprintf("/api/desafios/%d/entrar", d2), a.Token, nil) }()
		go func() {
			defer wg.Done()
			rp = api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", d2), c.A.Token, map[string]any{
				"data_evento": base.Add(time.Hour).Format(time.RFC3339), "data_fim_evento": base.Add(3 * time.Hour).Format(time.RFC3339)})
		}()
		wg.Wait()
		if re.Status == 200 && rp.Status == 200 {
			t.Fatalf("rodada %d: inscrição e mudança de horário passaram juntas (choque criado)", rodada)
		}
		if (re.Status != 200 && re.Status != 409) || (rp.Status != 200 && rp.Status != 409) || re.Status == rp.Status {
			t.Errorf("rodada %d: entrar %d (%s), patch %d (%s)", rodada, re.Status, re.Raw, rp.Status, rp.Raw)
		}
	}
}

// Anulação + relançamento com a divisão da academia (80/20): o relançamento divide pela MESMA tabela,
// os pendentes do 1º pódio são cancelados e o dinheiro fecha (Σ saldos = Σ ledger, invariantes).
func TestR6_RelancamentoUsaDivisaoDaAcademia(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	srv := servidorMod(1)
	c := cenarioComSaldo(t, 20000)
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 2, map[string]any{
		"premio_fixo_centavos": 2001, "dividir_pote": true, "percentual_pote": 50, "divisao_premio_bp": []int{8000, 2000},
	})), 201))
	us := atletas(t, 3, 3000)
	inscrever(t, id, us...)
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/iniciar", id), c.I.Token, nil), 200)
	for _, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", id), c.I.Token,
		podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"))), 200)
	// T = 3000, taxa 10% = 300, pote 50% = 1500: prêmio total 3501 -> 80/20 = 2800 + 700 (+1 no 1º)
	if v, w := pendenteDe(t, id, us[0].ID), pendenteDe(t, id, us[1].ID); v != 2801 || w != 700 {
		t.Fatalf("1º pódio: %d / %d", v, w)
	}
	esperar(t, anular(t, sa, id, models.AnulacaoRelancarPodio, c.I.ID, 0), 200)
	esperar(t, chamar(t, servidorMod(0), "POST", fmt.Sprintf("/api/desafios/%d/podio", id), c.A.Token,
		podioBody(entradaPodio(us[2], "1"), entradaPodio(us[0], "2"))), 200)
	liberarPagamentos(t)
	if v := saldo(t, us[2]); v != 2000+2801 {
		t.Errorf("novo 1º: saldo %d", v)
	}
	if v := saldo(t, us[0]); v != 2000+700 {
		t.Errorf("novo 2º: saldo %d", v)
	}
	if v := saldo(t, us[1]); v != 2000 {
		t.Errorf("2º do pódio anulado: saldo %d", v)
	}
	invariantes(t, id)
	ledgerConfere(t, append(us, c.A, c.I)...)
	conferirInvariante(t, db, "relançamento com divisão da academia")
}
