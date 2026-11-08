package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gymScore-backend/internal/client"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"
)

// timeoutIA limita o tempo de espera pela geração de razões via Claude.
// Estourado o prazo, a recomendação sai com a razão heurística.
const timeoutIA = 8 * time.Second

// RecomendacaoService define as operações de recomendação personalizada de desafios
type RecomendacaoService interface {
	Recomendar(userID uint) ([]models.DesafioRecomendado, error)
}

// desafioLister é o subconjunto de *repositories.DesafioRepository usado pela recomendação.
// Isolado em interface só para permitir mock nos testes unitários (o repositório em si é um struct).
type desafioLister interface {
	Listar(f repositories.FiltroDesafios) ([]models.Desafio, int64, error)
	ListarHistoricoUsuario(idUsuario uint) ([]models.Desafio, error)
}

type recomendacaoService struct {
	desafioRepo desafioLister
	treinoRepo  repositories.TreinoRepository
	usuarioRepo repositories.UsuarioRepository
	// anthropic é opcional: quando nil, o serviço opera em modo heurístico puro.
	anthropic *client.AnthropicClient
}

// NovoRecomendacaoService cria uma nova instância do serviço de recomendações.
// O cliente Anthropic pode ser nil — nesse caso só a heurística é usada.
func NovoRecomendacaoService(
	desafioRepo desafioLister,
	treinoRepo repositories.TreinoRepository,
	usuarioRepo repositories.UsuarioRepository,
	anthropic *client.AnthropicClient,
) RecomendacaoService {
	return &recomendacaoService{desafioRepo, treinoRepo, usuarioRepo, anthropic}
}

