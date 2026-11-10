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

// ─── Rodada 5: endereço estruturado, início/fim, equipe de instrutores, choque de horário ───

// equipeDe devolve a equipe do DTO (instrutores.id -> membro).
func equipeDe(t *testing.T, d map[string]any) map[uint]map[string]any {
	t.Helper()
	out := map[uint]map[string]any{}
	lista, _ := d["equipe"].([]any)
	for _, x := range lista {
		m := x.(map[string]any)
		out[uint(m["id"].(float64))] = m
	}
	return out
}

// corpoEquipe é um corpo de criação com equipe de `vagasEquipe` vagas e os indicados informados.
func corpoEquipe(c *cenario, entrada int64, vagasEquipe int, indicados []uint, extra map[string]any) map[string]any {
	b := corpoDesafio(c, entrada, 10, 2)
	delete(b, "id_instrutor")
	b["instrutores_vagas"] = vagasEquipe
	b["indicados"] = indicados
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// horario devolve início/fim em RFC3339 a partir de agora + h horas, com duração d.
func horario(h float64, d time.Duration) (string, string) {
	ini := time.Now().Add(time.Duration(h * float64(time.Hour))).Truncate(time.Second)
	return ini.Format(time.RFC3339), ini.Add(d).Format(time.RFC3339)
}

// acharItem devolve o item da lista com esse id (nil se não estiver).
func acharItem(r resp, id uint) map[string]any {
	for _, it := range r.data()["itens"].([]any) {
		if m := it.(map[string]any); uint(m["id"].(float64)) == id {
			return m
		}
	}
	return nil
}

func candidatarR5(t *testing.T, u *user, id uint) resp {
	t.Helper()
	return api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), u.Token, map[string]any{"mensagem": "topo as condições"})
}

