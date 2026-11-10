//go:build integration

package integration

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
)

// ─── Helpers do financeiro ───────────────────────────────────────────────────

// servidorTaxa sobe a aplicação com a taxa da plataforma pedida (config injetada no teste).
func servidorTaxa(modo string, pct int, tarifa int64) *fiber.App {
	cfg := *cfgBase
	cfg.PlataformaTaxaModo, cfg.PlataformaTaxaInscricaoPct, cfg.PlataformaTarifaSaqueCentavos = modo, pct, tarifa
	return novoServidor(&cfg)
}

func acaoS(t *testing.T, srv *fiber.App, u *user, id uint, a string, corpo any) resp {
	t.Helper()
	r := chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/%s", id, a), u.Token, corpo)
	if a == "podio" && r.Status == 200 {
		aposPodio(t, id) // créditos do pódio passam pela janela de contestação (pagamentos_pendentes)
	}
	return r
}

func detalheS(t *testing.T, srv *fiber.App, u *user, id uint) map[string]any {
	t.Helper()
	return esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200).data()
}

// Consultas de invariante: as mesmas do database/README.md, restritas a um desafio.
const (
	invEncerrado = `SELECT d.id FROM desafios d
LEFT JOIN (SELECT id_desafio, -SUM(valor_centavos) AS entradas FROM transacoes
            WHERE tipo IN ('entrada_desafio', 'estorno_desafio') AND status IN ('received','concluido') GROUP BY id_desafio) e ON e.id_desafio = d.id
LEFT JOIN (SELECT id_desafio,
                  SUM(CASE WHEN tipo = 'premio_desafio' THEN valor_centavos END) AS premios,
                  SUM(CASE WHEN tipo = 'pagamento_instrutor' THEN valor_centavos END) AS instrutor,
                  SUM(CASE WHEN tipo = 'receita_academia' THEN valor_centavos END) AS academia
             FROM transacoes WHERE status IN ('received','concluido') GROUP BY id_desafio) s ON s.id_desafio = d.id
LEFT JOIN (SELECT id_desafio, SUM(valor_centavos) AS taxa FROM plataforma_receitas
            WHERE origem = 'taxa_inscricao' GROUP BY id_desafio) r ON r.id_desafio = d.id
WHERE d.status = 'encerrado' AND d.id = ?
  AND COALESCE(e.entradas, 0) + d.reserva_academia_centavos
      <> COALESCE(s.premios, 0) + COALESCE(s.instrutor, 0) + COALESCE(s.academia, 0) + COALESCE(r.taxa, 0)`
	invReserva = `SELECT d.id, d.reserva_academia_centavos AS reserva_coluna, -COALESCE(SUM(t.valor_centavos), 0) AS reserva_ledger
FROM desafios d
LEFT JOIN transacoes t ON t.id_desafio = d.id AND t.tipo IN ('reserva_desafio', 'estorno_reserva') AND t.status IN ('received','concluido')
WHERE d.status <> 'cancelado' AND d.id = ?
GROUP BY d.id, d.reserva_academia_centavos
HAVING reserva_coluna <> reserva_ledger`
	invCancelado = `SELECT d.id, SUM(t.valor_centavos) AS liquido
FROM desafios d JOIN transacoes t ON t.id_desafio = d.id AND t.status IN ('received','concluido')
WHERE d.status = 'cancelado' AND d.id = ?
GROUP BY d.id HAVING liquido <> 0`
)

