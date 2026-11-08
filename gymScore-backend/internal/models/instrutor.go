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
	IDAcademia *uint `gorm:"default:null" json:"id_academia,omitempty"`
	// Convite de vínculo pendente (migration 00018): a academia convida e o instrutor aceita/recusa.
	IDAcademiaConvite *uint      `gorm:"column:id_academia_convite" json:"id_academia_convite,omitempty"`
	ConviteEm         *time.Time `gorm:"column:convite_em" json:"convite_em,omitempty"`
	// Aprovação pelo painel admin (migration 00015): novos cadastros nascem pendente.
	Status         string     `gorm:"column:status;type:enum('pendente','aprovada','rejeitada');not null;default:pendente" json:"status"`
	MotivoRejeicao *string    `gorm:"column:motivo_rejeicao;size:500" json:"motivo_rejeicao,omitempty"`
	AprovadoPor    *uint      `gorm:"column:aprovado_por" json:"-"`
	AprovadoEm     *time.Time `gorm:"column:aprovado_em" json:"aprovado_em,omitempty"`
	CriadoEm       time.Time  `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm   time.Time  `gorm:"autoUpdateTime" json:"atualizado_em"`
}

// TableName define o nome da tabela no banco de dados
func (Instrutor) TableName() string {
	return "instrutores"
}

// CriarInstrutorRequest é o DTO do POST /api/instrutores: o usuário LOGADO (JWT) ativa o próprio
// perfil de instrutor. Não aceita id_usuario nem id_academia: quem vincula é a academia
// (POST /api/academia/instrutores).
type CriarInstrutorRequest struct {
	CREF          string `json:"cref" validate:"required,min=4,max=20"`
	Especialidade string `json:"especialidade" validate:"omitempty,max=100"`
}

// VincularInstrutorRequest é o corpo do POST /api/academia/instrutores (e-mail de uma conta com perfil instrutor).
type VincularInstrutorRequest struct {
	Email string `json:"email" validate:"required,email,max=150"`
}

// InstrutorResponse é o item do diretório de instrutores (GET /api/instrutores): sem e-mail, CPF nem id de usuário.
type InstrutorResponse struct {
	ID            uint   `json:"id"`
	Nome          string `json:"nome"`
	Sobrenome     string `json:"sobrenome"`
	CREF          string `json:"cref"`
	Especialidade string `json:"especialidade"`
	Autonomo      bool   `json:"autonomo"` // true = sem vínculo com academia
	// Status só vem na resposta do próprio cadastro (pendente até a aprovação do admin).
	Status string `json:"status,omitempty"`
}

// InstrutorVinculadoResponse é um instrutor vinculado à academia logada (visão da própria academia).
type InstrutorVinculadoResponse struct {
	ID             uint   `json:"id"`
	IDUsuario      uint   `json:"id_usuario"`
	Nome           string `json:"nome"`
	Sobrenome      string `json:"sobrenome"`
	Email          string `json:"email"`
	CREF           string `json:"cref"`
	Especialidade  string `json:"especialidade"`
	DesafiosAtivos int    `json:"desafios_ativos"` // desafios abertos ou em andamento em que ele é o instrutor
	// ConvitePendente: convite enviado e ainda não aceito (resposta do POST e lista de convites).
	ConvitePendente bool       `json:"convite_pendente"`
	ConviteEm       *time.Time `json:"convite_em,omitempty"`
}

// ConviteInstrutorResponse é o convite de vínculo pendente visto pelo instrutor (GET /api/instrutor/convite).
type ConviteInstrutorResponse struct {
	// Convite é nil quando não há convite pendente.
	Convite *ConviteAcademiaResumo `json:"convite"`
	// IDAcademia é o vínculo atual (nil = autônomo).
	IDAcademia *uint `json:"id_academia"`
}

// ConviteAcademiaResumo identifica a academia que convidou.
type ConviteAcademiaResumo struct {
	IDAcademia   uint       `json:"id_academia"`
	NomeFantasia string     `json:"nome_fantasia"`
	ConviteEm    *time.Time `json:"convite_em,omitempty"`
}

// ListaInstrutoresVinculadosResponse é a página de instrutores vinculados.
type ListaInstrutoresVinculadosResponse struct {
	Itens  []InstrutorVinculadoResponse `json:"itens"`
	Total  int64                        `json:"total"`
	Pagina int                          `json:"pagina"`
	Limite int                          `json:"limite"`
}
