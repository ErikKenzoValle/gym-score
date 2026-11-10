package services_test

import (
	"reflect"
	"testing"

	"gymScore-backend/internal/services"
)

func somaInt64(v []int64) (s int64) {
	for _, x := range v {
		s += x
	}
	return
}

func TestDividirPremio_TabelaPadraoPorTamanhoDoPodio(t *testing.T) {
	casos := []struct {
		posicoes int
		pote     int64
		want     []int64
	}{
		{1, 10000, []int64{10000}},
		{2, 10000, []int64{7000, 3000}},
		{3, 10000, []int64{6000, 3000, 1000}},
		{4, 10000, []int64{5000, 2500, 1500, 1000}},
		{5, 10000, []int64{4000, 2500, 1500, 1200, 800}},
	}
	for _, c := range casos {
		if got := services.DividirPremio(c.pote, services.DivisaoPadraoBP(c.posicoes), c.posicoes); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%d posições, pote %d: esperava %v, obteve %v", c.posicoes, c.pote, c.want, got)
		}
	}
}

func TestDividirPremio_RestoDeCentavosVaiParaOPrimeiro(t *testing.T) {
	// 1001 em 60/30/10: 600 + 300 + 100 = 1000, resto 1 -> 1º
	if got, want := services.DividirPremio(1001, services.DivisaoPadraoBP(3), 3), []int64{601, 300, 100}; !reflect.DeepEqual(got, want) {
		t.Errorf("esperava %v, obteve %v", want, got)
	}
	// 7 centavos em 70/30: 4 + 2 = 6, resto 1 -> 1º
	if got, want := services.DividirPremio(7, services.DivisaoPadraoBP(2), 2), []int64{5, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("esperava %v, obteve %v", want, got)
	}
}

func TestDividirPremio_MenosPresentesQuePosicoesRenormaliza(t *testing.T) {
	// pódio de 3 com 2 presentes: 60/30 renormalizado = 2/3 e 1/3 de 9000
	if got, want := services.DividirPremio(9000, services.DivisaoPadraoBP(3), 2), []int64{6000, 3000}; !reflect.DeepEqual(got, want) {
		t.Errorf("3 posições/2 presentes: esperava %v, obteve %v", want, got)
	}
	// pódio de 5 com 1 presente: leva tudo
	if got, want := services.DividirPremio(12345, services.DivisaoPadraoBP(5), 1), []int64{12345}; !reflect.DeepEqual(got, want) {
		t.Errorf("5 posições/1 presente: esperava %v, obteve %v", want, got)
	}
	// pódio de 5 com 3 presentes: 40/25/15 sobre 80 -> 50%/31,25%/18,75% de 1600
	if got, want := services.DividirPremio(1600, services.DivisaoPadraoBP(5), 3), []int64{800, 500, 300}; !reflect.DeepEqual(got, want) {
		t.Errorf("5 posições/3 presentes: esperava %v, obteve %v", want, got)
	}
}

func TestDividirPremio_ConservaCentavosSempre(t *testing.T) {
	for posicoes := 1; posicoes <= 5; posicoes++ {
		for ocupadas := 1; ocupadas <= posicoes; ocupadas++ {
			for _, pote := range []int64{0, 1, 2, 3, 7, 99, 100, 101, 1234, 9999, 100001, 500_000_000} {
				got := services.DividirPremio(pote, services.DivisaoPadraoBP(posicoes), ocupadas)
				if len(got) != ocupadas {
					t.Fatalf("pote %d, %d/%d: esperava %d prêmios, obteve %d", pote, posicoes, ocupadas, ocupadas, len(got))
				}
				if s := somaInt64(got); s != pote {
					t.Errorf("pote %d, %d/%d: soma %d (%v) != pote", pote, posicoes, ocupadas, s, got)
				}
				for i, v := range got {
					if v < 0 {
						t.Errorf("prêmio negativo na posição %d: %v", i+1, got)
					}
				}
			}
		}
	}
}

func TestDividirPremio_PoteZeroEEntradasInvalidas(t *testing.T) {
	if got := services.DividirPremio(0, services.DivisaoPadraoBP(3), 3); !reflect.DeepEqual(got, []int64{0, 0, 0}) {
		t.Errorf("pote zero (desafio gratuito) deve dar prêmios zero, obteve %v", got)
	}
	if got := services.DividirPremio(1000, services.DivisaoPadraoBP(3), 0); got != nil {
		t.Errorf("sem ocupadas deve devolver nil, obteve %v", got)
	}
	// ocupadas maior que as posições é limitado às posições
	if got := services.DividirPremio(1000, services.DivisaoPadraoBP(2), 4); len(got) != 2 {
		t.Errorf("deveria limitar às posições configuradas, obteve %v", got)
	}
}