// invariantes confere as 3 consultas do README para o desafio (0 linhas cada) e que a soma do ledger do
// desafio é exatamente -taxa (encerrado) ou 0 (cancelado): nenhum centavo some ou aparece.
func invariantes(t *testing.T, id uint) {
	t.Helper()
	for i, q := range []string{invEncerrado, invReserva, invCancelado} {
		var linhas []map[string]any
		if err := db.Raw(q, id).Scan(&linhas).Error; err != nil {
			t.Fatalf("consulta de invariante %d: %v", i+1, err)
		}
		if len(linhas) != 0 {
			t.Errorf("invariante %d violada no desafio %d: %v", i+1, id, linhas)
		}
	}
	var liquido, taxa int64
	db.Model(&models.Transacao{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND status IN ?", id, []string{"received", "concluido"}).Scan(&liquido)
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND origem = ?", id, models.ReceitaTaxaInscricao).Scan(&taxa)
	var status string
	db.Model(&models.Desafio{}).Select("status").Where("id = ?", id).Scan(&status)
	switch status {
	case "encerrado":
		if liquido != -taxa {
			t.Errorf("desafio %d encerrado: ledger líquido %d != -taxa %d", id, liquido, taxa)
		}
	case "cancelado":
		if liquido != 0 || taxa != 0 {
			t.Errorf("desafio %d cancelado: ledger líquido %d, taxa %d (esperava 0 e 0)", id, liquido, taxa)
		}
	}
}

func contarReceita(t *testing.T, origem string) (n, soma int64) {
	t.Helper()
	db.Model(&models.PlataformaReceita{}).Where("origem = ?", origem).Count(&n)
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").Where("origem = ?", origem).Scan(&soma)
	return
}

func receitaDoDesafio(t *testing.T, id uint) int64 {
	t.Helper()
	var v int64
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND origem = ?", id, models.ReceitaTaxaInscricao).Scan(&v)
	return v
}

func ledgerDe(t *testing.T, u *user, tipo models.TipoTransacao) (n, soma int64) {
	t.Helper()
	db.Model(&models.Transacao{}).Where("id_usuario = ? AND tipo = ?", u.ID, tipo).Count(&n)
	db.Model(&models.Transacao{}).Select("COALESCE(SUM(valor_centavos),0)").Where("id_usuario = ? AND tipo = ?", u.ID, tipo).Scan(&soma)
	return
}

func num(m map[string]any, k string) int64 {
	v, ok := m[k].(float64)
	if !ok {
		return -1 << 62
	}
	return int64(v)
}

func sub(t *testing.T, m map[string]any, k string) map[string]any {
	t.Helper()
	v, _ := m[k].(map[string]any)
	return v
}

func listaInt(m map[string]any, k string) []int64 {
	var out []int64
	for _, v := range m[k].([]any) {
		out = append(out, int64(v.(float64)))
	}
	return out
}

func igual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func corpoFin(c *cenario, entrada int64, vagas, podio int, extra map[string]any) map[string]any {
	b := corpoDesafio(c, entrada, vagas, podio)
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// academiaComSaldo cria o cenário e deposita saldo na conta da academia.
func cenarioComSaldo(t *testing.T, centavos int64) *cenario {
	t.Helper()
	c := novoCenario(t)
	depositar(t, c.A, centavos)
	return c
}

func acionar(t *testing.T, srv *fiber.App, quem *user, id uint, a string, corpo any, status int) {
	t.Helper()
	esperar(t, acaoS(t, srv, quem, id, a, corpo), status)
}

// ─── Fluxo completo, modo "inscricao" (taxa 10%) ─────────────────────────────

func TestFin_FluxoCompletoDesignadoModoInscricao(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 200)
	c := cenarioComSaldo(t, 20000)

	// prêmio fixo R$ 50 + pote 50%; instrutor R$ 20 + 5%: reserva = 5000 + 2000 = 7000
	r := esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 10, 3, map[string]any{
		"premio_fixo_centavos": 5000, "dividir_pote": true, "percentual_pote": 50,
		"instrutor_modo_pagamento": "fixo_percentual", "instrutor_fixo_centavos": 2000, "instrutor_pct": 5,
	})), 201)
	id := idDe(r)
	if saldo(t, c.A) != 13000 {
		t.Fatalf("a reserva de 7000 deveria ser debitada da academia: saldo %d", saldo(t, c.A))
	}
	if n, soma := contarLedger(t, id, models.TipoReservaDesafio); n != 1 || soma != -7000 {
		t.Errorf("ledger reserva: n=%d soma=%d", n, soma)
	}
	var d models.Desafio
	db.First(&d, id)
	if d.TaxaPlataformaPct != 10 || d.ReservaAcademiaCentavos != 7000 || d.PercentualPote != 50 || !d.DividirPote ||
		d.PremioFixoCentavos != 5000 || d.InstrutorFixoCentavos != 2000 || d.InstrutorPct != 5 || d.InstrutorSelecao != "designado" {
		t.Errorf("campos gravados: %+v", d)
	}

	us := atletas(t, 8, 3000)
	for _, u := range us {
		acionar(t, srv, u, id, "entrar", nil, 200)
	}
	// T = 8 x 1000 = 8000: pote 4000, prêmio total 9000 (60/30/10 = 5400/2700/900), taxa 800, instrutor 400 + 2000
	// academia = 8000 + 7000 - 9000 - 2400 - 800 = 2800

	// ── visibilidade por perfil ──
	atl := detalheS(t, srv, us[0], id)
	prem := sub(t, atl, "premiacao")
	if num(prem, "premio_total_centavos") != 9000 || !igual(listaInt(prem, "por_posicao_centavos"), []int64{5400, 2700, 900}) {
		t.Errorf("premiação do atleta: %v", prem)
	}
	if !igual(listaInt(atl, "premiacao_prevista_centavos"), []int64{5400, 2700, 900}) {
		t.Errorf("premiacao_prevista_centavos: %v", atl["premiacao_prevista_centavos"])
	}
	for _, k := range []string{"financeiro", "instrutor_ganho", "vaga"} {
		if _, tem := atl[k]; tem {
			t.Errorf("atleta não pode ver %q: %v", k, atl[k])
		}
	}
	if strings.Contains(fmt.Sprint(prem), "taxa") || strings.Contains(fmt.Sprint(atl["premiacao"]), "reserva") {
		t.Errorf("premiacao do atleta vaza dados financeiros: %v", prem)
	}
	acad := detalheS(t, srv, c.A, id)
	f := sub(t, acad, "financeiro")
	esperado := map[string]int64{
		"arrecadacao_centavos": 8000, "premio_fixo_centavos": 5000, "pote_premio_centavos": 4000, "premio_total_centavos": 9000,
		"taxa_plataforma_pct": 10, "taxa_plataforma_centavos": 800, "instrutor_fixo_centavos": 2000, "instrutor_pct": 5,
		"instrutor_pct_centavos": 400, "instrutor_total_centavos": 2400, "liquido_previsto_centavos": 2800,
		"resultado_previsto_centavos": 2800 - 7000, "reserva_centavos": 7000, "candidaturas_pendentes": 0,
	}
	for k, v := range esperado {
		if num(f, k) != v {
			t.Errorf("financeiro.%s = %v, esperava %d", k, f[k], v)
		}
	}
	if _, tem := acad["instrutor_ganho"]; tem {
		t.Error("a academia não recebe instrutor_ganho")
	}
	ins := detalheS(t, srv, c.I, id)
	g := sub(t, ins, "instrutor_ganho")
	if num(g, "total_previsto_centavos") != 2400 || num(g, "fixo_centavos") != 2000 || num(g, "pct_centavos") != 400 {
		t.Errorf("ganho do instrutor: %v", g)
	}
	if _, tem := ins["financeiro"]; tem {
		t.Errorf("instrutor não vê o financeiro da academia: %v", ins["financeiro"])
	}
	// outra academia e o feed público não veem nada financeiro
	outra, _ := novaAcademia(t)
	if _, tem := detalheS(t, srv, outra, id)["financeiro"]; tem {
		t.Error("outra academia não pode ver o financeiro")
	}
	feed := esperar(t, chamar(t, srv, "GET", "/api/desafios?limite=100", us[1].Token, nil), 200)
	if it := feedItem(t, feed, id); it["financeiro"] != nil || it["premiacao"] == nil {
		t.Errorf("feed do atleta: financeiro=%v premiacao=%v", it["financeiro"], it["premiacao"])
	}

	// ── execução ──
	acionar(t, srv, c.I, id, "iniciar", nil, 200)
	for _, u := range us {
		esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), c.I.Token, map[string]any{"id_usuario": u.ID}), 200)
	}
	acionar(t, srv, c.I, id, "podio", podioBody(entradaPodio(us[2], "10:00"), entradaPodio(us[0], "10:30"), entradaPodio(us[5], "11:00")), 200)

	if s := saldo(t, us[2]); s != 3000-1000+5400 {
		t.Errorf("1º lugar: saldo %d", s)
	}
	if s := saldo(t, us[0]); s != 3000-1000+2700 {
		t.Errorf("2º lugar: saldo %d", s)
	}
	if s := saldo(t, us[5]); s != 3000-1000+900 {
		t.Errorf("3º lugar: saldo %d", s)
	}
	if s := saldo(t, us[1]); s != 2000 {
		t.Errorf("fora do pódio: saldo %d", s)
	}
	if s := saldo(t, c.I); s != 2400 {
		t.Errorf("instrutor deveria receber 2000 + 400: saldo %d", s)
	}
	if s := saldo(t, c.A); s != 13000+2800 {
		t.Errorf("academia: saldo %d, esperava %d", s, 13000+2800)
	}
	if n, soma := ledgerDe(t, c.I, models.TipoPagamentoInstrutor); n != 1 || soma != 2400 {
		t.Errorf("pagamento_instrutor: n=%d soma=%d", n, soma)
	}
	if n, soma := ledgerDe(t, c.A, models.TipoReceitaAcademia); n != 1 || soma != 2800 {
		t.Errorf("receita_academia: n=%d soma=%d", n, soma)
	}
	if rec := receitaDoDesafio(t, id); rec != 800 {
		t.Errorf("receita da plataforma: %d", rec)
	}
	// o pote dos atletas não foi reduzido pela taxa: 5400 + 2700 + 900 = 9000
	if _, soma := contarLedger(t, id, models.TipoPremioDesafio); soma != 9000 {
		t.Errorf("soma dos prêmios %d, esperava 9000", soma)
	}
	enc := detalheS(t, srv, c.A, id)
	if enc["status"] != "encerrado" || num(sub(t, enc, "financeiro"), "reserva_centavos") != 7000 {
		t.Errorf("encerrado mantém a reserva vigente na coluna: %v", enc["financeiro"])
	}
	if contarNotif(t, c.I, models.NotifPagamentoInstrutor) != 1 || contarNotif(t, c.A, models.NotifRepasseAcademia) != 1 {
		t.Error("instrutor e academia deveriam ser notificados do pagamento")
	}
	invariantes(t, id)
	ledgerConfere(t, append(us, c.A, c.I)...)
}

