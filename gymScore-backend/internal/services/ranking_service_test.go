package services_test

import (
	"net/http"
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
)

type rankingRepoFake struct{ desde time.Time }

func (r *rankingRepoFake) Amigos(id uint, desde time.Time) ([]models.RankingItem, error) {
	r.desde = desde
	return nil, nil
}

func TestRankingService_JanelaEValidacao(t *testing.T) {
	repo := &rankingRepoFake{}
	svc := services.NovoRankingService(repo)
	itens, err := svc.Amigos(1, 0) // 0 = padrão 30 dias
	if err != nil || itens == nil || len(itens) != 0 {
		t.Fatalf("lista vazia (não nil) esperada: %v %v", itens, err)
	}
	if d := time.Since(repo.desde); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Errorf("janela padrão deveria ser ~30 dias: %v", d)
	}
	for _, dias := range []int{-1, 366} {
		if _, err := svc.Amigos(1, dias); err == nil || statusDe(t, err) != http.StatusBadRequest {
			t.Errorf("dias=%d deveria dar 400: %v", dias, err)
		}
	}
}
