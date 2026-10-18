//go:build integration

package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/models"
)

func criarDesafio(t *testing.T, criador *user, entrada int64, vagas int) uint {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/desafios", criador.Token, map[string]any{
		"titulo": "Desafio de teste", "valor_entrada_centavos": entrada, "vagas": vagas,
	}), 201)
	return uint(r.data()["id"].(float64))
}

func acao(t *testing.T, u *user, id uint, a string, corpo any) resp {
	t.Helper()
	return api(t, "POST", fmt.Sprintf("/api/desafios/%d/%s", id, a), u.Token, corpo)
}

func statusDesafio(t *testing.T, u *user, id uint) string {
	t.Helper()
	r := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200)
	return r.data()["status"].(string)
}

func TestDesafio_CriarDebitaEntradaDoCriador(t *testing.T) {
	a := usuarioComSaldo(t, 5000)
	id := criarDesafio(t, a, 2000, 3)

	if s := saldo(t, a); s != 3000 {
		t.Fatalf("saldo após criar deveria ser 3000, obteve %d", s)
	}
	d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), a.Token, nil), 200).data()
	if d["pote_centavos"].(float64) != 2000 || d["participantes_count"].(float64) != 1 || d["voce_e_criador"] != true {
		t.Errorf("detalhe inesperado: %v", d)
	}
	ledgerConfere(t, a)
}

func TestDesafio_CriarSemSaldoNaoCriaNada(t *testing.T) {
	a := usuarioComSaldo(t, 500)
	r := api(t, "POST", "/api/desafios", a.Token, map[string]any{"titulo": "Caro demais", "valor_entrada_centavos": 2000})
	esperar(t, r, 422)
	var n int64
	db.Model(&models.Desafio{}).Where("id_criador = ?", a.ID).Count(&n)
	if n != 0 {
		t.Errorf("desafio não deveria existir (rollback), encontrou %d", n)
	}
	if s := saldo(t, a); s != 500 {
		t.Errorf("saldo deveria permanecer 500, obteve %d", s)
	}
}

// A identidade vem do JWT: campos de id no corpo são ignorados.
func TestDesafio_CriadorSempreEhOUsuarioDoToken(t *testing.T) {
	a := usuarioComSaldo(t, 5000)
	b := novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/desafios", a.Token, map[string]any{
		"titulo": "Tentando falsificar", "valor_entrada_centavos": 1000, "id_criador": b.ID, "id_usuario": b.ID,
	}), 201)
	if uint(r.data()["id_criador"].(float64)) != a.ID {
		t.Fatalf("criador deveria ser %d (token), obteve %v", a.ID, r.data()["id_criador"])
	}
	if s := saldo(t, b); s != 0 {
		t.Errorf("o outro usuário não pode ter sido cobrado (saldo %d)", s)
	}
}

