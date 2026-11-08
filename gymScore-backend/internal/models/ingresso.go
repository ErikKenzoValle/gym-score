package models

import "time"

// IngressoResponse é a participação de um atleta em um desafio: o ingresso (código real do QR)
// com os dados do evento e o status de presença.
type IngressoResponse struct {
	IDDesafio uint   `json:"id_desafio"`
	Titulo    string `json:"titulo"`
	Categoria string `json:"categoria"`
	// Data é a data do evento (data_evento); null só em desafios legados sem data.
	Data     *time.Time      `json:"data"`
	Local    string          `json:"local"`
	Status   string          `json:"status"`
	Academia *AcademiaResumo `json:"academia"` // null só em legados
	// CodigoIngresso é o conteúdo do QR (32 hex): o mesmo `seu_codigo_ingresso` do desafio.
	CodigoIngresso       string     `json:"codigo_ingresso"`
	ValorEntradaCentavos int64      `json:"valor_entrada_centavos"`
	Presente             bool       `json:"presente"`
	PresenteEm           *time.Time `json:"presente_em,omitempty"`
}

// HistoricoDesafioResponse é um item do histórico de desafios encerrados de um usuário (perfil
// público). Não usa DesafioResponse (que lista TODOS os participantes): o histórico é visto por
// terceiros e o que importa é só o resultado do usuário consultado.
type HistoricoDesafioResponse struct {
	ID                   uint            `json:"id"`
	Titulo               string          `json:"titulo"`
	Categoria            string          `json:"categoria"`
	Status               StatusDesafio   `json:"status"`
	Local                string          `json:"local,omitempty"`
	DataEvento           *time.Time      `json:"data_evento"`
	DataEncerramento     *time.Time      `json:"data_encerramento,omitempty"`
	Academia             *AcademiaResumo `json:"academia"`
	ValorEntradaCentavos int64           `json:"valor_entrada_centavos"`
	ValorEntrada         float64         `json:"valor_entrada"`
	ParticipantesCount   int             `json:"participantes_count"`
	// Presente: o usuário esteve no evento (só presentes entram no pódio).
	Presente bool `json:"presente"`
	// Posicao/Marca vêm do pódio; null/"" quando o usuário ficou fora do pódio.
	Posicao        *int      `json:"posicao"`
	Marca          string    `json:"marca"`
	Vencedor       bool      `json:"vencedor"` // 1º colocado
	PremioCentavos int64     `json:"premio_centavos"`
	Premio         float64   `json:"premio"`
	CriadoEm       time.Time `json:"criado_em"`
}
