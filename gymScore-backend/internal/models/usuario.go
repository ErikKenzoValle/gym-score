package models

import "time"

// Usuario representa a entidade de usuário no sistema GymScore.
// Nunca é serializada direto na API: use os DTOs (UsuarioResponse / UsuarioPublico).
type Usuario struct {
	ID             uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Nome           string `gorm:"type:varchar(100);not null" json:"nome"`
	Sobrenome      string `gorm:"type:varchar(100);not null" json:"sobrenome"`
	CPF            string `gorm:"type:varchar(14);uniqueIndex;not null" json:"-"`
	Email          string `gorm:"type:varchar(150);uniqueIndex;not null" json:"-"`
	Senha          string `gorm:"type:varchar(255);not null" json:"-"`
	DataNascimento string `gorm:"type:date;not null" json:"-"`
	Genero         string `gorm:"type:enum('M','F','O');not null" json:"-"`
	// SaldoCentavos é somente leitura para o GORM (`->`): só o CarteiraRepository altera o
	// saldo, via UPDATE atômico. Isso impede que um Save() com valor antigo sobrescreva o saldo.
	SaldoCentavos int64 `gorm:"column:saldo_centavos;->" json:"-"`
	// TokenVersao também é só leitura: só muda via UPDATE atômico nos repositórios.
	TokenVersao  int       `gorm:"column:token_versao;->" json:"-"`
	CriadoEm     time.Time `gorm:"autoCreateTime" json:"-"`
	AtualizadoEm time.Time `gorm:"autoUpdateTime" json:"-"`
}

// TableName define o nome da tabela no banco de dados
func (Usuario) TableName() string {
	return "usuarios"
}

// UsuarioPublico contém apenas dados que podem ser vistos por outros usuários.
type UsuarioPublico struct {
	ID        uint   `json:"id"`
	Nome      string `json:"nome"`
	Sobrenome string `json:"sobrenome"`
}

// Publico converte o model para o DTO público (sem e-mail, CPF ou saldo).
func (u *Usuario) Publico() UsuarioPublico {
	return UsuarioPublico{ID: u.ID, Nome: u.Nome, Sobrenome: u.Sobrenome}
}

// UsuarioResponse é o perfil completo, devolvido somente ao próprio usuário (/usuarios/me, login).
type UsuarioResponse struct {
	ID             uint      `json:"id"`
	Nome           string    `json:"nome"`
	Sobrenome      string    `json:"sobrenome"`
	Email          string    `json:"email"`
	CPF            string    `json:"cpf"`
	DataNascimento string    `json:"data_nascimento"`
	Genero         string    `json:"genero"`
	SaldoCentavos  int64     `json:"saldo_centavos"`
	Saldo          float64   `json:"saldo"` // reais, apenas para exibição
	CriadoEm       time.Time `json:"criado_em"`
}

// CriarUsuarioRequest é o DTO de entrada para criação de usuário
type CriarUsuarioRequest struct {
	Nome           string `json:"nome" validate:"required,min=2,max=100"`
	Sobrenome      string `json:"sobrenome" validate:"required,min=2,max=100"`
	CPF            string `json:"cpf" validate:"required"` // formato 000.000.000-00
	Email          string `json:"email" validate:"required,email,max=150"`
	Senha          string `json:"senha" validate:"required,min=8,max=72"`
	DataNascimento string `json:"data_nascimento" validate:"required,datetime=2006-01-02"`
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
	NovaSenha  string `json:"nova_senha" validate:"required,min=8,max=72"`
}

// EsqueciSenhaRequest inicia a recuperação de senha por e-mail.
type EsqueciSenhaRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// RedefinirSenhaRequest conclui a recuperação usando o token recebido por e-mail.
type RedefinirSenhaRequest struct {
	Token     string `json:"token" validate:"required,min=20,max=200"`
	NovaSenha string `json:"nova_senha" validate:"required,min=8,max=72"`
}

// AtualizarPerfilRequest é o DTO para atualização dos dados básicos do perfil autenticado
type AtualizarPerfilRequest struct {
	Nome      string `json:"nome" validate:"omitempty,min=2,max=100"`
	Sobrenome string `json:"sobrenome" validate:"omitempty,min=2,max=100"`
	Genero    string `json:"genero" validate:"omitempty,oneof=M F O"`
}

// PasswordResetToken guarda o HASH (SHA-256) do token de recuperação de senha.
type PasswordResetToken struct {
	ID        uint       `gorm:"primaryKey;autoIncrement"`
	IDUsuario uint       `gorm:"column:id_usuario;not null"`
	TokenHash string     `gorm:"column:token_hash;type:char(64);uniqueIndex;not null"`
	ExpiraEm  time.Time  `gorm:"column:expira_em;not null"`
	UsadoEm   *time.Time `gorm:"column:usado_em"`
	CriadoEm  time.Time  `gorm:"column:criado_em;autoCreateTime"`
}

// TableName define o nome da tabela
func (PasswordResetToken) TableName() string { return "password_reset_tokens" }