// Mesma conta com a taxa em 0 e pote 100% (comportamento anterior): nada vai para a academia nem para a plataforma.
func TestFin_PadraoSemTaxaMantemComportamentoAnterior(t *testing.T) {
	c := novoCenario(t) // academia sem saldo: reserva 0
	id := criarDesafioPodio(t, c, 1000, 4, 2)
	us := atletas(t, 3, 2000)
	inscrever(t, id, us...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[0], "a"), entradaPodio(us[1], "b"))), 200)
	if saldo(t, us[0]) != 1000+2100 || saldo(t, us[1]) != 1000+900 || saldo(t, c.I) != 0 || saldo(t, c.A) != 0 {
		t.Errorf("saldos: %d %d instrutor=%d academia=%d", saldo(t, us[0]), saldo(t, us[1]), saldo(t, c.I), saldo(t, c.A))
	}
	if receitaDoDesafio(t, id) != 0 {
		t.Error("sem taxa não há receita")
	}
	invariantes(t, id)
}

// Sem dividir o pote: só o prêmio fixo vai para os atletas e percentual_pote é gravado 0.
func TestFin_SemDividirPoteGravaPercentualZero(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 10000)
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 2, map[string]any{
		"premio_fixo_centavos": 3000, "dividir_pote": false, "percentual_pote": 100,
	})), 201))
	var d models.Desafio
	db.First(&d, id)
	if d.DividirPote || d.PercentualPote != 0 || d.ReservaAcademiaCentavos != 3000 {
		t.Fatalf("gravado: dividir=%v pote=%d reserva=%d", d.DividirPote, d.PercentualPote, d.ReservaAcademiaCentavos)
	}
	us := atletas(t, 4, 1000)
	for _, u := range us {
		acionar(t, srv, u, id, "entrar", nil, 200)
	}
	acionar(t, srv, c.I, id, "iniciar", nil, 200)
	for _, u := range us {
		esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), c.I.Token, map[string]any{"id_usuario": u.ID}), 200)
	}
	acionar(t, srv, c.I, id, "podio", podioBody(entradaPodio(us[0], "a"), entradaPodio(us[1], "b")), 200)
	// T=4000; prêmio 3000 (70/30 = 2100/900); taxa 400; instrutor 0; academia = 4000 + 3000 - 3000 - 400 = 3600
	if saldo(t, us[0]) != 2100 || saldo(t, us[1]) != 900 || saldo(t, us[2]) != 0 {
		t.Errorf("prêmios: %d %d %d", saldo(t, us[0]), saldo(t, us[1]), saldo(t, us[2]))
	}
	if saldo(t, c.A) != 7000+3600 || receitaDoDesafio(t, id) != 400 {
		t.Errorf("academia %d (esperava 10600), taxa %d", saldo(t, c.A), receitaDoDesafio(t, id))
	}
	invariantes(t, id)
}