func TestDesafio_FluxoCompletoComVencedor(t *testing.T) {
	a := usuarioComSaldo(t, 5000)
	b := usuarioComSaldo(t, 5000)
	c := usuarioComSaldo(t, 5000)
	id := criarDesafio(t, a, 1000, 3)

	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, c, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)

	// a e c votam em b; b vota em a => b vence (2 x 1); apuração automática ao último voto
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": b.ID}), 200)
	esperar(t, acao(t, c, id, "votar", map[string]any{"id_votado": b.ID}), 200)
	fim := esperar(t, acao(t, b, id, "votar", map[string]any{"id_votado": a.ID}), 200).data()
	if fim["status"] != "encerrado" {
		t.Fatalf("deveria apurar automaticamente, status=%v", fim["status"])
	}

	if s := saldo(t, b); s != 4000+3000 {
		t.Errorf("vencedor deveria ter 7000, obteve %d", s)
	}
	if s := saldo(t, a); s != 4000 {
		t.Errorf("perdedor a deveria ter 4000, obteve %d", s)
	}
	if s := saldo(t, c); s != 4000 {
		t.Errorf("perdedor c deveria ter 4000, obteve %d", s)
	}
	ledgerConfere(t, a, b, c)

	// resultado exposto
	for _, p := range fim["participantes"].([]any) {
		pm := p.(map[string]any)
		if uint(pm["id"].(float64)) == b.ID {
			if pm["vencedor"] != true || pm["premio_centavos"].(float64) != 3000 || pm["votos_recebidos"].(float64) != 2 {
				t.Errorf("participante vencedor inesperado: %v", pm)
			}
		}
	}

	// gamificação: vencedor (b) +30, demais participantes (a, c) +5
	if p := pontos(t, b); p != 30 {
		t.Errorf("vencedor deveria ter 30 pontos, obteve %d", p)
	}
	if p := pontos(t, a); p != 5 {
		t.Errorf("participante a deveria ter 5 pontos, obteve %d", p)
	}
	if p := pontos(t, c); p != 5 {
		t.Errorf("participante c deveria ter 5 pontos, obteve %d", p)
	}

	// reapurar é idempotente (rejeitado), não credita de novo nem pontua de novo
	esperar(t, acao(t, a, id, "apurar", nil), 409)
	if s := saldo(t, b); s != 7000 {
		t.Errorf("saldo não pode mudar após reapurar: %d", s)
	}
	if p := pontos(t, b); p != 30 {
		t.Errorf("pontos não podem dobrar após reapurar: %d", p)
	}
}

// TestDesafio_RecomendadosUsaDTOPublico confirma que /desafios/recomendados devolve o mesmo
// contrato snake_case (DesafioResponse) usado pelo resto da API — não o model Desafio cru
// (que marshalaria como PascalCase: "ID", "Titulo", "ValorEntradaCentavos").
func TestDesafio_RecomendadosUsaDTOPublico(t *testing.T) {
	criador := usuarioComSaldo(t, 5000)
	destinatario := usuarioComSaldo(t, 5000)
	criarDesafio(t, criador, 1000, 3)

	itens := esperar(t, api(t, "GET", "/api/desafios/recomendados", destinatario.Token, nil), 200).Body["data"].([]any)
	if len(itens) == 0 {
		t.Fatal("esperava ao menos uma recomendação (desafio aberto de outro usuário)")
	}
	item := itens[0].(map[string]any)
	if _, ok := item["razao"]; !ok {
		t.Errorf("item recomendado sem campo 'razao': %v", item)
	}
	d, ok := item["desafio"].(map[string]any)
	if !ok {
		t.Fatalf("campo 'desafio' ausente ou não é objeto: %v", item)
	}
	if _, ok := d["ID"]; ok {
		t.Errorf("desafio recomendado não deveria expor o model cru (campo PascalCase 'ID'): %v", d)
	}
	if _, ok := d["valor_entrada_centavos"]; !ok {
		t.Errorf("desafio recomendado deveria expor 'valor_entrada_centavos' (snake_case): %v", d)
	}
	if _, ok := d["id"]; !ok {
		t.Errorf("desafio recomendado deveria expor 'id' (snake_case): %v", d)
	}
}

