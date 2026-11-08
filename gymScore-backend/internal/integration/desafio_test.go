//go:build integration

package integration

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gymScore-backend/internal/models"
)

// ─── Cenário: academia + instrutor vinculado ─────────────────────────────────

var cnpjSeq atomic.Int64

func cnpjUnico() string {
	s := fmt.Sprintf("%014d", cnpjSeq.Add(1)+10_000_000)
	return fmt.Sprintf("%s.%s.%s/%s-%s", s[0:2], s[2:5], s[5:8], s[8:12], s[12:14])
}

// cenario é uma academia com um instrutor vinculado (os dois com token).
type cenario struct {
	A           *user // conta da academia
	AcademiaID  uint  // academias.id
	I           *user // conta do instrutor
	InstrutorID uint  // instrutores.id
}

func novaAcademia(t *testing.T) (*user, uint) {
	t.Helper()
	n := seq.Add(1)
	u := &user{Email: fmt.Sprintf("qa+acad%d_%d@gymscore.test", n, cnpjSeq.Add(1)), Senha: senhaPadrao, Nome: fmt.Sprintf("Academia%d", n)}
	r := esperar(t, api(t, "POST", "/api/academias", "", map[string]any{
		"nome": u.Nome, "email": u.Email, "senha": u.Senha, "razao_social": "Academia Teste LTDA",
		"nome_fantasia": fmt.Sprintf("Fit Teste %d", n), "cnpj": cnpjUnico(),
	}), 201)
	acadID := idDe(r)
	if st := r.data()["status"]; st != "pendente" {
		t.Fatalf("academia nova deveria nascer pendente, veio %v", st)
	}
	aprovarAcademia(t, acadID)
	login := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200)
	u.Token = tokenDaSessao(t, login)
	u.ID = uint(login.data()["usuario"].(map[string]any)["id"].(float64))
	return u, acadID
}

// novoInstrutorDe cria um usuário com perfil instrutor vinculado à academia (idAcademia 0 = autônomo).
func novoInstrutorDe(t *testing.T, idAcademia uint) (*user, uint) {
	t.Helper()
	u := novoUsuario(t)
	if err := db.Model(&models.Usuario{}).Where("id = ?", u.ID).Update("perfil", "instrutor").Error; err != nil {
		t.Fatal(err)
	}
	i := &models.Instrutor{IDUsuario: u.ID, CREF: "123456-G/SP", Status: models.CadastroAprovado}
	if idAcademia != 0 {
		i.IDAcademia = &idAcademia
	}
	if err := db.Create(i).Error; err != nil {
		t.Fatal(err)
	}
	return u, i.ID
}

