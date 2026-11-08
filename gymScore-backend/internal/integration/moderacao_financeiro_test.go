//go:build integration

package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/models"

	"github.com/gofiber/fiber/v2"
)

// Moderação parte 2: pódio com janela de contestação (pagamentos_pendentes), job liberar-pagamentos,
// retenção, anulação do resultado (relancar_podio / devolver_inscricoes), débitos (Q5) e saque bloqueado.
// Toda verificação de dinheiro fecha a conferência do dashboard (Σ saldos + custódia + pendentes −
// débitos + receita líquida = depósitos − saques): a divergência não muda com o cenário.

// servidorMod sobe a app com taxa da plataforma de 10% (modo inscricao) e a janela pedida (horas).
func servidorMod(janelaHoras int) *fiber.App {
	cfg := *cfgBase
	cfg.PlataformaTaxaModo, cfg.PlataformaTaxaInscricaoPct, cfg.PlataformaTarifaSaqueCentavos = "inscricao", 10, 0
	cfg.PremioJanelaContestacaoHoras = janelaHoras
	return novoServidor(&cfg)
}

type podioMod struct {
	c           *cenario
	id          uint
	us          []*user // us[0] = 1º lugar, us[1] = 2º lugar
	pontosAntes int     // pontos de us[0] antes do pódio
}

// lancarPodioMod: academia (saldo 20000) cria desafio (entrada 1000, prêmio fixo 2000 + 50% do pote,
// instrutor 1000 + 5%: reserva 3000), n atletas (saldo 3000) entram, presença de todos e pódio de 2
// posições lançado pelo instrutor no servidor `srv` (a janela vem dele). Não roda a liberação.
func lancarPodioMod(t *testing.T, srv *fiber.App, n int) *podioMod {
	t.Helper()
	return lancarPodioModCom(t, srv, cenarioComSaldo(t, 20000), n)
}

func lancarPodioModCom(t *testing.T, srv *fiber.App, c *cenario, n int) *podioMod {
	t.Helper()
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoFin(c, 1000, 6, 2, map[string]any{
		"premio_fixo_centavos": 2000, "dividir_pote": true, "percentual_pote": 50,
		"instrutor_modo_pagamento": "fixo_percentual", "instrutor_fixo_centavos": 1000, "instrutor_pct": 5,
	})), 201))
	us := atletas(t, n, 3000)
	inscrever(t, id, us...)
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/iniciar", id), c.I.Token, nil), 200)
	for _, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	p := &podioMod{c: c, id: id, us: us, pontosAntes: pontos(t, us[0])}
	esperar(t, chamar(t, srv, "POST", fmt.Sprintf("/api/desafios/%d/podio", id), c.I.Token,
		podioBody(entradaPodio(us[0], "10:00"), entradaPodio(us[1], "10:30"))), 200)
	return p
}

// pendenteDe soma os pendentes (qualquer status) do usuário no desafio.
func pendenteDe(t *testing.T, idDesafio, idUsuario uint) int64 {
	t.Helper()
	var v int64
	db.Model(&models.PagamentoPendente{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).Scan(&v)
	return v
}

func pendentesDo(t *testing.T, idDesafio uint) []models.PagamentoPendente {
	t.Helper()
	var ps []models.PagamentoPendente
	if err := db.Where("id_desafio = ?", idDesafio).Order("id").Find(&ps).Error; err != nil {
		t.Fatal(err)
	}
	return ps
}

// divergencia lê a divergência da conferência do dashboard (aplicações − origens).
func divergencia(t *testing.T, a *adminT) int64 {
	t.Helper()
	d := esperar(t, adm(t, "GET", "/api/admin/dashboard", a, nil), 200).data()
	return num(sub(t, d, "conferencia"), "divergencia_centavos")
}

func anular(t *testing.T, a *adminT, id uint, modo string, punido uint, idDenuncia uint) resp {
	t.Helper()
	corpo := map[string]any{"modo": modo, "justificativa": "Fraude comprovada no resultado do pódio",
		"id_usuario_punido": punido, "motivo": "anulação por fraude no pódio"}
	if idDenuncia != 0 {
		corpo["id_denuncia"] = idDenuncia
	}
	return adm(t, "POST", fmt.Sprintf("/api/admin/desafios/%d/anular-resultado", id), a, corpo)
}

func debitoAberto(t *testing.T, idUsuario uint) int64 {
	t.Helper()
	var v int64
	db.Model(&models.UsuarioDebito{}).Select("COALESCE(SUM(saldo_restante_centavos),0)").
		Where("id_usuario = ? AND quitado_em IS NULL", idUsuario).Scan(&v)
	return v
}

func carteira(t *testing.T, u *user) map[string]any {
	t.Helper()
	return esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 200).data()
}