func TestR5_CriarComEnderecoHorarioEEquipe(t *testing.T) {
	c := novoCenario(t)
	depositar(t, c.A, 100_000)
	i2, iid2 := novoInstrutorDe(t, c.AcademiaID)
	_ = i2
	antes := saldo(t, c.A)
	b := corpoEquipe(c, 1000, 3, []uint{c.InstrutorID, iid2}, map[string]any{
		"premio_fixo_centavos": 2000, "instrutor_modo_pagamento": "fixo_percentual", "instrutor_fixo_centavos": 1000,
		"instrutor_pct": 5, "id_responsavel": iid2, "complemento": "Portão 2", "local": "ignorado",
	})
	d := esperar(t, api(t, "POST", "/api/desafios", c.A.Token, b), 201).data()
	id := uint(d["id"].(float64))
	// reserva T1 = prêmio fixo + 3 vagas x 1000
	if got := antes - saldo(t, c.A); got != 5000 {
		t.Errorf("reserva debitada %d, esperava 5000", got)
	}
	end := sub(t, d, "endereco")
	if end["cep"] != "01310100" || end["uf"] != "SP" || end["complemento"] != "Portão 2" {
		t.Errorf("endereço: %v", end)
	}
	if loc, _ := d["local"].(string); !strings.Contains(loc, "Av. Paulista, 1000 - Portão 2") || strings.Contains(loc, "ignorado") {
		t.Errorf("local montado: %q", loc)
	}
	if d["data_fim_evento"] == nil {
		t.Error("data_fim_evento deveria vir preenchida")
	}
	eq := equipeDe(t, d)
	if len(eq) != 2 || eq[iid2]["responsavel"] != true || eq[c.InstrutorID]["responsavel"] != false ||
		eq[iid2]["origem"] != "indicado" || eq[iid2]["vinculado"] != true {
		t.Errorf("equipe: %v", d["equipe"])
	}
	if num(d, "instrutores_vagas") != 3 || num(d, "vagas_instrutor_abertas") != 1 || d["vaga_instrutor_aberta"] != true {
		t.Errorf("vagas da equipe: %v / %v", d["instrutores_vagas"], d["vagas_instrutor_abertas"])
	}
	if inst := sub(t, d, "instrutor"); uint(inst["id"].(float64)) != iid2 {
		t.Errorf("instrutor (compat) deveria ser o responsável: %v", inst)
	}
	fin := sub(t, d, "financeiro")
	if num(fin, "reserva_centavos") != 5000 || num(fin, "instrutores_vagas") != 3 || num(fin, "equipe_count") != 2 || num(fin, "instrutores_pagos") != 3 {
		t.Errorf("financeiro: %v", fin)
	}
	// espelho legado = responsável
	var espelho *uint
	db.Model(&models.Desafio{}).Select("id_instrutor").Where("id = ?", id).Scan(&espelho)
	if espelho == nil || *espelho != iid2 {
		t.Errorf("desafios.id_instrutor deveria espelhar o responsável %d: %v", iid2, espelho)
	}
	invariantes(t, id)
	ledgerConfere(t, c.A)

	// validações
	criar := func(mod func(m map[string]any)) resp {
		m := corpoEquipe(c, 0, 2, []uint{c.InstrutorID}, nil)
		mod(m)
		return api(t, "POST", "/api/desafios", c.A.Token, m)
	}
	ini, _ := horario(48, 0)
	for nome, caso := range map[string]func(m map[string]any){
		"fim antes do início": func(m map[string]any) { m["data_evento"], m["data_fim_evento"] = ini, ini },
		"sem fim":             func(m map[string]any) { delete(m, "data_fim_evento") },
		"sem cep":             func(m map[string]any) { delete(m, "cep") },
		"uf inválida":         func(m map[string]any) { m["uf"] = "XX" },
		"indicados > vagas":   func(m map[string]any) { m["instrutores_vagas"], m["indicados"] = 1, []uint{c.InstrutorID, iid2} },
		"indicado repetido":   func(m map[string]any) { m["indicados"] = []uint{c.InstrutorID, c.InstrutorID} },
		"responsável fora":    func(m map[string]any) { m["id_responsavel"] = iid2 },
		"vagas 21":            func(m map[string]any) { m["instrutores_vagas"] = 21 },
		"leilão (vaga_aberta)": func(m map[string]any) {
			m["instrutor_selecao"], m["instrutor_teto_centavos"] = "vaga_aberta", 1000
		},
		"% x vagas > 100": func(m map[string]any) {
			m["instrutor_modo_pagamento"], m["instrutor_pct"], m["instrutores_vagas"], m["percentual_pote"] = "percentual", 20, 4, 30
		},
	} {
		if r := criar(caso); r.Status != 400 {
			t.Errorf("%s: esperava 400, veio %d %s", nome, r.Status, r.Raw)
		}
	}
	// indicado de outra academia / autônomo não entra direto
	_, autonomo := novoInstrutorDe(t, 0)
	if r := criar(func(m map[string]any) { m["indicados"] = []uint{autonomo} }); r.Status != 400 {
		t.Errorf("autônomo indicado: %d", r.Status)
	}
	// sem indicados: equipe vazia, todas as vagas abertas, sem responsável
	d2 := esperar(t, criar(func(m map[string]any) { m["indicados"] = []uint{} }), 201).data()
	if len(equipeDe(t, d2)) != 0 || num(d2, "vagas_instrutor_abertas") != 2 || d2["instrutor"] != nil {
		t.Errorf("sem indicados: %v", d2)
	}
	// CEP com espaços nas pontas (R4-1) e máscara
	if r := criar(func(m map[string]any) { m["cep"] = " 01310-100 " }); r.Status != 201 {
		t.Errorf("CEP com espaços: %d %s", r.Status, r.Raw)
	}
}

func TestR5_R41_CEPComEspacosNoCadastroDaAcademia(t *testing.T) {
	r := api(t, "POST", "/api/academias", "", map[string]any{
		"nome": "Acad", "email": fmt.Sprintf("qa+r41_%d@gymscore.test", seq.Add(1)), "senha": senhaPadrao,
		"razao_social": "Academia R41 LTDA", "nome_fantasia": "R41", "cnpj": cnpjUnico(),
		"cep": " 20010000 ", "logradouro": "Rua A", "numero": "1", "bairro": "Centro", "cidade": "Rio", "uf": "rj",
	})
	esperar(t, r, 201)
	if r.data()["cep"] != "20010000" {
		t.Errorf("cep normalizado: %v", r.data()["cep"])
	}
}

