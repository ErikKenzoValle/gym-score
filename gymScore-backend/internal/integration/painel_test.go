//go:build integration

package integration

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"gymScore-backend/internal/models"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// virarInstrutor ativa o perfil de instrutor autônomo do próprio usuário pela API.
func virarInstrutor(t *testing.T, u *user) uint {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/instrutores", u.Token, map[string]any{
		"cref": "123456-G/SP", "especialidade": "Funcional",
	}), 201)
	if st := r.data()["status"]; st != "pendente" {
		t.Fatalf("instrutor novo deveria nascer pendente, veio %v", st)
	}
	aprovarInstrutor(t, idDe(r))
	return idDe(r)
}

func vincular(t *testing.T, a *user, email string) resp {
	t.Helper()
	return api(t, "POST", "/api/academia/instrutores", a.Token, map[string]any{"email": email})
}

func itensDe(t *testing.T, r resp) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, it := range r.data()["itens"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func idsDe(its []map[string]any, chave string) map[uint]bool {
	m := map[uint]bool{}
	for _, it := range its {
		m[uint(it[chave].(float64))] = true
	}
	return m
}

func perfilDe(t *testing.T, u *user) string {
	t.Helper()
	return esperar(t, api(t, "GET", "/api/usuarios/me", u.Token, nil), 200).data()["perfil"].(string)
}

// ─── POST /api/instrutores (sem IDOR) ────────────────────────────────────────

func TestInstrutor_CadastroUsaOUsuarioDoJWT(t *testing.T) {
	// sem login: 401 (era público)
	esperar(t, api(t, "POST", "/api/instrutores", "", map[string]any{"cref": "123456-G/SP"}), 401)

	vitima := novoUsuario(t)
	c := novoCenario(t) // tem uma academia real para tentar o vínculo forjado
	atacante := novoUsuario(t)

	// o corpo tenta promover OUTRO usuário e vincular a uma academia: os campos são ignorados
	r := esperar(t, api(t, "POST", "/api/instrutores", atacante.Token, map[string]any{
		"cref": "123456-G/SP", "especialidade": "Funcional", "id_usuario": vitima.ID, "id_academia": c.AcademiaID,
	}), 201)
	if r.data()["autonomo"] != true {
		t.Errorf("novo instrutor deve ser autônomo: %v", r.data())
	}
	if p := perfilDe(t, vitima); p != "atleta" {
		t.Errorf("a vítima não pode ser promovida; perfil = %s", p)
	}
	// rodada 4: o perfil continua o tipo de conta; o pedido aparece em `instrutor`
	if m := esperar(t, api(t, "GET", "/api/usuarios/me", atacante.Token, nil), 200).data(); m["perfil"] != "atleta" ||
		m["instrutor"] == nil || m["instrutor"].(map[string]any)["status"] != "pendente" {
		t.Errorf("o próprio usuário deveria ter o pedido de instrutor pendente: %v", m)
	}
	var i models.Instrutor
	if err := db.Where("id_usuario = ?", atacante.ID).First(&i).Error; err != nil {
		t.Fatal(err)
	}
	if i.IDAcademia != nil {
		t.Errorf("instrutor não pode nascer vinculado a academia: %v", *i.IDAcademia)
	}
	if n := func() (n int64) { db.Model(&models.Instrutor{}).Where("id_usuario = ?", vitima.ID).Count(&n); return }(); n != 0 {
		t.Errorf("vítima ganhou registro de instrutor")
	}

	// já é instrutor
	esperar(t, api(t, "POST", "/api/instrutores", atacante.Token, map[string]any{"cref": "123456-G/SP"}), 409)
	// validação
	outro := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/instrutores", outro.Token, map[string]any{"especialidade": "x"}), 400)
	esperar(t, api(t, "POST", "/api/instrutores", outro.Token, map[string]any{"cref": "1"}), 400)
	if p := perfilDe(t, outro); p != "atleta" {
		t.Errorf("cadastro inválido não pode mudar o perfil: %s", p)
	}
	// academia não vira instrutor
	esperar(t, api(t, "POST", "/api/instrutores", c.A.Token, map[string]any{"cref": "123456-G/SP"}), 403)
}

// Rodada 4 (substitui a N4 antiga): o pedido de instrutor NÃO depende das inscrições como atleta — o
// conflito de interesse é por desafio (ver rodada4_test.go).
func TestInstrutor_PedidoComInscricaoAtivaNaoMudaOAtleta(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	a := usuarioComSaldo(t, 3000)
	inscrever(t, id, a)
	virarInstrutor(t, a)
	if p := perfilDe(t, a); p != "atleta" {
		t.Errorf("o pedido não muda o tipo de conta: %s", p)
	}
	// continua inscrito e pode sair com estorno, como atleta
	esperar(t, acao(t, a, id, "sair", nil), 200)
	ledgerConfere(t, a)
}

func TestInstrutor_DiretorioExigeLoginENaoVazaDados(t *testing.T) {
	esperar(t, api(t, "GET", "/api/instrutores", "", nil), 401)
	c := novoCenario(t)
	r := esperar(t, api(t, "GET", "/api/instrutores", c.A.Token, nil), 200)
	if strings.Contains(r.Raw, "@gymscore.test") || strings.Contains(r.Raw, "cpf") {
		t.Errorf("diretório vaza e-mail/CPF: %s", r.Raw)
	}
	achou := false
	for _, it := range r.Body["data"].([]any) {
		m := it.(map[string]any)
		if uint(m["id"].(float64)) == c.InstrutorID {
			achou = true
			if m["nome"] != c.I.Nome || m["autonomo"] != false {
				t.Errorf("item do diretório: %v", m)
			}
		}
	}
	if !achou {
		t.Errorf("instrutor do cenário não apareceu no diretório")
	}
}

// ─── /academia/instrutores ───────────────────────────────────────────────────

func TestAcademia_VincularEDesvincularInstrutor(t *testing.T) {
	a, aid := novaAcademia(t)
	b, _ := novaAcademia(t)
	ins := novoUsuario(t)
	iid := virarInstrutor(t, ins)

	// só a academia usa as rotas
	atleta := novoUsuario(t)
	for _, tk := range []string{atleta.Token, ins.Token} {
		esperar(t, api(t, "GET", "/api/academia/instrutores", tk, nil), 403)
		esperar(t, api(t, "POST", "/api/academia/instrutores", tk, map[string]any{"email": ins.Email}), 403)
		esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", iid), tk, nil), 403)
	}
	esperar(t, api(t, "GET", "/api/academia/instrutores", "", nil), 401)

	// validações do vínculo
	esperar(t, vincular(t, a, ""), 400)
	esperar(t, vincular(t, a, "nao-e-email"), 400)
	esperar(t, vincular(t, a, "ninguem@gymscore.test"), 404)
	esperar(t, vincular(t, a, atleta.Email), 404) // conta existe mas não é instrutor

	// o POST é um CONVITE: só vira vínculo quando o instrutor aceitar
	r := esperar(t, vincular(t, a, ins.Email), 201)
	d := r.data()
	if uint(d["id"].(float64)) != iid || d["email"] != ins.Email || d["cref"] != "123456-G/SP" || d["convite_pendente"] != true {
		t.Errorf("resposta do convite: %v", d)
	}
	if contarNotif(t, ins, models.NotifConviteVinculo) != 1 {
		t.Errorf("instrutor deveria ser notificado do convite")
	}
	if l := esperar(t, api(t, "GET", "/api/academia/instrutores", a.Token, nil), 200); l.data()["total"].(float64) != 0 {
		t.Errorf("convite ainda não é vínculo: %s", l.Raw)
	}
	esperar(t, vincular(t, b, ins.Email), 409) // convite pendente de outra academia
	esperar(t, api(t, "POST", "/api/instrutor/convite/aceitar", ins.Token, nil), 200)
	if contarNotif(t, a, models.NotifInstrutorVinculado) != 1 {
		t.Errorf("academia deveria ser avisada do aceite")
	}
	esperar(t, vincular(t, a, ins.Email), 409) // já vinculado (a mim)
	esperar(t, vincular(t, b, ins.Email), 409) // vinculado a outra academia

	// listagem da academia A tem o instrutor; a de B não
	lista := esperar(t, api(t, "GET", "/api/academia/instrutores", a.Token, nil), 200)
	if lista.data()["total"].(float64) != 1 || !idsDe(itensDe(t, lista), "id")[iid] {
		t.Errorf("lista da academia A: %s", lista.Raw)
	}
	if esperar(t, api(t, "GET", "/api/academia/instrutores", b.Token, nil), 200).data()["total"].(float64) != 0 {
		t.Errorf("academia B não deveria ver instrutor de A")
	}

	// B não consegue desvincular o instrutor de A (404 igual a inexistente) e o vínculo segue
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", iid), b.Token, nil), 404)
	esperar(t, api(t, "DELETE", "/api/academia/instrutores/99999999", a.Token, nil), 404)
	esperar(t, api(t, "DELETE", "/api/academia/instrutores/abc", a.Token, nil), 400)
	var i models.Instrutor
	db.First(&i, iid)
	if i.IDAcademia == nil || *i.IDAcademia != aid {
		t.Fatalf("vínculo foi alterado por outra academia: %v", i.IDAcademia)
	}

	// desvincula: notifica e volta a ser autônomo
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", iid), a.Token, nil), 200)
	db.First(&i, iid)
	if i.IDAcademia != nil {
		t.Errorf("deveria estar autônomo")
	}
	if contarNotif(t, ins, models.NotifInstrutorRemovido) != 1 {
		t.Errorf("instrutor deveria ser notificado da remoção")
	}
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", iid), a.Token, nil), 404) // já desvinculado
	// e B agora pode convidar
	esperar(t, vincular(t, b, ins.Email), 201)
}

// aceitarConvite convida pela academia e aceita pelo instrutor (vínculo completo).
func aceitarConvite(t *testing.T, a, ins *user) {
	t.Helper()
	esperar(t, vincular(t, a, ins.Email), 201)
	esperar(t, api(t, "POST", "/api/instrutor/convite/aceitar", ins.Token, nil), 200)
}

func TestConvite_InstrutorAceitaRecusaEAcademiaCancela(t *testing.T) {
	a, aid := novaAcademia(t)
	b, bid := novaAcademia(t)
	ins := novoUsuario(t)
	iid := virarInstrutor(t, ins)
	atleta := novoUsuario(t)

	// sem convite
	cv := esperar(t, api(t, "GET", "/api/instrutor/convite", ins.Token, nil), 200).data()
	if cv["convite"] != nil || cv["id_academia"] != nil {
		t.Errorf("sem convite: %v", cv)
	}
	esperar(t, api(t, "POST", "/api/instrutor/convite/aceitar", ins.Token, nil), 404)
	esperar(t, api(t, "POST", "/api/instrutor/convite/recusar", ins.Token, nil), 404)
	esperar(t, api(t, "GET", "/api/instrutor/convite", atleta.Token, nil), 403) // não é instrutor

	// A convida; o instrutor vê o convite; A lista os convites; B não pode convidar
	esperar(t, vincular(t, a, ins.Email), 201)
	esperar(t, vincular(t, a, ins.Email), 201) // repetir renova
	cv = esperar(t, api(t, "GET", "/api/instrutor/convite", ins.Token, nil), 200).data()
	if c, _ := cv["convite"].(map[string]any); c == nil || uint(c["id_academia"].(float64)) != aid || c["nome_fantasia"] == "" {
		t.Errorf("convite pendente: %v", cv)
	}
	if l := esperar(t, api(t, "GET", "/api/academia/instrutores/convites", a.Token, nil), 200); !strings.Contains(l.Raw, ins.Email) {
		t.Errorf("lista de convites de A: %s", l.Raw)
	}
	if l := esperar(t, api(t, "GET", "/api/academia/instrutores/convites", b.Token, nil), 200); strings.Contains(l.Raw, ins.Email) {
		t.Errorf("B não deveria ver o convite de A: %s", l.Raw)
	}
	esperar(t, vincular(t, b, ins.Email), 409)

	// recusa: academia avisada, convite some, B pode convidar
	esperar(t, api(t, "POST", "/api/instrutor/convite/recusar", ins.Token, nil), 200)
	if contarNotif(t, a, models.NotifConviteRecusado) != 1 {
		t.Errorf("A deveria ser avisada da recusa")
	}
	var i models.Instrutor
	db.First(&i, iid)
	if i.IDAcademiaConvite != nil || i.IDAcademia != nil {
		t.Errorf("após recusa: convite %v vínculo %v", i.IDAcademiaConvite, i.IDAcademia)
	}

	// B convida e cancela; o instrutor não consegue mais aceitar
	esperar(t, vincular(t, b, ins.Email), 201)
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/convites/%d", iid), a.Token, nil), 404) // convite não é de A
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/convites/%d", iid), b.Token, nil), 200)
	esperar(t, api(t, "POST", "/api/instrutor/convite/aceitar", ins.Token, nil), 404)

	// B convida de novo e o instrutor aceita: vinculado a B
	aceitarConvite(t, b, ins)
	db.First(&i, iid)
	if i.IDAcademia == nil || *i.IDAcademia != bid || i.IDAcademiaConvite != nil {
		t.Errorf("após aceitar: vínculo %v convite %v", i.IDAcademia, i.IDAcademiaConvite)
	}
	cv = esperar(t, api(t, "GET", "/api/instrutor/convite", ins.Token, nil), 200).data()
	if cv["convite"] != nil || uint(cv["id_academia"].(float64)) != bid {
		t.Errorf("após aceitar: %v", cv)
	}
	esperar(t, vincular(t, a, ins.Email), 409) // vinculado a outra academia
}

// Presença grava quem marcou e como (QR do ingresso x lista manual), visível no detalhe do admin.
func TestPresenca_RegistraQuemMarcouEModo(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 0, 4)
	at := atletas(t, 2, 1000)
	inscrever(t, id, at...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, presencaPorCodigo(t, c.I, id, at[0]), 200)
	esperar(t, presencaPorID(c.A, t, id, at[1]), 200)
	esperar(t, presencaPorID(c.A, t, id, at[0]), 200) // já presente: não sobrescreve quem marcou

	var ps []models.DesafioParticipante
	db.Where("id_desafio = ?", id).Find(&ps)
	for _, p := range ps {
		switch p.IDUsuario {
		case at[0].ID:
			if p.PresencaModo == nil || *p.PresencaModo != models.PresencaQR || p.PresencaMarcadaPor == nil || *p.PresencaMarcadaPor != c.I.ID {
				t.Errorf("presença por QR: %+v", p)
			}
		case at[1].ID:
			if p.PresencaModo == nil || *p.PresencaModo != models.PresencaManual || p.PresencaMarcadaPor == nil || *p.PresencaMarcadaPor != c.A.ID {
				t.Errorf("presença manual: %+v", p)
			}
		}
	}
	a := novoAdminDB(t, models.PapelAdmin)
	d := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/desafios/%d", id), a, nil), 200)
	if !strings.Contains(d.Raw, `"presenca_modo":"qr"`) || !strings.Contains(d.Raw, `"presenca_modo":"manual"`) {
		t.Errorf("detalhe do admin sem o modo da presença: %s", d.Raw)
	}
}

