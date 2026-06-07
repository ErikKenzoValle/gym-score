package models

import "time"

// StatusDesafio define os possíveis estados de um desafio
type StatusDesafio string

const (
	StatusPendente    StatusDesafio = "pendente"
	StatusAberto      StatusDesafio = "aberto"
	StatusEmAndamento StatusDesafio = "em_andamento"
	StatusEncerrado   StatusDesafio = "encerrado"
)

// EstadoDesafio representa o estado operacional do desafio (coluna numérica)
const (
	EstadoAtivo     int8 = 1
	EstadoFechado   int8 = 0
	EstadoCancelado int8 = 2
)

// Desafio representa a entidade de desafio no sistema GymScore
type Desafio struct {
	ID         uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	Titulo     string        `gorm:"type:varchar(200);not null" json:"titulo"`
	Descricao  string        `gorm:"type:text" json:"descricao"`
	Valor      float64       `gorm:"type:decimal(10,2);not null" json:"valor"`
	Local      string        `gorm:"type:varchar(200)" json:"local"`
	Latitude   *float64      `gorm:"type:decimal(10,8)" json:"latitude,omitempty"`
	Longitude  *float64      `gorm:"type:decimal(11,8)" json:"longitude,omitempty"`
	Status           StatusDesafio `gorm:"type:enum('pendente','aberto','em_andamento','encerrado');default:'aberto'" json:"status"`
	Estado           int8          `gorm:"column:estado;default:1" json:"estado"`
	Vagas            int           `gorm:"column:vagas;default:2" json:"vagas"`
	DataEncerramento *time.Time    `gorm:"column:data_encerramento" json:"data_encerramento,omitempty"`
	IDCriador        uint          `gorm:"not null" json:"id_criador"`
	IDDesafiado *uint        `json:"id_desafiado,omitempty"`
	IDVencedor  *uint        `json:"id_vencedor,omitempty"`
	IDPerdedor  *uint        `json:"id_perdedor,omitempty"`
	CriadoEm   time.Time     `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time   `gorm:"autoUpdateTime" json:"atualizado_em"`

	// Relacionamentos
	Criador      *Usuario              `gorm:"foreignKey:IDCriador" json:"criador,omitempty"`
	Desafiado    *Usuario              `gorm:"foreignKey:IDDesafiado" json:"desafiado,omitempty"`
	Vencedor     *Usuario              `gorm:"foreignKey:IDVencedor" json:"vencedor,omitempty"`
	Participantes []DesafioParticipante `gorm:"foreignKey:IDDesafio" json:"participantes,omitempty"`
}

// DesafioParticipante representa um usuário inscrito em um desafio (ocupando uma vaga)
type DesafioParticipante struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	IDDesafio uint      `gorm:"column:id_desafio;not null;uniqueIndex:uq_desafio_usuario" json:"id_desafio"`
	IDUsuario uint      `gorm:"column:id_usuario;not null;uniqueIndex:uq_desafio_usuario" json:"id_usuario"`
	CriadoEm  time.Time `gorm:"autoCreateTime" json:"criado_em"`

	Usuario *Usuario `gorm:"foreignKey:IDUsuario" json:"usuario,omitempty"`
}

// TableName define o nome da tabela de participantes
func (DesafioParticipante) TableName() string {
	return "desafio_participantes"
}

// TableName define o nome da tabela no banco de dados
func (Desafio) TableName() string {
	return "desafios"
}

// CriarDesafioRequest é o DTO de entrada para criação de desafio
type CriarDesafioRequest struct {
	Titulo           string   `json:"titulo" validate:"required,min=3,max=200"`
	Descricao        string   `json:"descricao"`
	Valor            float64  `json:"valor" validate:"required,gt=0"`
	Local            string   `json:"local"`
	Latitude         *float64 `json:"latitude,omitempty"`
	Longitude        *float64 `json:"longitude,omitempty"`
	IDCriador        uint     `json:"id_criador" validate:"required"`
	Vagas            int      `json:"vagas"`
	DataEncerramento string   `json:"data_encerramento,omitempty"`
}

// AceitarDesafioRequest é o DTO para entrar em um desafio (ocupar uma vaga)
type AceitarDesafioRequest struct {
	IDDesafio uint `json:"id_desafio" validate:"required"`
	IDUsuario uint `json:"id_usuario" validate:"required"`
}

// IniciarDesafioRequest é o DTO para iniciar um desafio
type IniciarDesafioRequest struct {
	IDDesafio uint `json:"id_desafio" validate:"required"`
}

// EncerrarDesafioRequest é o DTO para encerrar um desafio indicando o vencedor
// (que deve estar entre os participantes). O prêmio é o valor bancado pelo criador.
type EncerrarDesafioRequest struct {
	IDDesafio  uint `json:"id_desafio" validate:"required"`
	IDVencedor uint `json:"id_vencedor" validate:"required"`
}
