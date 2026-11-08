//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gymScore-backend/internal/models"
)

// ─── Ranking ─────────────────────────────────────────────────────────────────

func TestRanking_SoVoceEAmigosComPesoDoCheckin(t *testing.T) {
	a, b, c := novoUsuario(t), novoUsuario(t), novoUsuario(t) // c não é amigo
	amigos(t, a, b)
	for i := 0; i < 2; i++ {
		esperar(t, api(t, "POST", "/api/treinos/concluir", a.Token, map[string]any{"grupo_muscular": "peito"}), 200)
	}
	for i := 0; i < 10; i++ {
		esperar(t, api(t, "POST", "/api/treinos/concluir", c.Token, map[string]any{"grupo_muscular": "peito"}), 200)
	}
	// b tem 1 check-in de desafio histórico (3 pontos). O check-in do atleta saiu da API no Sprint 6,
	// mas a tabela continua alimentando o ranking: semeia a linha direto no banco.
	depositar(t, b, 3000)
	cen := novoCenario(t)
	id := criarDesafio(t, cen, 1000, 2)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	if err := db.Exec("INSERT INTO desafio_checkins (id_desafio, id_usuario, data, criado_em) VALUES (?, ?, ?, ?)",
		id, b.ID, time.Now().Format("2006-01-02"), time.Now()).Error; err != nil {
		t.Fatal(err)
	}

	r := esperar(t, api(t, "GET", "/api/ranking/amigos", a.Token, nil), 200).Body["data"].([]any)
	if len(r) != 2 {
		t.Fatalf("ranking deve ter só eu + 1 amigo: %v", r)
	}
	p0, p1 := r[0].(map[string]any), r[1].(map[string]any)
	if uint(p0["id"].(float64)) != b.ID || p0["pontos"].(float64) != 3 || p0["checkins"].(float64) != 1 || p0["posicao"].(float64) != 1 {
		t.Errorf("1º lugar deveria ser b com 3 pontos: %v", p0)
	}
	if uint(p1["id"].(float64)) != a.ID || p1["pontos"].(float64) != 2 || p1["exercicios"].(float64) != 2 || p1["voce"] != true {
		t.Errorf("2º lugar deveria ser a (eu) com 2 pontos: %v", p1)
	}
	// c só vê a si mesmo
	rc := esperar(t, api(t, "GET", "/api/ranking/amigos", c.Token, nil), 200).Body["data"].([]any)
	if len(rc) != 1 || rc[0].(map[string]any)["pontos"].(float64) != 10 {
		t.Errorf("c deveria ver só ele com 10 pontos: %v", rc)
	}
}

func TestRanking_JanelaDeDiasEValidacao(t *testing.T) {
	a := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/treinos/concluir", a.Token, map[string]any{"grupo_muscular": "costas"}), 200)
	db.Exec("INSERT INTO treino_conclusoes (id_usuario, grupo_muscular, concluido_em) VALUES (?, 'pernas', ?)", a.ID, time.Now().AddDate(0, 0, -40))

	pontos := func(q string) float64 {
		r := esperar(t, api(t, "GET", "/api/ranking/amigos"+q, a.Token, nil), 200).Body["data"].([]any)
		return r[0].(map[string]any)["pontos"].(float64)
	}
	if p := pontos(""); p != 1 {
		t.Errorf("padrão (30 dias) deveria contar 1, contou %v", p)
	}
	if p := pontos("?dias=60"); p != 2 {
		t.Errorf("60 dias deveria contar 2, contou %v", p)
	}
	if p := pontos("?dias=7"); p != 1 {
		t.Errorf("7 dias deveria contar 1, contou %v", p)
	}
	esperar(t, api(t, "GET", "/api/ranking/amigos?dias=366", a.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/ranking/amigos?dias=-1", a.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/ranking/amigos", "", nil), 401)
}

// ─── Notificações ────────────────────────────────────────────────────────────