func TestAcademia_DesvincularBloqueadoComDesafioAtivo(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	desv := func() resp {
		return api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", c.InstrutorID), c.A.Token, nil)
	}
	r := esperar(t, desv(), 409)
	if !strings.Contains(fmt.Sprint(r.Body["error"]), fmt.Sprintf("#%d", id)) {
		t.Errorf("a mensagem deveria citar o desafio #%d: %v", id, r.Body["error"])
	}
	if lista := esperar(t, api(t, "GET", "/api/academia/instrutores", c.A.Token, nil), 200); itensDe(t, lista)[0]["desafios_ativos"].(float64) != 1 {
		t.Errorf("desafios_ativos deveria ser 1: %s", lista.Raw)
	}
	// em andamento também bloqueia
	at := atletas(t, 2, 3000)
	inscrever(t, id, at...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, desv(), 409)
	// encerrado libera
	for _, u := range at {
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(at[0], "1"), entradaPodio(at[1], "2"))), 200)
	esperar(t, desv(), 200)
	if contarNotif(t, c.I, models.NotifInstrutorRemovido) != 1 {
		t.Errorf("notificação de remoção ausente")
	}
}

func TestAcademia_TrocarInstrutorLiberaODesvinculo(t *testing.T) {
	c := novoCenario(t)
	i2, id2 := novoInstrutorDe(t, c.AcademiaID)
	_ = i2
	id := criarDesafio(t, c, 0, 4)
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", c.InstrutorID), c.A.Token, nil), 409)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", id), c.A.Token, map[string]any{"id_instrutor": id2}), 409) // equipe completa (1 vaga)
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, c.InstrutorID), c.A.Token, nil), 200)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", id), c.A.Token, map[string]any{"id_instrutor": id2}), 200)
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", c.InstrutorID), c.A.Token, nil), 200)
	// o novo instrutor passou a ter o desafio ativo e não pode sair
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/academia/instrutores/%d", id2), c.A.Token, nil), 409)
}