// aprovarAcademia / aprovarInstrutor simulam a aprovação do admin (o painel é a parte 2): os
// cadastros novos nascem pendentes e só operam aprovados.
func aprovarAcademia(t *testing.T, id uint) {
	t.Helper()
	if err := db.Exec("UPDATE academias SET status = 'aprovada', aprovado_em = NOW(3) WHERE id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
}

func aprovarInstrutor(t *testing.T, id uint) {
	t.Helper()
	if err := db.Exec("UPDATE instrutores SET status = 'aprovada', aprovado_em = NOW(3) WHERE id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
}

func novoCenario(t *testing.T) *cenario {
	t.Helper()
	a, aid := novaAcademia(t)
	i, iid := novoInstrutorDe(t, aid)
	return &cenario{A: a, AcademiaID: aid, I: i, InstrutorID: iid}
}

func futuro() string { return time.Now().Add(48 * time.Hour).Format(time.RFC3339) }

var tituloSeq atomic.Int64

func corpoDesafio(c *cenario, entrada int64, vagas, podio int) map[string]any {
	return map[string]any{
		"titulo": fmt.Sprintf("Desafio de teste %d", tituloSeq.Add(1)), "descricao": "Descrição do desafio",
		"categoria": "corrida", "local": "Praça central", "data_evento": futuro(),
		"vagas": vagas, "valor_entrada_centavos": entrada, "posicoes_podio": podio, "id_instrutor": c.InstrutorID,
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// criarDesafio cria (como a academia do cenário) um desafio com pódio de min(3, vagas) posições.
func criarDesafio(t *testing.T, c *cenario, entrada int64, vagas int) uint {
	t.Helper()
	return criarDesafioPodio(t, c, entrada, vagas, minInt(3, vagas))
}

func criarDesafioPodio(t *testing.T, c *cenario, entrada int64, vagas, podio int) uint {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoDesafio(c, entrada, vagas, podio)), 201)
	return idDe(r)
}

func acao(t *testing.T, u *user, id uint, a string, corpo any) resp {
	t.Helper()
	r := api(t, "POST", fmt.Sprintf("/api/desafios/%d/%s", id, a), u.Token, corpo)
	if a == "podio" && r.Status == 200 {
		aposPodio(t, id) // créditos do pódio passam pela janela de contestação (pagamentos_pendentes)
	}
	return r
}

func detalhe(t *testing.T, u *user, id uint) map[string]any {
	t.Helper()
	return esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200).data()
}

func statusDesafio(t *testing.T, u *user, id uint) string {
	t.Helper()
	return detalhe(t, u, id)["status"].(string)
}

// atletas cria n atletas com saldo.
func atletas(t *testing.T, n int, centavos int64) []*user {
	t.Helper()
	us := make([]*user, n)
	for i := range us {
		us[i] = usuarioComSaldo(t, centavos)
	}
	return us
}

func inscrever(t *testing.T, id uint, us ...*user) {
	t.Helper()
	for _, u := range us {
		esperar(t, acao(t, u, id, "entrar", nil), 200)
	}
}

func codigoDe(t *testing.T, u *user, id uint) string {
	t.Helper()
	c, _ := detalhe(t, u, id)["seu_codigo_ingresso"].(string)
	return c
}

func presencaPorCodigo(t *testing.T, quem *user, id uint, atleta *user) resp {
	t.Helper()
	return acao(t, quem, id, "presenca", map[string]any{"codigo_ingresso": codigoDe(t, atleta, id)})
}

func presencaPorID(quem *user, t *testing.T, id uint, atleta *user) resp {
	t.Helper()
	return acao(t, quem, id, "presenca", map[string]any{"id_usuario": atleta.ID})
}

func entradaPodio(u *user, marca string) map[string]any {
	return map[string]any{"id_participante": u.ID, "marca": marca}
}

func podioBody(entradas ...map[string]any) map[string]any {
	return map[string]any{"posicoes": entradas}
}

func contemID(r resp, id uint) bool {
	for _, it := range r.data()["itens"].([]any) {
		if uint(it.(map[string]any)["id"].(float64)) == id {
			return true
		}
	}
	return false
}

func contarLedger(t *testing.T, id uint, tipo models.TipoTransacao) (n int64, soma int64) {
	t.Helper()
	db.Model(&models.Transacao{}).Where("id_desafio = ? AND tipo = ?", id, tipo).Count(&n)
	db.Model(&models.Transacao{}).Select("COALESCE(SUM(valor_centavos),0)").Where("id_desafio = ? AND tipo = ?", id, tipo).Scan(&soma)
	return
}

var reCodigo = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ─── Criação ─────────────────────────────────────────────────────────────────

func TestDesafio_SoAcademiaCria(t *testing.T) {
	c := novoCenario(t)
	atleta := usuarioComSaldo(t, 5000)
	body := corpoDesafio(c, 1000, 5, 3)

	esperar(t, api(t, "POST", "/api/desafios", atleta.Token, body), 403)
	esperar(t, api(t, "POST", "/api/desafios", c.I.Token, body), 403)
	esperar(t, api(t, "POST", "/api/desafios", "", body), 401)
	if s := saldo(t, atleta); s != 5000 {
		t.Errorf("atleta que tentou criar não pode ser cobrado: %d", s)
	}

	r := esperar(t, api(t, "POST", "/api/desafios", c.A.Token, body), 201)
	d := r.data()
	id := idDe(r)
	if d["status"] != "aberto" || d["posicoes_podio"].(float64) != 3 || d["participantes_count"].(float64) != 0 ||
		d["categoria"] != "corrida" || d["vagas"].(float64) != 5 || d["pote_centavos"].(float64) != 0 {
		t.Errorf("desafio criado inesperado: %v", d)
	}
	ac := d["academia"].(map[string]any)
	if uint(ac["id"].(float64)) != c.AcademiaID || !strings.HasPrefix(ac["nome_fantasia"].(string), "Fit Teste") {
		t.Errorf("academia do DTO errada: %v", ac)
	}
	in := d["instrutor"].(map[string]any)
	if uint(in["id"].(float64)) != c.InstrutorID || !strings.HasPrefix(in["nome"].(string), "Teste") {
		t.Errorf("instrutor do DTO errado: %v", in)
	}
	if d["data_evento"] == nil {
		t.Error("data_evento deveria vir preenchida")
	}
	// a academia não vira participante nem paga nada
	var n int64
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ?", id).Count(&n)
	if n != 0 || saldo(t, c.A) != 0 {
		t.Errorf("academia não participa nem paga: participantes=%d saldo=%d", n, saldo(t, c.A))
	}
	// flags da academia dona
	if d["pode_editar"] != true || d["pode_cancelar"] != true || d["pode_entrar"] != false || d["pode_iniciar"] != false || d["pode_lancar_podio"] != false {
		t.Errorf("flags da academia errados: %v", d)
	}
	// o instrutor designado recebe a notificação
	if contarNotif(t, c.I, models.NotifInstrutorDesignado) != 1 {
		t.Error("instrutor designado deveria ser notificado")
	}
}

func TestDesafio_CriarValidacoes(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	autonomo, autonomoID := novoInstrutorDe(t, 0)
	_ = autonomo

	casos := []struct {
		nome   string
		mexer  func(m map[string]any)
		status int
	}{
		{"instrutor de outra academia", func(m map[string]any) { m["id_instrutor"] = outra.InstrutorID }, 400},
		{"instrutor autônomo (sem vínculo)", func(m map[string]any) { m["id_instrutor"] = autonomoID }, 400},
		{"instrutor inexistente", func(m map[string]any) { m["id_instrutor"] = 999999 }, 400},
		{"sem instrutor", func(m map[string]any) { delete(m, "id_instrutor") }, 400},
		{"pódio maior que as vagas", func(m map[string]any) { m["vagas"] = 3; m["posicoes_podio"] = 4 }, 400},
		{"pódio 0", func(m map[string]any) { m["posicoes_podio"] = 0 }, 400},
		{"pódio 6", func(m map[string]any) { m["posicoes_podio"] = 6 }, 400},
		{"vagas 1", func(m map[string]any) { m["vagas"] = 1; m["posicoes_podio"] = 1 }, 400},
		{"vagas 501", func(m map[string]any) { m["vagas"] = 501 }, 400},
		{"entrada negativa", func(m map[string]any) { m["valor_entrada_centavos"] = -1 }, 400},
		{"entrada acima de R$ 10.000", func(m map[string]any) { m["valor_entrada_centavos"] = 1_000_001 }, 400},
		{"data no passado", func(m map[string]any) { m["data_evento"] = time.Now().Add(-time.Hour).Format(time.RFC3339) }, 400},
		{"data inválida", func(m map[string]any) { m["data_evento"] = "amanhã" }, 400},
		{"sem data", func(m map[string]any) { delete(m, "data_evento") }, 400},
		{"categoria desconhecida", func(m map[string]any) { m["categoria"] = "xadrez" }, 400},
		{"sem local", func(m map[string]any) { delete(m, "local") }, 400},
		{"título curto", func(m map[string]any) { m["titulo"] = "ab" }, 400},
	}
	for _, cs := range casos {
		m := corpoDesafio(c, 1000, 5, 3)
		cs.mexer(m)
		if r := api(t, "POST", "/api/desafios", c.A.Token, m); r.Status != cs.status {
			t.Errorf("%s: esperava %d, obteve %d (%s)", cs.nome, cs.status, r.Status, r.Raw)
		}
	}
	// limites válidos: gratuito, teto de R$ 10.000, 500 vagas, pódio de 5, data em formato local, categoria padrão
	m := corpoDesafio(c, 0, 500, 5)
	m["data_evento"] = time.Now().Add(72 * time.Hour).Format("2006-01-02T15:04")
	delete(m, "categoria")
	r := esperar(t, api(t, "POST", "/api/desafios", c.A.Token, m), 201)
	if r.data()["categoria"] != "geral" || r.data()["valor_entrada_centavos"].(float64) != 0 {
		t.Errorf("gratuito/categoria padrão: %v", r.data())
	}
	esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoDesafio(c, 1_000_000, 2, 1)), 201)
}

// ─── Inscrição ───────────────────────────────────────────────────────────────

func TestDesafio_EntrarDebitaGeraIngressoESairReembolsa(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 2000, 4)
	a := usuarioComSaldo(t, 5000)

	d := esperar(t, acao(t, a, id, "entrar", nil), 200).data()
	if saldo(t, a) != 3000 {
		t.Fatalf("saldo após entrar deveria ser 3000, obteve %d", saldo(t, a))
	}
	cod, _ := d["seu_codigo_ingresso"].(string)
	if !reCodigo.MatchString(cod) {
		t.Errorf("seu_codigo_ingresso deveria ter 32 hex: %q", cod)
	}
	if d["voce_participa"] != true || d["voce_presente"] != false || d["pode_sair"] != true || d["pode_entrar"] != false ||
		d["pote_centavos"].(float64) != 2000 || d["participantes_count"].(float64) != 1 {
		t.Errorf("detalhe após entrar inesperado: %v", d)
	}
	// o código fica só com o próprio atleta
	outro := novoUsuario(t)
	do := detalhe(t, outro, id)
	if _, ok := do["seu_codigo_ingresso"]; ok {
		t.Errorf("código de ingresso não pode vazar para outro usuário: %v", do["seu_codigo_ingresso"])
	}
	for _, p := range do["participantes"].([]any) {
		if _, ok := p.(map[string]any)["codigo_ingresso"]; ok {
			t.Error("lista de participantes não pode expor codigo_ingresso")
		}
	}
	esperar(t, acao(t, a, id, "entrar", nil), 409) // já participa
	if n, soma := contarLedger(t, id, models.TipoEntradaDesafio); n != 1 || soma != -2000 {
		t.Errorf("ledger entrada_desafio: n=%d soma=%d", n, soma)
	}
	if contarNotif(t, a, models.NotifInscricaoConfirmada) != 1 {
		t.Error("inscrição confirmada deveria notificar o atleta")
	}

	// acesso a ingresso: aparece em /meus com o código e status
	meus := esperar(t, api(t, "GET", "/api/desafios/meus", a.Token, nil), 200)
	var achou bool
	for _, it := range meus.data()["itens"].([]any) {
		m := it.(map[string]any)
		if uint(m["id"].(float64)) == id {
			achou = m["seu_codigo_ingresso"] == cod && m["status"] == "aberto"
		}
	}
	if !achou {
		t.Error("GET /desafios/meus deveria trazer o desafio com seu_codigo_ingresso e status")
	}

	d = esperar(t, acao(t, a, id, "sair", nil), 200).data()
	if saldo(t, a) != 5000 || d["voce_participa"] != false || d["participantes_count"].(float64) != 0 {
		t.Errorf("sair deveria reembolsar e liberar a vaga: saldo=%d %v", saldo(t, a), d)
	}
	esperar(t, acao(t, a, id, "sair", nil), 409)
	// reentrando recebe um ingresso NOVO
	d = esperar(t, acao(t, a, id, "entrar", nil), 200).data()
	if d["seu_codigo_ingresso"] == cod {
		t.Error("novo ingresso deveria ter outro código")
	}
	ledgerConfere(t, a)
}