// TestUsuario_HistoricoExpoeResultadoDoUsuarioConsultado confirma que GET /usuarios/:id/historico
// usa o DTO dedicado (snake_case) e mostra o resultado (vencedor/prêmio) do usuário CONSULTADO
// (campo que, no desafio em grupo, vem de DesafioParticipante — não existe mais id_vencedor único).
func TestUsuario_HistoricoExpoeResultadoDoUsuarioConsultado(t *testing.T) {
	a := usuarioComSaldo(t, 5000)
	b := usuarioComSaldo(t, 5000)
	c := usuarioComSaldo(t, 5000)
	id := criarDesafio(t, a, 1000, 3)

	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, c, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)
	// a e c votam em b; b vota em a => b vence (2 x 1); apuração automática ao último voto
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": b.ID}), 200)
	esperar(t, acao(t, c, id, "votar", map[string]any{"id_votado": b.ID}), 200)
	esperar(t, acao(t, b, id, "votar", map[string]any{"id_votado": a.ID}), 200)

	// histórico do vencedor (b): vencedor=true, prêmio=3000
	itensB := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", b.ID), a.Token, nil), 200).Body["data"].([]any)
	if len(itensB) != 1 {
		t.Fatalf("esperava 1 desafio no histórico de b, obteve %d: %v", len(itensB), itensB)
	}
	hb := itensB[0].(map[string]any)
	if _, ok := hb["ID"]; ok {
		t.Errorf("histórico não deveria expor o model cru (campo PascalCase 'ID'): %v", hb)
	}
	if hb["valor_entrada_centavos"] == nil {
		t.Errorf("histórico deveria expor 'valor_entrada_centavos': %v", hb)
	}
	if hb["vencedor"] != true || hb["premio_centavos"].(float64) != 3000 {
		t.Errorf("histórico de b deveria mostrar vitória com prêmio 3000: %v", hb)
	}

	// histórico do perdedor (a): vencedor=false, prêmio=0 — mesmo desafio, resultado diferente
	itensA := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", a.ID), b.Token, nil), 200).Body["data"].([]any)
	if len(itensA) != 1 {
		t.Fatalf("esperava 1 desafio no histórico de a, obteve %d: %v", len(itensA), itensA)
	}
	ha := itensA[0].(map[string]any)
	if ha["vencedor"] != false || ha["premio_centavos"].(float64) != 0 {
		t.Errorf("histórico de a (perdedor) não deveria mostrar vitória nem prêmio: %v", ha)
	}
}

func TestDesafio_EmpateDivideOPote(t *testing.T) {
	us := make([]*user, 4)
	for i := range us {
		us[i] = usuarioComSaldo(t, 3000)
	}
	id := criarDesafio(t, us[0], 1000, 4)
	for _, u := range us[1:] {
		esperar(t, acao(t, u, id, "entrar", nil), 200)
	}
	esperar(t, acao(t, us[0], id, "iniciar", nil), 200)
	esperar(t, acao(t, us[0], id, "abrir-votacao", nil), 200)
	// us0->us1, us1->us0, us2->us0, us3->us1  => us0:2, us1:2 (empate); pote 4000 => 2000 cada
	votos := [][2]int{{0, 1}, {1, 0}, {2, 0}, {3, 1}}
	for _, v := range votos {
		esperar(t, acao(t, us[v[0]], id, "votar", map[string]any{"id_votado": us[v[1]].ID}), 200)
	}
	if statusDesafio(t, us[0], id) != "encerrado" {
		t.Fatal("deveria estar encerrado")
	}
	if s := saldo(t, us[0]); s != 2000+2000 {
		t.Errorf("empatado 0 deveria ter 4000, obteve %d", s)
	}
	if s := saldo(t, us[1]); s != 2000+2000 {
		t.Errorf("empatado 1 deveria ter 4000, obteve %d", s)
	}
	if s := saldo(t, us[2]); s != 2000 {
		t.Errorf("perdedor deveria ter 2000, obteve %d", s)
	}
	ledgerConfere(t, us...)

	// gamificação: os dois empatados na vitória (us0, us1) ganham 30 cada; os demais, 5
	if p := pontos(t, us[0]); p != 30 {
		t.Errorf("empatado 0 deveria ter 30 pontos, obteve %d", p)
	}
	if p := pontos(t, us[1]); p != 30 {
		t.Errorf("empatado 1 deveria ter 30 pontos, obteve %d", p)
	}
	if p := pontos(t, us[2]); p != 5 {
		t.Errorf("perdedor 2 deveria ter 5 pontos, obteve %d", p)
	}
	if p := pontos(t, us[3]); p != 5 {
		t.Errorf("perdedor 3 deveria ter 5 pontos, obteve %d", p)
	}
}

