package services

import (
	"math"
	"sort"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// ─── Nível GymScore (rodada 6, U3) ───────────────────────────────────────────
//
// Pontuação (todo o histórico; desafio só conta encerrado e com o resultado NÃO anulado):
//   - pódio: 1º 50, 2º 35, 3º 25, demais posições premiadas 15;
//   - presença confirmada em desafio encerrado: 10 (inclusive de quem foi ao pódio);
//   - treino registrado: 1 por DIA com exercício concluído (treino é autodeclarado: no máx. 1 ponto/dia).
//
// O pódio conta desde o lançamento (não espera a janela de contestação); a anulação pela moderação tira
// os pontos (o resultado anulado deixa de contar).
//
// Cache em usuarios.pontuacao_gymscore / nivel_gymscore / gymscore_calculado_em, gravado por UPDATE
// explícito: (1) na MESMA transação do evento (lançar/relançar pódio, anular resultado, registrar treino,
// inscrição); (2) logo depois do commit, numa transação curta que trava a linha do usuário ANTES de ler
// (corrige a corrida de dois eventos simultâneos do mesmo usuário); (3) pelo job `reconciliar-nivel`
// (nunca calculados primeiro, depois os mais antigos).
const (
	gsPontosPrimeiro  = 50
	gsPontosSegundo   = 35
	gsPontosTerceiro  = 25
	gsPontosPremiado  = 15 // 4º em diante
	gsPontosPresenca  = 10
	gsPontosDiaTreino = 1
)

// faixasNivelGymScore[i] é a pontuação em que o nível i+1 começa (nível 1 = 0..19, ..., nível 10 = 1100+).
// Referência: ~3 treinos/semana dão ~150 pontos/ano; cada desafio com presença +10 e um pódio de 25 a 50.
var faixasNivelGymScore = []int{0, 20, 50, 100, 175, 275, 400, 575, 800, 1100}

// PontuacaoGymScore aplica a tabela de pontos aos componentes.
func PontuacaoGymScore(c repositories.ComponentesGymScore) int {
	return c.Primeiros*gsPontosPrimeiro + c.Segundos*gsPontosSegundo + c.Terceiros*gsPontosTerceiro +
		c.OutrosPremiados*gsPontosPremiado + c.Presencas*gsPontosPresenca + c.DiasTreino*gsPontosDiaTreino
}

// NivelPorPontuacao converte a pontuação no nível 1..10 pelas faixas.
func NivelPorPontuacao(p int) int {
	nivel := models.NivelGymScoreMin
	for i, inicio := range faixasNivelGymScore {
		if p >= inicio {
			nivel = i + 1
		}
	}
	return nivel
}

// FaixaNivel devolve onde o nível começa e onde começa o próximo (nil no nível máximo).
func FaixaNivel(nivel int) (inicio int, proximo *int) {
	if nivel < models.NivelGymScoreMin {
		nivel = models.NivelGymScoreMin
	}
	if nivel > models.NivelGymScoreMax {
		nivel = models.NivelGymScoreMax
	}
	inicio = faixasNivelGymScore[nivel-1]
	if nivel < models.NivelGymScoreMax {
		p := faixasNivelGymScore[nivel]
		proximo = &p
	}
	return inicio, proximo
}

// recalcularGymScoreTx recalcula e grava o nível dos usuários (ordem crescente de id) na transação do
// evento. Devolve o nível calculado de cada um.
func recalcularGymScoreTx(tx *gorm.DB, ids []uint, agora time.Time) (map[uint]int, error) {
	ids = idsUnicosOrdenados(ids)
	niveis := make(map[uint]int, len(ids))
	for _, id := range ids {
		c, err := repositories.CarregarComponentesGymScore(tx, id)
		if err != nil {
			return nil, err
		}
		p := PontuacaoGymScore(c)
		niveis[id] = NivelPorPontuacao(p)
		if err := repositories.GravarGymScore(tx, id, p, niveis[id], agora.Truncate(time.Millisecond)); err != nil {
			return nil, err
		}
	}
	return niveis, nil
}

// RecalcularGymScoreNaTransacao é recalcularGymScoreTx exportado (seed e outros pacotes que já estão
// numa transação com as linhas dos usuários travadas ou sem concorrência).
func RecalcularGymScoreNaTransacao(tx *gorm.DB, ids []uint, agora time.Time) error {
	_, err := recalcularGymScoreTx(tx, ids, agora)
	return err
}

// RecalcularGymScore recalcula o nível de cada usuário numa transação curta própria que trava a linha
// dele ANTES de qualquer leitura (o snapshot começa depois da trava: enxerga tudo o que já foi confirmado).
// Usado depois do commit dos eventos e pelo job. Erros de um usuário não impedem os demais.
func RecalcularGymScore(db *gorm.DB, ids []uint, agora time.Time) error {
	var primeiro error
	for _, id := range idsUnicosOrdenados(ids) {
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := repositories.TravarUsuariosOrdenados(tx, []uint{id}); err != nil {
				return err
			}
			_, err := recalcularGymScoreTx(tx, []uint{id}, agora)
			return err
		})
		if err != nil && primeiro == nil {
			primeiro = err
		}
	}
	return primeiro
}