func TestDesafio_EntrarRegras(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 2)

	semSaldo := novoUsuario(t)
	esperar(t, acao(t, semSaldo, id, "entrar", nil), 422)
	var n int64
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ?", id).Count(&n)
	if n != 0 {
		t.Error("sem saldo não pode inscrever (rollback)")
	}
	// academia e instrutor não são atletas
	esperar(t, acao(t, c.A, id, "entrar", nil), 403)
	esperar(t, acao(t, c.I, id, "entrar", nil), 403)
	esperar(t, acao(t, c.A, id, "sair", nil), 403)
	esperar(t, acao(t, c.I, id, "sair", nil), 403)
	esperar(t, acao(t, usuarioComSaldo(t, 1000), 999999, "entrar", nil), 404)

	us := atletas(t, 3, 2000)
	inscrever(t, id, us[0], us[1])
	esperar(t, acao(t, us[2], id, "entrar", nil), 409) // lotado
	if saldo(t, us[2]) != 2000 {
		t.Error("quem não conseguiu vaga não pode ser cobrado")
	}
	// evento já aconteceu: inscrições encerradas
	id2 := criarDesafio(t, c, 1000, 4)
	db.Exec("UPDATE desafios SET data_evento = ? WHERE id = ?", time.Now().Add(-time.Hour), id2)
	esperar(t, acao(t, us[2], id2, "entrar", nil), 409)
	if d := detalhe(t, us[2], id2); d["pode_entrar"] != false {
		t.Error("pode_entrar deveria ser false após a data do evento")
	}
	// desafio em andamento não aceita entrada nem saída
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, acao(t, us[2], id, "entrar", nil), 409)
	esperar(t, acao(t, us[0], id, "sair", nil), 409)
	ledgerConfere(t, us...)
}

func TestDesafio_GratuitoNaoMexeNoLedger(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 0, 4, 2)
	us := atletas(t, 3, 1000)
	inscrever(t, id, us...)
	for _, u := range us {
		if saldo(t, u) != 1000 {
			t.Fatalf("desafio gratuito não pode cobrar: %d", saldo(t, u))
		}
	}
	if n, _ := contarLedger(t, id, models.TipoEntradaDesafio); n != 0 {
		t.Errorf("sem linhas de entrada no ledger, obteve %d", n)
	}
	esperar(t, acao(t, us[2], id, "sair", nil), 200)
	if n, _ := contarLedger(t, id, models.TipoEstornoDesafio); n != 0 {
		t.Errorf("sem estorno no ledger para gratuito, obteve %d", n)
	}
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, presencaPorCodigo(t, c.I, id, us[0]), 200)
	esperar(t, presencaPorID(c.I, t, id, us[1]), 200)
	d := esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[1], "10:00"), entradaPodio(us[0], "10:30"))), 200).data()
	if d["status"] != "encerrado" {
		t.Fatalf("deveria encerrar: %v", d)
	}
	for _, p := range d["podio"].([]any) {
		if p.(map[string]any)["premio_centavos"].(float64) != 0 {
			t.Errorf("prêmio de desafio gratuito deve ser 0: %v", p)
		}
	}
	if n, _ := contarLedger(t, id, models.TipoPremioDesafio); n != 0 {
		t.Errorf("sem linhas de prêmio no ledger, obteve %d", n)
	}
	// pontos mesmo sem dinheiro
	if pontos(t, us[1]) != 30 || pontos(t, us[0]) != 15 {
		t.Errorf("pontos: 1º=%d (30) 2º=%d (15)", pontos(t, us[1]), pontos(t, us[0]))
	}
	ledgerConfere(t, us...)
}

// ─── Fluxo completo ──────────────────────────────────────────────────────────

