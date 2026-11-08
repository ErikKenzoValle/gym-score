package models

import "time"

// Papéis de administrador (coluna administradores.papel).
const (
	PapelSuperadmin = "superadmin"
	PapelAdmin      = "admin"
)

// Administrador é uma conta do painel admin. Tabela própria, separada de `usuarios`
// (migration 00014). Nunca serializar direto na API: SenhaHash não pode vazar.
type Administrador struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	Nome      string `gorm:"column:nome;size:100;not null"`
	Email     string `gorm:"column:email;size:150;not null"`
	SenhaHash string `gorm:"column:senha_hash;size:255;not null"`
	Papel     string `gorm:"column:papel;type:enum('superadmin','admin');not null"`
	// Ativo SEM tag default de propósito: o GORM grava o valor do struct (false inclusive),
	// então quem cria precisa informar Ativo: true explicitamente.
	Ativo bool `gorm:"column:ativo;not null"`
	// TokenVersao é o claim `tv` do JWT admin. Somente leitura para o GORM: só muda nos UPDATEs
	// atômicos do AdminRepository (desativar, trocar papel, trocar senha, revogar).
	TokenVersao   int        `gorm:"column:token_versao;->"`
	UltimoLoginEm *time.Time `gorm:"column:ultimo_login_em"`
	CriadoPor     *uint      `gorm:"column:criado_por"`
	CriadoEm      time.Time  `gorm:"column:criado_em;autoCreateTime"`
	AtualizadoEm  time.Time  `gorm:"column:atualizado_em;autoUpdateTime"`
}

// TableName define o nome da tabela no banco de dados
func (Administrador) TableName() string { return "administradores" }

// AdminAuditoria é uma linha da trilha append-only de ações de escrita do painel admin.
// Deve ser gravada na MESMA transação da ação auditada (AuditoriaRepository.Registrar(tx, ...)).
type AdminAuditoria struct {
	ID       uint   `gorm:"primaryKey;autoIncrement"`
	IDAdmin  uint   `gorm:"column:id_admin;not null"`
	Acao     string `gorm:"column:acao;size:60;not null"`
	AlvoTipo string `gorm:"column:alvo_tipo;size:30;not null"`
	AlvoID   *uint  `gorm:"column:alvo_id"`
	Motivo   string `gorm:"column:motivo;size:500;not null"`
	// Detalhes é um documento JSON (texto). *string em vez de []byte: o driver MySQL envia []byte
	// como binário e o MySQL recusa JSON com charset binary.
	Detalhes *string   `gorm:"column:detalhes;type:json"`
	IP       string    `gorm:"column:ip;size:45;not null"`
	CriadoEm time.Time `gorm:"column:criado_em;autoCreateTime"`

	// Admin é carregado opcionalmente na listagem (só id/nome/email/papel).
	Admin *Administrador `gorm:"foreignKey:IDAdmin"`
}

// TableName define o nome da tabela no banco de dados
func (AdminAuditoria) TableName() string { return "admin_auditoria" }
