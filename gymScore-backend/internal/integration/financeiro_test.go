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
	esperar(t, criar(map[string]any{"instrutor_selecao": "vaga_aberta", "id_instrutor": nil}), 400)             // sem teto
	esperar(t, criar(map[string]any{"instrutor_selecao": "vaga_aberta", "instrutor_teto_centavos": 1000}), 400) // com id_instrutor
	esperar(t, criar(map[string]any{"instrutor_teto_centavos": 1000}), 400)                                     // teto em designado
	esperar(t, criar(map[string]any{"premio_fixo_centavos": -1}), 400)
	b := corpoFin(c, 1000, 6, 3, nil)
	delete(b, "id_instrutor")
	r = esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, b), 400)
	if !strings.Contains(erroDe(r), "id_instrutor") {
		t.Errorf("designado sem instrutor: %q", erroDe(r))
	}
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

// ─── Leilão de instrutor ─────────────────────────────────────────────────────

func candidatar(t *testing.T, srv *fiber.App, u *user, id uint, lance int64, msg string) resp {
	t.Helper()
	return chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), u.Token,
		map[string]any{"lance_centavos": lance, "mensagem": msg})
}

func candidaturasDe(t *testing.T, srv *fiber.App, u *user, id uint) []any {
	t.Helper()
	return esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), u.Token, nil), 200).Body["data"].([]any)
}

