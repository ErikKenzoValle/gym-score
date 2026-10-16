package services

import "sort"

// Premio é o resultado da apuração para um participante vencedor.
type Premio struct {
	IDUsuario uint
	Centavos  int64
}

// CalcularApuracao decide os vencedores e a divisão do pote.
//
//   - participantes: ids dos inscritos;
//   - votos: contagem de votos recebidos por id (ids fora de participantes são ignorados);
//   - pote: soma das entradas, em centavos.
//
// Regras: vence o mais votado; empate divide o pote igualmente entre os empatados e o
// resto (centavos) vai para o de menor id. Sem votos, todos empatam (cada um recebe de
// volta, proporcionalmente, o que pagou). Nunca perde nem cria centavos: a soma dos
// prêmios é sempre igual ao pote.
func CalcularApuracao(participantes []uint, votos map[uint]int, pote int64) []Premio {
	if len(participantes) == 0 {
		return nil
	}
	ids := append([]uint(nil), participantes...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	max := 0
	for _, id := range ids {
		if votos[id] > max {
			max = votos[id]
		}
	}
	var vencedores []uint
	for _, id := range ids {
		if max == 0 || votos[id] == max {
			vencedores = append(vencedores, id)
		}
	}

	n := int64(len(vencedores))
	parte, resto := pote/n, pote%n
	premios := make([]Premio, 0, len(vencedores))
	for i, id := range vencedores {
		valor := parte
		if i == 0 {
			valor += resto // menor id fica com o resto
		}
		premios = append(premios, Premio{IDUsuario: id, Centavos: valor})
	}
	return premios
}
