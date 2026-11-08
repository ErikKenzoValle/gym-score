package services

// percentuaisPodio é a divisão padrão do pote (em %) por tamanho de pódio configurado na criação
// do desafio (1 a 5 posições). Cada linha soma 100.
var percentuaisPodio = map[int][]int64{
	1: {100},
	2: {70, 30},
	3: {60, 30, 10},
	4: {50, 25, 15, 10},
	5: {40, 25, 15, 12, 8},
}

// Gamificação ao encerrar o desafio: 1º colocado +30, 2º a 5º +15, demais presentes +5.
const (
	pontosPrimeiro   = 30
	pontosPodio      = 15
	pontosPresente   = 5
	posicoesPodioMax = 5
)

// PontosPorColocacao devolve os pontos de quem ficou na posição (1..5) do pódio.
func PontosPorColocacao(posicao int) int {
	if posicao == 1 {
		return pontosPrimeiro
	}
	return pontosPodio
}

// DividirPremio reparte o total (centavos) entre as `presentes` (limitado às posições) primeiras posições do pódio configurado
// com `posicoes` posições. Se há menos presentes que posições, as posições vazias somem e os
// percentuais das ocupadas são renormalizados (ex.: pódio de 3 com 2 presentes: 60/30 -> 66,7%/33,3%).
// Cada prêmio é arredondado para baixo e o resto de centavos vai para o 1º colocado, então a soma
// dos prêmios é SEMPRE igual ao total. Retorna min(presentes, posicoes) valores (nil se presentes <= 0).
func DividirPremio(total int64, posicoes, presentes int) []int64 {
	if posicoes < 1 {
		posicoes = 1
	}
	if posicoes > posicoesPodioMax {
		posicoes = posicoesPodioMax
	}
	if presentes > posicoes {
		presentes = posicoes
	}
	if presentes <= 0 || total < 0 {
		return nil
	}
	pct := percentuaisPodio[posicoes][:presentes]
	var soma int64
	for _, p := range pct {
		soma += p
	}
	premios := make([]int64, presentes)
	var distribuido int64
	for i, p := range pct {
		premios[i] = total * p / soma
		distribuido += premios[i]
	}
	premios[0] += total - distribuido
	return premios
}
