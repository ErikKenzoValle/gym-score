package models

import "time"

// IngressoResponse representa a participação confirmada de um atleta em um desafio,
// incluindo o código de ingresso para check-in no evento
type IngressoResponse struct {
	IDDesafio      uint      `json:"id_desafio"`
	Titulo         string    `json:"titulo"`
	Data           time.Time `json:"data"`
	Local          string    `json:"local"`
	Status         string    `json:"status"`
	CodigoIngresso string    `json:"codigo_ingresso"`
}