func eventosDoDesafio(t *testing.T, idDesafio uint, acao string) int64 {
	t.Helper()
	var n int64
	db.Model(&models.ModeracaoEvento{}).Where("id_desafio = ? AND acao = ?", idDesafio, acao).Count(&n)
	return n
}

func todos(p *podioMod) []*user { return append(append([]*user{}, p.us...), p.c.A, p.c.I) }

// ─── Janela e liberação ──────────────────────────────────────────────────────

func TestPodio_JanelaDeContestacaoELiberacaoAposPrazo(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	div0 := divergencia(t, a)
	srv := servidorMod(1)
	p := lancarPodioMod(t, srv, 4)
	premio1 := pendenteDe(t, p.id, p.us[0].ID)
	if premio1 <= 0 {
		t.Fatalf("1º lugar sem pendente de prêmio")
	}
	if s := saldo(t, p.us[0]); s != 2000 {
		t.Errorf("o prêmio não pode entrar no saldo antes da janela: saldo %d", s)
	}
	conferirPendentesDoPodio(t, p.id)
	var d models.Desafio
	db.First(&d, p.id)
	if d.PodioLancadoEm == nil || d.PodioLancadoPor == nil || *d.PodioLancadoPor != p.c.I.ID {
		t.Fatalf("podio_lancado_em/por: %v %v", d.PodioLancadoEm, d.PodioLancadoPor)
	}
	tipos := map[string]bool{}
	ps := pendentesDo(t, p.id)
	for _, x := range ps {
		tipos[x.Tipo] = true
		if x.Status != models.PendenteALiberar {
			t.Errorf("pendente %d nasceu %s", x.ID, x.Status)
		}
		if dif := x.LiberarEm.Sub(*d.PodioLancadoEm) - time.Hour; dif < -2*time.Millisecond || dif > 2*time.Millisecond {
			t.Errorf("liberar_em = podio_lancado_em + janela: %v vs %v", x.LiberarEm, *d.PodioLancadoEm)
		}
		if (x.Tipo == models.PendenteTaxaInscricao) != (x.IDUsuario == nil) {
			t.Errorf("beneficiário do pendente %d (%s)", x.ID, x.Tipo)
		}
	}
	for _, tp := range []string{models.PendentePremio, models.PendentePagamentoInstrutor, models.PendenteReceitaAcademia, models.PendenteTaxaInscricao} {
		if !tipos[tp] {
			t.Errorf("faltou pendente %s: %v", tp, tipos)
		}
	}
	if eventosDoDesafio(t, p.id, "podio_lancado") != 1 {
		t.Error("evento podio_lancado")
	}
	// carteira e detalhe mostram o "a liberar"
	cw := carteira(t, p.us[0])
	if num(cw, "a_liberar_centavos") != premio1 || num(cw, "saldo_centavos") != 2000 || cw["saque_bloqueado"] != false {
		t.Errorf("carteira: %v", cw)
	}
	pl := esperar(t, api(t, "GET", "/api/carteira/pendentes", p.us[0].Token, nil), 200).data()
	itens := pl["itens"].([]any)
	if len(itens) != 1 || itens[0].(map[string]any)["status"] != "a_liberar" || num(pl, "a_liberar_centavos") != premio1 {
		t.Errorf("pendentes do atleta: %v", pl)
	}
	if _, vaza := itens[0].(map[string]any)["id_denuncia_retencao"]; vaza {
		t.Error("retenção é dado do admin")
	}
	if detalhe(t, p.us[0], p.id)["pagamentos_liberam_em"] == nil {
		t.Error("detalhe sem pagamentos_liberam_em")
	}
	if dv := divergencia(t, a); dv != div0 {
		t.Errorf("conferência com pendentes: divergência %d -> %d", div0, dv)
	}
	esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/pagamentos-pendentes?id_desafio=%d&status=a_liberar", p.id), a, nil), 200)

	// janela não venceu: o job não libera
	liberarPagamentos(t)
	if s := somaPendentes(t, p.id); s[models.PendenteLiberado] != 0 || saldo(t, p.us[0]) != 2000 {
		t.Fatalf("liberou antes da janela: %v", s)
	}
	// a janela "passa"
	db.Exec("UPDATE pagamentos_pendentes SET liberar_em = NOW(3) - INTERVAL 1 SECOND WHERE id_desafio = ?", p.id)
	academiaAntes, instrutorAntes := saldo(t, p.c.A), saldo(t, p.c.I)
	liberarPagamentos(t)
	for _, x := range pendentesDo(t, p.id) {
		if x.Status != models.PendenteLiberado || x.LiberadoEm == nil || (x.IDUsuario != nil) != (x.IDTransacao != nil) {
			t.Errorf("pendente %d depois do job: %+v", x.ID, x)
		}
	}
	if s := saldo(t, p.us[0]); s != 2000+premio1 {
		t.Errorf("1º lugar depois da liberação: %d", s)
	}
	if s := saldo(t, p.c.I); s != instrutorAntes+pendenteDe(t, p.id, p.c.I.ID) {
		t.Errorf("instrutor: %d", s)
	}
	if s := saldo(t, p.c.A); s != academiaAntes+pendenteDe(t, p.id, p.c.A.ID) {
		t.Errorf("academia: %d", s)
	}
	if n := eventosDoDesafio(t, p.id, "pagamento_liberado"); n != int64(len(ps)) {
		t.Errorf("eventos pagamento_liberado: %d de %d", n, len(ps))
	}
	if contarNotifDB(t, p.us[0].ID, models.NotifPagamentoLiberado) != 1 {
		t.Error("notificação pagamento_liberado")
	}
	// idempotente: rodar de novo não credita nada
	var nLedger int64
	db.Model(&models.Transacao{}).Where("id_desafio = ?", p.id).Count(&nLedger)
	liberarPagamentos(t)
	var nLedger2 int64
	db.Model(&models.Transacao{}).Where("id_desafio = ?", p.id).Count(&nLedger2)
	if nLedger2 != nLedger || saldo(t, p.us[0]) != 2000+premio1 {
		t.Errorf("liberação repetida mexeu no ledger: %d -> %d", nLedger, nLedger2)
	}
	invariantes(t, p.id)
	ledgerConfere(t, todos(p)...)
	if detalhe(t, p.us[0], p.id)["pagamentos_liberam_em"] != nil {
		t.Error("liberado: pagamentos_liberam_em deveria sumir")
	}
	if dv := divergencia(t, a); dv != div0 {
		t.Errorf("conferência depois da liberação: divergência %d -> %d", div0, dv)
	}
}

