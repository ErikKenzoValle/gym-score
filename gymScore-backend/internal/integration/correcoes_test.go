//go:build integration

package integration

// Testes das correções da rodada "DEV_BACKEND parte 1": saque sem pagamento duplo (B1), aprovação de
// academia/instrutor (B2), bloqueio (B7), job de expiração (B5), reconciliação PIX (B6) e
// elegibilidade do pódio (B11).

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"gymScore-backend/internal/app"
	"gymScore-backend/internal/jobs"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"github.com/gofiber/fiber/v2"
)

// servidorComJobs monta a app com as tarefas periódicas (para rodar o job na mão).
func servidorComJobs(t *testing.T, modoTaxa string, tarifa int64, carencia time.Duration) (*fiber.App, []jobs.Tarefa) {
	t.Helper()
	cfg := *cfgBase
	cfg.PlataformaTaxaModo, cfg.PlataformaTaxaInscricaoPct, cfg.PlataformaTarifaSaqueCentavos = modoTaxa, 10, tarifa
	cfg.DesafioCarencia = carencia
	return app.ConstruirComJobs(&cfg, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
}

func saqueDoBanco(t *testing.T, id uint) models.Transacao {
	t.Helper()
	var tr models.Transacao
	if err := db.First(&tr, id).Error; err != nil {
		t.Fatal(err)
	}
	return tr
}

// ─── B1: saque ───────────────────────────────────────────────────────────────

// Erro ambíguo do Asaas (timeout, 5xx, ou criada e resposta perdida): o saque fica pending, debitado,
// marcado para revisão, e NEM o valor NEM a tarifa voltam — nem na hora, nem no polling depois do
// prazo, nem no job de reconciliação.
func TestSaque_ErroAmbiguoNaoReembolsaValorNemTarifa(t *testing.T) {
	srv, tarefas := servidorComJobs(t, "inscricao_saque", 200, 48*time.Hour)
	for _, modo := range []string{modoTimeout, modo5xx, modoCriadaSemResposta} {
		t.Run(modo, func(t *testing.T) {
			u := usuarioComSaldo(t, 5000)
			_, receitaAntes := contarReceita(t, models.ReceitaTarifaSaque)
			asaas.modoTransf(modo)
			r := sacarS(t, srv, u, 1000)
			asaas.modoTransf("")
			if r.Status != 201 {
				t.Fatalf("ambíguo não é erro para o usuário (saque em processamento): %d %s", r.Status, r.Raw)
			}
			id := idDe(r)
			if r.data()["status"] != "pending" || r.data()["em_revisao"] != true || num(r.data(), "tarifa_centavos") != 200 {
				t.Errorf("resposta: %v", r.data())
			}
			if saldo(t, u) != 3800 {
				t.Fatalf("valor e tarifa continuam debitados: saldo %d", saldo(t, u))
			}
			if asaas.ultimoValor != 1000 {
				t.Errorf("o Asaas recebe o valor em centavos sem a tarifa: %d", asaas.ultimoValor)
			}
			if modo == modoCriadaSemResposta {
				if _, existe := asaas.transfRefs[fmt.Sprintf("saque-%d", id)]; !existe {
					t.Errorf("a transferência deveria existir no Asaas com externalReference saque-%d", id)
				}
			}
			tr := saqueDoBanco(t, id)
			if tr.Status != "pending" || !tr.EmRevisao || tr.RevisaoMotivo == nil || tr.AsaasTransferID != nil {
				t.Errorf("saque no banco: status=%s em_revisao=%v motivo=%v transfer=%v", tr.Status, tr.EmRevisao, tr.RevisaoMotivo, tr.AsaasTransferID)
			}

			// depois do prazo do órfão: polling e job não reembolsam
			db.Exec("UPDATE transacoes SET created_at = ? WHERE id = ?", time.Now().Add(-time.Hour), id)
			c := esperar(t, chamar(t, srv, "GET", fmt.Sprintf("/api/carteira/saque/%d", id), u.Token, nil), 200)
			if c.data()["status"] != "pending" || c.data()["em_revisao"] != true {
				t.Errorf("polling não muda nada: %v", c.data())
			}
			jobs.ExecutarAgora(tarefas...)
			if saldo(t, u) != 3800 {
				t.Errorf("nada pode voltar sem confirmação: saldo %d", saldo(t, u))
			}
			if _, depois := contarReceita(t, models.ReceitaTarifaSaque); depois-receitaAntes != 200 {
				t.Errorf("a receita da tarifa fica: +%d", depois-receitaAntes)
			}
			if n := contarNotif(t, u, models.NotifSaqueFalhou); n != 0 {
				t.Errorf("não notifica falha: %d", n)
			}
			ledgerConfere(t, u)
		})
	}
}

// Erro definitivo (4xx) reembolsa valor + tarifa na hora (o par do teste acima).
func TestSaque_ErroDefinitivoReembolsaValorETarifa(t *testing.T) {
	srv := servidorTaxa("inscricao_saque", 10, 200)
	u := usuarioComSaldo(t, 5000)
	asaas.modoTransf(modoDefinitivo)
	esperar(t, sacarS(t, srv, u, 1000), 502)
	asaas.modoTransf("")
	if saldo(t, u) != 5000 {
		t.Errorf("valor e tarifa voltam: %d", saldo(t, u))
	}
	var tr models.Transacao
	db.Where("id_usuario = ? AND tipo = 'saque'", u.ID).First(&tr)
	if tr.Status != "failed" || tr.EmRevisao {
		t.Errorf("saque failed sem revisão: %+v", tr)
	}
	ledgerConfere(t, u)
}

// Débito feito mas sem transferência gravada (processo caiu): o órfão NÃO é reembolsado sozinho; após
// o prazo vai para revisão do admin (polling e job), e segue debitado.
func TestSaque_OrfaoVaiParaRevisaoSemReembolso(t *testing.T) {
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	u := usuarioComSaldo(t, 2000)
	id := idDe(esperar(t, sacar(t, u, 1500), 201))
	db.Exec("UPDATE transacoes SET asaas_transfer_id = NULL WHERE id = ?", id)

	if c := esperar(t, consultarSaque(t, u, id), 200); c.data()["status"] != "pending" || c.data()["em_revisao"] != false {
		t.Fatalf("dentro do prazo segue pending sem revisão: %v", c.data())
	}
	db.Exec("UPDATE transacoes SET created_at = ? WHERE id = ?", time.Now().Add(-10*time.Minute), id)
	c := esperar(t, consultarSaque(t, u, id), 200)
	if c.data()["status"] != "pending" || c.data()["em_revisao"] != true {
		t.Fatalf("órfão antigo vai para revisão, sem reembolso: %v", c.data())
	}
	jobs.ExecutarAgora(tarefas...)
	if saldo(t, u) != 500 {
		t.Errorf("saldo continua debitado: %d", saldo(t, u))
	}
	// o job também marca órfãos que ninguém consultou
	id2 := idDe(esperar(t, sacar(t, u, 500), 201))
	db.Exec("UPDATE transacoes SET asaas_transfer_id = NULL, created_at = ? WHERE id = ?", time.Now().Add(-10*time.Minute), id2)
	jobs.ExecutarAgora(tarefas...)
	if tr := saqueDoBanco(t, id2); tr.Status != "pending" || !tr.EmRevisao {
		t.Errorf("job marca o órfão: %+v", tr)
	}
	if saldo(t, u) != 0 {
		t.Errorf("nada volta: %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}

// O job de reconciliação conclui/falha saques com transferência pelo status do Asaas, sem polling.
func TestReconciliacao_SaquesComTransferencia(t *testing.T) {
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	u := usuarioComSaldo(t, 5000)
	r1 := esperar(t, sacar(t, u, 1000), 201)
	r2 := esperar(t, sacar(t, u, 1000), 201)
	asaas.definirTransf(r1.data()["asaas_transfer_id"].(string), "DONE")
	asaas.definirTransf(r2.data()["asaas_transfer_id"].(string), "FAILED")
	db.Exec("UPDATE transacoes SET created_at = ? WHERE id IN ?", time.Now().Add(-time.Hour), []uint{idDe(r1), idDe(r2)})
	jobs.ExecutarAgora(tarefas...)
	jobs.ExecutarAgora(tarefas...) // idempotente
	if a, b := saqueDoBanco(t, idDe(r1)).Status, saqueDoBanco(t, idDe(r2)).Status; a != "concluido" || b != "failed" {
		t.Errorf("status após reconciliação: %s / %s", a, b)
	}
	if saldo(t, u) != 4000 {
		t.Errorf("só o saque falho volta: %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}

// ─── B6: depósito ────────────────────────────────────────────────────────────

func TestDeposito_LinhaLocalAntesDaCobrancaEReconciliacao(t *testing.T) {
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	u := novoUsuario(t)

	// cobrança falha: a linha local (criada antes) fica failed, sem crédito
	asaas.mu.Lock()
	asaas.cobrancaFalha = true
	asaas.mu.Unlock()
	r := api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 1000, "cpf": cpfValido()})
	asaas.mu.Lock()
	asaas.cobrancaFalha = false
	asaas.mu.Unlock()
	if r.Status < 500 {
		t.Fatalf("falha do Asaas deveria ser 5xx: %d", r.Status)
	}
	var falhos int64
	db.Model(&models.Transacao{}).Where("id_usuario = ? AND tipo = 'deposito' AND status = 'failed' AND asaas_payment_id IS NULL", u.ID).Count(&falhos)
	if falhos != 1 || saldo(t, u) != 0 {
		t.Errorf("linha local failed sem crédito: n=%d saldo=%d", falhos, saldo(t, u))
	}

	// pago no Asaas sem polling nem webhook: o job credita uma vez
	pr := esperar(t, api(t, "POST", "/api/pagamento/pix", u.Token, map[string]any{"valor_centavos": 2500, "cpf": cpfValido()}), 200)
	pid := pr.data()["asaas_payment_id"].(string)
	asaas.pagar(pid)
	db.Exec("UPDATE transacoes SET created_at = ? WHERE asaas_payment_id = ?", time.Now().Add(-time.Hour), pid)
	jobs.ExecutarAgora(tarefas...)
	jobs.ExecutarAgora(tarefas...)
	if saldo(t, u) != 2500 {
		t.Errorf("reconciliação credita uma vez: %d", saldo(t, u))
	}
	ledgerConfere(t, u)
}

// ─── B2: aprovação ───────────────────────────────────────────────────────────

func TestAprovacao_AcademiaEInstrutorPendentesNaoOperam(t *testing.T) {
	// academia recém-cadastrada (sem aprovar)
	n := seq.Add(1)
	a := &user{Email: fmt.Sprintf("qa+pend%d_%d@gymscore.test", n, cnpjSeq.Add(1)), Senha: senhaPadrao}
	r := esperar(t, api(t, "POST", "/api/academias", "", map[string]any{
		"nome": "Pendente", "email": a.Email, "senha": a.Senha, "razao_social": "Academia Pendente LTDA",
		"nome_fantasia": "Pendente", "cnpj": cnpjUnico(),
		"cep": "01310-100", "logradouro": "Av. Paulista", "numero": "1000", "bairro": "Bela Vista", "cidade": "São Paulo", "uf": "SP",
	}), 201)
	acadID := idDe(r)
	login := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": a.Email, "senha": a.Senha}), 200)
	a.Token = tokenDaSessao(t, login)
	if _, temCPF := login.data()["usuario"].(map[string]any)["cpf"]; temCPF {
		t.Error("login não devolve CPF")
	}
	ins, insID := novoInstrutorDe(t, acadID)
	c := &cenario{A: a, AcademiaID: acadID, I: ins, InstrutorID: insID}

	p := esperar(t, api(t, "GET", "/api/academia/dashboard", a.Token, nil), 403)
	if p.Body["status_cadastro"] != "pendente" {
		t.Errorf("403 informa o status: %v", p.Body)
	}
	esperar(t, api(t, "POST", "/api/desafios", a.Token, corpoDesafio(c, 0, 4, 3)), 403)
	// duplicidade vira 409 e corpo inválido 400 (validação na borda)
	esperar(t, api(t, "POST", "/api/academias", "", map[string]any{
		"nome": "Outra", "email": a.Email, "senha": a.Senha, "razao_social": "Xx LTDA", "nome_fantasia": "Xx", "cnpj": cnpjUnico(),
		"cep": "01310-100", "logradouro": "Av. Paulista", "numero": "1000", "bairro": "Bela Vista", "cidade": "São Paulo", "uf": "SP",
	}), 409)
	esperar(t, api(t, "POST", "/api/academias", "", map[string]any{"nome": "x", "email": "nao-e-email"}), 400)

	aprovarAcademia(t, acadID)
	esperar(t, api(t, "GET", "/api/academia/dashboard", a.Token, nil), 200)
	id := criarDesafio(t, c, 0, 4)

	// instrutor pendente: não opera e não pode ser designado
	u := novoUsuario(t)
	r = esperar(t, api(t, "POST", "/api/instrutores", u.Token, map[string]any{"cref": "654321-G/SP"}), 201)
	pendID := idDe(r)
	if r.data()["status"] != "pendente" {
		t.Errorf("instrutor nasce pendente: %v", r.data())
	}
	esperar(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 403)
	esperar(t, api(t, "GET", "/api/instrutor/vagas", u.Token, nil), 403)
	db.Exec("UPDATE instrutores SET id_academia = ? WHERE id = ?", acadID, pendID)
	corpo := corpoDesafio(c, 0, 4, 3)
	corpo["id_instrutor"] = pendID
	esperar(t, api(t, "POST", "/api/desafios", a.Token, corpo), 400)
	// pendente não opera o desafio nem se for o designado
	db.Exec("UPDATE desafios SET id_instrutor = ? WHERE id = ?", pendID, id)
	esperar(t, acao(t, u, id, "iniciar", nil), 403)
	aprovarInstrutor(t, pendID)
	// rodada 4: aprovado, entra no modo instrutor e só então opera como instrutor
	esperar(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 403)
	modoInstrutor(t, u)
	esperar(t, api(t, "GET", "/api/instrutor/agenda", u.Token, nil), 200)
}

// N3: o instrutor encerra o próprio vínculo (bloqueado enquanto designado em desafio ativo).
func TestInstrutor_SeDesvincula(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafio(t, c, 0, 4)
	esperar(t, api(t, "DELETE", "/api/instrutor/vinculo", c.I.Token, nil), 409)
	esperar(t, acao(t, c.A, id, "cancelar", nil), 200)
	esperar(t, api(t, "DELETE", "/api/instrutor/vinculo", c.I.Token, nil), 200)
	esperar(t, api(t, "DELETE", "/api/instrutor/vinculo", c.I.Token, nil), 409)
	if l := esperar(t, api(t, "GET", "/api/academia/instrutores", c.A.Token, nil), 200); l.data()["total"].(float64) != 0 {
		t.Errorf("academia sem instrutores: %v", l.data())
	}
	if n := contarNotif(t, c.A, models.NotifInstrutorRemovido); n != 1 {
		t.Errorf("academia avisada: %d", n)
	}
	esperar(t, api(t, "DELETE", "/api/instrutor/vinculo", novoUsuario(t).Token, nil), 403)
}

// ─── B7: bloqueio ────────────────────────────────────────────────────────────

func TestBloqueio_UsuarioPerdeAcesso(t *testing.T) {
	u := usuarioComSaldo(t, 1000)
	// Bloqueio unificado (MB3): banimento = punição + estado recalculado (token_versao+1).
	adm := novoAdminDB(t, models.PapelAdmin)
	pun := repositories.NovoPunicaoRepository(db)
	idAdm := adm.ID
	p := &models.Punicao{IDUsuario: u.ID, Tipo: models.PunicaoBanimento, Escopo: models.EscopoGlobal,
		Inicio: time.Now().Add(-time.Second).Truncate(time.Millisecond), AplicadaPorAdmin: &idAdm, Justificativa: "fraude"}
	if err := pun.Aplicar(nil, p); err != nil {
		t.Fatal(err)
	}
	if _, endureceu, err := pun.RecalcularEstadoConta(nil, u.ID, time.Now()); err != nil || !endureceu {
		t.Fatalf("recalcular: %v %v", endureceu, err)
	}
	// token antigo revogado (token_versao++) e login recusado
	esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 401)
	r := esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 403)
	if msg, _ := r.Body["error"].(string); msg == "" || r.Body["codigo"] != "conta_banida" {
		t.Errorf("mensagem/código de conta banida: %s", r.Raw)
	}
	// senha errada continua 401 (não revela o bloqueio)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": "errada-123"}), 401)

	// token emitido depois (bloqueio gravado sem mexer na versão): o middleware barra
	v := novoUsuario(t)
	db.Exec("UPDATE usuarios SET banido_em = NOW(3) WHERE id = ?", v.ID)
	esperar(t, api(t, "GET", "/api/carteira", v.Token, nil), 403)

	if ok, err := pun.Revogar(nil, p.ID, adm.ID, "engano", time.Now()); err != nil || !ok {
		t.Fatalf("revogar: %v %v", ok, err)
	}
	if _, _, err := pun.RecalcularEstadoConta(nil, u.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	tokenNovo(t, u)
}

// ─── B8: CSRF ────────────────────────────────────────────────────────────────

func TestCSRF_CookieExigeOrigemConfiavel(t *testing.T) {
	u := usuarioComSaldo(t, 2000)
	post := func(origin string) int {
		req := httptest.NewRequest("POST", "/api/carteira/saque", nil)
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.AddCookie(httpCookie{Name: "jwt", Value: u.Token}.c())
		res, err := servidor.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	if s := post("https://evil.example"); s != 403 {
		t.Errorf("origem estranha: %d", s)
	}
	if s := post(""); s != 403 {
		t.Errorf("sem Origin: %d", s)
	}
	if s := post("http://localhost:3000"); s != 400 { // passa o CSRF e cai na validação do corpo vazio
		t.Errorf("origem do app: %d", s)
	}
	if saldo(t, u) != 2000 {
		t.Error("nada debitado")
	}
}

// ─── B5: job de expiração ────────────────────────────────────────────────────

func TestJobExpiracao_CancelaVencidosDevolvendoEntradasEReserva(t *testing.T) {
	_, tarefas := servidorComJobs(t, "inscricao", 0, 48*time.Hour)
	c := cenarioComSaldo(t, 10000)
	corpo := corpoFin(c, 1000, 4, 2, map[string]any{"premio_fixo_centavos": 3000})
	andamento := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpo), 201))
	corpo = corpoFin(c, 1000, 4, 2, map[string]any{"premio_fixo_centavos": 2000})
	aberto := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpo), 201))
	recente := criarDesafio(t, c, 1000, 4)
	if saldo(t, c.A) != 5000 {
		t.Fatalf("reservas debitadas: %d", saldo(t, c.A))
	}
	as := atletas(t, 2, 3000)
	inscrever(t, andamento, as...)
	inscrever(t, aberto, as[0])
	inscrever(t, recente, as[1])
	esperar(t, acao(t, c.I, andamento, "iniciar", nil), 200)

	db.Exec("UPDATE desafios SET data_evento = ? WHERE id IN ?", time.Now().Add(-72*time.Hour), []uint{andamento, aberto})
	db.Exec("UPDATE desafios SET data_evento = ? WHERE id = ?", time.Now().Add(-1*time.Hour), recente) // dentro da carência

	jobs.ExecutarAgora(tarefas...)
	jobs.ExecutarAgora(tarefas...) // idempotente
	if s := statusDesafio(t, c.A, andamento); s != "cancelado" {
		t.Errorf("em andamento vencido: %s", s)
	}
	if s := statusDesafio(t, c.A, aberto); s != "cancelado" {
		t.Errorf("aberto vencido: %s", s)
	}
	if s := statusDesafio(t, c.A, recente); s != "aberto" {
		t.Errorf("dentro da carência não mexe: %s", s)
	}
	if saldo(t, c.A) != 10000 {
		t.Errorf("reservas devolvidas uma vez: %d", saldo(t, c.A))
	}
	if saldo(t, as[0]) != 3000 || saldo(t, as[1]) != 2000 {
		t.Errorf("entradas devolvidas uma vez: %d %d", saldo(t, as[0]), saldo(t, as[1]))
	}
	if n := contarNotif(t, c.A, models.NotifDesafioCancel); n != 2 {
		t.Errorf("academia avisada dos 2 cancelamentos automáticos: %d", n)
	}
	ledgerConfere(t, c.A, as[0], as[1])
}

