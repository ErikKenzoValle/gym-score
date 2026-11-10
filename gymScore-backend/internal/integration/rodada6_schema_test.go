//go:build integration

package integration

// Migration 00022 (rodada 6): premiação configurável (desafio_premio_posicoes, posições até 100), resumo/regras,
// filtros de nível/idade e o cache do Nível GymScore em usuarios.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMigracao_00022_PremiacaoRegrasFiltrosNivel(t *testing.T) {
	d, sqlDB := bancoQA(t, "r6", 21)
	ctx := context.Background()
	p := providerQA(t, sqlDB)

	// Estado da 00021: desafios com pódio de 1, 3 e 5 posições e descrições vazia / curta / longa.
	contaAc := usuarioLegado(t, d, "academia")
	atleta := usuarioLegado(t, d, "atleta")
	idAc := uint(execQA(t, d, "INSERT INTO academias (id_usuario, razao_social, nome_fantasia, cnpj, status) VALUES (?, 'R', 'F', ?, 'aprovada')", contaAc, cnpjUnico()))
	ev := time.Date(2026, 12, 1, 9, 0, 0, 0, time.Local)
	longa := strings.Repeat("regra ", 100) // 600 caracteres
	novo := func(titulo string, posicoes int, descricao *string) uint {
		return uint(execQA(t, d, `INSERT INTO desafios (titulo,descricao,categoria,valor_entrada_centavos,data_evento,data_fim_evento,status,vagas,posicoes_podio,id_criador,id_academia,
			premio_fixo_centavos,instrutor_modo_pagamento,instrutor_pct,percentual_pote,criado_em,atualizado_em)
			VALUES (?,?,'geral',1000,?,?,'aberto',200,?,?,?,0,'fixo',0,80,NOW(3),NOW(3))`, titulo, descricao, ev, ev.Add(2*time.Hour), posicoes, contaAc, idAc))
	}
	curta := "  Corrida de 5 km no parque.  "
	vazia := "   "
	d1 := novo("Desafio de um", 1, nil)
	d3 := novo("Desafio de três", 3, &curta)
	d5 := novo("Desafio de cinco", 5, &longa)
	dVazio := novo("Desafio vazio", 3, &vazia)

	if _, err := p.UpTo(ctx, 22); err != nil {
		t.Fatalf("UpTo 22: %v", err)
	}

	// U1: tabela fixa antiga em pontos-base.
	divisao := func(id uint) []int64 {
		var v []int64
		d.Raw("SELECT pct_bp FROM desafio_premio_posicoes WHERE id_desafio = ? ORDER BY posicao", id).Scan(&v)
		return v
	}
	igual := func(a, b []int64) bool {
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
	for id, esperado := range map[uint][]int64{d1: {10000}, d3: {6000, 3000, 1000}, d5: {4000, 2500, 1500, 1200, 800}, dVazio: {6000, 3000, 1000}} {
		if v := divisao(id); !igual(v, esperado) {
			t.Errorf("divisão do desafio %d = %v, esperada %v", id, v, esperado)
		}
	}
	if existe(t, d, `SELECT COUNT(*) FROM desafios d WHERE d.posicoes_podio <> (SELECT COUNT(*) FROM desafio_premio_posicoes p WHERE p.id_desafio = d.id)
		OR 10000 <> (SELECT SUM(pct_bp) FROM desafio_premio_posicoes p WHERE p.id_desafio = d.id)`) {
		t.Errorf("algum desafio ficou sem divisão completa (soma 10000, uma linha por posição)")
	}

	// U2: resumo/regras a partir da descrição.
	type textos struct {
		Resumo *string
		Regras *string
	}
	ler := func(id uint) (x textos) {
		d.Raw("SELECT resumo, regras FROM desafios WHERE id = ?", id).Scan(&x)
		return
	}
	if x := ler(d1); x.Resumo == nil || *x.Resumo != "Desafio de um" || x.Regras != nil {
		t.Errorf("sem descrição: resumo = título e regras NULL: %+v", x)
	}
	if x := ler(dVazio); x.Resumo == nil || *x.Resumo != "Desafio vazio" || x.Regras != nil {
		t.Errorf("descrição em branco: %+v", x)
	}
	if x := ler(d3); x.Resumo == nil || *x.Resumo != strings.TrimSpace(curta) || x.Regras == nil || *x.Regras != strings.TrimSpace(curta) {
		t.Errorf("descrição curta: %+v", x)
	}
	if x := ler(d5); x.Resumo == nil || len([]rune(*x.Resumo)) != 280 || !strings.HasSuffix(*x.Resumo, "…") ||
		x.Regras == nil || *x.Regras != strings.TrimSpace(longa) {
		t.Errorf("descrição longa: resumo de %d caracteres", len([]rune(*x.Resumo)))
	}

	// U3: cache do nível começa zerado e "nunca calculado".
	var pts, nivel int
	var calc *time.Time
	d.Raw("SELECT pontuacao_gymscore, nivel_gymscore, gymscore_calculado_em FROM usuarios WHERE id = ?", atleta).Row().Scan(&pts, &nivel, &calc)
	if pts != 0 || nivel != 1 || calc != nil {
		t.Errorf("usuário: pontuação %d, nível %d, calculado %v", pts, nivel, calc)
	}

	// CHECKs (3819).
	for _, c := range []struct {
		q    string
		args []any
	}{
		{"UPDATE desafios SET posicoes_podio = 0 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET posicoes_podio = 101 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET nivel_min = 0 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET nivel_max = 11 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET nivel_min = 6, nivel_max = 3 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET idade_max = 121 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET idade_min = 30, idade_max = 18 WHERE id = ?", []any{d3}},
		{"UPDATE desafios SET regras = ? WHERE id = ?", []any{strings.Repeat("x", 10001), d3}},
		{"UPDATE usuarios SET nivel_gymscore = 0 WHERE id = ?", []any{atleta}},
		{"UPDATE usuarios SET nivel_gymscore = 11 WHERE id = ?", []any{atleta}},
		{"UPDATE usuarios SET pontuacao_gymscore = -1 WHERE id = ?", []any{atleta}},
		{"INSERT INTO desafio_premio_posicoes (id_desafio, posicao, pct_bp) VALUES (?, 4, 0)", []any{d3}},
		{"INSERT INTO desafio_premio_posicoes (id_desafio, posicao, pct_bp) VALUES (?, 4, 10001)", []any{d3}},
		{"INSERT INTO desafio_premio_posicoes (id_desafio, posicao, pct_bp) VALUES (?, 101, 1)", []any{d3}},
	} {
		if err := d.Exec(c.q, c.args...).Error; err == nil || !strings.Contains(err.Error(), "3819") {
			t.Errorf("%s deveria violar um CHECK: %v", c.q, err)
		}
	}
	if err := d.Exec("INSERT INTO desafio_premio_posicoes (id_desafio, posicao, pct_bp) VALUES (?, 1, 5000)", d3).Error; err == nil || !strings.Contains(err.Error(), "1062") {
		t.Errorf("posição repetida deveria dar 1062: %v", err)
	}
	// Valores válidos: 100 premiados, 33,33%, filtros nos limites, regras com 10000 caracteres, pódio na 7ª posição.
	execQA(t, d, "UPDATE desafios SET posicoes_podio = 100, nivel_min = 1, nivel_max = 10, idade_min = 0, idade_max = 120, regras = ? WHERE id = ?", strings.Repeat("é", 10000), d5)
	execQA(t, d, "UPDATE desafios SET nivel_min = 4, nivel_max = 4, idade_min = 18 WHERE id = ?", d3)
	execQA(t, d, "DELETE FROM desafio_premio_posicoes WHERE id_desafio = ?", d3)
	execQA(t, d, "INSERT INTO desafio_premio_posicoes (id_desafio, posicao, pct_bp) VALUES (?, 1, 3334), (?, 2, 3333), (?, 3, 3333)", d3, d3, d3)
	execQA(t, d, "UPDATE usuarios SET pontuacao_gymscore = 135, nivel_gymscore = 10, gymscore_calculado_em = NOW(3) WHERE id = ?", atleta)
	execQA(t, d, "INSERT INTO desafio_participantes (id_desafio, id_usuario, codigo_ingresso, criado_em) VALUES (?, ?, ?, NOW(3))", d5, atleta, strings.Repeat("a", 32))
	execQA(t, d, "INSERT INTO podio_desafios (id_desafio, posicao, id_participante, marca, premio_centavos, criado_em) VALUES (?, 7, ?, '20:00', 0, NOW(3))", d5, atleta)

	// Down com dados que não cabem no modelo antigo: os CHECKs de 1..5 não voltam (nada é cortado).
	if _, err := p.DownTo(ctx, 21); err != nil {
		t.Fatalf("Down 22 (com 100 premiados): %v", err)
	}
	if colunaExiste(t, d, "desafios", "resumo") || colunaExiste(t, d, "desafios", "nivel_min") || colunaExiste(t, d, "desafios", "idade_max") ||
		colunaExiste(t, d, "usuarios", "nivel_gymscore") || colunaExiste(t, d, "usuarios", "gymscore_calculado_em") ||
		existe(t, d, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'desafio_premio_posicoes'") {
		t.Errorf("schema da 00022 sobrou após o Down")
	}
	checkExiste := func(nome string) bool {
		return existe(t, d, "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND constraint_name = ?", nome)
	}
	if checkExiste("chk_desafios_posicoes_podio") || checkExiste("chk_podio_posicao") || checkExiste("chk_desafios_posicoes_podio_100") {
		t.Errorf("Down com 100 premiados / pódio na 7ª: CHECKs de posição inesperados")
	}
	var desc *string
	d.Raw("SELECT descricao FROM desafios WHERE id = ?", d5).Scan(&desc)
	if desc == nil || *desc != longa {
		t.Errorf("Down mexeu na descrição legada")
	}

	// Up de novo e Up reaplicado (versão 22 apagada) sobre o schema migrado: não duplica a divisão.
	if _, err := p.UpTo(ctx, 22); err != nil {
		t.Fatalf("Up de novo: %v", err)
	}
	execQA(t, d, "DELETE FROM goose_db_version WHERE version_id = 22")
	if _, err := p.UpTo(ctx, 22); err != nil {
		t.Fatalf("Up reaplicado sobre o schema migrado: %v", err)
	}
	if v := divisao(d3); !igual(v, []int64{6000, 3000, 1000}) {
		t.Errorf("divisão após Down+Up: %v", v)
	}
	var nDiv int64
	d.Raw("SELECT COUNT(*) FROM desafio_premio_posicoes WHERE id_desafio = ?", d5).Scan(&nDiv)
	if nDiv != 0 { // posicoes_podio = 100 não tem tabela fixa: fica para a API (desafio criado pela 00022 já tem as linhas)
		t.Errorf("desafio com 100 posições ganhou %d linhas no backfill", nDiv)
	}
	if !checkExiste("chk_desafios_posicoes_podio_100") || !checkExiste("chk_podio_posicao_100") || checkExiste("chk_desafios_posicoes_podio") {
		t.Errorf("CHECKs de posição após Up")
	}

	// Down com dados que cabem: os CHECKs antigos voltam; depois sobe de novo.
	execQA(t, d, "DELETE FROM podio_desafios WHERE id_desafio = ?", d5)
	execQA(t, d, "UPDATE desafios SET posicoes_podio = 5 WHERE id = ?", d5)
	if _, err := p.DownTo(ctx, 21); err != nil {
		t.Fatalf("Down 22: %v", err)
	}
	if !checkExiste("chk_desafios_posicoes_podio") || !checkExiste("chk_podio_posicao") {
		t.Errorf("CHECKs de posição 1..5 não voltaram")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up final: %v", err)
	}
	if versaoGoose(t, d) != versaoFinal || !colunaExiste(t, d, "usuarios", "pontuacao_gymscore") {
		t.Errorf("estado após Up final incorreto")
	}
	if existe(t, d, "SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = DATABASE() AND routine_name LIKE '\\_goose\\_%'") {
		t.Errorf("sobrou procedure _goose_* temporária")
	}
}