// ─── Criação e edição ────────────────────────────────────────────────────────

func TestFin_CriarValidacoes(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 5000)
	criar := func(extra map[string]any) resp {
		return chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 3, extra))
	}
	erroDe := func(r resp) string { s, _ := r.Body["error"].(string); return s }

	// soma > 100: mensagem diz o máximo do pote com a taxa vigente
	r := esperar(t, criar(map[string]any{"percentual_pote": 95}), 400)
	for _, trecho := range []string{"não pode passar de 100%", "o máximo para o pote é 90%", "taxa de 10%"} {
		if !strings.Contains(erroDe(r), trecho) {
			t.Errorf("erro %q sem %q", erroDe(r), trecho)
		}
	}
	r = esperar(t, criar(map[string]any{"percentual_pote": 80, "instrutor_modo_pagamento": "percentual", "instrutor_pct": 15}), 400)
	if !strings.Contains(erroDe(r), "o máximo para o pote é 75%") {
		t.Errorf("máximo com instrutor 15%%: %q", erroDe(r))
	}
	// fronteira: 90 + 10 passa
	esperar(t, criar(map[string]any{"percentual_pote": 90}), 201)
	// percentual_pote omitido = máximo (90 com taxa 10)
	id := idDe(esperar(t, criar(nil), 201))
	var d models.Desafio
	db.First(&d, id)
	if d.PercentualPote != 90 {
		t.Errorf("percentual_pote omitido deveria virar 90, obteve %d", d.PercentualPote)
	}
	// instrutor
	esperar(t, criar(map[string]any{"instrutor_pct": 5}), 400)                                                       // pct no modo fixo
	esperar(t, criar(map[string]any{"instrutor_modo_pagamento": "percentual", "instrutor_fixo_centavos": 100}), 400) // fixo no modo %
	esperar(t, criar(map[string]any{"instrutor_modo_pagamento": "mensal"}), 400)
	// rodada 5: o leilão foi removido (vaga_aberta/teto -> 400 com a orientação do modelo novo)
	esperar(t, criar(map[string]any{"instrutor_selecao": "vaga_aberta", "id_instrutor": nil}), 400)
	esperar(t, criar(map[string]any{"instrutor_selecao": "vaga_aberta", "instrutor_teto_centavos": 1000}), 400)
	esperar(t, criar(map[string]any{"instrutor_teto_centavos": 1000}), 400)
	esperar(t, criar(map[string]any{"premio_fixo_centavos": -1}), 400)
	b := corpoFin(c, 1000, 6, 3, nil)
	delete(b, "id_instrutor")
	// rodada 5: sem indicados a equipe nasce vazia (todas as vagas abertas à candidatura)
	esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, b), 201)
}