func TestAcademia_ConcorrenciaDuasAcademiasVinculamOMesmoInstrutor(t *testing.T) {
	a, _ := novaAcademia(t)
	b, _ := novaAcademia(t)
	ins := novoUsuario(t)
	virarInstrutor(t, ins)
	res := make(chan int, 2)
	for _, ac := range []*user{a, b} {
		go func(ac *user) { res <- vincular(t, ac, ins.Email).Status }(ac)
	}
	s1, s2 := <-res, <-res
	if !((s1 == 201 && s2 == 409) || (s1 == 409 && s2 == 201)) {
		t.Errorf("esperava um 201 e um 409, obteve %d e %d", s1, s2)
	}
}

// ─── /academia/desafios e /academia/dashboard ────────────────────────────────

func TestAcademia_DesafiosSoDaPropriaAcademia(t *testing.T) {
	c1, c2 := novoCenario(t), novoCenario(t)
	d1 := criarDesafio(t, c1, 1000, 4)
	d1b := criarDesafio(t, c1, 0, 4)
	d2 := criarDesafio(t, c2, 1000, 4)
	esperar(t, acao(t, c1.A, d1b, "cancelar", nil), 200)

	r := esperar(t, api(t, "GET", "/api/academia/desafios", c1.A.Token, nil), 200)
	ids := idsDe(itensDe(t, r), "id")
	if !ids[d1] || !ids[d1b] || ids[d2] || r.data()["total"].(float64) != 2 {
		t.Errorf("academia 1 deveria ver só os seus (%d, %d): %s", d1, d1b, r.Raw)
	}
	// DesafioResponse completo + flags da academia dona
	for _, it := range itensDe(t, r) {
		if it["academia"].(map[string]any)["id"].(float64) != float64(c1.AcademiaID) {
			t.Errorf("desafio de outra academia na lista: %v", it)
		}
		if uint(it["id"].(float64)) == d1 && (it["pode_cancelar"] != true || it["pode_editar"] != true) {
			t.Errorf("flags da academia dona: %v", it)
		}
	}
	// filtro de status
	ab := esperar(t, api(t, "GET", "/api/academia/desafios?status=aberto", c1.A.Token, nil), 200)
	if its := itensDe(t, ab); len(its) != 1 || uint(its[0]["id"].(float64)) != d1 {
		t.Errorf("status=aberto: %s", ab.Raw)
	}
	ca := esperar(t, api(t, "GET", "/api/academia/desafios?status=cancelado", c1.A.Token, nil), 200)
	if its := itensDe(t, ca); len(its) != 1 || uint(its[0]["id"].(float64)) != d1b {
		t.Errorf("status=cancelado: %s", ca.Raw)
	}
	esperar(t, api(t, "GET", "/api/academia/desafios?status=qualquer", c1.A.Token, nil), 400)
	// paginação
	p := esperar(t, api(t, "GET", "/api/academia/desafios?limite=1&pagina=2", c1.A.Token, nil), 200)
	if len(itensDe(t, p)) != 1 || p.data()["total"].(float64) != 2 || p.data()["pagina"].(float64) != 2 {
		t.Errorf("paginação: %s", p.Raw)
	}
	// perfis errados
	esperar(t, api(t, "GET", "/api/academia/desafios", c1.I.Token, nil), 403)
	esperar(t, api(t, "GET", "/api/academia/desafios", usuarioComSaldo(t, 1000).Token, nil), 403)
	esperar(t, api(t, "GET", "/api/academia/desafios", "", nil), 401)
}

