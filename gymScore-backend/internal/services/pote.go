package services

import (
	"fmt"

	"gymScore-backend/internal/models"
)

// percentuaisPodio é a divisão SUGERIDA do pote (em %) para pódios de 1 a 5 posições — a tabela fixa
// usada até a rodada 6 e o backfill dos desafios legados (migration 00022). Cada linha soma 100.
// Desde a rodada 6 a academia define a divisão (desafio_premio_posicoes); isto é só o padrão.
var percentuaisPodio = map[int][]int{
	1: {100},
	2: {70, 30},
	3: {60, 30, 10},
	4: {50, 25, 15, 10},
	5: {40, 25, 15, 12, 8},
}

// Gamificação ao encerrar o desafio: 1º colocado +30, demais do pódio +15, demais presentes +5.
// (Pontos/Elo antigos; o Nível GymScore tem a sua própria tabela em gymscore.go.)
const (
	pontosPrimeiro = 30
	pontosPodio    = 15
	pontosPresente = 5
)

// PontosPorColocacao devolve os pontos (gamificação antiga) de quem ficou na posição do pódio.
func PontosPorColocacao(posicao int) int {
	if posicao == 1 {
		return pontosPrimeiro
	}
	return pontosPodio
}

// DivisaoPadraoBP devolve a divisão sugerida (pontos-base, soma 10000, não crescente, cada > 0) para um
// pódio de n posições (1..100): até 5 é a tabela fixa antiga; acima disso, pesos lineares decrescentes
// (n, n-1, ..., 1) arredondados para baixo, com a sobra no 1º. nil se n fora de 1..100.
func DivisaoPadraoBP(n int) []int {
	if n < 1 || n > models.PosicoesPodioTeto {
		return nil
	}
	if pct, ok := percentuaisPodio[n]; ok {
		out := make([]int, n)
		for i, p := range pct {
			out[i] = p * 100
		}
		return out
	}
	soma := n * (n + 1) / 2
	out := make([]int, n)
	distribuido := 0
	for i := 0; i < n; i++ {
		out[i] = models.PremioPctBPTotal * (n - i) / soma
		distribuido += out[i]
	}
	out[0] += models.PremioPctBPTotal - distribuido
	return out
}

// ValidarDivisaoBP confere a divisão do prêmio de um pódio de `posicoes` posições (U1): uma fatia por
// posição, cada uma > 0, soma exata 10000 (100,00%) e não crescente (a posição N+1 nunca recebe mais que
// a N). Devolve erro 400 com a mensagem para a tela.
func ValidarDivisaoBP(bp []int, posicoes int) error {
	if len(bp) != posicoes {
		return erroRequisicao(fmt.Sprintf("divisao_premio_bp deve ter exatamente %d valores (um por posição premiada)", posicoes))
	}
	soma := 0
	for i, v := range bp {
		if v <= 0 {
			return erroRequisicao(fmt.Sprintf("divisao_premio_bp: a %dª posição precisa receber mais que 0%%", i+1))
		}
		if v > models.PremioPctBPTotal {
			return erroRequisicao(fmt.Sprintf("divisao_premio_bp: a %dª posição passa de 100%%", i+1))
		}
		if i > 0 && v > bp[i-1] {
			return erroRequisicao(fmt.Sprintf("divisao_premio_bp: a %dª posição não pode receber mais que a %dª", i+1, i))
		}
		soma += v
	}
	if soma != models.PremioPctBPTotal {
		return erroRequisicao(fmt.Sprintf("divisao_premio_bp deve somar 10000 (100%%); soma atual %d (%s)", soma, pctBPTexto(soma)))
	}
	return nil
}

// pctBPTexto formata pontos-base como porcentagem brasileira ("33,33%").
func pctBPTexto(bp int) string {
	return fmt.Sprintf("%d,%02d%%", bp/100, bp%100)
}

// divisaoDoDesafio devolve a divisão do prêmio gravada no desafio (desafio_premio_posicoes, em ordem de
// posição). Se a tabela não estiver carregada ou não bater com posicoes_podio (não deveria acontecer:
// a 00022 preencheu todos e a API grava junto com o desafio), cai na divisão padrão — o mesmo
// comportamento de antes da rodada 6.
func divisaoDoDesafio(d *models.Desafio) []int {
	if len(d.PremioPosicoes) == d.PosicoesPodio && d.PosicoesPodio > 0 {
		out := make([]int, len(d.PremioPosicoes))
		for i, p := range d.PremioPosicoes {
			if p.Posicao != i+1 {
				return DivisaoPadraoBP(d.PosicoesPodio)
			}
			out[i] = p.PctBP
		}
		if ValidarDivisaoBP(out, d.PosicoesPodio) == nil {
			return out
		}
	}
	return DivisaoPadraoBP(d.PosicoesPodio)
}

// DividirPremio reparte o total (centavos) entre as `presentes` primeiras posições de um pódio cuja
// divisão é pctBP (pontos-base por posição, índice 0 = 1º; soma 10000). Se há menos presentes que
// posições, as posições vazias somem e as fatias das ocupadas são renormalizadas (ex.: 60/30/10 com 2
// presentes: 66,7%/33,3%). Cada prêmio é arredondado para baixo e o resto de centavos vai para o 1º
// colocado, então a soma dos prêmios é SEMPRE igual ao total. Retorna min(presentes, len(pctBP))
// valores (nil se presentes <= 0, total < 0 ou divisão vazia/inválida).
func DividirPremio(total int64, pctBP []int, presentes int) []int64 {
	if presentes > len(pctBP) {
		presentes = len(pctBP)
	}
	if presentes <= 0 || total < 0 {
		return nil
	}
	pct := pctBP[:presentes]
	var soma int64
	for _, p := range pct {
		if p < 0 {
			return nil
		}
		soma += int64(p)
	}
	premios := make([]int64, presentes)
	if soma == 0 {
		premios[0] = total
		return premios
	}
	var distribuido int64
	for i, p := range pct {
		// total <= 1e10 centavos e p <= 1e4: o produto cabe folgado em int64.
		premios[i] = total * int64(p) / soma
		distribuido += premios[i]
	}
	premios[0] += total - distribuido
	return premios
}