func TestFin_CriarSemSaldoFalhaSemCriarNada(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 3000)
	var antes int64
	db.Model(&models.Desafio{}).Where("id_criador = ?", c.A.ID).Count(&antes)
	r := esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 3, map[string]any{
		"premio_fixo_centavos": 2000, "instrutor_fixo_centavos": 1500,
	})), 422)
	msg, _ := r.Body["error"].(string)
	if !strings.Contains(msg, "/depositar") || !strings.Contains(msg, "R$ 35,00") {
		t.Errorf("mensagem deveria citar a reserva (R$ 35,00) e o link para depositar: %q", msg)
	}
	var depois int64
	db.Model(&models.Desafio{}).Where("id_criador = ?", c.A.ID).Count(&depois)
	if depois != antes || saldo(t, c.A) != 3000 {
		t.Errorf("nada deveria ter sido criado/debitado: desafios %d->%d, saldo %d", antes, depois, saldo(t, c.A))
	}
	// exatamente o saldo disponível passa
	esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 3, map[string]any{
		"premio_fixo_centavos": 2000, "instrutor_fixo_centavos": 1000,
	})), 201)
	if saldo(t, c.A) != 0 {
		t.Errorf("saldo deveria zerar: %d", saldo(t, c.A))
	}
	ledgerConfere(t, c.A)
}

func TestFin_EditarAjustaReservaPelaDiferenca(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 10000)
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 3, map[string]any{
		"premio_fixo_centavos": 4000, "percentual_pote": 60, "instrutor_fixo_centavos": 1000,
	})), 201))
	if saldo(t, c.A) != 5000 {
		t.Fatalf("reserva 5000: saldo %d", saldo(t, c.A))
	}
	patch := func(corpo map[string]any) resp {
		return chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, corpo)
	}
	// aumenta o prêmio fixo: debita a diferença
	esperar(t, patch(map[string]any{"premio_fixo_centavos": 6000}), 200)
	if saldo(t, c.A) != 3000 {
		t.Errorf("após +2000: saldo %d", saldo(t, c.A))
	}
	// reduz o instrutor fixo: devolve a diferença
	esperar(t, patch(map[string]any{"instrutor_fixo_centavos": 400}), 200)
	if saldo(t, c.A) != 3600 {
		t.Errorf("após -600: saldo %d", saldo(t, c.A))
	}
	if n, soma := contarLedger(t, id, models.TipoEstornoReserva); n != 1 || soma != 600 {
		t.Errorf("estorno_reserva: n=%d soma=%d", n, soma)
	}
	// sem saldo para a diferença: 422 e nada muda
	r := esperar(t, patch(map[string]any{"premio_fixo_centavos": 20000}), 422)
	if msg, _ := r.Body["error"].(string); !strings.Contains(msg, "/depositar") {
		t.Errorf("mensagem sem link para depositar: %q", msg)
	}
	if saldo(t, c.A) != 3600 || detalheS(t, srv, c.A, id)["financeiro"].(map[string]any)["reserva_centavos"].(float64) != 6400 {
		t.Error("edição recusada não pode alterar reserva nem saldo")
	}
	// soma de percentuais
	r = esperar(t, patch(map[string]any{"percentual_pote": 95}), 400)
	if msg, _ := r.Body["error"].(string); !strings.Contains(msg, "o máximo para o pote é 90%") {
		t.Errorf("mensagem: %q", msg)
	}
	// com atletas inscritos o prêmio só pode aumentar
	us := atletas(t, 2, 2000)
	inscrever(t, id, us...)
	esperar(t, patch(map[string]any{"premio_fixo_centavos": 5000}), 409)
	esperar(t, patch(map[string]any{"percentual_pote": 50}), 409)
	esperar(t, patch(map[string]any{"dividir_pote": false}), 409)
	esperar(t, patch(map[string]any{"premio_fixo_centavos": 7000, "percentual_pote": 70}), 200)
	invariantes(t, id)
	// cancelar devolve a reserva vigente inteira (7000 + 400)
	acionar(t, srv, c.A, id, "cancelar", nil, 200)
	if saldo(t, c.A) != 10000 {
		t.Errorf("após cancelar a academia deve recuperar tudo: %d", saldo(t, c.A))
	}
	invariantes(t, id)
	ledgerConfere(t, c.A, us[0], us[1])
}

// ─── Cancelamento ────────────────────────────────────────────────────────────

func TestFin_CancelarDevolveReservaEntradasESemTaxa(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 9000)
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1500, 6, 3, map[string]any{
		"premio_fixo_centavos": 5000, "instrutor_fixo_centavos": 2500,
	})), 201))
	us := atletas(t, 3, 4000)
	inscrever(t, id, us...)
	acionar(t, srv, c.I, id, "iniciar", nil, 200) // em andamento também pode cancelar
	acionar(t, srv, c.A, id, "cancelar", nil, 200)
	if saldo(t, c.A) != 9000 {
		t.Errorf("reserva devolvida inteira: saldo %d", saldo(t, c.A))
	}
	for _, u := range us {
		if saldo(t, u) != 4000 {
			t.Errorf("estorno integral (sem taxa): saldo %d", saldo(t, u))
		}
	}
	if receitaDoDesafio(t, id) != 0 || saldo(t, c.I) != 0 {
		t.Error("cancelamento não gera taxa nem paga o instrutor")
	}
	// a coluna guarda o valor reservado vigente: não é zerada ao cancelar
	var d models.Desafio
	db.First(&d, id)
	if d.ReservaAcademiaCentavos != 7500 || d.Status != "cancelado" {
		t.Errorf("reserva_academia_centavos=%d status=%s", d.ReservaAcademiaCentavos, d.Status)
	}
	acionar(t, srv, c.A, id, "cancelar", nil, 409) // idempotência: não devolve duas vezes
	if saldo(t, c.A) != 9000 {
		t.Errorf("segundo cancelar não pode creditar de novo: %d", saldo(t, c.A))
	}
	invariantes(t, id)
	ledgerConfere(t, append(us, c.A)...)
}

