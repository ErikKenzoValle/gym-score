//go:build integration

package integration

// Migration 00021 (rodada 5): endereço estruturado + data_fim_evento nos desafios, equipe de instrutores
// (desafio_instrutores, instrutores_vagas) e fim do leilão (lance NULL, vaga_aberta convertida).

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMigracao_00021_EquipeLocalHorario(t *testing.T) {
	d, sqlDB := bancoQA(t, "r5", 20)
	ctx := context.Background()
	p := providerQA(t, sqlDB)

	// Estado da 00020: academia, 3 instrutores, desafios designado / vaga aberta (aberta, com 2 candidaturas
	// pendentes) / vaga aberta preenchida / legado sem data_evento.
	contaAc := usuarioLegado(t, d, "academia")
	idAc := uint(execQA(t, d, "INSERT INTO academias (id_usuario, razao_social, nome_fantasia, cnpj, status) VALUES (?, 'R', 'F', ?, 'aprovada')", contaAc, cnpjUnico()))
	var inst [3]uint
	for i := range inst {
		u := usuarioLegado(t, d, "atleta")
		inst[i] = uint(execQA(t, d, "INSERT INTO instrutores (id_usuario, cref, id_academia, status) VALUES (?, ?, ?, 'aprovada')", u, "1-G/SP"+string(rune('a'+i)), idAc))
	}
	ev := time.Date(2026, 12, 1, 9, 0, 0, 0, time.Local)
	novo := func(status, selecao string, idInst *uint, teto *int64, modo string, reserva int64, dataEv *time.Time) uint {
		return uint(execQA(t, d, `INSERT INTO desafios (titulo,categoria,valor_entrada_centavos,data_evento,status,vagas,posicoes_podio,id_criador,id_academia,id_instrutor,
			premio_fixo_centavos,instrutor_modo_pagamento,instrutor_pct,instrutor_selecao,instrutor_teto_centavos,reserva_academia_centavos,percentual_pote,criado_em,atualizado_em)
			VALUES ('D','geral',1000,?,?,10,3,?,?,?,5000,?,10,?,?,?,80,NOW(3),NOW(3))`, dataEv, status, contaAc, idAc, idInst, modo, selecao, teto, reserva))
	}
	teto := int64(3000)
	dDesig := novo("aberto", "designado", &inst[0], nil, "fixo", 5000, &ev)
	dVaga := novo("aberto", "vaga_aberta", nil, &teto, "percentual", 8000, &ev)
	dVagaEsc := novo("encerrado", "vaga_aberta", &inst[1], &teto, "fixo", 7000, &ev)
	dLegado := novo("cancelado", "designado", nil, nil, "fixo", 0, nil)
	execQA(t, d, "INSERT INTO desafio_candidaturas_instrutor (id_desafio,id_instrutor,lance_centavos,status) VALUES (?,?,2500,'pendente'),(?,?,2000,'pendente'),(?,?,2000,'escolhida')",
		dVaga, inst[1], dVaga, inst[2], dVagaEsc, inst[1])

	if _, err := p.UpTo(ctx, 21); err != nil {
		t.Fatalf("UpTo 21: %v", err)
	}

	type membro struct {
		IDDesafio, IDInstrutor uint
		Origem                 string
		Responsavel            bool
	}
	var eq []membro
	d.Raw("SELECT id_desafio, id_instrutor, origem, responsavel FROM desafio_instrutores ORDER BY id_desafio").Scan(&eq)
	if len(eq) != 2 || eq[0] != (membro{dDesig, inst[0], "indicado", true}) || eq[1] != (membro{dVagaEsc, inst[1], "candidatura", true}) {
		t.Errorf("equipe migrada: %+v", eq)
	}
	type cfg struct {
		InstrutoresVagas        int
		InstrutorFixoCentavos   int64
		ReservaAcademiaCentavos int64
		InstrutorModoPagamento  string
		DataFimEvento           *time.Time
	}
	ler := func(id uint) (c cfg) {
		d.Raw("SELECT instrutores_vagas, instrutor_fixo_centavos, reserva_academia_centavos, instrutor_modo_pagamento, data_fim_evento FROM desafios WHERE id = ?", id).Scan(&c)
		return
	}
	if c := ler(dVaga); c.InstrutoresVagas != 1 || c.InstrutorFixoCentavos != teto || c.ReservaAcademiaCentavos != 8000 || c.InstrutorModoPagamento != "fixo_percentual" {
		t.Errorf("vaga aberta convertida: %+v", c)
	}
	if c := ler(dDesig); c.DataFimEvento == nil || !c.DataFimEvento.Equal(ev.Add(2*time.Hour)) {
		t.Errorf("backfill do fim: %+v", c)
	}
	if c := ler(dLegado); c.DataFimEvento != nil {
		t.Errorf("legado sem data_evento ganhou fim: %+v", c)
	}
	var nPend int64
	d.Raw("SELECT COUNT(*) FROM desafio_candidaturas_instrutor WHERE id_desafio = ? AND status = 'pendente'", dVaga).Scan(&nPend)
	if nPend != 2 {
		t.Errorf("candidaturas pendentes: %d", nPend)
	}

	// CHECKs e unicidades.
	for _, q := range []string{
		"UPDATE desafios SET cep = '1234567' WHERE id = ?",
		"UPDATE desafios SET uf = 'sp' WHERE id = ?",
		"UPDATE desafios SET data_fim_evento = data_evento WHERE id = ?",
		"UPDATE desafios SET instrutores_vagas = 0 WHERE id = ?",
		"UPDATE desafios SET instrutores_vagas = 21 WHERE id = ?",
		"UPDATE desafios SET instrutores_vagas = 2 WHERE id = ?", // 80 + 0 + 10*2 = 100 ok; 3 vagas = 110
	} {
		err := d.Exec(q, dDesig).Error
		if strings.Contains(q, "= 2 WHERE") {
			if err != nil {
				t.Errorf("%s deveria passar: %v", q, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "3819") {
			t.Errorf("%s deveria violar o CHECK: %v", q, err)
		}
	}
	if err := d.Exec("UPDATE desafios SET instrutores_vagas = 3 WHERE id = ?", dDesig).Error; err == nil || !strings.Contains(err.Error(), "3819") {
		t.Errorf("soma 80 + 10*3 deveria violar chk_desafios_soma_pct_equipe: %v", err)
	}
	if err := d.Exec("UPDATE desafios SET data_fim_evento = NOW() WHERE id = ?", dLegado).Error; err == nil {
		t.Errorf("fim sem início deveria violar o CHECK")
	}
	execQA(t, d, "UPDATE desafios SET cep='01310100', logradouro='Av. Paulista', numero='1000', bairro='Bela Vista', cidade='São Paulo', uf='SP', local=? WHERE id = ?", strings.Repeat("x", 400), dDesig)
	execQA(t, d, "INSERT INTO desafio_instrutores (id_desafio, id_instrutor, origem) VALUES (?, ?, 'indicado')", dDesig, inst[2])
	if err := d.Exec("INSERT INTO desafio_instrutores (id_desafio, id_instrutor, origem) VALUES (?, ?, 'candidatura')", dDesig, inst[2]).Error; err == nil || !strings.Contains(err.Error(), "1062") {
		t.Errorf("membro repetido deveria dar 1062: %v", err)
	}
	if err := d.Exec("UPDATE desafio_instrutores SET responsavel = 1 WHERE id_desafio = ? AND id_instrutor = ?", dDesig, inst[2]).Error; err == nil || !strings.Contains(err.Error(), "1062") {
		t.Errorf("segundo responsável deveria dar 1062: %v", err)
	}
	execQA(t, d, "INSERT INTO desafio_candidaturas_instrutor (id_desafio,id_instrutor,status) VALUES (?,?,'removida')", dDesig, inst[1]) // lance NULL

	// Down: só o responsável sobrevive; dVaga (aberto sem instrutor) volta a vaga_aberta com teto.
	if _, err := p.DownTo(ctx, 20); err != nil {
		t.Fatalf("Down 21: %v", err)
	}
	var idInst *uint
	d.Raw("SELECT id_instrutor FROM desafios WHERE id = ?", dDesig).Scan(&idInst)
	if idInst == nil || *idInst != inst[0] {
		t.Errorf("Down: id_instrutor do designado = %v", idInst)
	}
	var sel string
	var tetoDown *int64
	d.Raw("SELECT instrutor_selecao, instrutor_teto_centavos FROM desafios WHERE id = ?", dVaga).Row().Scan(&sel, &tetoDown)
	if sel != "vaga_aberta" || tetoDown == nil || *tetoDown != teto {
		t.Errorf("Down: vaga aberta %q %v", sel, tetoDown)
	}
	if colunaExiste(t, d, "desafios", "cep") || colunaExiste(t, d, "desafios", "data_fim_evento") || colunaExiste(t, d, "desafios", "instrutores_vagas") ||
		existe(t, d, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'desafio_instrutores'") {
		t.Errorf("schema da 00021 sobrou após o Down")
	}
	if !existe(t, d, "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND constraint_name = 'chk_desafios_soma_pct'") {
		t.Errorf("CHECK de soma antigo não voltou")
	}
	var st string
	var lance int64
	d.Raw("SELECT status, lance_centavos FROM desafio_candidaturas_instrutor WHERE id_desafio = ? AND id_instrutor = ?", dDesig, inst[1]).Row().Scan(&st, &lance)
	if st != "recusada" || lance != 0 {
		t.Errorf("Down: candidatura removida/lance NULL -> %q %d", st, lance)
	}
	var tam int64
	d.Raw("SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'desafios' AND column_name = 'local'").Scan(&tam)
	if tam != 500 {
		t.Errorf("local com 400 caracteres não deveria ter sido estreitado (%d)", tam)
	}

	// Up de novo (a equipe é remontada a partir de id_instrutor) e Up reaplicado sobre o schema migrado.
	// UpTo 21: com a 00022 aplicada o goose recusaria a 21 como "fora de ordem".
	if _, err := p.UpTo(ctx, 21); err != nil {
		t.Fatalf("Up de novo: %v", err)
	}
	execQA(t, d, "DELETE FROM goose_db_version WHERE version_id = 21")
	if _, err := p.UpTo(ctx, 21); err != nil {
		t.Fatalf("Up reaplicado sobre o schema migrado: %v", err)
	}
	var nEq int64
	d.Raw("SELECT COUNT(*) FROM desafio_instrutores").Scan(&nEq)
	if versaoGoose(t, d) != 21 || nEq != 2 || !colunaExiste(t, d, "desafios", "data_fim_evento") {
		t.Errorf("estado após Up de novo incorreto (equipe %d)", nEq)
	}
	if existe(t, d, "SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = DATABASE() AND routine_name LIKE '\\_goose\\_%'") {
		t.Errorf("sobrou procedure _goose_* temporária")
	}
}