func TestFin_LeilaoDeInstrutorFluxoCompleto(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 20000)
	// vaga aberta, teto R$ 30; prêmio fixo R$ 40; pote 50%; instrutor: lance (fixo) + 5%. Reserva = 4000 + 3000.
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, func() map[string]any {
		b := corpoFin(c, 1000, 10, 3, map[string]any{
			"instrutor_selecao": "vaga_aberta", "instrutor_teto_centavos": 3000, "instrutor_modo_pagamento": "fixo_percentual",
			"instrutor_pct": 5, "premio_fixo_centavos": 4000, "percentual_pote": 50,
		})
		delete(b, "id_instrutor")
		return b
	}()), 201))
	if saldo(t, c.A) != 13000 {
		t.Fatalf("reserva = prêmio fixo + teto = 7000: saldo %d", saldo(t, c.A))
	}
	acad := detalheS(t, srv, c.A, id)
	if acad["vaga_instrutor_aberta"] != true || acad["instrutor"] != nil || acad["pode_iniciar"] != false ||
		acad["pode_escolher_instrutor"] != true || acad["instrutor_selecao"] != "vaga_aberta" {
		t.Errorf("detalhe com vaga aberta: %v", acad)
	}
	acionar(t, srv, c.A, id, "iniciar", nil, 409) // sem instrutor escolhido não inicia
	atleta0 := usuarioComSaldo(t, 5000)
	inscrever(t, id, atleta0) // inscrições seguem abertas

	// instrutores: vinculado, autônomo e outro autônomo
	i1, i1id := novoInstrutorDe(t, c.AcademiaID)
	i2, i2id := novoInstrutorDe(t, 0)
	i3, _ := novoInstrutorDe(t, 0)

	// lances: mensagens de erro e permissões
	r := esperar(t, candidatar(t, srv, i3, id, 3500, ""), 400)
	if msg, _ := r.Body["error"].(string); !strings.Contains(msg, "não pode passar do teto de R$ 30,00") {
		t.Errorf("lance acima do teto: %q", msg)
	}
	esperar(t, candidatar(t, srv, i3, id, -1, ""), 400)
	esperar(t, candidatar(t, srv, atleta0, id, 1000, ""), 403)
	esperar(t, candidatar(t, srv, c.A, id, 1000, ""), 403)
	esperar(t, candidatar(t, srv, i3, id, 3000, strings.Repeat("x", 501)), 400)

	c1 := esperar(t, candidatar(t, srv, i1, id, 2500, "Tenho 10 anos de experiência"), 201).data()
	esperar(t, candidatar(t, srv, i2, id, 1800, ""), 201)
	if c1["status"] != "pendente" || num(c1, "lance_centavos") != 2500 {
		t.Errorf("candidatura criada: %v", c1)
	}
	esperar(t, candidatar(t, srv, i2, id, 1500, "baixei"), 200) // atualiza o lance (mesma linha)
	esperar(t, candidatar(t, srv, i3, id, 3000, ""), 201)
	r = esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", id), i3.Token, nil), 200)
	if r.data()["status"] != "retirada" {
		t.Errorf("retirada: %v", r.data())
	}
	esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", id), i3.Token, nil), 409) // já retirada
	esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", i3.ID+9999), i3.Token, nil), 404)
	var linhas int64
	db.Model(&models.CandidaturaInstrutor{}).Where("id_desafio = ?", id).Count(&linhas)
	if linhas != 3 {
		t.Errorf("uma linha por (desafio, instrutor): %d", linhas)
	}
	esperar(t, candidatar(t, srv, i3, id, 2000, "voltei"), 201) // reabre após retirar
	esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", id), i3.Token, nil), 200)

	// a academia vê os candidatos (do menor para o maior lance, sem os retirados)
	lista := candidaturasDe(t, srv, c.A, id)
	if len(lista) != 2 {
		t.Fatalf("2 candidatos ativos esperados: %v", lista)
	}
	p, s := lista[0].(map[string]any), lista[1].(map[string]any)
	if num(p, "lance_centavos") != 1500 || num(s, "lance_centavos") != 2500 {
		t.Errorf("ordem por lance: %v %v", p, s)
	}
	ins := sub(t, s, "instrutor")
	if ins["cref"] != "123456-G/SP" || ins["nome"] == "" || ins["vinculado"] != true || s["mensagem"] != "Tenho 10 anos de experiência" {
		t.Errorf("dados do candidato vinculado: %v", s)
	}
	if sub(t, p, "instrutor")["vinculado"] != false {
		t.Errorf("autônomo não é vinculado: %v", p)
	}
	if num(sub(t, detalheS(t, srv, c.A, id), "financeiro"), "candidaturas_pendentes") != 2 {
		t.Error("financeiro.candidaturas_pendentes = 2")
	}
	// só a academia dona vê a lista
	esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), i1.Token, nil), 403)
	esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), atleta0.Token, nil), 403)
	outra, _ := novaAcademia(t)
	esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), outra.Token, nil), 403)

	// o candidato vê o teto e o próprio lance, e a vaga na aba de vagas
	v := esperar(t, chamar(t, srv, "GET", "/api/instrutor/vagas?limite=100", i1.Token, nil), 200)
	it := feedItem(t, v, id)
	vaga := sub(t, it, "vaga")
	if num(vaga, "teto_centavos") != 3000 || num(sub(t, vaga, "minha_candidatura"), "lance_centavos") != 2500 || it["pode_candidatar"] != true {
		t.Errorf("vaga na visão do candidato: %v", vaga)
	}
	if it["financeiro"] != nil {
		t.Error("candidato não vê o financeiro da academia")
	}
	esperar(t, chamar(t, srv, "GET", "/api/instrutor/vagas", atleta0.Token, nil), 403)
	esperar(t, chamar(t, srv, "GET", "/api/instrutor/vagas", c.A.Token, nil), 403)
	// quem não é instrutor não vê o teto no detalhe
	if _, tem := detalheS(t, srv, atleta0, id)["vaga"]; tem {
		t.Error("atleta não vê a oferta da vaga")
	}

	// escolha: só a academia dona
	var cid2 uint
	for _, x := range candidaturasDe(t, srv, c.A, id) {
		if m := x.(map[string]any); num(m, "lance_centavos") == 1500 {
			cid2 = uint(m["id"].(float64))
		}
	}
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid2), i2.Token, nil), 403)
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid2), outra.Token, nil), 403)
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/999999/escolher", id), c.A.Token, nil), 404)
	e := esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid2), c.A.Token, nil), 200).data()
	if sub(t, e, "instrutor")["id"].(float64) != float64(i2id) || e["vaga_instrutor_aberta"] != false {
		t.Errorf("instrutor escolhido: %v", e["instrutor"])
	}
	// teto - lance = 1500 volta para a academia; reserva = 4000 + 1500
	if saldo(t, c.A) != 13000+1500 {
		t.Errorf("diferença teto-lance devolvida: saldo %d", saldo(t, c.A))
	}
	if n, soma := contarLedger(t, id, models.TipoEstornoReserva); n != 1 || soma != 1500 {
		t.Errorf("estorno_reserva: n=%d soma=%d", n, soma)
	}
	f := sub(t, e, "financeiro")
	if num(f, "reserva_centavos") != 5500 || num(f, "instrutor_fixo_centavos") != 1500 {
		t.Errorf("financeiro após a escolha: %v", f)
	}
	// o autônomo escolhido NÃO vira vinculado da academia
	var ai models.Instrutor
	db.First(&ai, i2id)
	if ai.IDAcademia != nil {
		t.Errorf("ser escolhido não cria vínculo: id_academia=%v", *ai.IDAcademia)
	}
	// notificações: escolhido e não escolhidos
	if contarNotif(t, i2, models.NotifCandidaturaEscolhida) != 1 || contarNotif(t, i1, models.NotifCandidaturaRecusada) != 1 {
		t.Error("escolhido e não escolhidos deveriam ser notificados")
	}
	if contarNotif(t, c.A, models.NotifCandidaturaRecebida) < 3 {
		t.Error("a academia é avisada de cada candidatura")
	}
	// depois da escolha: vaga preenchida, candidaturas congeladas, pagamento do instrutor travado
	esperar(t, candidatar(t, srv, i1, id, 100, ""), 409)
	esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", id), i2.Token, nil), 409) // escolhida não retira
	esperar(t, chamar(t, srv, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", id), i1.Token, nil), 409) // recusada
	var cid1 uint
	db.Model(&models.CandidaturaInstrutor{}).Select("id").Where("id_desafio = ? AND id_instrutor = ?", id, i1id).Scan(&cid1)
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid1), c.A.Token, nil), 409)
	esperar(t, chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"instrutor_fixo_centavos": 100}), 409)
	esperar(t, chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"id_instrutor": i1id}), 409)

	// permissões do instrutor designado (autônomo): vê quanto ganha; o outro candidato não opera
	g := sub(t, detalheS(t, srv, i2, id), "instrutor_ganho")
	if num(g, "fixo_centavos") != 1500 || num(g, "pct") != 5 {
		t.Errorf("ganho do instrutor escolhido: %v", g)
	}
	acionar(t, srv, i1, id, "iniciar", nil, 403)
	esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/desafios/%d/presentes", id), i1.Token, nil), 403)

	// execução: 4 atletas x 1000 (+ o atleta0 já inscrito = 5)
	us := atletas(t, 4, 2000)
	inscrever(t, id, us...)
	acionar(t, srv, i2, id, "iniciar", nil, 200)
	todos := append([]*user{atleta0}, us...)
	for _, u := range todos {
		esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), i2.Token, map[string]any{"id_usuario": u.ID}), 200)
	}
	acionar(t, srv, i2, id, "podio", podioBody(entradaPodio(us[0], "a"), entradaPodio(atleta0, "b"), entradaPodio(us[3], "c")), 200)
	// T = 5000: pote 50% = 2500, prêmio total 6500 (60/30/10 = 3900/1950/650), taxa 500,
	// instrutor 1500 + 5% (250) = 1750, academia = 5000 + 5500 - 6500 - 1750 - 500 = 1750
	if saldo(t, us[0]) != 1000+3900 || saldo(t, atleta0) != 4000+1950 || saldo(t, us[3]) != 1000+650 {
		t.Errorf("prêmios: %d %d %d", saldo(t, us[0]), saldo(t, atleta0), saldo(t, us[3]))
	}
	if saldo(t, i2) != 1750 {
		t.Errorf("instrutor escolhido recebe lance + %%: %d", saldo(t, i2))
	}
	if saldo(t, c.A) != 14500+1750 || receitaDoDesafio(t, id) != 500 {
		t.Errorf("academia %d (esperava %d), taxa %d", saldo(t, c.A), 14500+1750, receitaDoDesafio(t, id))
	}
	if saldo(t, i1) != 0 {
		t.Error("candidato não escolhido não recebe nada")
	}
	invariantes(t, id)
	ledgerConfere(t, append(todos, c.A, i1, i2, i3)...)
}