func TestDesafio_FluxoCompletoAcademiaInstrutorAtletas(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 1000, 6, 3)
	us := atletas(t, 6, 3000) // a1..a6
	a1, a2, a3, a4, a5, a6 := us[0], us[1], us[2], us[3], us[4], us[5]
	inscrever(t, id, us...)
	esperar(t, acao(t, a6, id, "sair", nil), 200) // a6 desiste e é reembolsado

	d := detalhe(t, c.A, id)
	if d["participantes_count"].(float64) != 5 || d["pote_centavos"].(float64) != 5000 {
		t.Fatalf("5 inscritos e pote 5000 esperados: %v", d)
	}
	prev := d["premiacao_prevista_centavos"].([]any)
	if len(prev) != 3 || prev[0].(float64) != 3000 || prev[1].(float64) != 1500 || prev[2].(float64) != 500 {
		t.Errorf("premiação prevista: %v", prev)
	}

	// presença antes de iniciar só no dia do evento (evento é em 48h)
	esperar(t, presencaPorCodigo(t, c.I, id, a1), 409)
	// iniciar: instrutor designado
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	if statusDesafio(t, a1, id) != "em_andamento" {
		t.Fatal("deveria estar em andamento")
	}
	if contarNotif(t, a1, models.NotifDesafioIniciado) != 1 {
		t.Error("atletas inscritos são notificados do início")
	}

	// presença: a1 e a3 pelo QR, a2 e a4 pela lista; a5 ausente
	r := esperar(t, presencaPorCodigo(t, c.I, id, a1), 200).data()
	if r["nome"] != a1.Nome || r["presente"] != true || r["ja_presente"] != false || r["presentes_count"].(float64) != 1 {
		t.Errorf("resposta da presença: %v", r)
	}
	esperar(t, presencaPorCodigo(t, c.I, id, a3), 200)
	esperar(t, presencaPorID(c.I, t, id, a2), 200)
	r = esperar(t, presencaPorID(c.A, t, id, a4), 200).data() // academia dona como reserva
	if r["presentes_count"].(float64) != 4 || r["participantes_count"].(float64) != 5 {
		t.Errorf("contagem de presentes: %v", r)
	}
	// idempotente: não duplica nem renotifica
	r = esperar(t, presencaPorCodigo(t, c.I, id, a1), 200).data()
	if r["ja_presente"] != true || r["presentes_count"].(float64) != 4 {
		t.Errorf("segunda leitura do mesmo QR deveria ser idempotente: %v", r)
	}
	if contarNotif(t, a1, models.NotifPresencaConfirmada) != 1 {
		t.Error("presença confirmada notifica uma única vez")
	}
	// coerência das colunas presente/presente_em
	var incoerentes int64
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ? AND ((presente = 1) <> (presente_em IS NOT NULL))", id).Count(&incoerentes)
	if incoerentes != 0 {
		t.Errorf("presente e presente_em incoerentes em %d linhas", incoerentes)
	}
	// lista de presentes (staff)
	pres := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/presentes", id), c.I.Token, nil), 200).Body["data"].([]any)
	if len(pres) != 4 {
		t.Errorf("esperava 4 presentes, obteve %d", len(pres))
	}
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/presentes", id), a1.Token, nil), 403)
	if dd := detalhe(t, a1, id); dd["voce_presente"] != true || dd["pode_avaliar"] != false {
		t.Errorf("a1 presente: %v", dd)
	}
	if dd := detalhe(t, a5, id); dd["voce_presente"] != false {
		t.Error("a5 não está presente")
	}

	// flags do instrutor antes do pódio
	di := detalhe(t, c.I, id)
	if di["pode_lancar_podio"] != true || di["pode_marcar_presenca"] != true || di["pode_cancelar"] != false || di["pode_editar"] != false || di["presentes_count"].(float64) != 4 {
		t.Errorf("flags do instrutor: %v", di)
	}

	// pódio de 3 entre 4 presentes: a2, a3, a1 (a4 é presente fora do pódio)
	d = esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(a2, "10:01"), entradaPodio(a3, "10:30"), entradaPodio(a1, "11:00"))), 200).data()
	if d["status"] != "encerrado" || d["pode_lancar_podio"] != false || d["pode_marcar_presenca"] != false {
		t.Fatalf("desafio deveria estar encerrado: %v", d)
	}
	podio := d["podio"].([]any)
	if len(podio) != 3 {
		t.Fatalf("pódio no detalhe: %v", podio)
	}
	want := []struct {
		u     *user
		valor float64
		marca string
	}{{a2, 3000, "10:01"}, {a3, 1500, "10:30"}, {a1, 500, "11:00"}}
	for i, w := range want {
		p := podio[i].(map[string]any)
		if p["posicao"].(float64) != float64(i+1) || uint(p["id_participante"].(float64)) != w.u.ID ||
			p["premio_centavos"].(float64) != w.valor || p["marca"] != w.marca || p["nome"] != w.u.Nome {
			t.Errorf("posição %d: %v (esperava %s %v)", i+1, p, w.u.Nome, w.valor)
		}
	}
	// saldos exatos: 3000 - 1000 de entrada + prêmio
	for u, s := range map[*user]int64{a2: 5000, a3: 3500, a1: 2500, a4: 2000, a5: 2000, a6: 3000} {
		if got := saldo(t, u); got != s {
			t.Errorf("usuário %d: saldo %d, esperava %d", u.ID, got, s)
		}
	}
	if n, soma := contarLedger(t, id, models.TipoPremioDesafio); n != 3 || soma != 5000 {
		t.Errorf("ledger de prêmios: n=%d soma=%d (esperava 3 e 5000)", n, soma)
	}
	if got := saldo(t, c.A); got != 0 {
		t.Errorf("a academia não recebe nada: %d", got)
	}
	ledgerConfere(t, us...)
	// pontos: 1º +30, 2º/3º +15, presente fora do pódio +5, ausente 0
	for u, p := range map[*user]int{a2: 30, a3: 15, a1: 15, a4: 5, a5: 0, a6: 0} {
		if got := pontos(t, u); got != p {
			t.Errorf("usuário %d: pontos %d, esperava %d", u.ID, got, p)
		}
	}
	// vencedor/prêmio nos participantes
	var venc []uint
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ? AND vencedor = 1", id).Pluck("id_usuario", &venc)
	if len(venc) != 1 || venc[0] != a2.ID {
		t.Errorf("só o 1º colocado é vencedor: %v", venc)
	}
	// notificações de resultado
	if contarNotif(t, a2, models.NotifResultado) != 1 || contarNotif(t, a5, models.NotifResultado) != 1 {
		t.Error("todos os inscritos recebem o resultado uma vez")
	}
	n1 := notifDoTipo(t, a2, models.NotifResultado)["mensagem"].(string)
	if !strings.Contains(n1, "1º lugar") || !strings.Contains(n1, "30,00") {
		t.Errorf("mensagem do 1º: %s", n1)
	}
	// encerrado: avaliação só de presente, uma vez
	if dd := detalhe(t, a4, id); dd["pode_avaliar"] != true {
		t.Errorf("a4 esteve presente e pode avaliar: %v", dd)
	}
	if dd := detalhe(t, a5, id); dd["pode_avaliar"] != false {
		t.Error("a5 faltou: não pode avaliar")
	}
	esperar(t, acao(t, a5, id, "avaliar", map[string]any{"nota": 5}), 403)
	esperar(t, acao(t, a4, id, "avaliar", map[string]any{"nota": 6}), 400)
	esperar(t, acao(t, a4, id, "avaliar", map[string]any{"nota": 4}), 201)
	esperar(t, acao(t, a4, id, "avaliar", map[string]any{"nota": 5}), 409)
	if dd := detalhe(t, a4, id); dd["pode_avaliar"] != false {
		t.Error("depois de avaliar, pode_avaliar volta a false")
	}
	// GET /podio
	pl := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/podio", id), a5.Token, nil), 200).Body["data"].([]any)
	if len(pl) != 3 {
		t.Errorf("GET /podio: %v", pl)
	}
	// histórico do vencedor e do presente fora do pódio
	h := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", a2.ID), a1.Token, nil), 200).Body["data"].([]any)
	if len(h) != 1 || h[0].(map[string]any)["vencedor"] != true || h[0].(map[string]any)["premio_centavos"].(float64) != 3000 {
		t.Errorf("histórico do vencedor: %v", h)
	}
	// encerrado: nenhuma ação adicional
	esperar(t, acao(t, c.A, id, "cancelar", nil), 409)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 409)
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(a1, "x"), entradaPodio(a2, "y"), entradaPodio(a3, "z"))), 409)
}

func TestDesafio_PodioComMenosPresentesQuePosicoesRenormaliza(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 1000, 4, 3)
	us := atletas(t, 3, 2000)
	inscrever(t, id, us...) // pote 3000
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, presencaPorCodigo(t, c.I, id, us[0]), 200)
	esperar(t, presencaPorCodigo(t, c.I, id, us[2]), 200)

	// 3 posições configuradas, 2 presentes: exige exatamente 2
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[0], "a"))), 400)
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[0], "a"), entradaPodio(us[2], "b"), entradaPodio(us[1], "c"))), 400)
	d := esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[2], "9,5s"), entradaPodio(us[0], "9,9s"))), 200).data()
	podio := d["podio"].([]any)
	// 60/30 renormalizado: 2000 e 1000
	if len(podio) != 2 || podio[0].(map[string]any)["premio_centavos"].(float64) != 2000 || podio[1].(map[string]any)["premio_centavos"].(float64) != 1000 {
		t.Errorf("pódio renormalizado: %v", podio)
	}
	if saldo(t, us[2]) != 1000+2000 || saldo(t, us[0]) != 1000+1000 || saldo(t, us[1]) != 1000 {
		t.Errorf("saldos: %d %d %d", saldo(t, us[0]), saldo(t, us[1]), saldo(t, us[2]))
	}
	ledgerConfere(t, us...)
}

func TestDesafio_PodioRestoDeCentavosVaiParaOPrimeiro(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 333, 3, 3) // pote 999: 599 + 299 + 99 = 997, resto 2 -> 1º
	us := atletas(t, 3, 1000)
	inscrever(t, id, us...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(us[2], "3"))), 200)
	if saldo(t, us[0]) != 1000-333+599+2 || saldo(t, us[1]) != 1000-333+299 || saldo(t, us[2]) != 1000-333+99 {
		t.Errorf("divisão com resto: %d %d %d", saldo(t, us[0]), saldo(t, us[1]), saldo(t, us[2]))
	}
	if _, soma := contarLedger(t, id, models.TipoPremioDesafio); soma != 999 {
		t.Errorf("soma dos prêmios deve ser o pote (999): %d", soma)
	}
	ledgerConfere(t, us...)
}

