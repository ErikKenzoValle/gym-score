package models

// DesafioPremioPosicao é a fatia do prêmio total de uma posição do pódio (tabela desafio_premio_posicoes,
// migration 00022). PctBP em pontos-base: 10000 = 100% (permite 33,33% = 3333).
// PK (id_desafio, posicao); posicao 1..100 e pct_bp 1..10000 têm CHECK no banco. Regras que cruzam linhas
// ficam na API: uma linha por posição 1..desafios.posicoes_podio, soma = 10000, não crescente.
// Gravar (substituir) na MESMA transação que cria/edita o desafio. Legados recebem a tabela fixa antiga.
type DesafioPremioPosicao struct {
	IDDesafio uint `gorm:"column:id_desafio;primaryKey;autoIncrement:false"`
	Posicao   int  `gorm:"column:posicao;primaryKey;autoIncrement:false"`
	PctBP     int  `gorm:"column:pct_bp;not null"`
}

func (DesafioPremioPosicao) TableName() string { return "desafio_premio_posicoes" }

// Limites da premiação configurável (00022).
const (
	PremioPctBPTotal  = 10000 // soma das fatias de um desafio
	PosicoesPodioTeto = 100   // CHECK chk_desafios_posicoes_podio_100 / chk_podio_posicao_100
)

// Limites dos filtros de entrada (CHECKs chk_desafios_nivel_filtro / chk_desafios_idade_filtro, 00022)
// e do Nível GymScore (chk_usuarios_nivel_gymscore).
const (
	NivelGymScoreMin = 1
	NivelGymScoreMax = 10
	IdadeFiltroMax   = 120
)