// Denúncia de desafio aberta × job de liberação, em paralelo: os pendentes terminam TODOS retidos
// (a denúncia venceu) ou TODOS liberados (o job venceu) — nunca metade; o dinheiro fecha nos dois casos.
func TestPodio_RetencaoXLiberacaoConcorrentes(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	div0 := divergencia(t, a)
	srv := servidorMod(0)
	retidos, liberados := 0, 0
	for rodada := 0; rodada < 5; rodada++ {
		p := lancarPodioMod(t, srv, 3)
		var rd resp
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			rd = denunciar(t, p.us[1], corpoDenuncia(p.c.I.ID, models.DenunciaManipulacaoPodio, p.id))
		}()
		go func() { defer wg.Done(); liberarPagamentos(t) }()
		wg.Wait()
		esperar(t, rd, 201)
		s := somaPendentes(t, p.id)
		total := s[models.PendenteRetido] + s[models.PendenteLiberado] + s[models.PendenteALiberar]
		switch {
		case s[models.PendenteRetido] == total:
			retidos++
		case s[models.PendenteLiberado] == total:
			liberados++
		default:
			t.Errorf("rodada %d: pendentes misturados %v", rodada, s)
		}
		conferirPendentesDoPodio(t, p.id)
		ledgerConfere(t, todos(p)...)
	}
	t.Logf("retenção venceu %d vez(es), liberação %d", retidos, liberados)

	// Determinístico: denúncia antes do job retém; o job não libera retido; julgada improcedente, volta a
	// a_liberar e o próximo ciclo libera.
	p := lancarPodioMod(t, srv, 3)
	den := idDe(esperar(t, denunciar(t, p.us[1], corpoDenuncia(p.c.I.ID, models.DenunciaManipulacaoPodio, p.id)), 201))
	liberarPagamentos(t)
	liberarPagamentos(t)
	if s := somaPendentes(t, p.id); s[models.PendenteLiberado] != 0 || s[models.PendenteALiberar] != 0 {
		t.Fatalf("retido não pode ser liberado pelo job: %v", s)
	}
	if detalhe(t, p.us[0], p.id)["pagamentos_retidos"] != true {
		t.Error("detalhe: pagamentos_retidos")
	}
	if num(carteira(t, p.us[0]), "retido_centavos") != pendenteDe(t, p.id, p.us[0].ID) {
		t.Error("carteira: retido_centavos")
	}
	esperar(t, adm(t, "POST", fmt.Sprintf("/api/admin/denuncias/%d/decidir", den), a, map[string]any{
		"status": "improcedente", "decisao_texto": "Sem provas de manipulação", "motivo": "análise concluída"}), 200)
	liberarPagamentos(t)
	if s := somaPendentes(t, p.id); s[models.PendenteRetido] != 0 || s[models.PendenteALiberar] != 0 {
		t.Errorf("depois de julgada improcedente: %v", s)
	}
	invariantes(t, p.id)
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, a); dv != div0 {
		t.Errorf("conferência: divergência %d -> %d", div0, dv)
	}
}

