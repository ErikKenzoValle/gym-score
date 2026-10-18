package models

import "time"

// Saque representa uma solicitação de retirada de saldo pelo usuário via PIX
type Saque struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	IDUsuario    uint      `gorm:"not null;index" json:"id_usuario"`
	Usuario      *Usuario  `gorm:"foreignKey:IDUsuario" json:"usuario,omitempty"`
	Valor        float64   `gorm:"type:decimal(10,2);not null" json:"valor"`
	// ChavePix chave de destino informada pelo usuário no momento da solicitação
	ChavePix     string    `gorm:"type:varchar(200);not null" json:"chave_pix"`
	// Status ciclo de vida: pendente → processando → pago | rejeitado
	Status       string    `gorm:"type:enum('pendente','processando','pago','rejeitado');default:'pendente'" json:"status"`
	CriadoEm    time.Time `gorm:"autoCreateTime" json:"criado_em"`
	AtualizadoEm time.Time `gorm:"autoUpdateTime" json:"atualizado_em"`
}

// TableName define o nome da tabela no banco de dados
func (Saque) TableName() string {
	return "saques"
}

// SolicitarSaqueRequest é o DTO de entrada para solicitação de saque
type SolicitarSaqueRequest struct {
	Valor    float64 `json:"valor" validate:"required"`
	ChavePix string  `json:"chave_pix" validate:"required"`
}

// SaqueResponse é o DTO de retorno de saque (sem dados do usuário)
type SaqueResponse struct {
	ID       uint    `json:"id"`
	Valor    float64 `json:"valor"`
	ChavePix string  `json:"chave_pix"`
	Status   string  `json:"status"`
	CriadoEm string  `json:"criado_em"`
}