func listaNotif(t *testing.T, u *user) []map[string]any {
	t.Helper()
	r := esperar(t, api(t, "GET", "/api/notificacoes?limite=100", u.Token, nil), 200)
	var out []map[string]any
	for _, it := range r.data()["itens"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

// notifDoTipo devolve a notificação mais recente do tipo (o pódio agora gera também pagamento_liberado).
func notifDoTipo(t *testing.T, u *user, tipo string) map[string]any {
	t.Helper()
	for _, n := range listaNotif(t, u) {
		if n["tipo"] == tipo {
			return n
		}
	}
	t.Fatalf("usuário %d sem notificação %s", u.ID, tipo)
	return nil
}

func contarNotif(t *testing.T, u *user, tipo string) int {
	t.Helper()
	n := 0
	for _, it := range listaNotif(t, u) {
		if it["tipo"] == tipo {
			n++
		}
	}
	return n
}

func TestNotif_EventosDeAmizade(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)
	if contarNotif(t, b, models.NotifAmizadeRecebida) != 1 || contarNotif(t, a, models.NotifAmizadeRecebida) != 0 {
		t.Fatal("só o destinatário é notificado da solicitação")
	}
	n := listaNotif(t, b)[0]
	if !strings.Contains(n["mensagem"].(string), a.Nome) || n["link"] != "/amigos" {
		t.Errorf("mensagem/link inesperados: %v", n)
	}
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 200)
	if contarNotif(t, a, models.NotifAmizadeAceita) != 1 {
		t.Error("o remetente deve ser notificado do aceite")
	}
	// aceitar de novo (404) não duplica
	esperar(t, api(t, "POST", "/api/amigos/aceitar", b.Token, map[string]any{"id_amigo": a.ID}), 404)
	if contarNotif(t, a, models.NotifAmizadeAceita) != 1 {
		t.Error("não pode duplicar notificação")
	}
}

func TestNotif_CicloDoDesafio(t *testing.T) {
	c := novoCenario(t)
	us := atletas(t, 3, 5000)
	a, b, d := us[0], us[1], us[2]
	amigo := novoUsuario(t)
	amigos(t, a, amigo)
	id := criarDesafioPodio(t, c, 1000, 4, 2)

	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 200)
	if contarNotif(t, amigo, models.NotifConviteDesafio) != 1 {
		t.Fatal("amigo convidado deveria receber o convite")
	}
	conv := listaNotif(t, amigo)[0]
	if !strings.Contains(conv["link"].(string), fmt.Sprintf("id=%d", id)) {
		t.Errorf("link do convite deveria apontar para o desafio: %v", conv["link"])
	}
	if contarNotif(t, c.I, models.NotifInstrutorDesignado) != 1 {
		t.Error("instrutor designado deveria ser notificado na criação")
	}

	inscrever(t, id, a, b, d)
	for _, u := range us {
		if contarNotif(t, u, models.NotifInscricaoConfirmada) != 1 {
			t.Errorf("usuário %d deveria receber a confirmação de inscrição", u.ID)
		}
	}
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, u := range us {
		if contarNotif(t, u, models.NotifDesafioIniciado) != 1 {
			t.Errorf("usuário %d deveria ser notificado do início", u.ID)
		}
	}
	esperar(t, presencaPorCodigo(t, c.I, id, a), 200)
	esperar(t, presencaPorCodigo(t, c.I, id, b), 200)
	if contarNotif(t, a, models.NotifPresencaConfirmada) != 1 || contarNotif(t, d, models.NotifPresencaConfirmada) != 0 {
		t.Error("só quem teve presença marcada recebe a confirmação")
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(b, "12 reps"), entradaPodio(a, "10 reps"))), 200)
	for _, u := range us {
		if contarNotif(t, u, models.NotifResultado) != 1 {
			t.Errorf("usuário %d deveria receber o resultado exatamente uma vez", u.ID)
		}
	}
	vencedor := notifDoTipo(t, b, models.NotifResultado)
	// pote 3000, pódio de 2: 70% = 21,00
	if !strings.Contains(vencedor["mensagem"].(string), "1º lugar") || !strings.Contains(vencedor["mensagem"].(string), "21,00") {
		t.Errorf("mensagem do vencedor: %v", vencedor["mensagem"])
	}
	// janela de contestação: o resultado avisa que o prêmio será liberado; a liberação avisa de novo
	if !strings.Contains(vencedor["mensagem"].(string), "receberá") || contarNotif(t, b, models.NotifPagamentoLiberado) != 1 {
		t.Errorf("aviso de prêmio a liberar / liberado: %v", vencedor["mensagem"])
	}
	// relançar (409) não notifica de novo
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(b, "1"), entradaPodio(a, "2"))), 409)
	if contarNotif(t, b, models.NotifResultado) != 1 {
		t.Error("relançamento não pode duplicar notificações")
	}
}