func TestFin_LeilaoModoPercentualELanceZero(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 5000)
	b := corpoFin(c, 1000, 6, 2, map[string]any{
		"instrutor_selecao": "vaga_aberta", "instrutor_modo_pagamento": "percentual", "instrutor_pct": 20,
		"instrutor_teto_centavos": 0,
	})
	delete(b, "id_instrutor")
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, b), 201))
	// pote omitido: 100 - 10 - 20 = 70
	var d models.Desafio
	db.First(&d, id)
	if d.PercentualPote != 70 || d.ReservaAcademiaCentavos != 0 || saldo(t, c.A) != 5000 {
		t.Errorf("pote=%d reserva=%d saldo=%d", d.PercentualPote, d.ReservaAcademiaCentavos, saldo(t, c.A))
	}
	i, _ := novoInstrutorDe(t, 0)
	cid := uint(esperar(t, candidatar(t, srv, i, id, 0, "só percentual"), 201).data()["id"].(float64))
	esperar(t, candidatar(t, srv, i, id, 1, ""), 400) // teto 0
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid), c.A.Token, nil), 200)
	us := atletas(t, 3, 1000)
	for _, u := range us {
		acionar(t, srv, u, id, "entrar", nil, 200)
	}
	acionar(t, srv, i, id, "iniciar", nil, 200)
	for _, u := range us {
		esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/presenca", id), i.Token, map[string]any{"id_usuario": u.ID}), 200)
	}
	acionar(t, srv, i, id, "podio", podioBody(entradaPodio(us[1], "a"), entradaPodio(us[2], "b")), 200)
	// T = 3000: pote 70% = 2100 (70/30 = 1470/630), taxa 300, instrutor 20% = 600, academia = 3000 - 2100 - 300 - 600 = 0
	if saldo(t, us[1]) != 1470 || saldo(t, us[2]) != 630 || saldo(t, i) != 600 || saldo(t, c.A) != 5000 || receitaDoDesafio(t, id) != 300 {
		t.Errorf("saldos: %d %d instrutor=%d academia=%d taxa=%d", saldo(t, us[1]), saldo(t, us[2]), saldo(t, i), saldo(t, c.A), receitaDoDesafio(t, id))
	}
	invariantes(t, id)
}