func TestAcademia_DashboardSoComDadosDaPropriaAcademia(t *testing.T) {
	c1, c2 := novoCenario(t), novoCenario(t)

	// academia sem nada: tudo zerado, 4 chaves de status
	vazio := esperar(t, api(t, "GET", "/api/academia/dashboard", c1.A.Token, nil), 200).data()
	if vazio["total_desafios"].(float64) != 0 || vazio["taxa_presenca"].(float64) != 0 || vazio["pote_em_jogo_centavos"].(float64) != 0 ||
		len(vazio["desafios_por_status"].(map[string]any)) != 4 || len(vazio["proximos_eventos"].([]any)) != 0 {
		t.Errorf("dashboard vazio: %v", vazio)
	}

	// academia 2 tem movimento que NÃO pode aparecer na 1
	d2 := criarDesafio(t, c2, 5000, 4)
	inscrever(t, d2, atletas(t, 2, 10000)...)

	// academia 1: A (aberto, 2 inscritos, 1000), B (em andamento, 3 inscritos 1 presente, 2000), C (encerrado), D (cancelado)
	a := criarDesafio(t, c1, 1000, 4)
	b := criarDesafio(t, c1, 2000, 4)
	cc := criarDesafioPodio(t, c1, 0, 4, 1)
	dd := criarDesafio(t, c1, 500, 4)
	inscrever(t, a, atletas(t, 2, 5000)...)
	ub := atletas(t, 3, 5000)
	inscrever(t, b, ub...)
	esperar(t, acao(t, c1.I, b, "iniciar", nil), 200)
	esperar(t, presencaPorID(c1.I, t, b, ub[0]), 200)
	uc := atletas(t, 1, 1000)
	inscrever(t, cc, uc...)
	esperar(t, acao(t, c1.I, cc, "iniciar", nil), 200)
	esperar(t, presencaPorID(c1.I, t, cc, uc[0]), 200)
	esperar(t, acao(t, c1.I, cc, "podio", podioBody(entradaPodio(uc[0], "9:59"))), 200)
	esperar(t, acao(t, uc[0], cc, "avaliar", map[string]any{"nota": 4}), 201)
	inscrever(t, dd, atletas(t, 1, 5000)...)
	esperar(t, acao(t, c1.A, dd, "cancelar", nil), 200)

	d := esperar(t, api(t, "GET", "/api/academia/dashboard", c1.A.Token, nil), 200).data()
	st := d["desafios_por_status"].(map[string]any)
	if st["aberto"] != 1.0 || st["em_andamento"] != 1.0 || st["encerrado"] != 1.0 || st["cancelado"] != 1.0 || d["total_desafios"] != 4.0 {
		t.Errorf("desafios por status: %v total=%v", st, d["total_desafios"])
	}
	// inscritos no mês: 2 + 3 + 1 (o cancelado não conta)
	if d["inscritos_mes"] != 6.0 {
		t.Errorf("inscritos_mes = %v", d["inscritos_mes"])
	}
	// pote em jogo: aberto 2x1000 + em andamento 3x2000
	if d["pote_em_jogo_centavos"] != 8000.0 || d["pote_em_jogo"] != 80.0 {
		t.Errorf("pote em jogo = %v / %v", d["pote_em_jogo_centavos"], d["pote_em_jogo"])
	}
	// presença: em andamento (1 de 3) + encerrado (1 de 1) = 2 de 4 = 50%
	if d["taxa_presenca"] != 50.0 {
		t.Errorf("taxa_presenca = %v", d["taxa_presenca"])
	}
	// avaliação só dos desafios desta academia
	if d["avaliacao_media"] != 4.0 || d["avaliacoes_total"] != 1.0 {
		t.Errorf("avaliação = %v (%v)", d["avaliacao_media"], d["avaliacoes_total"])
	}
	if d["instrutores_vinculados"] != 1.0 {
		t.Errorf("instrutores_vinculados = %v", d["instrutores_vinculados"])
	}
	if ac := d["academia"].(map[string]any); ac["id"] != float64(c1.AcademiaID) {
		t.Errorf("academia do dashboard: %v", ac)
	}
	prox := d["proximos_eventos"].([]any)
	if len(prox) != 1 || uint(prox[0].(map[string]any)["id"].(float64)) != a {
		t.Errorf("próximos eventos deveriam ter só o desafio aberto %d: %v", a, prox)
	}
	// a academia 2 vê só o dela
	d2r := esperar(t, api(t, "GET", "/api/academia/dashboard", c2.A.Token, nil), 200).data()
	if d2r["total_desafios"] != 1.0 || d2r["pote_em_jogo_centavos"] != 10000.0 || d2r["avaliacoes_total"] != 0.0 {
		t.Errorf("dashboard da academia 2: %v", d2r)
	}
	esperar(t, api(t, "GET", "/api/academia/dashboard", c1.I.Token, nil), 403)
	esperar(t, api(t, "GET", "/api/academia/dashboard", "", nil), 401)
}