func TestNotif_CancelamentoEDeposito(t *testing.T) {
	c := novoCenario(t)
	a, b := usuarioComSaldo(t, 3000), usuarioComSaldo(t, 3000)
	if contarNotif(t, a, models.NotifDeposito) != 1 {
		t.Fatal("depósito confirmado deveria notificar uma vez")
	}
	// consultar o mesmo pagamento de novo não duplica
	var pid string
	db.Model(&models.Transacao{}).Select("asaas_payment_id").Where("id_usuario = ? AND tipo = 'deposito'", a.ID).Scan(&pid)
	esperar(t, api(t, "GET", "/api/pagamento/pix/"+pid, a.Token, nil), 200)
	esperar(t, webhook(t, "token-webhook-de-teste", "PAYMENT_RECEIVED", pid), 200)
	if contarNotif(t, a, models.NotifDeposito) != 1 {
		t.Error("confirmação repetida não pode duplicar a notificação")
	}
	id := criarDesafio(t, c, 1000, 3)
	inscrever(t, id, a, b)
	esperar(t, acao(t, c.A, id, "cancelar", nil), 200)
	if contarNotif(t, b, models.NotifDesafioCancel) != 1 || contarNotif(t, a, models.NotifDesafioCancel) != 1 {
		t.Error("cancelamento notifica os atletas inscritos")
	}
	if !strings.Contains(listaNotif(t, b)[0]["mensagem"].(string), "reembolsada") {
		t.Errorf("cancelamento de desafio pago cita o reembolso: %v", listaNotif(t, b)[0])
	}
}

func TestNotif_EndpointsEIsolamento(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	for i := 0; i < 3; i++ { // 3 solicitações de amigos diferentes para b
		x := novoUsuario(t)
		esperar(t, api(t, "POST", "/api/amigos/adicionar", x.Token, map[string]any{"id_amigo": b.ID}), 200)
	}
	esperar(t, api(t, "POST", "/api/amigos/adicionar", a.Token, map[string]any{"id_amigo": b.ID}), 200)

	cont := func(u *user) float64 {
		return esperar(t, api(t, "GET", "/api/notificacoes/nao-lidas", u.Token, nil), 200).data()["nao_lidas"].(float64)
	}
	if cont(b) != 4 || cont(a) != 0 {
		t.Fatalf("contadores errados: b=%v a=%v", cont(b), cont(a))
	}
	lista := listaNotif(t, b)
	primeira := uint(lista[0]["id"].(float64))

	// uma notificação alheia não pode ser lida por outro usuário nem revelar existência
	esperar(t, api(t, "POST", fmt.Sprintf("/api/notificacoes/%d/lida", primeira), a.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/notificacoes/99999999/lida", b.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/notificacoes/abc/lida", b.Token, nil), 400)
	if cont(b) != 4 {
		t.Fatal("tentativa de outro usuário não pode marcar como lida")
	}
	esperar(t, api(t, "POST", fmt.Sprintf("/api/notificacoes/%d/lida", primeira), b.Token, nil), 200)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/notificacoes/%d/lida", primeira), b.Token, nil), 200) // idempotente
	if cont(b) != 3 {
		t.Errorf("deveria restar 3 não lidas, restam %v", cont(b))
	}
	// não lidas primeiro: a lida vai para o fim
	lista = listaNotif(t, b)
	if lista[len(lista)-1]["lida"] != true || lista[0]["lida"] != false {
		t.Errorf("ordem deveria ser não lidas primeiro: %v", lista)
	}
	// paginação
	p := esperar(t, api(t, "GET", "/api/notificacoes?limite=2&pagina=2", b.Token, nil), 200).data()
	if len(p["itens"].([]any)) != 2 || p["total"].(float64) != 4 || p["nao_lidas"].(float64) != 3 {
		t.Errorf("paginação errada: %v", p)
	}
	m := esperar(t, api(t, "POST", "/api/notificacoes/lidas", b.Token, nil), 200).data()
	if m["marcadas"].(float64) != 3 || cont(b) != 0 {
		t.Errorf("marcar todas: %v, restam %v", m, cont(b))
	}
	esperar(t, api(t, "GET", "/api/notificacoes", "", nil), 401)
}
