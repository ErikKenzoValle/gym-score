package models

import "time"

// StatusAmizade define os possíveis estados de uma amizade
type StatusAmizade string

const (
	StatusAmizadePendente StatusAmizade = "pendente"
	StatusAmizadeAceita   StatusAmizade = "aceita"
	StatusAmizadeRecusada StatusAmizade = "recusada"
)

// Amizade representa o relacionamento entre dois usuários.
// IDUsuario é quem enviou a solicitação; IDAmigo é o destinatário.
type Amizade struct {
	ID           uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario    uint          `gorm:"not null;index" json:"id_usuario"`
	IDAmigo      uint          `gorm:"not null;index" json:"id_amigo"`
	Status       StatusAmizade `gorm:"type:enum('pendente','aceita','recusada');default:'pendente'" json:"status"`
	CriadoEm     time.Time     `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time     `gorm:"autoUpdateTime" json:"atualizado_em"`
	// Par é coluna gerada pelo banco (menor-maior): somente leitura, nunca escrita.
	Par string `gorm:"->" json:"-"`

	// Relacionamentos
	Usuario *Usuario `gorm:"foreignKey:IDUsuario" json:"-"`
	Amigo   *Usuario `gorm:"foreignKey:IDAmigo" json:"-"`
}

// TableName define o nome da tabela no banco de dados
func (Amizade) TableName() string {
	return "amizades"
}

// AmigoAlvoRequest é o DTO das ações de amizade (adicionar/aceitar/recusar/remover).
// A identidade de quem age vem sempre do JWT; aqui só vai o ALVO da ação.
// Em "aceitar"/"recusar", id_amigo é quem enviou a solicitação.
type AmigoAlvoRequest struct {
	IDAmigo uint `json:"id_amigo" validate:"required"`
}

// AmigoResponse é o DTO de retorno com dados do amigo (sem e-mail/CPF)
type AmigoResponse struct {
	ID        uint   `json:"id"`
	Nome      string `json:"nome"`
	Sobrenome string `json:"sobrenome"`
	Status    string `json:"status"`
	// Recebida indica que esta é uma solicitação pendente recebida pelo usuário consultado
	// (ou seja, ele pode aceitá-la/recusá-la). False quando foi ele quem enviou.
	Recebida bool `json:"recebida"`
}
