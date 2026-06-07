package models

import "time"

// Treino representa um treino criado por um usuário, com uma sequência de exercícios.
type Treino struct {
	ID        uint              `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario uint              `gorm:"column:id_usuario;not null;index" json:"id_usuario"`
	Nome      string            `gorm:"type:varchar(120);not null" json:"nome"`
	Descricao string            `gorm:"type:varchar(300)" json:"descricao"`
	Codigo    string            `gorm:"type:varchar(12);uniqueIndex;not null" json:"codigo"`
	CriadoEm  time.Time         `gorm:"autoCreateTime" json:"criado_em"`
	Exercicios []TreinoExercicio `gorm:"foreignKey:IDTreino;constraint:OnDelete:CASCADE" json:"exercicios,omitempty"`
}

func (Treino) TableName() string { return "treinos" }

// TreinoExercicio é um exercício dentro de um treino, na ordem de execução.
type TreinoExercicio struct {
	ID            uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	IDTreino      uint   `gorm:"column:id_treino;not null;index" json:"id_treino"`
	Nome          string `gorm:"type:varchar(120);not null" json:"nome"`
	GrupoMuscular string `gorm:"column:grupo_muscular;type:varchar(40);not null" json:"grupo_muscular"`
	Ordem         int    `gorm:"column:ordem;not null;default:0" json:"ordem"`
	Series        int    `gorm:"column:series;default:0" json:"series"`
	Repeticoes    int    `gorm:"column:repeticoes;default:0" json:"repeticoes"`
}

func (TreinoExercicio) TableName() string { return "treino_exercicios" }

// TreinoConclusao registra cada exercício concluído por um usuário.
// Alimenta o radar de grupos musculares (quanto cada área foi treinada).
type TreinoConclusao struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario     uint      `gorm:"column:id_usuario;not null;index" json:"id_usuario"`
	IDTreino      uint      `gorm:"column:id_treino;index" json:"id_treino"`
	GrupoMuscular string    `gorm:"column:grupo_muscular;type:varchar(40);not null" json:"grupo_muscular"`
	ConcluidoEm   time.Time `gorm:"autoCreateTime" json:"concluido_em"`
}

func (TreinoConclusao) TableName() string { return "treino_conclusoes" }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// CriarTreinoRequest é o DTO de entrada para criar um treino.
type CriarTreinoRequest struct {
	IDUsuario  uint                       `json:"id_usuario"`
	Nome       string                     `json:"nome" validate:"required,min=2,max=120"`
	Descricao  string                     `json:"descricao"`
	Exercicios []CriarTreinoExercicioItem `json:"exercicios" validate:"required,min=1"`
}

// CriarTreinoExercicioItem é um exercício no payload de criação.
type CriarTreinoExercicioItem struct {
	Nome          string `json:"nome" validate:"required"`
	GrupoMuscular string `json:"grupo_muscular" validate:"required"`
	Series        int    `json:"series"`
	Repeticoes    int    `json:"repeticoes"`
}

// ImportarTreinoRequest é o DTO para importar um treino pelo código.
type ImportarTreinoRequest struct {
	IDUsuario uint   `json:"id_usuario"`
	Codigo    string `json:"codigo" validate:"required"`
}

// ConcluirExercicioRequest registra um exercício concluído (para o radar).
type ConcluirExercicioRequest struct {
	IDUsuario     uint   `json:"id_usuario"`
	IDTreino      uint   `json:"id_treino"`
	GrupoMuscular string `json:"grupo_muscular" validate:"required"`
}

// RadarGrupo é um ponto do radar: total de conclusões de um grupo muscular.
type RadarGrupo struct {
	GrupoMuscular string `json:"grupo_muscular"`
	Total         int    `json:"total"`
}
