package services_test

import (
	"testing"

	"gymScore-backend/internal/services"
)

func somaPremios(ps []services.Premio) int64 {
	var s int64
	for _, p := range ps {
		s += p.Centavos
	}
	return s
}

func TestCalcularApuracao_VencedorUnicoLevaOPote(t *testing.T) {
	ps := services.CalcularApuracao([]uint{3, 1, 2}, map[uint]int{2: 2, 1: 1}, 3000)
	if len(ps) != 1 || ps[0].IDUsuario != 2 || ps[0].Centavos != 3000 {
		t.Fatalf("esperava usuário 2 com 3000, obteve %+v", ps)
	}
}

func TestCalcularApuracao_EmpateDivideComRestoParaMenorID(t *testing.T) {
	// 3 empatados, pote 1000 => 333 cada + 1 de resto para o menor id
	ps := services.CalcularApuracao([]uint{9, 5, 7}, map[uint]int{9: 1, 5: 1, 7: 1}, 1000)
	if len(ps) != 3 {
		t.Fatalf("esperava 3 vencedores, obteve %d", len(ps))
	}
	if ps[0].IDUsuario != 5 || ps[0].Centavos != 334 {
		t.Errorf("menor id deveria receber 334: %+v", ps[0])
	}
	if ps[1].Centavos != 333 || ps[2].Centavos != 333 {
		t.Errorf("demais deveriam receber 333: %+v", ps)
	}
	if somaPremios(ps) != 1000 {
		t.Errorf("soma dos prêmios deve ser o pote, obteve %d", somaPremios(ps))
	}
}

func TestCalcularApuracao_SemVotosTodosEmpatam(t *testing.T) {
	ps := services.CalcularApuracao([]uint{1, 2}, nil, 2000)
	if len(ps) != 2 || ps[0].Centavos != 1000 || ps[1].Centavos != 1000 {
		t.Fatalf("esperava devolução igual, obteve %+v", ps)
	}
}

func TestCalcularApuracao_IgnoraVotoEmNaoParticipante(t *testing.T) {
	ps := services.CalcularApuracao([]uint{1, 2}, map[uint]int{99: 5, 1: 1}, 500)
	if len(ps) != 1 || ps[0].IDUsuario != 1 {
		t.Fatalf("voto em id fora dos participantes deve ser ignorado: %+v", ps)
	}
}

func TestCalcularApuracao_ConservaCentavos(t *testing.T) {
	for pote := int64(0); pote < 200; pote++ {
		for n := 1; n <= 7; n++ {
			ids := make([]uint, n)
			for i := range ids {
				ids[i] = uint(i + 1)
			}
			if got := somaPremios(services.CalcularApuracao(ids, nil, pote)); got != pote {
				t.Fatalf("pote %d, n=%d: soma %d", pote, n, got)
			}
		}
	}
}