func TestPontosPorColocacao(t *testing.T) {
	if services.PontosPorColocacao(1) != 30 {
		t.Error("1º deve ganhar 30 pontos")
	}
	for p := 2; p <= 5; p++ {
		if services.PontosPorColocacao(p) != 15 {
			t.Errorf("%dº deve ganhar 15 pontos", p)
		}
	}
}

func TestDivisaoPadraoBP_ValidaParaTodoTamanho(t *testing.T) {
	if got, want := services.DivisaoPadraoBP(3), []int{6000, 3000, 1000}; !reflect.DeepEqual(got, want) {
		t.Errorf("3 posições: %v, esperava %v", got, want)
	}
	// 6 posições: pesos 6..1 de 21 -> floor(10000*k/21), sobra para o 1º.
	if got, want := services.DivisaoPadraoBP(6), []int{2860, 2380, 1904, 1428, 952, 476}; !reflect.DeepEqual(got, want) {
		t.Errorf("6 posições: %v, esperava %v", got, want)
	}
	for n := 1; n <= 100; n++ {
		bp := services.DivisaoPadraoBP(n)
		if err := services.ValidarDivisaoBP(bp, n); err != nil {
			t.Fatalf("divisão padrão de %d posições inválida (%v): %v", n, bp, err)
		}
	}
	if services.DivisaoPadraoBP(0) != nil || services.DivisaoPadraoBP(101) != nil {
		t.Error("fora de 1..100 deveria dar nil")
	}
}

func TestValidarDivisaoBP_Regras(t *testing.T) {
	ok := [][]int{{10000}, {3334, 3333, 3333}, {5000, 5000}, {9997, 1, 1, 1}}
	for _, bp := range ok {
		if err := services.ValidarDivisaoBP(bp, len(bp)); err != nil {
			t.Errorf("%v deveria ser válida: %v", bp, err)
		}
	}
	ruins := []struct {
		bp []int
		n  int
	}{
		{[]int{6000, 4000}, 3},       // quantidade errada
		{[]int{5000, 4000, 1000}, 2}, // quantidade errada
		{[]int{6000, 3000, 999}, 3},  // soma 9999
		{[]int{3000, 6000, 1000}, 3}, // crescente
		{[]int{10000, 0}, 2},         // zero
		{[]int{12000, -2000}, 2},     // negativo / > 100%
	}
	for _, c := range ruins {
		if err := services.ValidarDivisaoBP(c.bp, c.n); err == nil {
			t.Errorf("%v (n=%d) deveria ser recusada", c.bp, c.n)
		}
	}
}

func TestDividirPremio_TabelaDaAcademia(t *testing.T) {
	// 33,34% / 33,33% / 33,33% de 10000 centavos: 3334 + 3333 + 3333.
	if got, want := services.DividirPremio(10000, []int{3334, 3333, 3333}, 3), []int64{3334, 3333, 3333}; !reflect.DeepEqual(got, want) {
		t.Errorf("33/33/33: %v, esperava %v", got, want)
	}
	// 1 centavo em 3: tudo para o 1º.
	if got, want := services.DividirPremio(1, []int{3334, 3333, 3333}, 3), []int64{1, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("1 centavo: %v, esperava %v", got, want)
	}
	// 10 posições, 4 presentes: renormaliza entre as 4 primeiras e conserva os centavos.
	bp := services.DivisaoPadraoBP(10)
	got := services.DividirPremio(99_999, bp, 4)
	if len(got) != 4 || somaInt64(got) != 99_999 {
		t.Fatalf("10 posições/4 presentes: %v (soma %d)", got, somaInt64(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i] > got[i-1] {
			t.Errorf("prêmios deveriam ser não crescentes: %v", got)
		}
	}
	// 100 posições, todos presentes: conserva centavos para vários totais.
	bp100 := services.DivisaoPadraoBP(100)
	for _, total := range []int64{0, 1, 99, 10_000, 1_234_567, 500_000_000} {
		g := services.DividirPremio(total, bp100, 100)
		if len(g) != 100 || somaInt64(g) != total {
			t.Errorf("100 posições, total %d: soma %d", total, somaInt64(g))
		}
	}
}