// ─── Anulação ────────────────────────────────────────────────────────────────

// Anulação (devolver_inscricoes) × liberação em paralelo: em qualquer ordem, no fim todos voltam ao que
// tinham antes do desafio (entrada integral e reserva devolvidas, pódio estornado, taxa estornada se
// liberada), nada fica a liberar e a conferência fecha.
func TestAnulacao_XLiberacaoConcorrentes(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	srv := servidorMod(0)
	for rodada := 0; rodada < 4; rodada++ {
		p := lancarPodioMod(t, srv, 4)
		var ra resp
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			ra = anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0)
		}()
		go func() { defer wg.Done(); liberarPagamentos(t) }()
		wg.Wait()
		esperar(t, ra, 200)
		s := somaPendentes(t, p.id)
		if s[models.PendenteALiberar] != 0 || s[models.PendenteRetido] != 0 {
			t.Errorf("rodada %d: sobrou pendente vivo %v", rodada, s)
		}
		liberarPagamentos(t) // nada mais a fazer
		for _, u := range p.us {
			if v := saldo(t, u); v != 3000 {
				t.Errorf("rodada %d: atleta %d com saldo %d (esperava 3000)", rodada, u.ID, v)
			}
		}
		if v := saldo(t, p.c.A); v != 20000 {
			t.Errorf("rodada %d: academia com %d (esperava 20000)", rodada, v)
		}
		if v := saldo(t, p.c.I); v != 0 || debitoAberto(t, p.c.I.ID) != 0 {
			t.Errorf("rodada %d: instrutor com %d / débito %d", rodada, v, debitoAberto(t, p.c.I.ID))
		}
		var receita int64
		db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").Where("id_desafio = ?", p.id).Scan(&receita)
		if receita != 0 {
			t.Errorf("rodada %d: receita líquida do desafio %d (taxa liberada precisa ser estornada)", rodada, receita)
		}
		ledgerConfere(t, todos(p)...)
	}
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência: divergência %d -> %d", div0, dv)
	}
}

