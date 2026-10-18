package repositories

import (
	"testing"

	"gymScore-backend/internal/models"
)

func TestSortRanking_PontosDescEmpateCompartilhaPosicao(t *testing.T) {
	itens := []models.RankingItem{
		{ID: 1, Nome: "Zeca", Pontos: 5},
		{ID: 2, Nome: "Ana", Pontos: 9},
		{ID: 3, Nome: "Bia", Pontos: 5},
		{ID: 4, Nome: "Caio", Pontos: 0},
	}
	sortRanking(itens)
	ordem := []uint{2, 3, 1, 4}
	pos := []int{1, 2, 2, 4}
	for i := range itens {
		if itens[i].ID != ordem[i] || itens[i].Posicao != pos[i] {
			t.Errorf("posição %d: obteve id=%d pos=%d, esperava id=%d pos=%d", i, itens[i].ID, itens[i].Posicao, ordem[i], pos[i])
		}
	}
}
