package models

import "time"

// Instrutor representa um profissional de educação física cadastrado no sistema
type Instrutor struct {
	ID        uint     `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario uint     `gorm:"uniqueIndex;not null" json:"id_usuario"`
	Usuario   *Usuario `gorm:"foreignKey:IDUsuario" json:"usuario,omitempty"`
	// CREF é o registro profissional obrigatório (ex: 001234-G/SP)
	CREF          string `gorm:"type:varchar(20);not null" json:"cref"`
	Especialidade string `gorm:"type:varchar(100)" json:"especialidade"`
	// IDAcademia vínculo opcional com academia — null se autônomo
	IDAcademia   *uint     `gorm:"default:null" json:"id_academia,omitempty"`
	CriadoEm     time.Time `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time `gorm:"autoUpdateTime" json:"atualizado_em"`
}

// TableName define o nome da tabela no banco de dados
func (Instrutor) TableName() string {
	return "instrutores"
}

// CriarInstrutorRequest é o DTO de entrada para cadastro de instrutor
type CriarInstrutorRequest struct {
	IDUsuario     uint   `json:"id_usuario" validate:"required"`
	CREF          string `json:"cref" validate:"required"`
	Especialidade string `json:"especialidade"`
	IDAcademia    *uint  `json:"id_academia,omitempty"`
}