// ─── B11: pódio só para atletas ──────────────────────────────────────────────

func TestPodio_AcademiaEInstrutorInscritosNaoCompetem(t *testing.T) {
	c := novoCenario(t)
	id := criarDesafioPodio(t, c, 0, 4, 2)
	a := novoUsuario(t)
	inscrever(t, id, a)
	// inscrição legada da própria academia e do instrutor (antes das regras da Sprint 6)
	for i, u := range []*user{c.A, c.I} {
		if err := db.Create(&models.DesafioParticipante{IDDesafio: id, IDUsuario: u.ID,
			CodigoIngresso: fmt.Sprintf("%032x", time.Now().UnixNano()+int64(i))}).Error; err != nil {
			t.Fatal(err)
		}
	}
	esperar(t, acao(t, c.I, id, "iniciar", nil), 200)
	esperar(t, presencaPorID(c.A, t, id, c.A), 409)
	esperar(t, presencaPorID(c.I, t, id, c.I), 409)
	esperar(t, presencaPorID(c.I, t, id, a), 200)
	// presença antiga (gravada direto) também não conta
	db.Exec("UPDATE desafio_participantes SET presente = 1, presente_em = NOW(3) WHERE id_desafio = ? AND id_usuario = ?", id, c.A.ID)
	esperar(t, acao(t, c.I, id, "podio", map[string]any{"posicoes": []map[string]any{
		{"id_participante": c.A.ID, "marca": "1 min"},
	}}), 400)
	esperar(t, acao(t, c.I, id, "podio", map[string]any{"posicoes": []map[string]any{
		{"id_participante": a.ID, "marca": "1 min"},
	}}), 200)
	esperar(t, acao(t, c.I, id, "podio", map[string]any{"posicoes": []map[string]any{
		{"id_participante": a.ID, "marca": "1 min"},
	}}), 409)
}