// O antigo leilão de instrutor (rodada 5: removido) é coberto pela equipe em rodada5_test.go.

// ─── Tarifa de saque ─────────────────────────────────────────────────────────

func sacarS(t *testing.T, srv *fiber.App, u *user, centavos int64) resp {
	t.Helper()
	return chamar(t, srv, "POST", "/api/carteira/saque", u.Token, map[string]any{
		"valor_centavos": centavos, "chave_pix": "teste@gymscore.test", "tipo_chave": "EMAIL",
	})
}

func TestFin_TarifaDeSaque(t *testing.T) {
	srv := servidorTaxa("inscricao_saque", 10, 200)
	u := usuarioComSaldo(t, 5000)
	if s := esperar(t, chamar(t, srv, "GET", "/api/carteira", u.Token, nil), 200).data(); num(s, "tarifa_saque_centavos") != 200 {
		t.Errorf("a carteira informa a tarifa: %v", s)
	}
	tx := esperar(t, chamar(t, srv, "GET", "/api/plataforma/taxas", u.Token, nil), 200).data()
	if tx["modo"] != "inscricao_saque" || num(tx, "taxa_inscricao_pct") != 10 || num(tx, "tarifa_saque_centavos") != 200 {
		t.Errorf("taxas: %v", tx)
	}
	_, antes := contarReceita(t, models.ReceitaTarifaSaque)

	r := esperar(t, sacarS(t, srv, u, 1000), 201)
	if num(r.data(), "valor_centavos") != 1000 || num(r.data(), "tarifa_centavos") != 200 {
		t.Errorf("resposta do saque: %v", r.data())
	}
	if saldo(t, u) != 5000-1200 {
		t.Errorf("débito = valor + tarifa: saldo %d", saldo(t, u))
	}
	if n, soma := ledgerDe(t, u, models.TipoTarifaSaque); n != 1 || soma != -200 {
		t.Errorf("linha tarifa_saque: n=%d soma=%d", n, soma)
	}
	if _, depois := contarReceita(t, models.ReceitaTarifaSaque); depois-antes != 200 {
		t.Errorf("receita da tarifa: +%d", depois-antes)
	}
	var rec models.PlataformaReceita
	db.Where("origem = ? AND id_transacao IS NOT NULL", models.ReceitaTarifaSaque).Order("id DESC").First(&rec)
	if rec.IDTransacao == nil || rec.ValorCentavos != 200 {
		t.Errorf("a receita aponta para a linha do ledger: %+v", rec)
	}
	ledgerConfere(t, u)

	// saldo insuficiente para valor + tarifa: com 1000 de saldo, sacar 900 (900 + 200 > 1000) falha
	pobre := usuarioComSaldo(t, 1000)
	re := esperar(t, sacarS(t, srv, pobre, 900), 422)
	if msg, _ := re.Body["error"].(string); !strings.Contains(msg, "tarifa") {
		t.Errorf("mensagem deveria citar a tarifa: %q", msg)
	}
	if saldo(t, pobre) != 1000 {
		t.Errorf("nada debitado: %d", saldo(t, pobre))
	}
	esperar(t, sacarS(t, srv, pobre, 800), 201) // 800 + 200 = 1000: passa e zera
	if saldo(t, pobre) != 0 {
		t.Errorf("saldo zerado: %d", saldo(t, pobre))
	}
	ledgerConfere(t, pobre)

	// servidor sem tarifa (modo inscricao): nenhuma linha
	sem := usuarioComSaldo(t, 3000)
	esperar(t, sacar(t, sem, 1000), 201)
	if n, _ := ledgerDe(t, sem, models.TipoTarifaSaque); n != 0 || saldo(t, sem) != 2000 {
		t.Errorf("sem tarifa: n=%d saldo=%d", n, saldo(t, sem))
	}
}