func TestAnulacao_RelancarPodio(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	comum := novoAdminDB(t, models.PapelAdmin)
	div0 := divergencia(t, sa)
	p := lancarPodioMod(t, servidorMod(1), 4) // janela de 1h: nada liberado
	den := idDe(esperar(t, denunciar(t, p.us[1], corpoDenuncia(p.c.I.ID, models.DenunciaManipulacaoPodio, p.id)), 201))
	if s := somaPendentes(t, p.id); s[models.PendenteRetido] == 0 || s[models.PendenteALiberar] != 0 {
		t.Fatalf("denúncia deveria reter: %v", s)
	}
	pontosPodio := pontos(t, p.us[0])
	n := len(pendentesDo(t, p.id))

	// só superadmin; validações
	esperar(t, anular(t, comum, p.id, models.AnulacaoRelancarPodio, p.c.I.ID, den), 403)
	esperar(t, anular(t, sa, p.id, "xpto", p.c.I.ID, den), 400)
	esperar(t, anular(t, sa, 999999999, models.AnulacaoRelancarPodio, p.c.I.ID, 0), 404)
	esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.us[2].ID, den), 409) // denúncia é contra outro

	r := esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.c.I.ID, den), 200).data()
	if r["status_desafio"] != "em_andamento" || num(r, "pendentes_cancelados") != int64(n) || num(r, "estornado_centavos") != 0 {
		t.Errorf("resposta da anulação: %v", r)
	}
	esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.c.I.ID, 0), 409) // já anulado
	var dn models.Denuncia
	db.First(&dn, den)
	if dn.Status != models.DenunciaProcedente {
		t.Errorf("denúncia de origem deveria virar procedente: %s", dn.Status)
	}
	var pun models.Punicao
	if err := db.Where("id_desafio = ? AND tipo = ?", p.id, models.PunicaoAnulacaoResultado).First(&pun).Error; err != nil ||
		pun.IDUsuario != p.c.I.ID || pun.AplicadaPorAdmin == nil || pun.IDDenuncia == nil || *pun.IDDenuncia != den {
		t.Errorf("punição anulacao_resultado: %+v (%v)", pun, err)
	}
	if contarAuditoria(t, "desafio_anular_resultado", "desafio", p.id) != 1 || eventosDoDesafio(t, p.id, "resultado_anulado") != 1 {
		t.Error("auditoria/evento da anulação")
	}
	if s := somaPendentes(t, p.id); s[models.PendenteCancelado] == 0 || s[models.PendenteRetido]+s[models.PendenteALiberar]+s[models.PendenteLiberado] != 0 {
		t.Errorf("pendentes depois da anulação: %v", s)
	}
	if v := pontos(t, p.us[0]); v != p.pontosAntes || pontosPodio <= p.pontosAntes {
		t.Errorf("pontos do pódio não desfeitos: antes %d, no pódio %d, agora %d", p.pontosAntes, pontosPodio, v)
	}
	da := detalhe(t, p.c.A, p.id)
	if da["status"] != "em_andamento" || da["resultado_anulado"] != true || da["aguardando_relancamento"] != true ||
		da["pode_lancar_podio"] != true || len(da["podio"].([]any)) != 0 {
		t.Errorf("detalhe para a academia: %v", da)
	}
	if di := detalhe(t, p.c.I, p.id); di["pode_lancar_podio"] != false {
		t.Errorf("instrutor não relança: %v", di["pode_lancar_podio"])
	}
	for _, u := range p.us {
		if v := saldo(t, u); v != 2000 {
			t.Errorf("nada foi creditado nem estornado: atleta com %d", v)
		}
	}
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência depois da anulação (custódia de volta): divergência %d -> %d", div0, dv)
	}

	// o job de expiração não cancela na carência contada da anulação, mesmo com data_evento vencida
	db.Exec("UPDATE desafios SET data_evento = NOW() - INTERVAL 10 DAY WHERE id = ?", p.id)
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	for _, tf := range tarefas {
		if tf.Nome == "expirar-desafios" {
			if err := tf.Executar(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if st := statusDesafio(t, p.c.A, p.id); st != "em_andamento" {
		t.Fatalf("desafio aguardando relançamento não pode expirar na carência: %s", st)
	}

	// só a academia dona relança
	corpo := podioBody(entradaPodio(p.us[2], "9:58"), entradaPodio(p.us[3], "10:01"))
	esperar(t, chamar(t, servidorMod(0), "POST", fmt.Sprintf("/api/desafios/%d/podio", p.id), p.c.I.Token, corpo), 403)
	d := esperar(t, chamar(t, servidorMod(0), "POST", fmt.Sprintf("/api/desafios/%d/podio", p.id), p.c.A.Token, corpo), 200).data()
	if d["status"] != "encerrado" || d["resultado_anulado"] != false || len(d["podio"].([]any)) != 2 {
		t.Errorf("relançamento: %v", d)
	}
	conferirPendentesDoPodio(t, p.id)
	liberarPagamentos(t)
	novo1 := pendenteDe(t, p.id, p.us[2].ID)
	if v := saldo(t, p.us[2]); novo1 <= 0 || v != 2000+novo1 {
		t.Errorf("novo 1º lugar: saldo %d, prêmio %d", v, novo1)
	}
	if v := saldo(t, p.us[0]); v != 2000 {
		t.Errorf("o 1º do pódio anulado não recebe nada: %d", v)
	}
	invariantes(t, p.id)
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência depois do relançamento: divergência %d -> %d", div0, dv)
	}

	// com pagamento liberado não há mais relançamento (Q3)
	esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.c.I.ID, 0), 409)
	esperar(t, anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0), 200)
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência depois da 2ª anulação: divergência %d -> %d", div0, dv)
	}
}