func TestR5_CandidaturaSemLanceAceitarRecusarUltimaVaga(t *testing.T) {
	c := novoCenario(t)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 2, []uint{c.InstrutorID},
		map[string]any{"instrutor_modo_pagamento": "percentual", "instrutor_pct": 10})), 201))
	x, xid := novoInstrutorDe(t, 0)
	y, yid := novoInstrutorDe(t, 0)
	z, _ := novoInstrutorDe(t, 0)

	// trabalhos oferecidos: aparece com as condições por instrutor
	lista := esperar(t, api(t, "GET", "/api/instrutor/trabalhos?limite=100", x.Token, nil), 200)
	it := feedItem(t, lista, id)
	if it == nil {
		t.Fatal("desafio com vaga livre deveria estar nos trabalhos oferecidos")
	}
	vaga := sub(t, it, "vaga")
	if vaga["modo_pagamento"] != "percentual" || num(vaga, "pct") != 10 || num(vaga, "vagas_abertas") != 1 || it["pode_candidatar"] != true {
		t.Errorf("oferta: %v", it)
	}
	// candidatura sem lance (lance enviado é ignorado)
	cx := esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas", id), x.Token, map[string]any{"lance_centavos": 999}), 201).data()
	if _, tem := cx["lance_centavos"]; tem {
		t.Errorf("candidatura não deveria ter lance: %v", cx)
	}
	var lance *int64
	db.Model(&models.CandidaturaInstrutor{}).Select("lance_centavos").Where("id = ?", uint(cx["id"].(float64))).Scan(&lance)
	if lance != nil {
		t.Errorf("lance_centavos deveria ser NULL, veio %d", *lance)
	}
	esperar(t, candidatarR5(t, x, id), 200) // atualiza a mensagem
	cy := esperar(t, candidatarR5(t, y, id), 201).data()
	esperar(t, candidatarR5(t, z, id), 201)
	// membro indicado não se candidata
	esperar(t, candidatarR5(t, c.I, id), 409)
	// retirar e voltar
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/desafios/%d/candidaturas", id), z.Token, nil), 200)
	esperar(t, candidatarR5(t, z, id), 201)

	cands := esperar(t, api(t, "GET", fmt.Sprintf("/api/desafios/%d/candidaturas", id), c.A.Token, nil), 200).Body["data"].([]any)
	if len(cands) != 3 {
		t.Fatalf("candidaturas: %v", cands)
	}
	// só a dona aceita/recusa
	cidY := uint(cy["id"].(float64))
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cidY), c.I.Token, nil), 403)
	// recusar Z
	cidZ := uint(cands[2].(map[string]any)["id"].(float64))
	rz := esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/recusar", id, cidZ), c.A.Token, nil), 200).data()
	if rz["status"] != "recusada" {
		t.Errorf("recusa: %v", rz)
	}
	// aceitar Y preenche a última vaga: X vira recusada
	d := esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cidY), c.A.Token, nil), 200).data()
	eq := equipeDe(t, d)
	if len(eq) != 2 || eq[yid]["origem"] != "candidatura" || eq[c.InstrutorID]["responsavel"] != true || d["vaga_instrutor_aberta"] != false {
		t.Errorf("equipe depois do aceite: %v", d["equipe"])
	}
	var stX string
	db.Model(&models.CandidaturaInstrutor{}).Select("status").Where("id_desafio = ? AND id_instrutor = ?", id, xid).Scan(&stX)
	if stX != models.CandidaturaRecusada {
		t.Errorf("pendente de X deveria virar recusada ao fechar a equipe: %s", stX)
	}
	esperar(t, candidatarR5(t, x, id), 409) // equipe completa
	if it := acharItem(esperar(t, api(t, "GET", "/api/instrutor/vagas?limite=100", x.Token, nil), 200), id); it != nil {
		t.Error("equipe completa não deveria aparecer nos trabalhos oferecidos")
	}
	// Y na equipe vê o próprio ganho
	dy := detalhe(t, y, id)
	if dy["voce_na_equipe"] != true || dy["voce_responsavel"] != false || dy["instrutor_ganho"] == nil {
		t.Errorf("visão do membro: %v", dy)
	}
	// trocar responsável para Y; remover Y (responsável) devolve a responsabilidade ao indicado
	d = esperar(t, api(t, "PUT", fmt.Sprintf("/api/desafios/%d/equipe/responsavel", id), c.A.Token, map[string]any{"id_instrutor": yid}), 200).data()
	if equipeDe(t, d)[yid]["responsavel"] != true {
		t.Errorf("responsável: %v", d["equipe"])
	}
	esperar(t, api(t, "PUT", fmt.Sprintf("/api/desafios/%d/equipe/responsavel", id), c.A.Token, map[string]any{"id_instrutor": xid}), 404)
	d = esperar(t, api(t, "DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, yid), c.A.Token, nil), 200).data()
	eq = equipeDe(t, d)
	if len(eq) != 1 || eq[c.InstrutorID]["responsavel"] != true {
		t.Errorf("depois de remover o responsável: %v", d["equipe"])
	}
	var espelho *uint
	db.Model(&models.Desafio{}).Select("id_instrutor").Where("id = ?", id).Scan(&espelho)
	if espelho == nil || *espelho != c.InstrutorID {
		t.Errorf("espelho id_instrutor: %v", espelho)
	}
	var stY string
	db.Model(&models.CandidaturaInstrutor{}).Select("status").Where("id = ?", cidY).Scan(&stY)
	if stY != models.CandidaturaRemovida {
		t.Errorf("candidatura de Y deveria ficar removida: %s", stY)
	}
	// vaga reaberta: X pode se candidatar de novo; indicar pela academia (vinculado) também fecha
	esperar(t, candidatarR5(t, x, id), 201)
	_, iid3 := novoInstrutorDe(t, c.AcademiaID)
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", id), c.A.Token, map[string]any{"id_instrutor": xid}), 400) // autônomo não é indicado
	d = esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/equipe", id), c.A.Token, map[string]any{"id_instrutor": iid3}), 200).data()
	if len(equipeDe(t, d)) != 2 {
		t.Errorf("indicação: %v", d["equipe"])
	}
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, c.InstrutorID), c.I.Token, nil), 403)
}