func TestDesafio_CancelarReembolsaTodos(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, b, id, "entrar", nil), 200)

	// quem não é criador não cancela
	esperar(t, acao(t, b, id, "cancelar", nil), 403)
	esperar(t, acao(t, a, id, "cancelar", nil), 200)
	if saldo(t, a) != 3000 || saldo(t, b) != 3000 {
		t.Errorf("reembolso integral esperado, a=%d b=%d", saldo(t, a), saldo(t, b))
	}
	if statusDesafio(t, a, id) != "cancelado" {
		t.Error("status deveria ser cancelado")
	}
	// idempotência: cancelar de novo não reembolsa duas vezes
	esperar(t, acao(t, a, id, "cancelar", nil), 409)
	if saldo(t, a) != 3000 {
		t.Errorf("saldo de a mudou após segundo cancelar: %d", saldo(t, a))
	}
	// não dá mais para entrar
	esperar(t, acao(t, usuarioComSaldo(t, 2000), id, "entrar", nil), 409)
	ledgerConfere(t, a, b)

	// cancelar não é apuração: ninguém pontua
	if p := pontos(t, a); p != 0 {
		t.Errorf("criador não deveria pontuar ao cancelar, obteve %d", p)
	}
	if p := pontos(t, b); p != 0 {
		t.Errorf("participante não deveria pontuar ao cancelar, obteve %d", p)
	}
}

func TestDesafio_SairReembolsaECriadorNaoPodeSair(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	if saldo(t, b) != 2000 {
		t.Fatalf("b deveria ter pago a entrada")
	}
	esperar(t, acao(t, a, id, "sair", nil), 409) // criador
	esperar(t, acao(t, b, id, "sair", nil), 200)
	if saldo(t, b) != 3000 {
		t.Errorf("b deveria ser reembolsado, saldo %d", saldo(t, b))
	}
	esperar(t, acao(t, b, id, "sair", nil), 409) // já saiu: não reembolsa de novo
	if saldo(t, b) != 3000 {
		t.Errorf("segundo sair não pode creditar: %d", saldo(t, b))
	}
	d := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), a.Token, nil), 200).data()
	if d["participantes_count"].(float64) != 1 {
		t.Errorf("vaga deveria ter sido liberada: %v", d["participantes_count"])
	}
	ledgerConfere(t, a, b)
}

func TestDesafio_EntrarRegras(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	pobre := novoUsuario(t)
	id := criarDesafio(t, a, 1000, 2)

	esperar(t, acao(t, a, id, "entrar", nil), 409)     // criador já participa
	esperar(t, acao(t, pobre, id, "entrar", nil), 422) // sem saldo
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, b, id, "entrar", nil), 409)                        // duplicado
	esperar(t, acao(t, usuarioComSaldo(t, 2000), id, "entrar", nil), 409) // lotado
	if saldo(t, pobre) != 0 {
		t.Error("saldo do pobre não pode mudar")
	}
	esperar(t, api(t, "POST", "/api/desafios/999999/entrar", b.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/desafios/abc/entrar", b.Token, nil), 400)
}

func TestDesafio_AutorizacaoPorPapel(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	estranho := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, b, id, "entrar", nil), 200)

	esperar(t, acao(t, b, id, "iniciar", nil), 403)
	esperar(t, acao(t, estranho, id, "iniciar", nil), 403)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, acao(t, b, id, "abrir-votacao", nil), 403)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)
	esperar(t, acao(t, b, id, "apurar", nil), 403)
	// estranho (não participante) não vota nem vê o detalhe de desafio em andamento
	esperar(t, acao(t, estranho, id, "votar", map[string]any{"id_votado": a.ID}), 403)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), estranho.Token, nil), 403)
	// ações fora do estado correto
	esperar(t, acao(t, a, id, "iniciar", nil), 409)
	esperar(t, acao(t, a, id, "cancelar", nil), 409)
	esperar(t, acao(t, estranho, id, "entrar", nil), 409)
}

func TestDesafio_IniciarExigeDoisParticipantes(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, a, id, "iniciar", nil), 409)
}