// ─── Permissões ──────────────────────────────────────────────────────────────

func TestDesafio_PermissoesPorPapel(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	us := atletas(t, 2, 3000)
	inscrever(t, id, us...)
	cod := codigoDe(t, us[0], id)

	// iniciar
	esperar(t, acao(t, us[0], id, "iniciar", nil), 403)
	esperar(t, acao(t, outra.I, id, "iniciar", nil), 403)
	esperar(t, acao(t, outra.A, id, "iniciar", nil), 403)
	// cancelar: só a academia dona
	esperar(t, acao(t, us[0], id, "cancelar", nil), 403)
	esperar(t, acao(t, c.I, id, "cancelar", nil), 403)
	esperar(t, acao(t, outra.A, id, "cancelar", nil), 403)
	esperar(t, acao(t, outra.I, id, "cancelar", nil), 403)
	// presença
	corpo := map[string]any{"codigo_ingresso": cod}
	esperar(t, acao(t, us[0], id, "presenca", corpo), 403)
	esperar(t, acao(t, us[1], id, "presenca", corpo), 403)
	esperar(t, acao(t, outra.I, id, "presenca", corpo), 403)
	esperar(t, acao(t, outra.A, id, "presenca", corpo), 403)
	// editar
	patch := func(u *user) resp {
		return api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), u.Token, map[string]any{"titulo": "Novo título"})
	}
	esperar(t, patch(us[0]), 403)
	esperar(t, patch(c.I), 403)
	esperar(t, patch(outra.A), 403)
	esperar(t, patch(outra.I), 403)
	// lista de presentes
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/presentes", id), outra.A.Token, nil), 403)

	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	// pódio: nenhum presente ainda, depois permissões
	esperar(t, presencaPorID(c.I, t, id, us[0]), 200)
	body := podioBody(entradaPodio(us[0], "1"))
	esperar(t, acao(t, us[0], id, "podio", body), 403)
	esperar(t, acao(t, us[1], id, "podio", body), 403)
	esperar(t, acao(t, outra.I, id, "podio", body), 403)
	esperar(t, acao(t, outra.A, id, "podio", body), 403)
	if statusDesafio(t, c.A, id) != "em_andamento" {
		t.Fatal("tentativas negadas não podem mudar o desafio")
	}
	// a academia dona lança como reserva
	esperar(t, acao(t, c.A, id, "podio", body), 200)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), "", nil), 401)
}

func TestDesafio_VisibilidadeDoDetalhe(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	us := atletas(t, 2, 2000)
	inscrever(t, id, us[0])
	// aberto: qualquer autenticado vê
	for _, u := range []*user{us[1], outra.A, outra.I, c.A, c.I} {
		esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200)
	}
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	// em andamento: só participantes e staff
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), us[1].Token, nil), 403)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), outra.A.Token, nil), 403)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), outra.I.Token, nil), 403)
	esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/podio", id), us[1].Token, nil), 403)
	for _, u := range []*user{us[0], c.A, c.I} {
		esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d", id), u.Token, nil), 200)
	}
}

func TestDesafio_PresencaRegras(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	idOutro := criarDesafio(t, outra, 1000, 4)
	us := atletas(t, 3, 3000)
	inscrever(t, id, us[0], us[1])
	inscrever(t, idOutro, us[2])

	// corpo inválido: nenhum ou os dois
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{}), 400)
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": codigoDe(t, us[0], id), "id_usuario": us[0].ID}), 400)

	// aberto: só no dia do evento (evento é em 48h)
	esperar(t, presencaPorCodigo(t, c.I, id, us[0]), 409)
	if d := detalhe(t, c.I, id); d["pode_marcar_presenca"] != false {
		t.Error("pode_marcar_presenca deveria ser false em aberto fora do dia do evento")
	}
	// leva o evento para hoje (00:00 do dia): em aberto a presença passa
	hoje := time.Now()
	inicioDoDia := time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 0, 0, 0, 0, time.Local)
	db.Exec("UPDATE desafios SET data_evento = ? WHERE id = ?", inicioDoDia, id)
	if d := detalhe(t, c.I, id); d["pode_marcar_presenca"] != true {
		t.Error("pode_marcar_presenca deveria ser true no dia do evento")
	}
	esperar(t, presencaPorCodigo(t, c.I, id, us[0]), 200)

	// código de OUTRO desafio, inexistente e malformado
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": codigoDe(t, us[2], idOutro)}), 404)
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": strings.Repeat("0", 32)}), 404)
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": "../../etc/passwd"}), 400)
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": "abc"}), 400)
	// usuário não inscrito (por id) e do outro desafio
	esperar(t, presencaPorID(c.I, t, id, us[2]), 404)
	esperar(t, presencaPorID(c.I, t, 999999, us[0]), 404)
	// o código aceita maiúsculas (QR de câmera)
	cod := strings.ToUpper(codigoDe(t, us[1], id))
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": cod}), 200)

	// cancelado: sem presença
	esperar(t, acao(t, c.A, id, "cancelar", nil), 200)
	esperar(t, presencaPorID(c.I, t, id, us[0]), 409)
}

func TestDesafio_PodioValidacoes(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 1000, 5, 3)
	us := atletas(t, 4, 2000)
	inscrever(t, id, us...)
	body3 := podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(us[2], "3"))

	esperar(t, acao(t, c.I, id, "podio", body3), 409) // ainda aberto
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, acao(t, c.I, id, "podio", body3), 409) // ninguém presente
	if d := detalhe(t, c.I, id); d["pode_lancar_podio"] != false {
		t.Error("sem presentes não pode lançar pódio")
	}
	for _, u := range us[:3] {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	casos := []struct {
		nome string
		body any
	}{
		{"menos posições que o esperado", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"))},
		{"mais posições que o esperado", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(us[2], "3"), entradaPodio(us[3], "4"))},
		{"atleta ausente no pódio", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(us[3], "3"))},
		{"atleta repetido", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[0], "2"), entradaPodio(us[1], "3"))},
		{"marca vazia", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "  "), entradaPodio(us[2], "3"))},
		{"marca longa demais", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], strings.Repeat("x", 51)), entradaPodio(us[2], "3"))},
		{"usuário fora do desafio", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(&user{ID: 999999}, "3"))},
		{"lista vazia", podioBody()},
		{"seis posições", podioBody(entradaPodio(us[0], "1"), entradaPodio(us[1], "2"), entradaPodio(us[2], "3"), entradaPodio(us[0], "4"), entradaPodio(us[1], "5"), entradaPodio(us[2], "6"))},
	}
	for _, cs := range casos {
		if r := acao(t, c.I, id, "podio", cs.body); r.Status != 400 {
			t.Errorf("%s: esperava 400, obteve %d (%s)", cs.nome, r.Status, r.Raw)
		}
	}
	if statusDesafio(t, c.A, id) != "em_andamento" {
		t.Fatal("pódios inválidos não podem encerrar o desafio")
	}
	for _, u := range us {
		if saldo(t, u) != 1000 {
			t.Fatalf("nada pode ser creditado em pódio inválido: %d", saldo(t, u))
		}
	}
	esperar(t, acao(t, c.I, id, "podio", body3), 200)
}

// ─── Cancelamento, edição e início ───────────────────────────────────────────