func TestR5_FluxoDinheiroEquipeComVagaVazia(t *testing.T) {
	srv := servidorTaxa("inscricao", 10, 0)
	c := cenarioComSaldo(t, 100_000)
	x, xid := novoInstrutorDe(t, 0)
	// 3 vagas x (R$ 10 + 5%), prêmio fixo R$ 20, pote 50%: reserva 2000 + 3000 = 5000
	id := idDe(esperar(t, chamar(t, srv, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 1000, 3, []uint{c.InstrutorID},
		map[string]any{"premio_fixo_centavos": 2000, "percentual_pote": 50, "instrutor_modo_pagamento": "fixo_percentual",
			"instrutor_fixo_centavos": 1000, "instrutor_pct": 5})), 201))
	cid := uint(esperar(t, candidatarR5(t, x, id), 201).data()["id"].(float64))
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/escolher", id, cid), c.A.Token, nil), 200) // nome antigo
	at := atletas(t, 4, 5000)
	inscrever(t, id, at...)
	// membro não responsável: marca presença, mas não inicia nem lança o pódio
	esperar(t, acaoS(t, srv, x, id, "iniciar", nil), 403)
	esperar(t, acaoS(t, srv, c.I, id, "iniciar", nil), 200)
	for _, a := range at[:3] {
		esperar(t, presencaPorID(x, t, id, a), 200)
	}
	podio := podioBody(entradaPodio(at[0], "20:00"), entradaPodio(at[1], "21:00"))
	esperar(t, acaoS(t, srv, x, id, "podio", podio), 403)
	antesX, antesI, antesA := saldo(t, x), saldo(t, c.I), saldo(t, c.A)
	esperar(t, acaoS(t, srv, c.I, id, "podio", podio), 200)
	// T = 4000; cada membro: 1000 + 200 = 1200; taxa 400; prêmio 2000 + 2000 = 4000
	// academia = 4000 + 5000 - 4000 - 2400 - 400 = 2200 (inclui os 1000 da vaga não preenchida)
	if g := saldo(t, x) - antesX; g != 1200 {
		t.Errorf("membro por candidatura recebeu %d, esperava 1200", g)
	}
	if g := saldo(t, c.I) - antesI; g != 1200 {
		t.Errorf("responsável recebeu %d, esperava 1200", g)
	}
	if g := saldo(t, c.A) - antesA; g != 2200 {
		t.Errorf("academia recebeu %d, esperava 2200", g)
	}
	_, pagos := contarLedger(t, id, models.TipoPagamentoInstrutor)
	if pagos != 2400 {
		t.Errorf("pagamento_instrutor total %d", pagos)
	}
	_ = xid
	invariantes(t, id)
	ledgerConfere(t, append([]*user{c.A, c.I, x}, at...)...)
}

