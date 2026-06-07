package models

import "time"

// StatusAmizade define os possíveis estados de uma amizade
type StatusAmizade string

const (
	StatusAmizadePendente StatusAmizade = "pendente"
	StatusAmizadeAceita   StatusAmizade = "aceita"
	StatusAmizadeRecusada StatusAmizade = "recusada"
)

// Amizade representa o relacionamento entre dois usuários
type Amizade struct {
	ID         uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario  uint          `gorm:"not null;index" json:"id_usuario"`
	IDAmigo    uint          `gorm:"not null;index" json:"id_amigo"`
	Status     StatusAmizade `gorm:"type:enum('pendente','aceita','recusada');default:'pendente'" json:"status"`
	CriadoEm  time.Time     `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time  `gorm:"autoUpdateTime" json:"atualizado_em"`

	// Relacionamentos
	Usuario *Usuario `gorm:"foreignKey:IDUsuario" json:"usuario,omitempty"`
	Amigo   *Usuario `gorm:"foreignKey:IDAmigo" json:"amigo,omitempty"`
}

// TableName define o nome da tabela no banco de dados
func (Amizade) TableName() string {
	return "amizades"
}

// AdicionarAmigoRequest é o DTO para enviar solicitação de amizade
type AdicionarAmigoRequest struct {
	IDUsuario uint `json:"id_usuario" validate:"required"`
	IDAmigo   uint `json:"id_amigo" validate:"required"`
}

// AceitarAmizadeRequest é o DTO para aceitar uma solicitação de amizade
type AceitarAmizadeRequest struct {
	IDUsuario uint `json:"id_usuario" validate:"required"`
	IDAmigo   uint `json:"id_amigo" validate:"required"`
}

// RemoverAmigoRequest é o DTO para remover um amigo
type RemoverAmigoRequest struct {
	IDUsuario uint `json:"id_usuario" validate:"required"`
	IDAmigo   uint `json:"id_amigo" validate:"required"`
}

// AmigoResponse é o DTO de retorno com dados do amigo
type AmigoResponse struct {
	ID        uint   `json:"id"`
	Nome      string `json:"nome"`
	Sobrenome string `json:"sobrenome"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	// Recebida indica que esta é uma solicitação pendente recebida pelo usuário consultado
	// (ou seja, ele pode aceitá-la). False quando foi ele quem enviou.
	Recebida bool `json:"recebida"`
}
