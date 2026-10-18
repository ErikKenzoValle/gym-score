package models

import "time"

// Academia representa os dados de pessoa jurídica de uma conta com perfil "academia".
// Vincula-se 1:1 a um Usuario (via IDUsuario) que carrega os dados de acesso (email/senha).
type Academia struct {
	ID        uint     `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario uint     `gorm:"uniqueIndex;not null" json:"id_usuario"`
	Usuario   *Usuario `gorm:"foreignKey:IDUsuario" json:"usuario,omitempty"`
	// RazaoSocial é a denominação legal da empresa perante a Receita Federal
	RazaoSocial string `gorm:"type:varchar(150);not null" json:"razao_social"`
	// NomeFantasia é o nome comercial exibido no app
	NomeFantasia string `gorm:"type:varchar(150);not null" json:"nome_fantasia"`
	// CNPJ é armazenado formatado ("00.000.000/0000-00")
	CNPJ         string    `gorm:"type:varchar(18);uniqueIndex;not null" json:"cnpj"`
	Endereco     string    `gorm:"type:varchar(255)" json:"endereco,omitempty"`
	Telefone     string    `gorm:"type:varchar(20)" json:"telefone,omitempty"`
	CriadoEm     time.Time `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time `gorm:"autoUpdateTime" json:"atualizado_em"`
}

// TableName define o nome da tabela no banco de dados
func (Academia) TableName() string {
	return "academias"
}

// CriarAcademiaRequest é o DTO de entrada para o cadastro público de conta de academia
// (fluxo próprio, sem sessão prévia — diferente do upgrade de perfil de instrutor)
type CriarAcademiaRequest struct {
	Nome         string `json:"nome" validate:"required,min=2,max=100"`
	Sobrenome    string `json:"sobrenome" validate:"omitempty,max=100"`
	Email        string `json:"email" validate:"required,email"`
	Senha        string `json:"senha" validate:"required,min=6"`
	RazaoSocial  string `json:"razao_social" validate:"required,min=2,max=150"`
	NomeFantasia string `json:"nome_fantasia" validate:"required,min=2,max=150"`
	CNPJ         string `json:"cnpj" validate:"required"` // Aceita com ou sem pontuação
	Endereco     string `json:"endereco"`
	Telefone     string `json:"telefone"`
}