func TestFin_TarifaDeSaqueReembolsadaQuandoOPixFalha(t *testing.T) {
	srv := servidorTaxa("inscricao_saque", 10, 200)

	// (1) Asaas recusa na criação: valor e tarifa voltam, receita revertida
	u := usuarioComSaldo(t, 5000)
	_, antes := contarReceita(t, models.ReceitaTarifaSaque)
	asaas.modoFalhaTransf(true)
	esperar(t, sacarS(t, srv, u, 1000), 502)
	asaas.modoFalhaTransf(false)
	if saldo(t, u) != 5000 {
		t.Errorf("falha na criação devolve valor e tarifa: saldo %d", saldo(t, u))
	}
	if _, depois := contarReceita(t, models.ReceitaTarifaSaque); depois != antes {
		t.Errorf("receita da tarifa deveria ser revertida: %d -> %d", antes, depois)
	}
	ledgerConfere(t, u)

	// (2) Asaas aceita e depois falha (polling): idem, e consultar de novo não reembolsa duas vezes
	r := esperar(t, sacarS(t, srv, u, 1000), 201)
	id, tr := idDe(r), r.data()["asaas_transfer_id"].(string)
	if saldo(t, u) != 3800 {
		t.Fatalf("saldo após o saque: %d", saldo(t, u))
	}
	asaas.definirTransf(tr, "FAILED")
	f := esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/carteira/saque/%d", id), u.Token, nil), 200).data()
	if f["status"] != "failed" || num(f, "tarifa_centavos") != 0 {
		t.Errorf("saque falho: %v", f)
	}
	if saldo(t, u) != 5000 {
		t.Errorf("valor e tarifa devolvidos: %d", saldo(t, u))
	}
	esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/carteira/saque/%d", id), u.Token, nil), 200)
	if saldo(t, u) != 5000 {
		t.Errorf("reembolso idempotente: %d", saldo(t, u))
	}
	if _, depois := contarReceita(t, models.ReceitaTarifaSaque); depois != antes {
		t.Errorf("receita revertida uma vez: %d -> %d", antes, depois)
	}
	var falhas int64
	db.Model(&models.Transacao{}).Where("id_usuario = ? AND tipo = ? AND status = ?", u.ID, models.TipoTarifaSaque, "failed").Count(&falhas)
	if falhas != 2 {
		t.Errorf("as duas linhas de tarifa ficam failed (fora do ledger efetivo): %d", falhas)
	}
	ledgerConfere(t, u)

	// (3) saque concluído mantém a tarifa
	r = esperar(t, sacarS(t, srv, u, 1000), 201)
	asaas.definirTransf(r.data()["asaas_transfer_id"].(string), "DONE")
	d := esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/carteira/saque/%d", idDe(r)), u.Token, nil), 200).data()
	if d["status"] != "concluido" || num(d, "tarifa_centavos") != 200 || saldo(t, u) != 3800 {
		t.Errorf("saque concluído: %v saldo %d", d, saldo(t, u))
	}
	ledgerConfere(t, u)
}

func TestFin_ConcorrenciaSaquesComTarifa(t *testing.T) {
	srv := servidorTaxa("inscricao_saque", 10, 200)
	u := usuarioComSaldo(t, 5000) // cada saque custa 1200: cabem 4 (4800)
	_, antes := contarReceita(t, models.ReceitaTarifaSaque)
	var wg sync.WaitGroup
	var ok, sem int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch sacarS(t, srv, u, 1000).Status {
			case 201:
				atomic.AddInt64(&ok, 1)
			case 422:
				atomic.AddInt64(&sem, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 4 || sem != 6 {
		t.Fatalf("esperava 4 saques e 6 recusas por saldo, obteve %d e %d", ok, sem)
	}
	if saldo(t, u) != 200 {
		t.Errorf("saldo final %d (5000 - 4 x 1200)", saldo(t, u))
	}
	if _, depois := contarReceita(t, models.ReceitaTarifaSaque); depois-antes != 800 {
		t.Errorf("receita +%d, esperava 800", depois-antes)
	}
	if n, soma := ledgerDe(t, u, models.TipoTarifaSaque); n != 4 || soma != -800 {
		t.Errorf("tarifas: n=%d soma=%d", n, soma)
	}
	ledgerConfere(t, u)
}

// ─── Concorrência no desafio ─────────────────────────────────────────────────

// Cancelar contra lançar pódio: exatamente um vence e o dinheiro fecha nos dois desfechos.
func TestFin_ConcorrenciaCancelarVsLancarPodio(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	var encerrados, cancelados int
	for rodada := 0; rodada < 8; rodada++ {
		c := cenarioComSaldo(t, 10000)
		id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 2, map[string]any{
			"premio_fixo_centavos": 2000, "percentual_pote": 80, "instrutor_fixo_centavos": 500,
		})), 201))
		us := atletas(t, 3, 2000)
		for _, u := range us {
			acionar(t, srv, u, id, "entrar", nil, 200)
		}
		acionar(t, srv, c.I, id, "iniciar", nil, 200)
		for _, u := range us {
			esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), c.I.Token, map[string]any{"id_usuario": u.ID}), 200)
		}
		var wg sync.WaitGroup
		var rc, rp resp
		wg.Add(2)
		go func() { defer wg.Done(); rc = acaoS(t, srv, c.A, id, "cancelar", nil) }()
		go func() {
			defer wg.Done()
			rp = acaoS(t, srv, c.I, id, "podio", podioBody(entradaPodio(us[0], "a"), entradaPodio(us[1], "b")))
		}()
		wg.Wait()
		if (rc.Status == 200) == (rp.Status == 200) {
			t.Fatalf("rodada %d: exatamente um deve vencer (cancelar=%d podio=%d)", rodada, rc.Status, rp.Status)
		}
		if rc.Status == 200 {
			cancelados++
			if rp.Status != 409 || saldo(t, c.A) != 10000 || saldo(t, us[0]) != 2000 || saldo(t, c.I) != 0 {
				t.Fatalf("rodada %d (cancelado): podio=%d academia=%d atleta=%d instrutor=%d", rodada, rp.Status, saldo(t, c.A), saldo(t, us[0]), saldo(t, c.I))
			}
		} else {
			encerrados++
			if rc.Status != 409 || saldo(t, c.I) != 500 {
				t.Fatalf("rodada %d (encerrado): cancelar=%d instrutor=%d", rodada, rc.Status, saldo(t, c.I))
			}
		}
		invariantes(t, id)
		ledgerConfere(t, append(us, c.A, c.I)...)
	}
	t.Logf("desfechos: %d encerrados, %d cancelados", encerrados, cancelados)
}

