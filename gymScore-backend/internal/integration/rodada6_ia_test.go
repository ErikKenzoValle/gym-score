//go:build integration

package integration

import (
	"testing"
	"time"
)

// A6-1 (rodada 6): a recomendação usa o MESMO pode_entrar do feed — conflito de interesse e choque de
// horário excluem o desafio — e cada item traz o "por que" (razao + razao_fonte + base). Sem chave da
// Anthropic na suíte: a razão é sempre a determinística (nenhuma chamada à API real).
func TestR6_RecomendacaoRespeitaConflitoChoqueEExplica(t *testing.T) {
	c := novoCenario(t)
	outra := novoCenario(t)
	criarEm := func(cen *cenario, h float64, dur time.Duration) uint {
		ini, fim := horario(h, dur)
		b := corpoDesafio(cen, 0, 10, 1)
		b["data_evento"], b["data_fim_evento"] = ini, fim
		return idDe(esperar(t, api(t, "POST", "/api/desafios", cen.A.Token, b), 201))
	}
	a := criarEm(c, 30, 2*time.Hour)     // [30, 32)
	b := criarEm(c, 31, 2*time.Hour)     // [31, 33): choca com A (mesma academia)
	d := criarEm(outra, 30, 2*time.Hour) // outra academia, mesmo horário: não choca

	u := novoUsuario(t)
	esperar(t, acao(t, u, a, "entrar", nil), 200)
	v, _ := novoInstrutorDe(t, c.AcademiaID) // instrutor vinculado à academia de c, no modo atleta
	modoAtleta(t, v)

	// o banco é compartilhado: tira os demais abertos do "futuro" para o top 5 só ter os deste teste
	db.Exec("UPDATE desafios SET data_evento = ? WHERE status = 'aberto' AND id NOT IN ?",
		time.Now().Add(-24*time.Hour), []uint{a, b, d})

	recomendados := func(quem *user) map[uint]map[string]any {
		t.Helper()
		out := map[uint]map[string]any{}
		for _, it := range esperar(t, api(t, "GET", "/api/desafios/recomendados", quem.Token, nil), 200).Body["data"].([]any) {
			m := it.(map[string]any)
			out[uint(m["desafio"].(map[string]any)["id"].(float64))] = m
		}
		return out
	}

	// u: já está em A; B choca com A (mesma academia) → só D
	ru := recomendados(u)
	if _, ok := ru[b]; ok {
		t.Errorf("desafio com choque de horário não pode ser recomendado: %v", ru[b])
	}
	if _, ok := ru[a]; ok {
		t.Error("desafio em que já está inscrito não pode ser recomendado")
	}
	item, ok := ru[d]
	if !ok {
		t.Fatalf("desafio de outra academia (sem choque) deveria ser recomendado: %d itens", len(ru))
	}
	if item["razao"] == "" || item["razao_fonte"] != "regras" {
		t.Errorf("sem chave da IA a razão deve ser a determinística: %v / %v", item["razao"], item["razao_fonte"])
	}
	base, _ := item["base"].(map[string]any)
	if base == nil || base["categoria"] != "corrida" || base["nivel"] == nil || base["atributos"] == nil || base["score"] == nil {
		t.Errorf("base da recomendação incompleta: %v", item["base"])
	}
	if item["desafio"].(map[string]any)["pode_entrar"] != true {
		t.Errorf("recomendado deveria sair com pode_entrar=true: %v", item["desafio"])
	}

	// v: vinculado à academia de c → A e B ficam de fora (conflito de interesse); D aparece
	rv := recomendados(v)
	if _, ok := rv[a]; ok {
		t.Error("instrutor vinculado não pode receber desafio da própria academia (A)")
	}
	if _, ok := rv[b]; ok {
		t.Error("instrutor vinculado não pode receber desafio da própria academia (B)")
	}
	if _, ok := rv[d]; !ok {
		t.Errorf("desafio de outra academia deveria ser recomendado ao instrutor no modo atleta: %d itens", len(rv))
	}
}