// Recomendar retorna até 5 desafios abertos recomendados para o usuário,
// baseados no histórico de treinos, valores anteriores e perfil de participação.
func (s *recomendacaoService) Recomendar(userID uint) ([]models.DesafioRecomendado, error) {
	// 1. Busca os desafios abertos e futuros (com a inscrição do usuário, para excluir os que ele já faz)
	agora := time.Now()
	todos, _, err := s.desafioRepo.Listar(repositories.FiltroDesafios{
		IDUsuario: userID, Status: models.StatusAberto, Futuros: true, Agora: agora,
		Ordem: repositories.OrdemEventoProx, Pagina: 1, Limite: 500,
	})
	if err != nil {
		return nil, err
	}

	// 2. Histórico de desafios encerrados do usuário
	historico, _ := s.desafioRepo.ListarHistoricoUsuario(userID)
	participados := make(map[uint]bool)
	for _, d := range historico {
		participados[d.ID] = true
	}

	// 3. Radar de grupos musculares frequentes do usuário
	grupos, _ := s.treinoRepo.RadarPorUsuario(userID)
	gruposFreq := make(map[string]int)
	for _, g := range grupos {
		gruposFreq[strings.ToLower(g.GrupoMuscular)] = g.Total
	}

	// 4. Valor médio dos desafios anteriores para compatibilidade financeira
	var valMedia float64
	if len(historico) > 0 {
		var soma float64
		for _, d := range historico {
			soma += utils.CentavosParaReais(d.ValorEntradaCentavos)
		}
		valMedia = soma / float64(len(historico))
	}

	// 5. Pontua cada desafio aberto ainda não participado
	type scored struct {
		d     models.Desafio
		score int
		razao string
	}
	var candidatos []scored

	for _, d := range todos {
		if d.Status != models.StatusAberto {
			continue
		}
		if participados[d.ID] {
			continue
		}
		if d.MinhaParticipacao != nil || (d.Vagas > 0 && d.QtdParticipantes >= d.Vagas) {
			continue
		}

		score := 0
		razoes := []string{}

		// Pontos base por ser um desafio ativo
		score += 5
		razoes = append(razoes, "desafio recente")

		// Compatibilidade de valor com histórico (±30%)
		valorDesafio := utils.CentavosParaReais(d.ValorEntradaCentavos)
		if valMedia > 0 && valorDesafio >= valMedia*0.7 && valorDesafio <= valMedia*1.3 {
			score += 10
			razoes = append(razoes, fmt.Sprintf("compatível com seu histórico de R$ %.0f", valMedia))
		}

		// Grupos musculares do radar aparecem no título ou descrição
		for grupo, freq := range gruposFreq {
			if strings.Contains(strings.ToLower(d.Titulo), grupo) ||
				strings.Contains(strings.ToLower(derefStr(d.Descricao)), grupo) {
				score += freq * 3
				razoes = append(razoes, fmt.Sprintf("baseado nos seus treinos de %s", grupo))
				break
			}
		}

		razao := "Desafio recomendado para você"
		if len(razoes) > 0 {
			razao = strings.Join(razoes, " • ")
		}

		candidatos = append(candidatos, scored{d, score, razao})
	}

	// Ordenar por score decrescente (insertion sort — listas pequenas)
	for i := 1; i < len(candidatos); i++ {
		for j := i; j > 0 && candidatos[j].score > candidatos[j-1].score; j-- {
			candidatos[j], candidatos[j-1] = candidatos[j-1], candidatos[j]
		}
	}

	// Retornar top 5
	limit := 5
	if len(candidatos) < limit {
		limit = len(candidatos)
	}

	// 6. Enriquecer as razões com o Claude (quando disponível).
	//    A seleção e a ordem continuam sendo da heurística — a IA só reescreve
	//    o texto de justificativa. Qualquer falha mantém a razão heurística.
	if s.anthropic != nil && limit > 0 {
		candidatosPrompt := make([]map[string]any, 0, limit)
		for i := 0; i < limit; i++ {
			d := candidatos[i].d
			candidatosPrompt = append(candidatosPrompt, map[string]any{
				"id":        d.ID,
				"titulo":    truncar(d.Titulo, 120),
				"descricao": truncar(derefStr(d.Descricao), 200),
				"valor":     utils.CentavosParaReais(d.ValorEntradaCentavos),
				"local":     truncar(derefStr(d.Local), 80),
				"vagas":     d.Vagas,
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeoutIA)
		defer cancel()

		razoesIA := s.anthropic.GerarRazoesRecomendacao(ctx, s.montarPerfil(userID, gruposFreq, valMedia), candidatosPrompt)
		for i := 0; i < limit; i++ {
			if razaoIA, ok := razoesIA[candidatos[i].d.ID]; ok && razaoIA != "" {
				candidatos[i].razao = razaoIA
			}
		}
	}

	ctx := &contextoUsuario{ID: userID, Perfil: "atleta"}
	if s.usuarioRepo != nil {
		if u, err := s.usuarioRepo.BuscarPorID(userID); err == nil && u != nil {
			ctx.Perfil = u.Perfil
		}
	}
	result := make([]models.DesafioRecomendado, limit)
	for i := 0; i < limit; i++ {
		result[i] = models.DesafioRecomendado{
			Desafio: montarResposta(&candidatos[i].d, ctx, false, agora),
			Razao:   candidatos[i].razao,
		}
	}
	return result, nil
}

// montarPerfil gera a descrição textual do atleta enviada ao Claude.
// Mantém-se propositalmente sem PII (nome, email, CPF): só dados de jogo.
func (s *recomendacaoService) montarPerfil(userID uint, gruposFreq map[string]int, valMedia float64) string {
	elo, pontos := utils.CalcularElo(0), 0
	if s.usuarioRepo != nil {
		if u, err := s.usuarioRepo.BuscarPorID(userID); err == nil && u != nil {
			pontos = u.Pontos
			elo = utils.CalcularElo(pontos)
		}
	}

	grupos := make([]string, 0, len(gruposFreq))
	for g := range gruposFreq {
		grupos = append(grupos, g)
	}
	sort.Strings(grupos) // ordem estável — evita variar o prompt a cada chamada
	gruposStr := strings.Join(grupos, ", ")
	if gruposStr == "" {
		gruposStr = "sem histórico de treinos"
	}

	mediaStr := "sem histórico de apostas"
	if valMedia > 0 {
		mediaStr = fmt.Sprintf("R$ %.0f", valMedia)
	}

	return fmt.Sprintf("Elo: %s | Pontos: %d | Grupos de treino frequentes: %s | Valor médio histórico: %s",
		elo, pontos, gruposStr, mediaStr)
}