func TestDesafio_CancelarReembolsaTodos(t *testing.T) {
	c := novoCenario(t)
	for _, iniciado := range []bool{false, true} {
		id := criarDesafio(t, c, 1500, 4)
		us := atletas(t, 3, 3000)
		inscrever(t, id, us...)
		if iniciado {
			esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
			esperar(t, presencaPorID(c.I, t, id, us[0]), 200) // até quem esteve presente é reembolsado
		}
		d := esperar(t, acao(t, c.A, id, "cancelar", nil), 200).data()
		if d["status"] != "cancelado" || d["pode_cancelar"] != false {
			t.Errorf("deveria cancelar: %v", d)
		}
		for _, u := range us {
			if saldo(t, u) != 3000 {
				t.Errorf("iniciado=%v: usuário %d não foi reembolsado: %d", iniciado, u.ID, saldo(t, u))
			}
			if contarNotif(t, u, models.NotifDesafioCancel) != 1 {
				t.Errorf("iniciado=%v: cancelamento deveria notificar o atleta", iniciado)
			}
		}
		if n, soma := contarLedger(t, id, models.TipoEstornoDesafio); n != 3 || soma != 4500 {
			t.Errorf("estornos: n=%d soma=%d", n, soma)
		}
		ledgerConfere(t, us...)
		esperar(t, acao(t, c.A, id, "cancelar", nil), 409)
		esperar(t, acao(t, us[1], id, "entrar", nil), 409)
		esperar(t, acao(t, c.I, id, "iniciar", nil), 409)
	}
	if contarNotif(t, c.I, models.NotifDesafioCancel) != 2 {
		t.Error("o instrutor designado também é avisado do cancelamento")
	}
}

func TestDesafio_IniciarRegras(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 409) // sem inscritos
	if d := detalhe(t, c.I, id); d["pode_iniciar"] != false {
		t.Error("sem inscritos não pode iniciar")
	}
	u := usuarioComSaldo(t, 2000)
	inscrever(t, id, u)
	if d := detalhe(t, c.I, id); d["pode_iniciar"] != true {
		t.Error("com inscrito o instrutor pode iniciar")
	}
	esperar(t, acao(t, c.A, id, "iniciar", nil), 200) // academia dona também pode
	esperar(t, acao(t, c.I, id, "iniciar", nil), 409)
}

func TestDesafio_EditarRegras(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	i2, i2ID := novoInstrutorDe(t, c.AcademiaID)
	id := criarDesafio(t, c, 1000, 5)
	patch := func(u *user, m map[string]any) resp {
		return api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), u.Token, m)
	}

	d := esperar(t, patch(c.A, map[string]any{"titulo": "Título novo", "categoria": "crossfit", "valor_entrada_centavos": 1500, "vagas": 8, "posicoes_podio": 4, "descricao": ""}), 200).data()
	if d["titulo"] != "Título novo" || d["categoria"] != "crossfit" || d["valor_entrada_centavos"].(float64) != 1500 ||
		d["vagas"].(float64) != 8 || d["posicoes_podio"].(float64) != 4 || d["descricao"] != "" {
		t.Errorf("edição não aplicada: %v", d)
	}
	esperar(t, patch(c.A, map[string]any{"posicoes_podio": 9}), 400)
	esperar(t, patch(c.A, map[string]any{"vagas": 3}), 400) // pódio (4) > vagas (3)
	esperar(t, patch(c.A, map[string]any{"categoria": "xadrez"}), 400)
	esperar(t, patch(c.A, map[string]any{"data_evento": time.Now().Add(-time.Hour).Format(time.RFC3339)}), 400)
	esperar(t, patch(c.A, map[string]any{"id_instrutor": outra.InstrutorID}), 400) // de outra academia
	esperar(t, patch(c.A, map[string]any{"id_instrutor": 999999}), 400)
	esperar(t, patch(c.A, map[string]any{"local": ""}), 400)

	// troca de instrutor: antigo e novo notificados
	esperar(t, patch(c.A, map[string]any{"id_instrutor": i2ID}), 200)
	d = detalhe(t, c.A, id)
	if uint(d["instrutor"].(map[string]any)["id"].(float64)) != i2ID {
		t.Errorf("instrutor não trocou: %v", d["instrutor"])
	}
	if contarNotif(t, c.I, models.NotifInstrutorRemovido) != 1 || contarNotif(t, i2, models.NotifInstrutorDesignado) != 1 {
		t.Error("troca de instrutor deveria notificar o removido e o designado")
	}
	// o instrutor antigo perde as permissões, o novo ganha
	u := usuarioComSaldo(t, 3000)
	inscrever(t, id, u)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 403)
	if detalhe(t, i2, id)["pode_iniciar"] != true {
		t.Error("o novo instrutor deve poder iniciar")
	}

	// com inscrito: entrada imutável (mesmo valor é aceito), vagas não ficam abaixo dos inscritos
	esperar(t, patch(c.A, map[string]any{"valor_entrada_centavos": 2000}), 409)
	esperar(t, patch(c.A, map[string]any{"valor_entrada_centavos": 1500, "titulo": "Titulo ok"}), 200)
	extra := usuarioComSaldo(t, 3000)
	inscrever(t, id, extra)
	esperar(t, patch(c.A, map[string]any{"vagas": 4}), 200)
	// 2 inscritos: vagas 1 é inválido (min 2) e vagas 2 passaria; posicoes_podio 4 > 2
	esperar(t, patch(c.A, map[string]any{"vagas": 2}), 400)
	esperar(t, patch(c.A, map[string]any{"vagas": 2, "posicoes_podio": 2}), 200)
	esperar(t, patch(c.A, map[string]any{"vagas": 500, "posicoes_podio": 1}), 200)

	// não aberto: sem edição
	esperar(t, acao(t, i2, id, "iniciar", nil), 200)
	esperar(t, patch(c.A, map[string]any{"titulo": "Tarde demais"}), 409)
	if d := detalhe(t, c.A, id); d["pode_editar"] != false {
		t.Error("pode_editar deveria ser false fora de aberto")
	}
	esperar(t, api(t, "PATCH", "/api/desafios/999999", c.A.Token, map[string]any{"titulo": "xxx"}), 404)
}

// ─── Listagem ────────────────────────────────────────────────────────────────

