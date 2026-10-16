package models

import "time"

// StatusDesafio define os possíveis estados de um desafio
type StatusDesafio string

const (
	StatusAberto      StatusDesafio = "aberto"
	StatusEmAndamento StatusDesafio = "em_andamento"
	StatusEmVotacao   StatusDesafio = "em_votacao"
	StatusEncerrado   StatusDesafio = "encerrado"
	StatusCancelado   StatusDesafio = "cancelado"
)

// Valor de entrada: limites em centavos (R$ 1,00 a R$ 10.000,00).
const (
	EntradaMinCentavos int64 = 100
	EntradaMaxCentavos int64 = 1_000_000
)

// Desafio representa a entidade de desafio no sistema GymScore
type Desafio struct {
	ID                   uint          `gorm:"primaryKey;autoIncrement"`
	Titulo               string        `gorm:"type:varchar(200);not null"`
	Descricao            *string       `gorm:"type:text"`
	ValorEntradaCentavos int64         `gorm:"column:valor_entrada_centavos;not null"`
	Local                *string       `gorm:"type:varchar(200)"`
	Latitude             *float64      `gorm:"type:decimal(10,8)"`
	Longitude            *float64      `gorm:"type:decimal(11,8)"`
	Status               StatusDesafio `gorm:"type:enum('aberto','em_andamento','em_votacao','encerrado','cancelado');default:'aberto'"`
	Vagas                int           `gorm:"column:vagas;not null;default:2"`
	DataEncerramento     *time.Time    `gorm:"column:data_encerramento"`
	VotacaoAbertaEm      *time.Time    `gorm:"column:votacao_aberta_em"`
	IDCriador            uint          `gorm:"not null"`
	CriadoEm             time.Time     `gorm:"autoCreateTime"`
	AtualizadoEm         time.Time     `gorm:"autoUpdateTime"`

	// Relacionamentos
	Criador       *Usuario              `gorm:"foreignKey:IDCriador"`
	Participantes []DesafioParticipante `gorm:"foreignKey:IDDesafio"`
}

// TableName define o nome da tabela no banco de dados
func (Desafio) TableName() string {
	return "desafios"
}

// DesafioParticipante representa um usuário inscrito em um desafio (ocupando uma vaga)
type DesafioParticipante struct {
	ID             uint      `gorm:"primaryKey;autoIncrement"`
	IDDesafio      uint      `gorm:"column:id_desafio;not null;uniqueIndex:uq_desafio_usuario"`
	IDUsuario      uint      `gorm:"column:id_usuario;not null;uniqueIndex:uq_desafio_usuario"`
	PremioCentavos int64     `gorm:"column:premio_centavos;not null;default:0"`
	Vencedor       bool      `gorm:"column:vencedor;not null;default:false"`
	CriadoEm       time.Time `gorm:"autoCreateTime"`

	Usuario *Usuario `gorm:"foreignKey:IDUsuario"`
}

// TableName define o nome da tabela de participantes
func (DesafioParticipante) TableName() string {
	return "desafio_participantes"
}

// DesafioVoto é o voto de um participante em outro (1 por eleitor por desafio).
type DesafioVoto struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	IDDesafio uint      `gorm:"column:id_desafio;not null"`
	IDEleitor uint      `gorm:"column:id_eleitor;not null"`
	IDVotado  uint      `gorm:"column:id_votado;not null"`
	CriadoEm  time.Time `gorm:"column:criado_em;autoCreateTime"`
}

// TableName define o nome da tabela de votos
func (DesafioVoto) TableName() string { return "desafio_votos" }

// ─── DTOs de entrada ─────────────────────────────────────────────────────────

// CriarDesafioRequest é o DTO de entrada para criação de desafio.
// O criador é sempre o usuário do JWT e paga a entrada ao criar.
type CriarDesafioRequest struct {
	Titulo               string   `json:"titulo" validate:"required,min=3,max=200"`
	Descricao            string   `json:"descricao" validate:"max=2000"`
	ValorEntradaCentavos int64    `json:"valor_entrada_centavos" validate:"required,gte=100,lte=1000000"`
	Local                string   `json:"local" validate:"max=200"`
	Latitude             *float64 `json:"latitude,omitempty" validate:"omitempty,gte=-90,lte=90"`
	Longitude            *float64 `json:"longitude,omitempty" validate:"omitempty,gte=-180,lte=180"`
	Vagas                int      `json:"vagas" validate:"omitempty,gte=2,lte=50"`
	// DataEncerramento aceita "YYYY-MM-DD" (vale até o fim do dia) ou RFC3339.
	DataEncerramento string `json:"data_encerramento,omitempty"`
}

// VotarRequest é o DTO do voto: id_votado é o ALVO (outro participante).
type VotarRequest struct {
	IDVotado uint `json:"id_votado" validate:"required"`
}

// ConvidarRequest é o DTO do convite: id_amigo é o ALVO (amigo aceito do criador).
type ConvidarRequest struct {
	IDAmigo uint `json:"id_amigo" validate:"required"`
}

// ─── DTOs de saída ───────────────────────────────────────────────────────────

// ParticipanteResponse é um participante no detalhe do desafio.
type ParticipanteResponse struct {
	UsuarioPublico
	Criador        bool       `json:"criador"`
	PremioCentavos int64      `json:"premio_centavos"`
	Premio         float64    `json:"premio"`
	Vencedor       bool       `json:"vencedor"`
	VotosRecebidos *int       `json:"votos_recebidos,omitempty"` // só com desafio encerrado
	JaVotou        *bool      `json:"ja_votou,omitempty"`        // só em votação (transparência sem revelar em quem)
	Checkins       int64      `json:"checkins"`                  // total de check-ins no desafio
	UltimoCheckin  *time.Time `json:"ultimo_checkin_em,omitempty"`
}

// DesafioResponse é o DTO de desafio devolvido pela API.
type DesafioResponse struct {
	ID                   uint                   `json:"id"`
	Titulo               string                 `json:"titulo"`
	Descricao            string                 `json:"descricao"`
	ValorEntradaCentavos int64                  `json:"valor_entrada_centavos"`
	ValorEntrada         float64                `json:"valor_entrada"`
	PoteCentavos         int64                  `json:"pote_centavos"`
	Pote                 float64                `json:"pote"`
	Local                string                 `json:"local"`
	Latitude             *float64               `json:"latitude,omitempty"`
	Longitude            *float64               `json:"longitude,omitempty"`
	Status               StatusDesafio          `json:"status"`
	Vagas                int                    `json:"vagas"`
	ParticipantesCount   int                    `json:"participantes_count"`
	DataEncerramento     *time.Time             `json:"data_encerramento,omitempty"`
	VotacaoAbertaEm      *time.Time             `json:"votacao_aberta_em,omitempty"`
	IDCriador            uint                   `json:"id_criador"`
	Criador              *UsuarioPublico        `json:"criador,omitempty"`
	CriadoEm             time.Time              `json:"criado_em"`
	VoceParticipa        bool                   `json:"voce_participa"`
	VoceECriador         bool                   `json:"voce_e_criador"`
	VoceVotou            bool                   `json:"voce_votou"`
	VoceFezCheckinHoje   bool                   `json:"voce_fez_checkin_hoje"`
	CheckinsTotal        int64                  `json:"checkins_total"`
	Participantes        []ParticipanteResponse `json:"participantes"`
}

// ListaDesafiosResponse é a página de desafios.
type ListaDesafiosResponse struct {
	Itens  []DesafioResponse `json:"itens"`
	Total  int64             `json:"total"`
	Pagina int               `json:"pagina"`
	Limite int               `json:"limite"`
}