// Anulado para relançar e ninguém relança: depois da carência (contada da anulação) o job cancela e
// devolve tudo.
func TestAnulacao_RelancarExpiraAposCarencia(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	p := lancarPodioMod(t, servidorMod(1), 3)
	esperar(t, anular(t, sa, p.id, models.AnulacaoRelancarPodio, p.c.I.ID, 0), 200)
	db.Exec("UPDATE desafios SET resultado_anulado_em = NOW(3) - INTERVAL 3 DAY WHERE id = ?", p.id)
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	for _, tf := range tarefas {
		if tf.Nome == "expirar-desafios" {
			if err := tf.Executar(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if st := statusDesafio(t, p.c.A, p.id); st != "cancelado" {
		t.Fatalf("depois da carência o job cancela: %s", st)
	}
	for _, u := range p.us {
		if v := saldo(t, u); v != 3000 {
			t.Errorf("atleta reembolsado: %d", v)
		}
	}
	if v := saldo(t, p.c.A); v != 20000 {
		t.Errorf("reserva devolvida: %d", v)
	}
	invariantes(t, p.id)
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência: divergência %d -> %d", div0, dv)
	}
}

func TestAnulacao_DevolverInscricoesSemLiberacao(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	p := lancarPodioMod(t, servidorMod(1), 4)
	r := esperar(t, anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0), 200).data()
	if r["status_desafio"] != "encerrado" || num(r, "inscricoes_devolvidas_centavos") != 4000 ||
		num(r, "reserva_devolvida_centavos") != 3000 || num(r, "taxa_estornada_centavos") != 0 || num(r, "estornado_centavos") != 0 {
		t.Errorf("resposta: %v", r)
	}
	for _, u := range p.us {
		if v := saldo(t, u); v != 3000 {
			t.Errorf("entrada INTEGRAL devolvida: atleta com %d", v)
		}
	}
	if v := saldo(t, p.c.A); v != 20000 {
		t.Errorf("academia: %d", v)
	}
	if v := pontos(t, p.us[0]); v != p.pontosAntes {
		t.Errorf("pontos: %d (antes %d)", v, p.pontosAntes)
	}
	var estornos int64
	db.Model(&models.PlataformaReceita{}).Where("id_desafio = ? AND origem = ?", p.id, models.ReceitaEstornoTaxa).Count(&estornos)
	if estornos != 0 || receitaDoDesafio(t, p.id) != 0 {
		t.Error("taxa não liberada é só cancelada (sem estorno_taxa)")
	}
	d := detalhe(t, p.us[0], p.id)
	if d["status"] != "encerrado" || d["resultado_anulado"] != true || d["resultado_anulacao_modo"] != "devolver_inscricoes" || d["pode_avaliar"] == nil {
		t.Errorf("detalhe: %v", d)
	}
	liberarPagamentos(t)
	if v := saldo(t, p.us[0]); v != 3000 {
		t.Errorf("cancelado não é liberado: %d", v)
	}
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência: divergência %d -> %d", div0, dv)
	}
}