func TestDesafio_VotoRegras(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	c := usuarioComSaldo(t, 3000)
	fora := novoUsuario(t)
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, c, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)

	// votar antes de abrir a votação
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": b.ID}), 409)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)

	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": a.ID}), 400)    // em si mesmo
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": fora.ID}), 400) // não participante
	esperar(t, acao(t, a, id, "votar", map[string]any{}), 400)                     // sem alvo
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": b.ID}), 200)
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": c.ID}), 409) // 2º voto
}

func TestDesafio_ApurarAntecipadoExigeQuorumOuPrazo(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	c := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 3)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, c, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)

	// 0 votos de 3: sem quórum e sem prazo
	esperar(t, acao(t, a, id, "apurar", nil), 409)
	// 1 voto: ainda abaixo de metade+1 (=2)
	esperar(t, acao(t, b, id, "votar", map[string]any{"id_votado": c.ID}), 200)
	esperar(t, acao(t, a, id, "apurar", nil), 409)
	// 2 votos = metade+1: pode apurar. c vence com 2 votos
	esperar(t, acao(t, a, id, "votar", map[string]any{"id_votado": c.ID}), 200)
	esperar(t, acao(t, a, id, "apurar", nil), 200)
	if saldo(t, c) != 2000+3000 {
		t.Errorf("c deveria ganhar o pote: %d", saldo(t, c))
	}
	ledgerConfere(t, a, b, c)
}

func TestDesafio_ApurarSemVotosAposPrazoDevolve(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 2)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)
	esperar(t, acao(t, a, id, "apurar", nil), 409)

	// simula 25h desde a abertura da votação
	db.Exec("UPDATE desafios SET votacao_aberta_em = ? WHERE id = ?", time.Now().Add(-25*time.Hour), id)
	esperar(t, acao(t, a, id, "apurar", nil), 200)
	if saldo(t, a) != 3000 || saldo(t, b) != 3000 {
		t.Errorf("sem votos todos recebem a entrada de volta: a=%d b=%d", saldo(t, a), saldo(t, b))
	}
	ledgerConfere(t, a, b)
}

func TestDesafio_Convidar(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	amigo := novoUsuario(t)
	estranho := novoUsuario(t)
	b := usuarioComSaldo(t, 3000)
	amigos(t, a, amigo)
	id := criarDesafio(t, a, 1000, 3)

	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": estranho.ID}), 400) // não é amigo
	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": a.ID}), 400)
	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 200)
	esperar(t, acao(t, b, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 403) // não é criador
	esperar(t, acao(t, a, id, "convidar", map[string]any{}), 400)
}

func TestDesafio_ListagemEVisibilidade(t *testing.T) {
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 2)

	contem := func(r resp, id uint) bool {
		for _, it := range r.data()["itens"].([]any) {
			if uint(it.(map[string]any)["id"].(float64)) == id {
				return true
			}
		}
		return false
	}
	if !contem(esperar(t, api(t, "GET", "/api/desafios?limite=100", b.Token, nil), 200), id) {
		t.Error("desafio aberto com vaga deve aparecer para outros")
	}
	if !contem(esperar(t, api(t, "GET", "/api/desafios/meus", a.Token, nil), 200), id) {
		t.Error("deve aparecer em /meus do criador")
	}
	if contem(esperar(t, api(t, "GET", "/api/desafios/meus", b.Token, nil), 200), id) {
		t.Error("não deve aparecer em /meus de quem não participa")
	}
	esperar(t, acao(t, b, id, "entrar", nil), 200) // lota
	c := novoUsuario(t)
	if contem(esperar(t, api(t, "GET", "/api/desafios?limite=100", c.Token, nil), 200), id) {
		t.Error("desafio lotado não deve aparecer para terceiros")
	}
	if !contem(esperar(t, api(t, "GET", "/api/desafios?limite=100", b.Token, nil), 200), id) {
		t.Error("desafio lotado continua visível a participantes")
	}
	esperar(t, api(t, "GET", "/api/desafios?status=invalido", b.Token, nil), 400)
	r := esperar(t, api(t, "GET", "/api/desafios/meus?limite=1&pagina=1", a.Token, nil), 200)
	if len(r.data()["itens"].([]any)) > 1 {
		t.Error("limite=1 deve paginar no SQL")
	}
}