func TestR5_EditarVagasEPagamentoECancelarDevolveReserva(t *testing.T) {
	c := cenarioComSaldo(t, 100_000)
	antes := saldo(t, c.A)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 2, []uint{c.InstrutorID},
		map[string]any{"instrutor_fixo_centavos": 1000})), 201))
	patch := func(m map[string]any) resp {
		return api(t, "PATCH", fmt.Sprintf("/api/desafios/%d", id), c.A.Token, m)
	}
	if d := antes - saldo(t, c.A); d != 2000 {
		t.Errorf("reserva inicial 2 x 1000: %d", d)
	}
	esperar(t, patch(map[string]any{"instrutores_vagas": 4}), 200) // +2 x 1000
	if d := antes - saldo(t, c.A); d != 4000 {
		t.Errorf("reserva com 4 x 1000: %d", d)
	}
	esperar(t, patch(map[string]any{"instrutor_fixo_centavos": 1500}), 200) // só indicados: pode
	if d := antes - saldo(t, c.A); d != 6000 {
		t.Errorf("reserva com 4 x 1500: %d", d)
	}
	invariantes(t, id)
	x, _ := novoInstrutorDe(t, 0)
	esperar(t, candidatarR5(t, x, id), 201)
	esperar(t, patch(map[string]any{"instrutor_fixo_centavos": 500}), 409) // candidato aceitou estas condições
	esperar(t, patch(map[string]any{"instrutores_vagas": 0}), 400)
	esperar(t, patch(map[string]any{"id_instrutor": c.InstrutorID}), 400)
	esperar(t, patch(map[string]any{"instrutor_teto_centavos": 100}), 400)
	// fechar a equipe no tamanho atual recusa a pendente e devolve as vagas
	esperar(t, patch(map[string]any{"instrutores_vagas": 1}), 200)
	var st string
	db.Model(&models.CandidaturaInstrutor{}).Select("status").Where("id_desafio = ?", id).Scan(&st)
	if st != models.CandidaturaRecusada {
		t.Errorf("pendente ao fechar a equipe: %s", st)
	}
	if d := antes - saldo(t, c.A); d != 1500 {
		t.Errorf("reserva com 1 x 1500: %d", d)
	}
	// endereço e horário na edição
	ini, fim := horario(72, 3*time.Hour)
	esperar(t, patch(map[string]any{"cep": "20010000"}), 400) // endereço incompleto
	esperar(t, patch(enderecoTeste(map[string]any{"data_evento": ini, "data_fim_evento": fim})), 200)
	esperar(t, patch(map[string]any{"data_fim_evento": ini}), 400)
	invariantes(t, id)
	esperar(t, acao(t, c.A, id, "cancelar", nil), 200)
	if saldo(t, c.A) != antes {
		t.Errorf("cancelar deveria devolver a reserva inteira")
	}
	invariantes(t, id)
	ledgerConfere(t, c.A)
}

