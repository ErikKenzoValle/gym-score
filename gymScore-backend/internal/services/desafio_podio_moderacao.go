package services

import (
	"encoding/json"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// Ações da linha do tempo do pódio e do dinheiro do desafio (moderacao_eventos).
const (
	EventoPodioLancado         = "podio_lancado"
	EventoPagamentoLiberado    = "pagamento_liberado"
	EventoPagamentoCancelado   = "pagamento_cancelado"
	EventoPagamentoEstornado   = "pagamento_estornado"
	EventoResultadoAnulado     = "resultado_anulado"
	EventoInscricoesDevolvidas = "inscricoes_devolvidas"
	EventoDebitoCriado         = "debito_criado"
)

// dadosPodio é o que o evento podio_lancado guarda: posições, marcas, prêmios, pendentes e o delta de
// pontos de cada um (a anulação desfaz exatamente esses pontos; o pódio removido continua aqui).
type dadosPodio struct {
	Posicoes []posicaoPodio `json:"posicoes"`
	// Pontos: id do usuário (string) -> pontos somados no lançamento.
	Pontos    map[string]int  `json:"pontos"`
	Pendentes []pendentePodio `json:"pendentes"`
	Retido    bool            `json:"retido"`
	Relancado bool            `json:"relancamento"`
	LiberarEm *time.Time      `json:"liberar_em,omitempty"`
}

type posicaoPodio struct {
	Posicao   int    `json:"posicao"`
	IDUsuario uint   `json:"id_usuario"`
	Marca     string `json:"marca"`
	Premio    int64  `json:"premio_centavos"`
}

type pendentePodio struct {
	ID        uint   `json:"id"`
	IDUsuario *uint  `json:"id_usuario"`
	Tipo      string `json:"tipo"`
	Valor     int64  `json:"valor_centavos"`
}

// registrarPodioLancado grava o evento podio_lancado (ator academia dona ou instrutor designado).
func (s *desafioService) registrarPodioLancado(tx *gorm.DB, ctx *contextoUsuario, d *models.Desafio, req *models.LancarPodioRequest,
	premios []int64, lote []models.PagamentoPendente, pontos map[string]int, relancamento bool) error {
	dp := dadosPodio{Pontos: pontos, Relancado: relancamento}
	for i, e := range req.Posicoes {
		dp.Posicoes = append(dp.Posicoes, posicaoPodio{Posicao: i + 1, IDUsuario: e.IDParticipante,
			Marca: strings.TrimSpace(e.Marca), Premio: premios[i]})
	}
	for _, p := range lote {
		dp.Pendentes = append(dp.Pendentes, pendentePodio{ID: p.ID, IDUsuario: p.IDUsuario, Tipo: p.Tipo, Valor: p.ValorCentavos})
		dp.Retido = p.Status == models.PendenteRetido
		em := p.LiberarEm
		dp.LiberarEm = &em
	}
	b, err := json.Marshal(map[string]any{"depois": dp})
	if err != nil {
		return err
	}
	papel := models.AtorUsuario
	if ctx.dono(d) {
		papel = models.AtorAcademia
	}
	id, idDesafio := ctx.ID, d.ID
	return s.eventosMod.Registrar(tx, &models.ModeracaoEvento{IDAtor: &id, AtorPapel: papel, Acao: EventoPodioLancado,
		Entidade: models.EntidadeDesafio, IDEntidade: d.ID, IDDesafio: &idDesafio, Dados: b})
}

// pontosDoPodioLancado lê, do último evento podio_lancado do desafio, os pontos somados a cada usuário.
// ok=false se o desafio foi encerrado antes da janela de contestação (sem evento).
func pontosDoPodioLancado(tx *gorm.DB, idDesafio uint) (map[string]int, bool, error) {
	e, err := repositories.UltimoEventoDoDesafio(tx, idDesafio, EventoPodioLancado)
	if err != nil || e == nil {
		return nil, false, err
	}
	var dados struct {
		Depois dadosPodio `json:"depois"`
	}
	if err := json.Unmarshal(e.Dados, &dados); err != nil {
		return nil, false, err
	}
	return dados.Depois.Pontos, true, nil
}

// estadoResultado preenche, em qualquer resposta de desafio, o estado do resultado (anulação) e ajusta o
// pode_lancar_podio no relançamento (Q3: só a academia dona). Não consulta o banco.
func estadoResultado(resp *models.DesafioResponse, d *models.Desafio, ctx *contextoUsuario) {
	resp.PodioLancadoEm = d.PodioLancadoEm
	if d.ResultadoAnuladoEm == nil {
		return
	}
	resp.ResultadoAnulado = true
	resp.ResultadoAnuladoEm = d.ResultadoAnuladoEm
	resp.ResultadoAnulacaoModo = d.ResultadoAnulacaoModo
	resp.AguardandoRelancamento = d.Status == models.StatusEmAndamento
	if ctx == nil || !ctx.dono(d) {
		resp.PodeLancarPodio = false
	}
}

// completarLiberacao preenche, no detalhe do desafio encerrado, quando os pagamentos do pódio liberam
// (menor liberar_em dos pendentes a liberar/retidos) e se há retenção por contestação.
func (s *desafioService) completarLiberacao(resp *models.DesafioResponse, d *models.Desafio) error {
	if d.Status != models.StatusEncerrado || d.PodioLancadoEm == nil || d.ResultadoAnuladoEm != nil {
		return nil
	}
	l, err := repositories.ResumoLiberacao(s.db, d.ID)
	if err != nil || l.ALiberar+l.Retidos == 0 {
		return err
	}
	resp.PagamentosLiberamEm = l.LiberarEm
	resp.PagamentosRetidos = l.Retidos > 0
	return nil
}