func TestDesafio_FeedEFiltros(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	viewer := usuarioComSaldo(t, 5000)

	mk := func(cen *cenario, mexer func(m map[string]any)) uint {
		m := corpoDesafio(cen, 1000, 5, 3)
		mexer(m)
		return idDe(esperar(t, api(t, "POST", "/api/desafios", cen.A.Token, m), 201))
	}
	corrida := mk(c, func(m map[string]any) { m["categoria"] = "corrida"; m["titulo"] = "Corrida zebra azul" })
	cross := mk(c, func(m map[string]any) { m["categoria"] = "crossfit" })
	passado := mk(c, func(m map[string]any) {})
	db.Exec("UPDATE desafios SET data_evento = ? WHERE id = ?", time.Now().Add(-2*time.Hour), passado)
	deOutra := mk(outra, func(m map[string]any) {})
	cancelado := mk(c, func(m map[string]any) {})
	esperar(t, acao(t, c.A, cancelado, "cancelar", nil), 200)

	// O banco é compartilhado entre os testes: todo assert do feed é restrito à academia do cenário.
	feed := func(cen *cenario, q string) resp {
		return esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios?limite=100&id_academia=%d%s", cen.AcademiaID, q), viewer.Token, nil), 200)
	}
	r := feed(c, "")
	if !contemID(r, corrida) || !contemID(r, cross) || !contemID(feed(outra, ""), deOutra) {
		t.Error("abertos e futuros devem estar no feed")
	}
	if contemID(r, passado) || contemID(r, cancelado) {
		t.Error("evento passado e cancelado não aparecem no feed padrão")
	}
	if contemID(r, deOutra) {
		t.Error("filtro id_academia")
	}
	if r = feed(c, "&categoria=crossfit"); !contemID(r, cross) || contemID(r, corrida) {
		t.Error("filtro categoria")
	}
	if r = esperar(t, api(t, "GET", "/api/desafios?limite=100&q=zebra", viewer.Token, nil), 200); !contemID(r, corrida) || contemID(r, cross) {
		t.Error("busca q por título")
	}
	nomeOutra := detalhe(t, viewer, deOutra)["academia"].(map[string]any)["nome_fantasia"].(string)
	if r = esperar(t, api(t, "GET", "/api/desafios?limite=100&q="+strings.ReplaceAll(nomeOutra, " ", "%20"), viewer.Token, nil), 200); !contemID(r, deOutra) || contemID(r, corrida) {
		t.Error("busca q por nome da academia")
	}
	if r = feed(c, "&q=%25"); contemID(r, corrida) {
		t.Error("o curinga % não pode virar 'tudo'")
	}
	// status não aberto: só quem está envolvido
	if r = esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios?status=cancelado&limite=100&id_academia=%d", c.AcademiaID), viewer.Token, nil), 200); contemID(r, cancelado) {
		t.Error("cancelado de terceiros não pode vazar no feed")
	}
	if r = esperar(t, api(t, "GET", "/api/desafios?status=cancelado&limite=100", c.A.Token, nil), 200); !contemID(r, cancelado) {
		t.Error("a academia dona vê seus cancelados")
	}
	esperar(t, api(t, "GET", "/api/desafios?status=em_votacao", viewer.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/desafios?categoria=xadrez", viewer.Token, nil), 400)
	esperar(t, api(t, "GET", "/api/desafios?id_academia=abc", viewer.Token, nil), 400)

	// envelope paginado e ordenação por data do evento
	p := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios?limite=1&pagina=2&id_academia=%d", c.AcademiaID), viewer.Token, nil), 200).data()
	if len(p["itens"].([]any)) != 1 || p["total"].(float64) != 2 || p["pagina"].(float64) != 2 || p["limite"].(float64) != 1 {
		t.Errorf("paginação: %v", p)
	}
	item := feedItem(t, feed(c, ""), corrida)
	for _, k := range []string{"academia", "instrutor", "data_evento", "categoria", "posicoes_podio", "premiacao_prevista_centavos",
		"participantes_count", "presentes_count", "voce_participa", "pode_entrar", "pode_sair", "pode_iniciar", "pode_marcar_presenca",
		"pode_lancar_podio", "pode_cancelar", "pode_editar", "pode_avaliar", "pote_centavos", "vagas"} {
		if _, ok := item[k]; !ok {
			t.Errorf("campo %q ausente do item do feed", k)
		}
	}
	if item["pode_entrar"] != true {
		t.Error("atleta com evento futuro e vaga: pode_entrar")
	}

	// /meus: academia vê os seus (qualquer status), atleta só onde participa
	inscrever(t, corrida, viewer)
	if m := esperar(t, api(t, "GET", "/api/desafios/meus?limite=100", viewer.Token, nil), 200); !contemID(m, corrida) || contemID(m, cross) {
		t.Error("/meus do atleta")
	}
	if m := esperar(t, api(t, "GET", "/api/desafios/meus?limite=100", c.A.Token, nil), 200); !contemID(m, cancelado) || !contemID(m, cross) || contemID(m, deOutra) {
		t.Error("/meus da academia")
	}
	if m := esperar(t, api(t, "GET", "/api/desafios/meus?limite=100", c.I.Token, nil), 200); !contemID(m, corrida) || contemID(m, deOutra) {
		t.Error("/meus do instrutor lista os designados a ele")
	}
	cat := esperar(t, api(t, "GET", "/api/desafios/categorias", viewer.Token, nil), 200).Body["data"].([]any)
	if len(cat) == 0 || cat[0] != "geral" {
		t.Errorf("categorias: %v", cat)
	}
}

func feedItem(t *testing.T, r resp, id uint) map[string]any {
	t.Helper()
	for _, it := range r.data()["itens"].([]any) {
		if m := it.(map[string]any); uint(m["id"].(float64)) == id {
			return m
		}
	}
	t.Fatalf("desafio %d não está na lista", id)
	return nil
}

func TestDesafio_RemovidosVotacaoECheckinDoAtleta(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	u := usuarioComSaldo(t, 2000)
	inscrever(t, id, u)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, caso := range []struct{ metodo, rota string }{
		{"POST", "abrir-votacao"}, {"POST", "votar"}, {"POST", "apurar"}, {"POST", "checkins"}, {"GET", "checkins"},
	} {
		r := api(t, caso.metodo, fmt.Sprintf("/api/desafios/%d/%s", id, caso.rota), u.Token, map[string]any{"id_votado": c.I.ID})
		if r.Status != 404 {
			t.Errorf("%s /%s deveria ter sido removida (404), obteve %d", caso.metodo, caso.rota, r.Status)
		}
	}
}

func TestDesafio_RecomendadosUsaDTOPublico(t *testing.T) {
	c := novoCenario(t)
	destinatario := usuarioComSaldo(t, 5000)
	id := criarDesafio(t, c, 1000, 3)

	// o banco é compartilhado: tira os demais abertos do "futuro" para o desafio entrar no top 5
	db.Exec("UPDATE desafios SET data_evento = ? WHERE status = 'aberto' AND id <> ?", time.Now().Add(-24*time.Hour), id)
	itens := esperar(t, api(t, "GET", "/api/desafios/recomendados", destinatario.Token, nil), 200).Body["data"].([]any)
	var item map[string]any
	for _, it := range itens {
		if d := it.(map[string]any)["desafio"].(map[string]any); uint(d["id"].(float64)) == id {
			item = it.(map[string]any)
		}
	}
	if item == nil {
		t.Fatalf("o desafio aberto de outra academia deveria ser recomendado: %d itens", len(itens))
	}
	d := item["desafio"].(map[string]any)
	if _, ok := item["razao"]; !ok {
		t.Errorf("item recomendado sem campo 'razao': %v", item)
	}
	if _, ok := d["ID"]; ok {
		t.Errorf("desafio recomendado não deveria expor o model cru: %v", d)
	}
	if d["academia"] == nil || d["valor_entrada_centavos"] == nil || d["pode_entrar"] != true {
		t.Errorf("DTO recomendado incompleto: %v", d)
	}
	// quem já participa não recebe o próprio desafio como recomendação
	inscrever(t, id, destinatario)
	for _, it := range esperar(t, api(t, "GET", "/api/desafios/recomendados", destinatario.Token, nil), 200).Body["data"].([]any) {
		if uint(it.(map[string]any)["desafio"].(map[string]any)["id"].(float64)) == id {
			t.Error("desafio em que o usuário já está inscrito não deve ser recomendado")
		}
	}
}

// TestUsuario_HistoricoExpoeResultadoDoUsuarioConsultado confirma que GET /usuarios/:id/historico
// usa o DTO dedicado (snake_case) e mostra o resultado (vencedor/prêmio) do usuário CONSULTADO.
func TestUsuario_HistoricoExpoeResultadoDoUsuarioConsultado(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 1000, 3, 2)
	us := atletas(t, 3, 3000)
	inscrever(t, id, us...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[1], "1"), entradaPodio(us[0], "2"))), 200)

	itensB := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", us[1].ID), us[0].Token, nil), 200).Body["data"].([]any)
	if len(itensB) != 1 {
		t.Fatalf("esperava 1 desafio no histórico, obteve %d: %v", len(itensB), itensB)
	}
	hb := itensB[0].(map[string]any)
	if _, ok := hb["ID"]; ok {
		t.Errorf("histórico não deveria expor o model cru: %v", hb)
	}
	// pote 3000, pódio de 2: 70/30 = 2100/900
	if hb["vencedor"] != true || hb["premio_centavos"].(float64) != 2100 {
		t.Errorf("histórico do vencedor: %v", hb)
	}
	itensA := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", us[0].ID), us[1].Token, nil), 200).Body["data"].([]any)
	ha := itensA[0].(map[string]any)
	if ha["vencedor"] != false || ha["premio_centavos"].(float64) != 900 {
		t.Errorf("histórico do 2º colocado: %v", ha)
	}
	itensC := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", us[2].ID), us[0].Token, nil), 200).Body["data"].([]any)
	if itensC[0].(map[string]any)["premio_centavos"].(float64) != 0 {
		t.Errorf("fora do pódio não tem prêmio: %v", itensC)
	}
}