func TestR5_IniciarExigeEquipeEResponsavel(t *testing.T) {
	c := novoCenario(t)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 2, []uint{}, nil)), 201))
	inscrever(t, id, novoUsuario(t))
	esperar(t, acao(t, c.A, id, "iniciar", nil), 409) // equipe vazia
	x, xid := novoInstrutorDe(t, 0)
	cid := uint(esperar(t, candidatarR5(t, x, id), 201).data()["id"].(float64))
	d := esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cid), c.A.Token, nil), 200).data()
	if equipeDe(t, d)[xid]["responsavel"] != true {
		t.Errorf("primeiro aceito sem indicados vira responsável: %v", d["equipe"])
	}
	y, _ := novoInstrutorDe(t, 0)
	esperar(t, candidatarR5(t, y, id), 201)
	if dx := detalhe(t, x, id); dx["pode_iniciar"] != true {
		t.Errorf("responsável deveria poder iniciar: %v", dx["pode_iniciar"])
	}
	esperar(t, acao(t, x, id, "iniciar", nil), 200) // inicia com 1 de 2 vagas
	var st string
	db.Model(&models.CandidaturaInstrutor{}).Select("status").Where("id_desafio = ? AND status <> 'escolhida'", id).Scan(&st)
	if st != models.CandidaturaRecusada {
		t.Errorf("pendente ao iniciar deveria ser recusada: %s", st)
	}
	esperar(t, api(t, "DELETE", fmt.Sprintf("/api/desafios/%d/equipe/%d", id, xid), c.A.Token, nil), 409) // já iniciou
}

func TestR5_ConflitoDeInteresseEDenunciaConsideramTodaAEquipe(t *testing.T) {
	c := novoCenario(t)
	x, xid := novoInstrutorDe(t, 0)
	id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 2, []uint{c.InstrutorID}, nil)), 201))
	cid := uint(esperar(t, candidatarR5(t, x, id), 201).data()["id"].(float64))
	esperar(t, api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cid), c.A.Token, nil), 200)
	// X (membro, não responsável) no modo atleta: não entra, com o motivo
	modoAtleta(t, x)
	if d := detalhe(t, x, id); d["pode_entrar"] != false || !strings.Contains(fmt.Sprint(d["motivo_bloqueio_entrada"]), "equipe") {
		t.Errorf("membro como atleta: %v / %v", d["pode_entrar"], d["motivo_bloqueio_entrada"])
	}
	esperar(t, acao(t, x, id, "entrar", nil), 409)
	_ = xid
	// atleta inscrito denuncia o membro NÃO responsável pelo desafio: vínculo aceito
	at := novoUsuario(t)
	inscrever(t, id, at)
	esperar(t, denunciar(t, at, corpoDenuncia(x.ID, models.DenunciaSubornoInstrutor, id)), 201)
}

func TestR5_ChoqueDeHorarioNaMesmaAcademia(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	criarEm := func(cen *cenario, h float64, dur time.Duration) uint {
		ini, fim := horario(h, dur)
		b := corpoDesafio(cen, 0, 10, 1)
		b["data_evento"], b["data_fim_evento"] = ini, fim
		return idDe(esperar(t, api(t, "POST", "/api/desafios", cen.A.Token, b), 201))
	}
	a := criarEm(c, 48, 2*time.Hour)     // [48, 50)
	b := criarEm(c, 49, 2*time.Hour)     // [49, 51) sobrepõe A
	cc := criarEm(c, 50, time.Hour)      // [50, 51) encosta no fim de A: não sobrepõe A, sobrepõe B
	d := criarEm(outra, 48, 2*time.Hour) // outra academia
	u := novoUsuario(t)
	esperar(t, acao(t, u, a, "entrar", nil), 200)
	r := esperar(t, acao(t, u, b, "entrar", nil), 409)
	if !strings.Contains(r.Raw, "no mesmo horário") {
		t.Errorf("mensagem do choque: %s", r.Raw)
	}
	if det := detalhe(t, u, b); det["pode_entrar"] != false || !strings.Contains(fmt.Sprint(det["motivo_bloqueio_entrada"]), "mesmo horário") {
		t.Errorf("previsão do choque: %v / %v", det["pode_entrar"], det["motivo_bloqueio_entrada"])
	}
	if it := feedItem(t, esperar(t, api(t, "GET", "/api/desafios?limite=100", u.Token, nil), 200), b); it == nil || it["pode_entrar"] != false {
		t.Errorf("feed sem o choque: %v", it)
	}
	esperar(t, acao(t, u, cc, "entrar", nil), 200) // [50,51) não choca com [48,50)
	esperar(t, acao(t, u, d, "entrar", nil), 200)  // outra academia
	// sair de A libera B? B ainda choca com C: continua bloqueado
	esperar(t, acao(t, u, a, "sair", nil), 200)
	esperar(t, acao(t, u, b, "entrar", nil), 409)
	esperar(t, acao(t, u, cc, "sair", nil), 200)
	esperar(t, acao(t, u, b, "entrar", nil), 200)
}