// devolver_inscricoes com tudo já liberado e parte sacada: estorno de taxa (Q6), estorno do saldo e
// débito do que faltou; débito bloqueia o saque (409) e é abatido por depósito e por liberação de pódio.
func TestAnulacao_DevolverComTaxaLiberadaDebitoEAbatimentos(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	srv := servidorMod(0)
	p := lancarPodioMod(t, srv, 4)
	liberarPagamentos(t)
	premio1, pInst := pendenteDe(t, p.id, p.us[0].ID), pendenteDe(t, p.id, p.c.I.ID)
	taxa := receitaDoDesafio(t, p.id)
	if taxa <= 0 || premio1 <= 1100 || pInst < 500 {
		t.Fatalf("cenário: taxa %d prêmio %d instrutor %d", taxa, premio1, pInst)
	}
	// 1º lugar saca quase tudo (fica com 100); o instrutor saca tudo
	esperar(t, sacar(t, p.us[0], 2000+premio1-100), 201)
	esperar(t, sacar(t, p.c.I, pInst), 201)

	r := esperar(t, anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0), 200).data()
	if num(r, "taxa_estornada_centavos") != taxa || num(r, "inscricoes_devolvidas_centavos") != 4000 {
		t.Errorf("resposta: %v", r)
	}
	var estorno int64
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND origem = ?", p.id, models.ReceitaEstornoTaxa).Scan(&estorno)
	if estorno != -taxa {
		t.Errorf("estorno_taxa: %d (taxa %d)", estorno, taxa)
	}
	// 1º: tinha 100, recebeu a entrada (1000) de volta e perdeu o que dava; o resto vira débito
	deb1 := premio1 - 1100
	if v := saldo(t, p.us[0]); v != 0 || debitoAberto(t, p.us[0].ID) != deb1 {
		t.Errorf("1º lugar: saldo %d, débito %d (esperava 0 e %d)", v, debitoAberto(t, p.us[0].ID), deb1)
	}
	if v := saldo(t, p.c.I); v != 0 || debitoAberto(t, p.c.I.ID) != pInst {
		t.Errorf("instrutor: saldo %d, débito %d (esperava 0 e %d)", v, debitoAberto(t, p.c.I.ID), pInst)
	}
	if n, soma := ledgerDe(t, p.us[0], models.TipoEstornoAnulacao); n != 1 || soma != -1100 {
		t.Errorf("estorno_anulacao do 1º: n=%d soma=%d", n, soma)
	}
	if contarNotifDB(t, p.us[0].ID, models.NotifDebitoCriado) != 1 {
		t.Error("notificação debito_criado")
	}
	cw := carteira(t, p.us[0])
	if num(cw, "debito_aberto_centavos") != deb1 || cw["saque_bloqueado"] != true {
		t.Errorf("carteira com débito: %v", cw)
	}
	cd := esperar(t, api(t, "GET", "/api/carteira/debitos", p.us[0].Token, nil), 200).data()
	if num(cd, "aberto_centavos") != deb1 || len(cd["itens"].([]any)) != 1 {
		t.Errorf("GET /carteira/debitos: %v", cd)
	}
	ad := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/debitos?aberto=true&id_usuario=%d", p.us[0].ID), sa, nil), 200).data()
	if num(ad, "total") != 1 {
		t.Errorf("admin débitos: %v", ad)
	}
	dash := esperar(t, adm(t, "GET", "/api/admin/dashboard", sa, nil), 200).data()
	if num(sub(t, dash, "pendencias"), "debitos_abertos") < 2 {
		t.Errorf("dashboard: pendências de débito %v", dash["pendencias"])
	}
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência com débitos e estorno de taxa: divergência %d -> %d", div0, dv)
	}
	ledgerConfere(t, todos(p)...)

	// saque com débito aberto: 409 debito_aberto (mesmo com saldo)
	rs := sacar(t, p.us[0], 500)
	esperar(t, rs, 409)
	if rs.Body["codigo"] != "debito_aberto" {
		t.Errorf("409 sem codigo debito_aberto: %v", rs.Body)
	}

	// abatimento por depósito: parcial e depois quitação
	depositar(t, p.us[0], 1000)
	if v := saldo(t, p.us[0]); v != 0 || debitoAberto(t, p.us[0].ID) != deb1-1000 {
		t.Errorf("abatimento parcial: saldo %d, débito %d", v, debitoAberto(t, p.us[0].ID))
	}
	if contarNotifDB(t, p.us[0].ID, models.NotifDebitoAbatido) != 1 {
		t.Error("notificação debito_abatido")
	}
	depositar(t, p.us[0], 5000)
	if v := saldo(t, p.us[0]); v != 5000-(deb1-1000) || debitoAberto(t, p.us[0].ID) != 0 {
		t.Errorf("quitação: saldo %d, débito %d", v, debitoAberto(t, p.us[0].ID))
	}
	if contarNotifDB(t, p.us[0].ID, models.NotifDebitoQuitado) != 1 {
		t.Error("notificação debito_quitado")
	}
	if n, soma := ledgerDe(t, p.us[0], models.TipoAbatimentoDebito); n != 2 || soma != -deb1 {
		t.Errorf("abatimento_debito: n=%d soma=%d (débito %d)", n, soma, deb1)
	}
	var evs int64
	db.Model(&models.ModeracaoEvento{}).Where("entidade = 'debito' AND acao IN ('debito_abatido','debito_quitado') AND id_desafio = ?", p.id).Count(&evs)
	if evs < 2 {
		t.Errorf("eventos de abatimento: %d", evs)
	}
	esperar(t, sacar(t, p.us[0], 500), 201)
	ledgerConfere(t, todos(p)...)

	// abatimento por liberação de pódio: o instrutor devedor recebe o pagamento de outro desafio
	p2 := lancarPodioModCom(t, srv, p.c, 3)
	pInst2 := pendenteDe(t, p2.id, p.c.I.ID)
	liberarPagamentos(t)
	esperadoDeb, esperadoSaldo := max(pInst-pInst2, 0), max(pInst2-pInst, 0)
	if v := saldo(t, p.c.I); v != esperadoSaldo || debitoAberto(t, p.c.I.ID) != esperadoDeb {
		t.Errorf("instrutor depois da liberação: saldo %d débito %d (esperava %d e %d)", v, debitoAberto(t, p.c.I.ID), esperadoSaldo, esperadoDeb)
	}
	if n, _ := ledgerDe(t, p.c.I, models.TipoAbatimentoDebito); n != 1 {
		t.Errorf("abatimento na liberação: %d linhas", n)
	}
	invariantes(t, p2.id)
	ledgerConfere(t, append(todos(p), p2.us...)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência final: divergência %d -> %d", div0, dv)
	}
}

