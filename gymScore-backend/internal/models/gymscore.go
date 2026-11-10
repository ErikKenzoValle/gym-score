package models

import "time"

// EixoEstatistica descreve um eixo de um gráfico de teia (ordem de exibição + rótulo).
type EixoEstatistica struct {
	Chave  string `json:"chave"`
	Rotulo string `json:"rotulo"`
}

// EixosEstatisticas é a ordem e o rótulo dos eixos dos dois gráficos.
type EixosEstatisticas struct {
	Atributos  []EixoEstatistica `json:"atributos"`
	Categorias []EixoEstatistica `json:"categorias"`
}

// ComposicaoPontuacao detalha de onde vem a pontuação do Nível GymScore (todo o histórico).
type ComposicaoPontuacao struct {
	Primeiros       int `json:"primeiros"`
	Segundos        int `json:"segundos"`
	Terceiros       int `json:"terceiros"`
	OutrosPremiados int `json:"outros_premiados"`
	Presencas       int `json:"presencas"`
	DiasTreino      int `json:"dias_treino"`
	PontosPodios    int `json:"pontos_podios"`
	PontosPresencas int `json:"pontos_presencas"`
	PontosTreinos   int `json:"pontos_treinos"`
}

// DadosJanela resume o que alimentou os gráficos (janela de JanelaDias dias).
type DadosJanela struct {
	JanelaDias int `json:"janela_dias"`
	Exercicios int `json:"exercicios"`  // exercícios concluídos
	DiasTreino int `json:"dias_treino"` // dias distintos com treino
	Desafios   int `json:"desafios"`    // desafios encerrados com presença
	Podios     int `json:"podios"`      // desses, em quantos ficou no pódio
}

// EstatisticasResponse é GET /api/usuarios/:id/estatisticas (rodada 6, U5).
// atributos (6 eixos) e categorias (8 eixos) vão de 0 a 100; `eixos` dá a ordem e os rótulos.
type EstatisticasResponse struct {
	IDUsuario uint `json:"id_usuario"`
	Nivel     int  `json:"nivel"`     // 1..10
	NivelMax  int  `json:"nivel_max"` // 10
	Pontuacao int  `json:"pontuacao"`
	// NivelInicioEm: pontuação em que o nível atual começa; ProximoNivelEm: pontuação do próximo nível
	// (null no nível máximo); FaltamPontos: quanto falta (0 no máximo). Barra = (pontuacao - inicio) / (proximo - inicio).
	NivelInicioEm  int                 `json:"nivel_inicio_em"`
	ProximoNivelEm *int                `json:"proximo_nivel_em"`
	FaltamPontos   int                 `json:"faltam_pontos"`
	Composicao     ComposicaoPontuacao `json:"composicao"`
	Atributos      map[string]int      `json:"atributos"`
	Categorias     map[string]int      `json:"categorias"`
	Eixos          EixosEstatisticas   `json:"eixos"`
	Dados          DadosJanela         `json:"dados"`
	// SemDados: nada na janela (eixos todos 0) — Mensagem explica o que fazer.
	SemDados    bool      `json:"sem_dados"`
	Mensagem    string    `json:"mensagem,omitempty"`
	BaseCalculo string    `json:"base_calculo"`
	CalculadoEm time.Time `json:"calculado_em"`
}