// ─── /instrutor/agenda ───────────────────────────────────────────────────────

func TestInstrutor_AgendaSoDosDesafiosDesignados(t *testing.T) {
	c := novoCenario(t)
	i2, id2 := novoInstrutorDe(t, c.AcademiaID)
	outro := novoCenario(t) // outra academia, outro instrutor

	mine := criarDesafio(t, c, 1000, 4)
	mine2 := criarDesafio(t, c, 0, 4)
	corpo := corpoDesafio(c, 0, 4, 2)
	corpo["id_instrutor"] = id2
	theirs := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpo), 201))
	foreign := criarDesafio(t, outro, 0, 4)

	r := esperar(t, api(t, "GET", "/api/instrutor/agenda", c.I.Token, nil), 200)
	ids := idsDe(itensDe(t, r), "id")
	if !ids[mine] || !ids[mine2] || ids[theirs] || ids[foreign] || r.data()["total"].(float64) != 2 {
		t.Errorf("agenda do instrutor 1: %s", r.Raw)
	}
	// DesafioResponse (snake_case), com flags de instrutor
	it := itensDe(t, r)[0]
	for _, k := range []string{"titulo", "data_evento", "academia", "instrutor", "participantes_count", "pode_iniciar", "pode_marcar_presenca", "premiacao_prevista_centavos"} {
		if _, ok := it[k]; !ok {
			t.Errorf("campo %q ausente da agenda: %v", k, it)
		}
	}
	if _, cru := it["ID"]; cru {
		t.Errorf("agenda devolveu o model cru")
	}
	// ordenada por data_evento crescente
	if its := itensDe(t, r); its[0]["data_evento"].(string) > its[1]["data_evento"].(string) {
		t.Errorf("agenda fora de ordem: %v", its)
	}
	// o outro instrutor vê só o dele
	r2 := esperar(t, api(t, "GET", "/api/instrutor/agenda", i2.Token, nil), 200)
	if ids2 := idsDe(itensDe(t, r2), "id"); !ids2[theirs] || len(ids2) != 1 {
		t.Errorf("agenda do instrutor 2: %s", r2.Raw)
	}

	// status: encerrado/cancelado ficam fora do padrão e aparecem quando pedidos
	esperar(t, acao(t, c.A, mine2, "cancelar", nil), 200)
	r = esperar(t, api(t, "GET", "/api/instrutor/agenda", c.I.Token, nil), 200)
	if ids := idsDe(itensDe(t, r), "id"); ids[mine2] || !ids[mine] {
		t.Errorf("cancelado não deveria estar na agenda padrão: %s", r.Raw)
	}
	r = esperar(t, api(t, "GET", "/api/instrutor/agenda?status=cancelado", c.I.Token, nil), 200)
	if ids := idsDe(itensDe(t, r), "id"); !ids[mine2] || len(ids) != 1 {
		t.Errorf("status=cancelado: %s", r.Raw)
	}
	esperar(t, api(t, "GET", "/api/instrutor/agenda?status=xx", c.I.Token, nil), 400)

	// instrutor autônomo sem desafios: envelope vazio; atleta/academia: 403
	solo := novoUsuario(t)
	virarInstrutor(t, solo)
	modoInstrutor(t, solo)
	vazio := esperar(t, api(t, "GET", "/api/instrutor/agenda", solo.Token, nil), 200)
	if len(itensDe(t, vazio)) != 0 || vazio.data()["total"].(float64) != 0 {
		t.Errorf("agenda vazia: %s", vazio.Raw)
	}
	esperar(t, api(t, "GET", "/api/instrutor/agenda", usuarioComSaldo(t, 1000).Token, nil), 403)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", c.A.Token, nil), 403)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", "", nil), 401)
}