func TestR5_ConcorrenciaInscricoesSobrepostasDoMesmoAtleta(t *testing.T) {
	c := novoCenario(t)
	for rodada := 0; rodada < 4; rodada++ {
		ini, fim := horario(48+float64(rodada)*10, 2*time.Hour)
		ids := make([]uint, 2)
		for i := range ids {
			b := corpoDesafio(c, 500, 10, 1)
			b["data_evento"], b["data_fim_evento"] = ini, fim
			ids[i] = idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, b), 201))
		}
		u := usuarioComSaldo(t, 5000)
		var wg sync.WaitGroup
		st := make([]int, 2)
		for i := range ids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				st[i] = api(t, "POST", fmt.Sprintf("/api/desafios/%d/entrar", ids[i]), u.Token, nil).Status
			}(i)
		}
		wg.Wait()
		ok := 0
		for _, s := range st {
			switch s {
			case 200:
				ok++
			case 409:
			default:
				t.Errorf("rodada %d: status inesperado %v", rodada, st)
			}
		}
		if ok != 1 {
			t.Errorf("rodada %d: exatamente 1 inscrição deveria passar, vieram %v", rodada, st)
		}
		if s := saldo(t, u); s != 4500 {
			t.Errorf("rodada %d: saldo %d (só 1 entrada debitada)", rodada, s)
		}
		ledgerConfere(t, u)
	}
}

func TestR5_ConcorrenciaDuasAceitacoesNaUltimaVaga(t *testing.T) {
	c := novoCenario(t)
	for rodada := 0; rodada < 4; rodada++ {
		id := idDe(esperar(t, api(t, "POST", "/api/desafios", c.A.Token, corpoEquipe(c, 0, 2, []uint{c.InstrutorID}, nil)), 201))
		cids := make([]uint, 2)
		for i := range cids {
			x, _ := novoInstrutorDe(t, 0)
			cids[i] = uint(esperar(t, candidatarR5(t, x, id), 201).data()["id"].(float64))
		}
		var wg sync.WaitGroup
		st := make([]int, 2)
		for i := range cids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				st[i] = api(t, "POST", fmt.Sprintf("/api/desafios/%d/candidaturas/%d/aceitar", id, cids[i]), c.A.Token, nil).Status
			}(i)
		}
		wg.Wait()
		if !(st[0] == 200 && st[1] == 409 || st[0] == 409 && st[1] == 200) {
			t.Errorf("rodada %d: esperava um 200 e um 409, veio %v", rodada, st)
		}
		var n int64
		db.Model(&models.DesafioInstrutor{}).Where("id_desafio = ?", id).Count(&n)
		if n != 2 {
			t.Errorf("rodada %d: equipe com %d membros (máx. 2)", rodada, n)
		}
	}
}

func TestR5_AdminDetalheMostraEquipeEnderecoEHorario(t *testing.T) {
	a := novoAdminDB(t, models.PapelAdmin)
	c := novoCenario(t)
	id := criarDesafio(t, c, 0, 4)
	d := esperar(t, adm(t, "GET", fmt.Sprintf("/api/admin/desafios/%d", id), a, nil), 200).data()
	eq, _ := d["equipe"].([]any)
	if len(eq) != 1 || eq[0].(map[string]any)["responsavel"] != true {
		t.Errorf("equipe no admin: %v", d["equipe"])
	}
	if sub(t, d, "endereco")["cep"] != "01310100" || d["local"] == "" {
		t.Errorf("endereço no admin: %v / %v", d["endereco"], d["local"])
	}
	des := sub(t, d, "desafio")
	if des["data_fim_evento"] == nil || num(des, "instrutores_vagas") != 1 {
		t.Errorf("resumo admin: %v", des)
	}
	if _, tem := sub(t, d, "financeiro")["instrutor_selecao"]; tem {
		t.Errorf("desafio novo não deveria trazer o leilão legado: %v", d["financeiro"])
	}
}