func TestFin_LeilaoCancelarDevolveTetoEDispensaCandidatos(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 9000)
	b := corpoFin(c, 0, 4, 1, map[string]any{"instrutor_selecao": "vaga_aberta", "instrutor_teto_centavos": 2000, "premio_fixo_centavos": 3000})
	delete(b, "id_instrutor")
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, b), 201))
	i, _ := novoInstrutorDe(t, 0)
	esperar(t, candidatar(t, srv, i, id, 1000, ""), 201)
	// baixar o teto abaixo de um lance pendente é conflito; acima passa e ajusta a reserva
	esperar(t, chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"instrutor_teto_centavos": 500}), 409)
	esperar(t, chamar(t, srv, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, map[string]any{"instrutor_teto_centavos": 2500}), 200)
	if saldo(t, c.A) != 9000-3000-2500 {
		t.Errorf("reserva = 3000 + 2500: saldo %d", saldo(t, c.A))
	}
	acionar(t, srv, c.A, id, "cancelar", nil, 200)
	if saldo(t, c.A) != 9000 {
		t.Errorf("cancelar devolve a reserva inteira: %d", saldo(t, c.A))
	}
	if contarNotif(t, i, models.NotifCandidaturaRecusada) != 1 {
		t.Error("candidato pendente deveria ser avisado do cancelamento")
	}
	esperar(t, candidatar(t, srv, i, id, 100, ""), 409) // desafio cancelado
	invariantes(t, id)
	ledgerConfere(t, c.A)
}

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

// Duas escolhas ao mesmo tempo (candidatos diferentes e o mesmo candidato): uma vence, o resto 409,
// a diferença da reserva volta uma única vez.
func TestFin_ConcorrenciaDuasEscolhasDeCandidato(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	for rodada := 0; rodada < 4; rodada++ {
		c := cenarioComSaldo(t, 10000)
		b := corpoFin(c, 1000, 4, 1, map[string]any{"instrutor_selecao": "vaga_aberta", "instrutor_teto_centavos": 3000, "premio_fixo_centavos": 2000})
		delete(b, "id_instrutor")
		id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, b), 201))
		var cids []uint
		for _, lance := range []int64{1000, 1200, 1400} {
			i, _ := novoInstrutorDe(t, 0)
			cids = append(cids, uint(esperar(t, candidatar(t, srv, i, id, lance, ""), 201).data()["id"].(float64)))
		}
		var wg sync.WaitGroup
		var ok, conflito int64
		for k := 0; k < 6; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				cid := cids[k%len(cids)]
				switch chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid), c.A.Token, nil).Status {
				case 200:
					atomic.AddInt64(&ok, 1)
				case 409:
					atomic.AddInt64(&conflito, 1)
				}
			}(k)
		}
		wg.Wait()
		if ok != 1 || conflito != 5 {
			t.Fatalf("rodada %d: esperava 1 escolha e 5 conflitos, obteve %d e %d", rodada, ok, conflito)
		}
		var escolhidas, recusadas int64
		db.Model(&models.CandidaturaInstrutor{}).Where("id_desafio = ? AND status = ?", id, "escolhida").Count(&escolhidas)
		db.Model(&models.CandidaturaInstrutor{}).Where("id_desafio = ? AND status = ?", id, "recusada").Count(&recusadas)
		if escolhidas != 1 || recusadas != 2 {
			t.Fatalf("rodada %d: escolhidas=%d recusadas=%d", rodada, escolhidas, recusadas)
		}
		if n, _ := contarLedger(t, id, models.TipoEstornoReserva); n != 1 {
			t.Fatalf("rodada %d: a diferença deve voltar uma única vez (%d linhas)", rodada, n)
		}
		var d models.Desafio
		db.First(&d, id)
		if d.IDInstrutor == nil || d.ReservaAcademiaCentavos != 2000+d.InstrutorFixoCentavos || saldo(t, c.A) != 10000-d.ReservaAcademiaCentavos {
			t.Fatalf("rodada %d: reserva %d, instrutor_fixo %d, saldo %d", rodada, d.ReservaAcademiaCentavos, d.InstrutorFixoCentavos, saldo(t, c.A))
		}
		invariantes(t, id)
		ledgerConfere(t, c.A)
	}
}

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