// ─── /usuarios/ingressos e /usuarios/:id/historico ───────────────────────────

func TestUsuario_IngressosTrazOCodigoReal(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 1000, 4)
	outroDesafio := criarDesafio(t, c, 0, 4)
	a := usuarioComSaldo(t, 3000)
	b := usuarioComSaldo(t, 3000)
	inscrever(t, id, a, b)
	inscrever(t, outroDesafio, b)

	r := esperar(t, api(t, "GET", "/api/usuarios/ingressos", a.Token, nil), 200)
	its := []map[string]any{}
	for _, x := range r.Body["data"].([]any) {
		its = append(its, x.(map[string]any))
	}
	if len(its) != 1 {
		t.Fatalf("A tem 1 ingresso, obteve %d: %s", len(its), r.Raw)
	}
	ing := its[0]
	codigo := codigoDe(t, a, id)
	if ing["codigo_ingresso"] != codigo || !reCodigo.MatchString(codigo) {
		t.Errorf("codigo_ingresso = %v, esperava o seu_codigo_ingresso %q", ing["codigo_ingresso"], codigo)
	}
	if strings.Contains(r.Raw, "AC-") {
		t.Errorf("código fictício AC-... ainda presente: %s", r.Raw)
	}
	if ing["status"] != "aberto" || ing["presente"] != false || ing["data"] == nil || ing["categoria"] != "corrida" ||
		ing["valor_entrada_centavos"] != 1000.0 || uint(ing["id_desafio"].(float64)) != id {
		t.Errorf("campos do ingresso: %v", ing)
	}
	if ac, _ := ing["academia"].(map[string]any); ac == nil || ac["id"] != float64(c.AcademiaID) || ac["nome_fantasia"] == "" {
		t.Errorf("academia do ingresso: %v", ing["academia"])
	}
	// B tem 2 e não vê o código de A
	rb := esperar(t, api(t, "GET", "/api/usuarios/ingressos", b.Token, nil), 200)
	if len(rb.Body["data"].([]any)) != 2 || strings.Contains(rb.Raw, codigo) {
		t.Errorf("ingressos de B: %s", rb.Raw)
	}

	// o código do ingresso vale na leitura do QR e o ingresso passa a mostrar a presença
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, acao(t, c.I, id, "presenca", map[string]any{"codigo_ingresso": ing["codigo_ingresso"]}), 200)
	ing = esperar(t, api(t, "GET", "/api/usuarios/ingressos", a.Token, nil), 200).Body["data"].([]any)[0].(map[string]any)
	if ing["presente"] != true || ing["presente_em"] == nil {
		t.Errorf("ingresso deveria mostrar a presença: %v", ing)
	}
	// sem ingressos: lista vazia
	vazio := esperar(t, api(t, "GET", "/api/usuarios/ingressos", novoUsuario(t).Token, nil), 200)
	if l, ok := vazio.Body["data"].([]any); !ok || len(l) != 0 {
		t.Errorf("sem ingressos deveria ser []: %s", vazio.Raw)
	}
	esperar(t, api(t, "GET", "/api/usuarios/ingressos", "", nil), 401)
}

