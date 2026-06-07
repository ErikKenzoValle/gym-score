package models

import "time"

// Usuario representa a entidade de usuário no sistema GymScore
type Usuario struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Nome           string    `gorm:"type:varchar(100);not null" json:"nome"`
	Sobrenome      string    `gorm:"type:varchar(100);not null" json:"sobrenome"`
	CPF            string    `gorm:"type:varchar(14);uniqueIndex;not null" json:"cpf"`
	Email          string    `gorm:"type:varchar(150);uniqueIndex;not null" json:"email"`
	Senha          string    `gorm:"type:varchar(255);not null" json:"-"`
	DataNascimento string    `gorm:"type:date;not null" json:"data_nascimento"`
	Genero         string    `gorm:"type:enum('M','F','O');not null" json:"genero"`
	Saldo          float64   `gorm:"type:decimal(10,2);default:0.00" json:"saldo"`
	CriadoEm       time.Time `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm   time.Time `gorm:"autoUpdateTime" json:"atualizado_em"`
}

// TableName define o nome da tabela no banco de dados
func (Usuario) TableName() string {
	return "usuarios"
}

// UsuarioResponse é o DTO de retorno público (sem senha)
type UsuarioResponse struct {
	ID             uint      `json:"id"`
	Nome           string    `json:"nome"`
	Sobrenome      string    `json:"sobrenome"`
	Email          string    `json:"email"`
	CPF            string    `json:"cpf"`
	DataNascimento string    `json:"data_nascimento"`
	Genero         string    `json:"genero"`
	Saldo          float64   `json:"saldo"`
	CriadoEm       time.Time `json:"criado_em"`
}

// CriarUsuarioRequest é o DTO de entrada para criação de usuário
type CriarUsuarioRequest struct {
	Nome           string `json:"nome" validate:"required,min=2,max=100"`
	Sobrenome      string `json:"sobrenome" validate:"required,min=2,max=100"`
	CPF            string `json:"cpf" validate:"required"` // Aceita com ou sem pontuação
	Email          string `json:"email" validate:"required,email"`
	Senha          string `json:"senha" validate:"required,min=6"`
	DataNascimento string `json:"data_nascimento" validate:"required"`
	Genero         string `json:"genero" validate:"required,oneof=M F O"`
}

// LoginRequest é o DTO de entrada para autenticação
type LoginRequest struct {
	Email string `json:"email" validate:"required,email"`
	Senha string `json:"senha" validate:"required"`
}

// LoginResponse é o DTO de retorno após autenticação bem-sucedida
type LoginResponse struct {
	Token   string          `json:"token"`
	Usuario UsuarioResponse `json:"usuario"`
}

// AlterarSenhaRequest é o DTO para troca de senha autenticada
type AlterarSenhaRequest struct {
	SenhaAtual string `json:"senha_atual" validate:"required"`
	NovaSenha  string `json:"nova_senha" validate:"required,min=6"`
}

// RecuperarSenhaRequest é o DTO para redefinição de senha via e-mail + CPF (sem e-mail)
type RecuperarSenhaRequest struct {
	Email     string `json:"email" validate:"required,email"`
	CPF       string `json:"cpf" validate:"required"`
	NovaSenha string `json:"nova_senha" validate:"required,min=6"`
}

// AtualizarPerfilRequest é o DTO para atualização dos dados básicos do perfil autenticado
type AtualizarPerfilRequest struct {
	Nome      string `json:"nome"`
	Sobrenome string `json:"sobrenome"`
	Genero    string `json:"genero"`
}