func TestDesafio_Convidar(t *testing.T) {
	c := novoCenario(t)
	a := usuarioComSaldo(t, 3000)
	amigo := novoUsuario(t)
	estranho := novoUsuario(t)
	inscrito := usuarioComSaldo(t, 3000)
	amigos(t, a, amigo)
	amigos(t, a, inscrito)
	id := criarDesafio(t, c, 1000, 3)
	inscrever(t, id, inscrito)

	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": estranho.ID}), 400) // não é amigo
	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": a.ID}), 400)
	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": inscrito.ID}), 409) // já participa
	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 200)
	esperar(t, acao(t, a, id, "convidar", map[string]any{}), 400)
	esperar(t, acao(t, c.A, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 403) // academia não convida
	esperar(t, acao(t, c.I, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 403)
	if contarNotif(t, amigo, models.NotifConviteDesafio) != 1 {
		t.Fatal("amigo convidado deveria receber o convite")
	}
	conv := listaNotif(t, amigo)[0]
	if !strings.Contains(conv["link"].(string), fmt.Sprintf("id=%d", id)) || !strings.Contains(conv["mensagem"].(string), a.Nome) {
		t.Errorf("convite: %v", conv)
	}
	// desafio iniciado: não convida mais
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, acao(t, a, id, "convidar", map[string]any{"id_amigo": amigo.ID}), 409)
}

// ─── Concorrência ────────────────────────────────────────────────────────────

// 20 inscrições paralelas em desafio com 3 vagas: só 3 entram, ninguém é cobrado a mais.
func TestDesafio_ConcorrenciaUltimaVaga(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 3)
	const n = 20
	users := atletas(t, n, 2000)

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
	for i, cd := range codigos {
		switch cd {
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
			t.Errorf("status inesperado %d", cd)
		}
	}
	if ok != 3 || cheio != n-3 {
		t.Fatalf("esperava 3 entradas e %d recusas, obteve %d e %d", n-3, ok, cheio)
	}
	var participantes, distintos int64
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ?", id).Count(&participantes)
	db.Model(&models.DesafioParticipante{}).Where("id_desafio = ?", id).Distinct("codigo_ingresso").Count(&distintos)
	if participantes != 3 || distintos != 3 {
		t.Errorf("3 participantes com 3 ingressos distintos esperados: %d / %d", participantes, distintos)
	}
	ledgerConfere(t, users...)
}

// O mesmo usuário tentando entrar 10x em paralelo paga uma única vez.
func TestDesafio_ConcorrenciaMesmoUsuarioPagaUmaVez(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 5)
	u := usuarioComSaldo(t, 5000)
	var wg sync.WaitGroup
	var ok int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if acao(t, u, id, "entrar", nil).Status == 200 {
				atomic.AddInt64(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || saldo(t, u) != 4000 {
		t.Fatalf("esperava 1 entrada e saldo 4000, obteve %d entradas e saldo %d", ok, saldo(t, u))
	}
	ledgerConfere(t, u)
}

// Lançar o pódio 8x em paralelo: um só vence; prêmios e pontos saem uma única vez.
func TestDesafio_ConcorrenciaPodioLancadoDuasVezes(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 1000, 4, 2)
	us := atletas(t, 3, 2000)
	inscrever(t, id, us...) // pote 3000 -> 2100 / 900
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, u := range us {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	body := podioBody(entradaPodio(us[0], "a"), entradaPodio(us[1], "b"))

	var wg sync.WaitGroup
	var ok, conflito int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			quem := c.I
			if i%2 == 1 {
				quem = c.A // instrutor e academia disputando
			}
			switch acao(t, quem, id, "podio", body).Status {
			case 200:
				atomic.AddInt64(&ok, 1)
			case 409:
				atomic.AddInt64(&conflito, 1)
			}
		}(i)
	}
	wg.Wait()
	if ok != 1 || conflito != 7 {
		t.Fatalf("esperava 1 sucesso e 7 conflitos, obteve %d e %d", ok, conflito)
	}
	if n, soma := contarLedger(t, id, models.TipoPremioDesafio); n != 2 || soma != 3000 {
		t.Errorf("prêmios creditados uma vez: n=%d soma=%d", n, soma)
	}
	var linhas int64
	db.Model(&models.PodioDesafio{}).Where("id_desafio = ?", id).Count(&linhas)
	if linhas != 2 {
		t.Errorf("pódio com 2 linhas, obteve %d", linhas)
	}
	if pontos(t, us[0]) != 30 || pontos(t, us[1]) != 15 || pontos(t, us[2]) != 5 {
		t.Errorf("pontos aplicados uma vez: %d %d %d", pontos(t, us[0]), pontos(t, us[1]), pontos(t, us[2]))
	}
	ledgerConfere(t, us...)
}

// Cancelar concorrente com entrar: nunca sobra dinheiro preso.
func TestDesafio_ConcorrenciaCancelarVsEntrar(t *testing.T) {
	for rodada := 0; rodada < 5; rodada++ {
		c := novoCenario(t)
		id := criarDesafio(t, c, 1000, 10)
		us := atletas(t, 6, 3000)
		var wg sync.WaitGroup
		for _, u := range us {
			wg.Add(1)
			go func(u *user) { defer wg.Done(); acao(t, u, id, "entrar", nil) }(u)
		}
		wg.Add(1)
		go func() { defer wg.Done(); acao(t, c.A, id, "cancelar", nil) }()
		wg.Wait()

		// Após cancelar, todos devem ter o saldo original (quem entrou antes foi reembolsado)
		for _, u := range us {
			if saldo(t, u) != 3000 {
				t.Fatalf("participante com dinheiro preso: saldo %d", saldo(t, u))
			}
		}
		ledgerConfere(t, us...)
	}
}

// Presença paralela do mesmo atleta (QR lido duas vezes): uma marcação, uma notificação.
func TestDesafio_ConcorrenciaPresencaDuplicada(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 500, 4)
	u := usuarioComSaldo(t, 2000)
	inscrever(t, id, u)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	cod := codigoDe(t, u, id)
	var wg sync.WaitGroup
	var novos int64
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": cod})
			if r.Status == 200 && r.data()["ja_presente"] == false {
				atomic.AddInt64(&novos, 1)
			}
		}()
	}
	wg.Wait()
	if novos != 1 || contarNotif(t, u, models.NotifPresencaConfirmada) != 1 {
		t.Errorf("uma única marcação nova esperada: %d, notificações %d", novos, contarNotif(t, u, models.NotifPresencaConfirmada))
	}
}