func TestUsuario_HistoricoComPosicaoMarcaEPremio(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 1000, 4, 3)
	us := atletas(t, 4, 3000)
	inscrever(t, id, us...)
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	for _, u := range us[:3] { // us[3] ausente
		esperar(t, presencaPorID(c.I, t, id, u), 200)
	}
	esperar(t, acao(t, c.I, id, "podio", podioBody(entradaPodio(us[2], "10:01"), entradaPodio(us[0], "10:30"), entradaPodio(us[1], "11:00"))), 200)
	// um desafio aberto e outro cancelado não entram no histórico
	criarDesafio(t, c, 0, 4)

	hist := func(u *user) []map[string]any {
		r := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", u.ID), us[0].Token, nil), 200)
		out := []map[string]any{}
		for _, x := range r.Body["data"].([]any) {
			out = append(out, x.(map[string]any))
		}
		return out
	}
	// pote 4000, pódio de 3 (60/30/10): 2400 / 1200 / 400
	cenarios := []struct {
		u        *user
		posicao  float64
		marca    string
		premio   float64
		vencedor bool
		presente bool
	}{
		{us[2], 1, "10:01", 2400, true, true},
		{us[0], 2, "10:30", 1200, false, true},
		{us[1], 3, "11:00", 400, false, true},
		{us[3], 0, "", 0, false, false},
	}
	for _, cn := range cenarios {
		h := hist(cn.u)
		if len(h) != 1 {
			t.Fatalf("histórico de %d: %v", cn.u.ID, h)
		}
		x := h[0]
		if cn.posicao == 0 {
			if x["posicao"] != nil || x["marca"] != "" {
				t.Errorf("fora do pódio não tem posição/marca: %v", x)
			}
		} else if x["posicao"] != cn.posicao || x["marca"] != cn.marca {
			t.Errorf("posição/marca de %d: %v", cn.u.ID, x)
		}
		if x["premio_centavos"] != cn.premio || x["vencedor"] != cn.vencedor || x["presente"] != cn.presente ||
			x["status"] != "encerrado" || x["titulo"] == "" || x["participantes_count"] != 4.0 || x["categoria"] != "corrida" ||
			math.Abs(x["premio"].(float64)-cn.premio/100) > 1e-9 {
			t.Errorf("histórico de %d: %v", cn.u.ID, x)
		}
		if ac, _ := x["academia"].(map[string]any); ac == nil || ac["id"] != float64(c.AcademiaID) {
			t.Errorf("academia no histórico: %v", x["academia"])
		}
		// nada dos outros participantes
		raw := fmt.Sprint(x)
		for _, o := range cenarios {
			if o.u != cn.u && strings.Contains(raw, o.u.Nome) {
				t.Errorf("histórico expõe outro participante: %s", raw)
			}
		}
	}
	// usuário sem histórico: lista vazia
	r := esperar(t, api(t, "GET", fmt.Sprintf("/api/usuarios/%d/historico", novoUsuario(t).ID), us[0].Token, nil), 200)
	if l, ok := r.Body["data"].([]any); !ok || len(l) != 0 {
		t.Errorf("histórico vazio deveria ser []: %s", r.Raw)
	}
}