// Saque × anulação em paralelo (débito × saque): a checagem do débito acontece dentro da transação do
// saque com o usuário travado. Saque primeiro -> anulação gera débito; anulação primeiro -> saque recusado
// por saldo. Nunca saldo negativo, nunca dinheiro criado; com débito, o próximo saque é 409.
func TestDebito_SaqueXAnulacaoConcorrentes(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	srv := servidorMod(0)
	for rodada := 0; rodada < 4; rodada++ {
		p := lancarPodioMod(t, srv, 3)
		liberarPagamentos(t)
		premio1 := pendenteDe(t, p.id, p.us[0].ID)
		tudo := saldo(t, p.us[0]) // 2000 + prêmio
		var rs, ra resp
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); rs = sacar(t, p.us[0], tudo) }()
		go func() { defer wg.Done(); ra = anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0) }()
		wg.Wait()
		esperar(t, ra, 200)
		switch rs.Status {
		case 201: // saque venceu: sobrou só a entrada devolvida, que o estorno levou; o resto é débito
			if v, d := saldo(t, p.us[0]), debitoAberto(t, p.us[0].ID); v != 0 || d != premio1-1000 {
				t.Errorf("rodada %d (saque antes): saldo %d débito %d (prêmio %d)", rodada, v, d, premio1)
			}
			r2 := sacar(t, p.us[0], 500)
			if r2.Status != 409 || r2.Body["codigo"] != "debito_aberto" {
				t.Errorf("rodada %d: saque com débito deveria ser 409 debito_aberto: %d %v", rodada, r2.Status, r2.Body)
			}
		case 422: // anulação venceu: prêmio estornado, entrada devolvida, sem débito
			if v, d := saldo(t, p.us[0]), debitoAberto(t, p.us[0].ID); v != 3000 || d != 0 {
				t.Errorf("rodada %d (anulação antes): saldo %d débito %d", rodada, v, d)
			}
		default:
			t.Errorf("rodada %d: saque %d %v", rodada, rs.Status, rs.Body)
		}
		ledgerConfere(t, todos(p)...)
	}
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência: divergência %d -> %d", div0, dv)
	}
}

// Débito aberto × depósito que quita × saque, em paralelo: o saque só passa se enxergar o débito quitado;
// em qualquer ordem o saldo fecha com o ledger e nunca fica negativo.
func TestDebito_DepositoQuitaXSaqueConcorrentes(t *testing.T) {
	sa := novoAdminDB(t, models.PapelSuperadmin)
	div0 := divergencia(t, sa)
	p := lancarPodioMod(t, servidorMod(0), 3)
	liberarPagamentos(t)
	premio1 := pendenteDe(t, p.id, p.us[0].ID)
	esperar(t, sacar(t, p.us[0], 2000+premio1), 201)
	esperar(t, anular(t, sa, p.id, models.AnulacaoDevolverInscricoes, p.c.I.ID, 0), 200)
	deb := debitoAberto(t, p.us[0].ID)
	if deb != premio1-1000 {
		t.Fatalf("débito %d", deb)
	}
	// depósito pendente que vai quitar e sobrar 2000
	r := esperar(t, api(t, "POST", "/api/pagamento/pix", p.us[0].Token, map[string]any{"valor_centavos": deb + 2000, "cpf": cpfValido()}), 200)
	pid := r.data()["asaas_payment_id"].(string)
	asaas.pagar(pid)
	var rs resp
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); esperar(t, api(t, "GET", "/api/pagamento/pix/"+pid, p.us[0].Token, nil), 200) }()
	go func() { defer wg.Done(); rs = sacar(t, p.us[0], 1500) }()
	wg.Wait()
	switch rs.Status {
	case 201:
		if v := saldo(t, p.us[0]); v != 500 {
			t.Errorf("saque depois da quitação: saldo %d", v)
		}
	case 409:
		if v := saldo(t, p.us[0]); v != 2000 {
			t.Errorf("saque antes da quitação: saldo %d", v)
		}
	default:
		t.Errorf("saque: %d %v", rs.Status, rs.Body)
	}
	if debitoAberto(t, p.us[0].ID) != 0 {
		t.Error("débito deveria estar quitado")
	}
	ledgerConfere(t, todos(p)...)
	if dv := divergencia(t, sa); dv != div0 {
		t.Errorf("conferência: divergência %d -> %d", div0, dv)
	}
}