// ─── Concorrência ────────────────────────────────────────────────────────────

// 20 entradas paralelas em desafio com vagas=3: só 2 entram além do criador.
func TestDesafio_ConcorrenciaEntradasRespeitamVagas(t *testing.T) {
	criador := usuarioComSaldo(t, 5000)
	id := criarDesafio(t, criador, 1000, 3)
	const n = 20
	users := make([]*user, n)
	for i := range users {
		users[i] = usuarioComSaldo(t, 2000)
	}

	var wg sync.WaitGroup
	codigos := make([]int, n)
	for i := range users {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codigos[i] = acao(t, users[i], id, "entrar", nil).Status
		}(i)
	}
	wg.Wait()

	ok, cheio := 0, 0
	for i, c := range codigos {
		switch c {
		case 200:
			ok++
			if s := saldo(t, users[i]); s != 1000 {
				t.Errorf("quem entrou deveria ter 1000, tem %d", s)
			}
		case 409:
			cheio++
			if s := saldo(t, users[i]); s != 2000 {
				t.Errorf("quem não entrou não pode ser cobrado: %d", s)
			}
		default:
			t.Errorf("status inesperado %d", c)
		}
	}
	if ok != 2 || cheio != n-2 {
		t.Fatalf("esperava 2 entradas e %d recusas, obteve %d e %d", n-2, ok, cheio)
	}
	var participantes int64
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ?", id).Count(&participantes)
	if participantes != 3 {
		t.Errorf("deveriam existir 3 participantes, existem %d", participantes)
	}
	ledgerConfere(t, append(users, criador)...)
}

// O mesmo usuário tentando entrar 10x em paralelo paga uma única vez.
func TestDesafio_ConcorrenciaMesmoUsuarioPagaUmaVez(t *testing.T) {
	criador := usuarioComSaldo(t, 5000)
	id := criarDesafio(t, criador, 1000, 5)
	u := usuarioComSaldo(t, 5000)
	var wg sync.WaitGroup
	var ok int64
	var mu sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if acao(t, u, id, "entrar", nil).Status == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 || saldo(t, u) != 4000 {
		t.Fatalf("esperava 1 entrada e saldo 4000, obteve %d entradas e saldo %d", ok, saldo(t, u))
	}
	ledgerConfere(t, u)
}

// Cancelar concorrente com entrar: nunca sobra dinheiro preso.
func TestDesafio_ConcorrenciaCancelarVsEntrar(t *testing.T) {
	for rodada := 0; rodada < 5; rodada++ {
		criador := usuarioComSaldo(t, 3000)
		id := criarDesafio(t, criador, 1000, 10)
		us := make([]*user, 6)
		for i := range us {
			us[i] = usuarioComSaldo(t, 3000)
		}
		var wg sync.WaitGroup
		for _, u := range us {
			wg.Add(1)
			go func(u *user) { defer wg.Done(); acao(t, u, id, "entrar", nil) }(u)
		}
		wg.Add(1)
		go func() { defer wg.Done(); acao(t, criador, id, "cancelar", nil) }()
		wg.Wait()

		// Após cancelar, todos devem ter o saldo original (quem entrou antes foi reembolsado)
		if saldo(t, criador) != 3000 {
			t.Fatalf("criador deveria ser reembolsado: %d", saldo(t, criador))
		}
		for _, u := range us {
			if saldo(t, u) != 3000 {
				t.Fatalf("participante com dinheiro preso: saldo %d", saldo(t, u))
			}
		}
		ledgerConfere(t, append(us, criador)...)
	}
}
