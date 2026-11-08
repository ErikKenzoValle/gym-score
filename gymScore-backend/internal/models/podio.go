package models

import "time"

// PodioDesafio representa a posição de um participante no pódio de um desafio encerrado
// Constraint única (id_desafio, posicao) — garante que cada posição do pódio é ocupada por um único participante
type PodioDesafio struct {
	ID             uint     `gorm:"primaryKey;autoIncrement" json:"id"`
	IDDesafio      uint     `gorm:"not null;uniqueIndex:uq_podio_posicao" json:"id_desafio"`
	Posicao        int      `gorm:"not null;uniqueIndex:uq_podio_posicao" json:"posicao"`
	IDParticipante uint     `gorm:"not null" json:"id_participante"`
	Participante   *Usuario `gorm:"foreignKey:IDParticipante" json:"participante,omitempty"`
	// Marca registro da performance do participante (tempo, peso, repetições, etc.)
	Marca string `gorm:"type:varchar(50)" json:"marca"`
	// PremioCentavos é o prêmio creditado a esta posição (divisão do pote)
	PremioCentavos int64     `gorm:"column:premio_centavos;not null;default:0" json:"premio_centavos"`
	CriadoEm       time.Time `gorm:"autoCreateTime" json:"criado_em"`
}

// TableName define o nome da tabela no banco de dados
func (PodioDesafio) TableName() string {
	return "podio_desafios"
}

// PodioEntrada representa uma posição individual no payload de lançamento de pódio
type PodioEntrada struct {
	IDParticipante uint   `json:"id_participante" validate:"required"`
	Marca          string `json:"marca" validate:"required,max=50"`
}

// LancarPodioRequest é o DTO para lançamento do pódio de um desafio (índice 0 = 1º lugar).
// A quantidade exata de entradas (min(posicoes_podio, presentes)) é validada no service.
type LancarPodioRequest struct {
	Posicoes []PodioEntrada `json:"posicoes" validate:"required,min=1,max=5,dive"`
}

// AvaliacaoDesafio representa a avaliação de um usuário sobre um desafio que participou
// Constraint única (id_desafio, id_usuario) — um usuário avalia cada desafio apenas uma vez
type AvaliacaoDesafio struct {
	ID        uint `gorm:"primaryKey;autoIncrement" json:"id"`
	IDDesafio uint `gorm:"not null;uniqueIndex:uq_avaliacao" json:"id_desafio"`
	IDUsuario uint `gorm:"not null;uniqueIndex:uq_avaliacao" json:"id_usuario"`
	// Nota de 1 a 5; o check constraint é aplicado na camada de serviço e no validate
	Nota     int       `gorm:"not null;check:nota >= 1 AND nota <= 5" json:"nota"`
	CriadoEm time.Time `gorm:"autoCreateTime" json:"criado_em"`
}

// TableName define o nome da tabela no banco de dados
func (AvaliacaoDesafio) TableName() string {
	return "avaliacoes_desafios"
}

// AvaliarDesafioRequest é o DTO de entrada para avaliação de desafio
type AvaliarDesafioRequest struct {
	Nota int `json:"nota" validate:"required,min=1,max=5"`
}
