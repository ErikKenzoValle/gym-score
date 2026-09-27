package models

import "time"

// Usuario representa a entidade de usuário no sistema GymScore
type Usuario struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Nome           string     `gorm:"type:varchar(100);not null" json:"nome"`
	Sobrenome      string     `gorm:"type:varchar(100);not null" json:"sobrenome"`
	// CPF é obrigatório para atletas/instrutores; nulo para contas de perfil "academia" (pessoa jurídica).
	// Mantido como ponteiro (nullable) em vez de string vazia porque o MySQL/InnoDB permite múltiplos
	// valores NULL em uma uniqueIndex, mas trataria "" duplicado como violação de unicidade.
	CPF            *string    `gorm:"type:varchar(14);uniqueIndex;default:null" json:"cpf,omitempty"`
	Email          string     `gorm:"type:varchar(150);uniqueIndex;not null" json:"email"`
	Senha          string     `gorm:"type:varchar(255);not null" json:"-"`
	// DataNascimento e Genero são obrigatórios para atletas/instrutores; nulos para contas de perfil "academia"
	// (pessoa jurídica), que não coleta esses dados. Ponteiros para permitir NULL no MySQL (mesmo racional do CPF acima).
	DataNascimento *string    `gorm:"type:date;default:null" json:"data_nascimento,omitempty"`
	Genero         *string    `gorm:"type:enum('M','F','O');default:null" json:"genero,omitempty"`
	Saldo          float64    `gorm:"type:decimal(10,2);default:0.00" json:"saldo"`
	// Campos de perfil e gamificação
	Perfil    string     `gorm:"type:enum('atleta','academia','instrutor');default:'atleta'" json:"perfil"`
	Username  string     `gorm:"type:varchar(50);uniqueIndex;default:null" json:"username"`
	Elo       string     `gorm:"type:varchar(20);default:'Iniciante'" json:"elo"`
	Pontos    int        `gorm:"default:0" json:"pontos"`
	UltimaVez *time.Time `gorm:"default:null" json:"ultima_vez,omitempty"`
	CriadoEm      time.Time  `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm  time.Time  `gorm:"autoUpdateTime" json:"atualizado_em"`
}

// TableName define o nome da tabela no banco de dados
func (Usuario) TableName() string {
	return "usuarios"
}

// UsuarioResponse é o DTO de retorno autenticado (sem senha)
type UsuarioResponse struct {
	ID             uint       `json:"id"`
	Nome           string     `json:"nome"`
	Sobrenome      string     `json:"sobrenome"`
	Email          string     `json:"email"`
	CPF            *string    `json:"cpf,omitempty"`
	DataNascimento string     `json:"data_nascimento"`
	Genero         string     `json:"genero"`
	Saldo          float64    `json:"saldo"`
	Perfil         string     `json:"perfil"`
	Username       string     `json:"username"`
	Elo            string     `json:"elo"`
	Pontos         int        `json:"pontos"`
	UltimaVez      *time.Time `json:"ultima_vez,omitempty"`
	CriadoEm       time.Time  `json:"criado_em"`
}

// UsuarioPublicoResponse é o DTO de perfil público (sem CPF, Email, Saldo, Senha)
type UsuarioPublicoResponse struct {
	ID        uint       `json:"id"`
	Nome      string     `json:"nome"`
	Sobrenome string     `json:"sobrenome"`
	Username  string     `json:"username"`
	Elo       string     `json:"elo"`
	Pontos    int        `json:"pontos"`
	UltimaVez *time.Time `json:"ultima_vez,omitempty"`
	Perfil    string     `json:"perfil"`
}

// CriarUsuarioRequest é o DTO de entrada para criação de usuário
type CriarUsuarioRequest struct {
	Nome           string `json:"nome" validate:"required,min=2,max=100"`
	Sobrenome      string `json:"sobrenome" validate:"omitempty,max=100"`
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