// Config (taxa e tarifa) é lida na criação do app; o desafio guarda o snapshot da taxa.
func TestFin_TaxaDoDesafioEUmSnapshot(t *testing.T) {
	srv10 := servidorTaxa("inscricao", 10, 0)
	srv20 := servidorTaxa("inscricao", 20, 0)
	c := cenarioComSaldo(t, 1000)
	id := idDe(esperar(t, chamar(t, srv10, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 4, 1, nil)), 201))
	us := atletas(t, 2, 1000)
	for _, u := range us {
		acionar(t, srv20, u, id, "entrar", nil, 200) // outro app (taxa 20%) opera o mesmo desafio
	}
	acionar(t, srv20, c.I, id, "iniciar", nil, 200)
	for _, u := range us {
		esperar(t, chamar(t, srv20, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), c.I.Token, map[string]any{"id_usuario": u.ID}), 200)
	}
	acionar(t, srv20, c.I, id, "podio", podioBody(entradaPodio(us[0], "a")), 200)
	// T=2000; taxa do snapshot (10%) = 200; pote 90% = 1800 para o 1º; academia fica com 0
	if saldo(t, us[0]) != 1000-1000+1800 || receitaDoDesafio(t, id) != 200 {
		t.Errorf("snapshot da taxa: vencedor %d, receita %d", saldo(t, us[0]), receitaDoDesafio(t, id))
	}
	invariantes(t, id)
	_ = config.TaxaModoInscricao
}

// Desvincular o instrutor contra criar desafio com ele: nunca os dois dão certo (o instrutor é
// travado com FOR UPDATE nos dois lados), e nenhum desafio ativo fica com instrutor desvinculado.
func TestFin_ConcorrenciaDesvinculoContraCriacao(t *testing.T) {
	var criou, desvinculou int
	for rodada := 0; rodada < 10; rodada++ {
		c := novoCenario(t)
		var wg sync.WaitGroup
		var rc, rd resp
		wg.Add(2)
		go func() {
			defer wg.Done()
			rc = chamar(t, servidor, "POST", "/api/desafios", c.A.Token, corpoDesafio(c, 0, 4, 1))
		}()
		go func() {
			defer wg.Done()
			rd = chamar(t, servidor, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", c.InstrutorID), c.A.Token, nil)
		}()
		wg.Wait()
		okC, okD := rc.Status == 201, rd.Status >= 200 && rd.Status < 300
		if okC == okD {
			t.Fatalf("rodada %d: criar=%d (%s) desvincular=%d (%s): exatamente um deve vencer", rodada, rc.Status, rc.Raw, rd.Status, rd.Raw)
		}
		if okC {
			criou++
			if rd.Status != 409 {
				t.Fatalf("rodada %d: desvincular deveria dar 409, obteve %d", rodada, rd.Status)
			}
		} else {
			desvinculou++
			if rc.Status != 400 {
				t.Fatalf("rodada %d: criar deveria dar 400, obteve %d", rodada, rc.Status)
			}
		}
		var invalidos int64
		db.Raw(`SELECT COUNT(*) FROM desafios d JOIN instrutores i ON i.id = d.id_instrutor
			WHERE d.id_academia = ? AND d.status IN ('aberto','em_andamento') AND (i.id_academia IS NULL OR i.id_academia <> d.id_academia)`,
			c.AcademiaID).Scan(&invalidos)
		if invalidos != 0 {
			t.Fatalf("rodada %d: desafio ativo com instrutor desvinculado", rodada)
		}
	}
	t.Logf("desfechos: %d criações, %d desvínculos", criou, desvinculou)
}

// Autorização vence validação: quem não pode editar/candidatar recebe 403 (ou 404) mesmo com corpo inválido.
func TestFin_AutorizacaoAntesDaValidacaoDoCorpo(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	outra, _ := novaAcademia(t)
	invalido := map[string]any{"vagas": 100000, "premio_fixo_centavos": -5, "titulo": "x"}
	esperar(t, api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), outra.Token, invalido), 403)
	esperar(t, api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.I.Token, invalido), 403)
	esperar(t, api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id+99999), outra.Token, invalido), 404)
	esperar(t, api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, invalido), 400) // a dona recebe o 400
	// candidatura: quem não é instrutor recebe 403 com corpo inválido
	atleta := novoUsuario(t)
	ruim := map[string]any{"lance_centavos": -1, "mensagem": strings.Repeat("x", 600)}
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), atleta.Token, ruim), 403)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), c.A.Token, ruim), 403)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id+99999), c.I.Token, ruim), 404)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), c.I.Token, ruim), 400)
}