func idsUnicosOrdenados(ids []uint) []uint {
	vistos := make(map[uint]bool, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !vistos[id] {
			vistos[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ─── Atributos e categorias (rodada 6, U5) ───────────────────────────────────

// JanelaEstatisticasDias é a janela dos gráficos de teia; dentro dela o recente pesa mais (peso linear
// de 1,0 hoje a 0,5 no fim da janela).
const JanelaEstatisticasDias = 180

// Eixos dos gráficos, na ordem de exibição.
var (
	EixosAtributos = []models.EixoEstatistica{
		{Chave: "forca", Rotulo: "Força"}, {Chave: "resistencia", Rotulo: "Resistência"},
		{Chave: "velocidade", Rotulo: "Velocidade"}, {Chave: "agilidade", Rotulo: "Agilidade"},
		{Chave: "mobilidade", Rotulo: "Mobilidade"}, {Chave: "constancia", Rotulo: "Constância"},
	}
	EixosCategorias = []models.EixoEstatistica{
		{Chave: "corrida", Rotulo: "Corrida"}, {Chave: "musculacao", Rotulo: "Musculação"},
		{Chave: "crossfit", Rotulo: "CrossFit"}, {Chave: "funcional", Rotulo: "Funcional"},
		{Chave: "ciclismo", Rotulo: "Ciclismo"}, {Chave: "natacao", Rotulo: "Natação"},
		{Chave: "calistenia", Rotulo: "Calistenia"}, {Chave: "luta", Rotulo: "Luta"},
	}
)

// TABELA DE PESOS (único lugar): quanto cada grupo muscular treinado e cada categoria de desafio
// alimenta os atributos. Cada linha soma 1. "constancia" não entra aqui: vem dos dias ativos.
var (
	pesosGrupoMuscular = map[string]map[string]float64{
		"peito":     {"forca": 1},
		"costas":    {"forca": 0.8, "mobilidade": 0.2},
		"ombro":     {"forca": 0.7, "mobilidade": 0.3},
		"biceps":    {"forca": 1},
		"triceps":   {"forca": 1},
		"antebraco": {"forca": 0.8, "agilidade": 0.2},
		"pernas":    {"forca": 0.6, "resistencia": 0.2, "velocidade": 0.2},
		"abdominal": {"mobilidade": 0.5, "agilidade": 0.3, "resistencia": 0.2},
	}
	pesoGrupoDesconhecido = map[string]float64{"forca": 0.5, "resistencia": 0.5}

	pesosCategoria = map[string]map[string]float64{
		"corrida":    {"resistencia": 0.5, "velocidade": 0.5},
		"musculacao": {"forca": 1},
		"crossfit":   {"forca": 0.35, "resistencia": 0.35, "agilidade": 0.3},
		"funcional":  {"agilidade": 0.4, "mobilidade": 0.3, "resistencia": 0.3},
		"ciclismo":   {"resistencia": 0.6, "velocidade": 0.4},
		"natacao":    {"resistencia": 0.5, "mobilidade": 0.3, "velocidade": 0.2},
		"calistenia": {"forca": 0.4, "mobilidade": 0.3, "agilidade": 0.3},
		"luta":       {"agilidade": 0.4, "forca": 0.3, "resistencia": 0.3},
	}
	// geral / outros / desconhecida: espalha igualmente pelos 5 atributos físicos.
	pesoCategoriaGenerica = map[string]float64{"forca": 0.2, "resistencia": 0.2, "velocidade": 0.2, "agilidade": 0.2, "mobilidade": 0.2}
)

// Unidades e saturação (valor = 100 x (1 - e^(-unidades/K)), nunca passa de 100).
const (
	unidadeExercicio        = 1.0 // cada exercício concluído
	unidadePresenca         = 4.0 // presença em desafio encerrado
	saturacaoAtributo       = 30.0
	saturacaoConstancia     = 24.0 // dias ativos ponderados (~2/semana no semestre dá ~90)
	saturacaoCategoria      = 12.0 // 1 presença ~28, 3 presenças ~63, 1 vitória ~49
	pesoRecenteMinimo       = 0.5
	bonusPodioPrimeiro      = 4.0
	bonusPodioSegundo       = 3.0
	bonusPodioTerceiro      = 2.0
	bonusPodioPremiado      = 1.0
	baseCalculoEstatisticas = "Nível GymScore (1 a 10): soma de pontos de todo o histórico — pódio (1º 50, 2º 35, 3º 25, demais premiados 15), " +
		"10 por presença confirmada em desafio encerrado e 1 por dia com treino registrado; resultados anulados pela moderação não contam. " +
		"Gráficos: últimos 180 dias, com mais peso no que é recente. Cada exercício concluído soma no atributo do grupo muscular " +
		"(ex.: peito → Força; pernas → Força, Resistência e Velocidade); cada desafio com presença soma na sua categoria e nos atributos " +
		"dela (ex.: corrida → Resistência e Velocidade), com bônus para pódio. Constância = dias ativos (treino ou desafio). " +
		"Cada eixo vai de 0 a 100 e satura: quanto mais perto de 100, mais difícil subir."
)

func pesoRecencia(quando, agora time.Time) float64 {
	dias := agora.Sub(quando).Hours() / 24
	if dias < 0 {
		dias = 0
	}
	if dias > JanelaEstatisticasDias {
		dias = JanelaEstatisticasDias
	}
	return 1 - (1-pesoRecenteMinimo)*dias/JanelaEstatisticasDias
}

func saturar(unidades, k float64) int {
	if unidades <= 0 {
		return 0
	}
	v := int(math.Round(100 * (1 - math.Exp(-unidades/k))))
	if v > 100 {
		v = 100
	}
	return v
}

func bonusPodio(posicao *int) float64 {
	if posicao == nil {
		return 0
	}
	switch *posicao {
	case 1:
		return bonusPodioPrimeiro
	case 2:
		return bonusPodioSegundo
	case 3:
		return bonusPodioTerceiro
	default:
		return bonusPodioPremiado
	}
}

// CalcularEixos transforma os treinos e desafios da janela nos dois gráficos (0..100 cada eixo).
// Função pura (testável): a mesma conta serve à tela e à recomendação.
func CalcularEixos(treinos []repositories.TreinoNaJanela, desafios []repositories.DesafioNaJanela, agora time.Time) (map[string]int, map[string]int, models.DadosJanela) {
	unidAtr := map[string]float64{}
	unidCat := map[string]float64{}
	diasAtivos := map[string]float64{} // dia -> peso
	dados := models.DadosJanela{JanelaDias: JanelaEstatisticasDias}
	diasTreino := map[string]bool{}
	for _, t := range treinos {
		w := pesoRecencia(t.Dia, agora)
		pesos, ok := pesosGrupoMuscular[strings.ToLower(strings.TrimSpace(t.GrupoMuscular))]
		if !ok {
			pesos = pesoGrupoDesconhecido
		}
		for a, p := range pesos {
			unidAtr[a] += float64(t.N) * unidadeExercicio * p * w
		}
		dia := t.Dia.Format("2006-01-02")
		diasAtivos[dia] = w
		diasTreino[dia] = true
		dados.Exercicios += t.N
	}
	dados.DiasTreino = len(diasTreino)
	for _, d := range desafios {
		w := pesoRecencia(d.Quando, agora)
		u := (unidadePresenca + bonusPodio(d.Posicao)) * w
		pesos, ok := pesosCategoria[d.Categoria]
		if !ok {
			pesos = pesoCategoriaGenerica
		}
		for a, p := range pesos {
			unidAtr[a] += u * p
		}
		if _, ok := pesosCategoria[d.Categoria]; ok {
			unidCat[d.Categoria] += u
		}
		diasAtivos[d.Quando.Format("2006-01-02")] = w
		dados.Desafios++
		if d.Posicao != nil {
			dados.Podios++
		}
	}
	var constancia float64
	for _, w := range diasAtivos {
		constancia += w
	}
	atributos := make(map[string]int, len(EixosAtributos))
	for _, e := range EixosAtributos {
		if e.Chave == "constancia" {
			atributos[e.Chave] = saturar(constancia, saturacaoConstancia)
			continue
		}
		atributos[e.Chave] = saturar(unidAtr[e.Chave], saturacaoAtributo)
	}
	categorias := make(map[string]int, len(EixosCategorias))
	for _, e := range EixosCategorias {
		categorias[e.Chave] = saturar(unidCat[e.Chave], saturacaoCategoria)
	}
	return atributos, categorias, dados
}

// EstatisticasAtleta é o pacote de dados do atleta para a tela de perfil E para a recomendação
// (A6-1, ai-engineer): nível, pontuação e os dois gráficos. Obter com GymScoreService.EstatisticasDoAtleta.
type EstatisticasAtleta struct {
	IDUsuario      uint
	Nivel          int
	Pontuacao      int
	Composicao     repositories.ComponentesGymScore
	Atributos      map[string]int // chaves de EixosAtributos, 0..100
	Categorias     map[string]int // chaves de EixosCategorias, 0..100
	Dados          models.DadosJanela
	DataNascimento *time.Time
	CalculadoEm    time.Time
}

// SemDados: nada na janela dos gráficos.
func (e *EstatisticasAtleta) SemDados() bool { return e.Dados.Exercicios == 0 && e.Dados.Desafios == 0 }

// Elegibilidade devolve os dados para os filtros de entrada (U4).
func (e *EstatisticasAtleta) Elegibilidade() PerfilElegibilidade {
	return PerfilElegibilidade{Nivel: e.Nivel, DataNascimento: e.DataNascimento}
}

// GymScoreService expõe o Nível GymScore e as estatísticas do atleta.
type GymScoreService interface {
	// Estatisticas é GET /api/usuarios/:id/estatisticas (mesma visibilidade do perfil público).
	Estatisticas(idAlvo uint) (*models.EstatisticasResponse, error)
	// EstatisticasDoAtleta é o ponto reutilizável (recomendação/IA): nil se o usuário não existe.
	EstatisticasDoAtleta(idUsuario uint) (*EstatisticasAtleta, error)
	// ComRecalculo roda fn numa transação com a linha do usuário travada antes e recalcula o nível dele
	// na MESMA transação (evento de treino).
	ComRecalculo(idUsuario uint, fn func(tx *gorm.DB) error) error
	// ReconciliarNiveis é o passo do job `reconciliar-nivel`: recalcula até `limite` usuários (nunca
	// calculados primeiro, depois os calculados há mais de `idade`). Devolve quantos recalculou.
	ReconciliarNiveis(limite int, idade time.Duration) (int, error)
}

type gymScoreService struct {
	db    *gorm.DB
	agora func() time.Time
}

// NovoGymScoreService cria o serviço do Nível GymScore.
func NovoGymScoreService(db *gorm.DB) GymScoreService {
	return &gymScoreService{db: db, agora: time.Now}
}

func (s *gymScoreService) ComRecalculo(idUsuario uint, fn func(tx *gorm.DB) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := repositories.TravarUsuariosOrdenados(tx, []uint{idUsuario}); err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			return err
		}
		_, err := recalcularGymScoreTx(tx, []uint{idUsuario}, s.agora())
		return err
	})
}

func (s *gymScoreService) ReconciliarNiveis(limite int, idade time.Duration) (int, error) {
	agora := s.agora()
	ids, err := repositories.UsuariosParaReconciliarGymScore(s.db, agora.Add(-idade), limite)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return len(ids), RecalcularGymScore(s.db, ids, agora)
}

// nivelVigente devolve a pontuação/nível de agora: calcula e, se o cache está vazio ou diferente,
// regrava (transação curta com a linha travada). Assim a tela e a inscrição veem o mesmo número.
func (s *gymScoreService) nivelVigente(p *repositories.PerfilGymScore, agora time.Time) (repositories.ComponentesGymScore, int, int, error) {
	c, err := repositories.CarregarComponentesGymScore(s.db, p.ID)
	if err != nil {
		return c, 0, 0, err
	}
	pontos := PontuacaoGymScore(c)
	nivel := NivelPorPontuacao(pontos)
	if p.GymScoreCalculadoEm == nil || p.PontuacaoGymScore != pontos || p.NivelGymScore != nivel {
		if err := RecalcularGymScore(s.db, []uint{p.ID}, agora); err != nil {
			return c, 0, 0, err
		}
	}
	return c, pontos, nivel, nil
}

func (s *gymScoreService) EstatisticasDoAtleta(idUsuario uint) (*EstatisticasAtleta, error) {
	p, err := repositories.BuscarPerfilGymScore(s.db, idUsuario)
	if err != nil || p == nil {
		return nil, err
	}
	agora := s.agora()
	c, pontos, nivel, err := s.nivelVigente(p, agora)
	if err != nil {
		return nil, err
	}
	desde := agora.AddDate(0, 0, -JanelaEstatisticasDias)
	treinos, err := repositories.TreinosNaJanela(s.db, idUsuario, desde)
	if err != nil {
		return nil, err
	}
	desafios, err := repositories.DesafiosNaJanela(s.db, idUsuario, desde)
	if err != nil {
		return nil, err
	}
	atr, cat, dados := CalcularEixos(treinos, desafios, agora)
	return &EstatisticasAtleta{IDUsuario: idUsuario, Nivel: nivel, Pontuacao: pontos, Composicao: c,
		Atributos: atr, Categorias: cat, Dados: dados, DataNascimento: p.DataNascimento, CalculadoEm: agora}, nil
}

func (s *gymScoreService) Estatisticas(idAlvo uint) (*models.EstatisticasResponse, error) {
	e, err := s.EstatisticasDoAtleta(idAlvo)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, erroNaoEncontrado("Usuário não encontrado")
	}
	inicio, proximo := FaixaNivel(e.Nivel)
	faltam := 0
	if proximo != nil {
		faltam = *proximo - e.Pontuacao
	}
	c := e.Composicao
	resp := &models.EstatisticasResponse{
		IDUsuario: e.IDUsuario, Nivel: e.Nivel, NivelMax: models.NivelGymScoreMax, Pontuacao: e.Pontuacao,
		NivelInicioEm: inicio, ProximoNivelEm: proximo, FaltamPontos: faltam,
		Composicao: models.ComposicaoPontuacao{Primeiros: c.Primeiros, Segundos: c.Segundos, Terceiros: c.Terceiros,
			OutrosPremiados: c.OutrosPremiados, Presencas: c.Presencas, DiasTreino: c.DiasTreino,
			PontosPodios:    c.Primeiros*gsPontosPrimeiro + c.Segundos*gsPontosSegundo + c.Terceiros*gsPontosTerceiro + c.OutrosPremiados*gsPontosPremiado,
			PontosPresencas: c.Presencas * gsPontosPresenca, PontosTreinos: c.DiasTreino * gsPontosDiaTreino},
		Atributos: e.Atributos, Categorias: e.Categorias,
		Eixos: models.EixosEstatisticas{Atributos: EixosAtributos, Categorias: EixosCategorias},
		Dados: e.Dados, SemDados: e.SemDados(), BaseCalculo: baseCalculoEstatisticas, CalculadoEm: e.CalculadoEm,
	}
	if resp.SemDados {
		resp.Mensagem = "Ainda não há treinos nem desafios nos últimos 180 dias: registre treinos e participe de desafios para montar o gráfico."
	}
	return resp, nil
}
