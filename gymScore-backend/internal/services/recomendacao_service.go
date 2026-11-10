package services

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gymScore-backend/internal/client"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"
)

// ─── Recomendação de desafios (rodada 6, A6-1) ───────────────────────────────
//
// Fluxo: desafios abertos → filtro de elegibilidade (MESMA regra de `pode_entrar` do feed: vagas, já
// inscrito, evento passado, conflito de interesse, choque de horário, nível/idade) → score DETERMINÍSTICO
// (Nível GymScore + 6 atributos + 8 categorias + valor) → top 5 → "por que" de cada um: o Claude escreve a
// frase a partir dos mesmos números (saída estruturada e validada); qualquer falha → frase determinística.
// A seleção e a ordem são sempre do backend: a IA só redige o texto.

// timeoutIA limita a espera pelo Claude. Estourado o prazo, a recomendação sai com a razão determinística.
const timeoutIA = 8 * time.Second

// limiteRecomendacoes é o tamanho da lista devolvida.
const limiteRecomendacoes = 5

// Fontes do "por que" (campo razao_fonte).
const (
	RazaoFonteIA     = "ia"
	RazaoFonteRegras = "regras"
)

// Limites da frase do modelo (validação; fora disso vale a determinística).
const (
	razaoIAMinRunas    = 12
	razaoIAMaxRunas    = 220
	razaoIAMaxPalavras = 35
)

// RecomendacaoService define as operações de recomendação personalizada de desafios
type RecomendacaoService interface {
	Recomendar(userID uint) ([]models.DesafioRecomendado, error)
}

// GeradorRazoesIA é o subconjunto do cliente Anthropic usado aqui (interface para mock nos testes).
type GeradorRazoesIA interface {
	GerarRazoesRecomendacao(ctx context.Context, perfil client.PerfilAtletaIA, candidatos []client.CandidatoIA) ([]client.RazaoIA, error)
}

// desafioLister é o subconjunto de *repositories.DesafioRepository usado pela recomendação.
// Isolado em interface só para permitir mock nos testes unitários (o repositório em si é um struct).
type desafioLister interface {
	Listar(f repositories.FiltroDesafios) ([]models.Desafio, int64, error)
	ListarHistoricoUsuario(idUsuario uint) ([]models.Desafio, error)
}

type recomendacaoService struct {
	desafioRepo desafioLister
	usuarioRepo repositories.UsuarioRepository
	// ia é opcional: nil = só razões determinísticas.
	ia GeradorRazoesIA
	// gymScore (opcional) fornece nível + eixos; sem ele o perfil sai "sem dados" (nível do cache).
	gymScore GymScoreService
	// contexto (opcional) é o contexto completo do desafioService no modo atleta (conflito de interesse,
	// inscrições com horário, elegibilidade): com ele a recomendação aplica exatamente o `pode_entrar` do feed.
	contexto  func(userID uint) (*contextoUsuario, error)
	timeoutIA time.Duration
	agora     func() time.Time
}

// OpcaoRecomendacao configura dependências opcionais do serviço.
type OpcaoRecomendacao func(*recomendacaoService)

// ComEstatisticas injeta o GymScoreService (nível + atributos + categorias).
func ComEstatisticas(g GymScoreService) OpcaoRecomendacao {
	return func(s *recomendacaoService) { s.gymScore = g }
}

// ComRegrasDeEntrada faz a recomendação usar o MESMO contexto de `pode_entrar` do serviço de desafios (sem
// duplicar regra): conflito de interesse e choque de horário passam a excluir candidatos.
func ComRegrasDeEntrada(d DesafioService) OpcaoRecomendacao {
	return func(s *recomendacaoService) {
		if ds, ok := d.(*desafioService); ok && ds != nil {
			atleta := ds.ComModo(models.ModoAtleta).(*desafioService)
			s.contexto = atleta.contexto
		}
	}
}

// ComTimeoutIA troca o prazo da chamada ao modelo (testes).
func ComTimeoutIA(t time.Duration) OpcaoRecomendacao {
	return func(s *recomendacaoService) { s.timeoutIA = t }
}

// NovoRecomendacaoService cria o serviço. ia pode ser nil (ou um *client.AnthropicClient nil: sem chave).
func NovoRecomendacaoService(
	desafioRepo desafioLister,
	usuarioRepo repositories.UsuarioRepository,
	ia GeradorRazoesIA,
	opcoes ...OpcaoRecomendacao,
) RecomendacaoService {
	if c, ok := ia.(*client.AnthropicClient); ok && c == nil {
		ia = nil // ponteiro nulo dentro da interface = IA desabilitada
	}
	s := &recomendacaoService{desafioRepo: desafioRepo, usuarioRepo: usuarioRepo, ia: ia,
		timeoutIA: timeoutIA, agora: time.Now}
	for _, o := range opcoes {
		o(s)
	}
	return s
}

// perfilRecomendacao é o que o score e as razões olham do atleta.
type perfilRecomendacao struct {
	nivel      int
	pontuacao  int
	atributos  map[string]int
	categorias map[string]int
	semDados   bool
	valorMedio float64 // R$; 0 = sem histórico
}

// candidatoRecomendacao é um desafio elegível com o score e os fatos que o justificam.
type candidatoRecomendacao struct {
	d       *models.Desafio
	resp    *models.DesafioResponse
	score   int
	base    models.BaseRecomendacao
	razao   string
	sinais  []string
	ordemOr int // posição na lista original (desempate: evento mais próximo)
}

// Recomendar retorna até 5 desafios abertos em que o atleta PODE entrar, ordenados pelo score.
func (s *recomendacaoService) Recomendar(userID uint) ([]models.DesafioRecomendado, error) {
	agora := s.agora()
	todos, _, err := s.desafioRepo.Listar(repositories.FiltroDesafios{
		IDUsuario: userID, Status: models.StatusAberto, Futuros: true, Agora: agora,
		Ordem: repositories.OrdemEventoProx, Pagina: 1, Limite: 500,
	})
	if err != nil {
		return nil, err
	}

	historico, _ := s.desafioRepo.ListarHistoricoUsuario(userID)
	participados := make(map[uint]bool, len(historico))
	var somaValor float64
	for _, d := range historico {
		participados[d.ID] = true
		somaValor += utils.CentavosParaReais(d.ValorEntradaCentavos)
	}

	perfil := s.carregarPerfil(userID)
	if len(historico) > 0 {
		perfil.valorMedio = somaValor / float64(len(historico))
	}

	uctx, err := s.contextoDe(userID, perfil)
	if err != nil {
		return nil, err
	}

	candidatos := make([]candidatoRecomendacao, 0, len(todos))
	for i := range todos {
		d := &todos[i]
		if d.Status != models.StatusAberto || participados[d.ID] {
			continue
		}
		resp := montarResposta(d, uctx, false, agora)
		if !elegivel(d, resp, uctx, agora) {
			continue
		}
		c := pontuar(d, perfil)
		c.resp, c.ordemOr = resp, i
		candidatos = append(candidatos, c)
	}

	// Score decrescente; empate mantém a ordem do repositório (evento mais próximo primeiro).
	sort.SliceStable(candidatos, func(i, j int) bool { return candidatos[i].score > candidatos[j].score })
	if len(candidatos) > limiteRecomendacoes {
		candidatos = candidatos[:limiteRecomendacoes]
	}

	razoesIA := s.razoesDaIA(perfil, candidatos)

	result := make([]models.DesafioRecomendado, len(candidatos))
	for i := range candidatos {
		c := &candidatos[i]
		item := models.DesafioRecomendado{Desafio: c.resp, Razao: c.razao, RazaoFonte: RazaoFonteRegras}
		if r, ok := razoesIA[c.d.ID]; ok {
			item.Razao, item.RazaoFonte = r, RazaoFonteIA
		}
		base := c.base
		item.Base = &base
		result[i] = item
	}
	return result, nil
}

// elegivel aplica a regra de `pode_entrar` (calculada por montarResposta com o contexto do atleta). Para
// contas que não são de atleta (academia), pode_entrar é sempre falso: mantém só os filtros básicos.
func elegivel(d *models.Desafio, resp *models.DesafioResponse, uctx *contextoUsuario, agora time.Time) bool {
	if uctx.Perfil == models.ModoAtleta {
		return resp.PodeEntrar
	}
	if d.MinhaParticipacao != nil || d.QtdParticipantes >= d.Vagas {
		return false
	}
	return uctx.elegibilidade == nil || MotivoFiltroEntrada(d, *uctx.elegibilidade, agora) == ""
}

// carregarPerfil lê nível e eixos (GymScoreService). Falha ou ausência → perfil "sem dados" com o nível do
// cache de usuarios (a recomendação nunca quebra por causa das estatísticas).
func (s *recomendacaoService) carregarPerfil(userID uint) perfilRecomendacao {
	p := perfilRecomendacao{nivel: models.NivelGymScoreMin, atributos: map[string]int{}, categorias: map[string]int{}, semDados: true}
	if s.gymScore != nil {
		e, err := s.gymScore.EstatisticasDoAtleta(userID)
		if err != nil {
			log.Printf("[Recomendacao] estatísticas indisponíveis para %d: %v — seguindo sem eixos", userID, err)
		} else if e != nil {
			p.nivel, p.pontuacao, p.semDados = e.Nivel, e.Pontuacao, e.SemDados()
			if e.Atributos != nil {
				p.atributos = e.Atributos
			}
			if e.Categorias != nil {
				p.categorias = e.Categorias
			}
			return p
		}
	}
	if s.usuarioRepo != nil {
		if u, err := s.usuarioRepo.BuscarPorID(userID); err == nil && u != nil {
			if u.NivelGymScore > p.nivel {
				p.nivel = u.NivelGymScore
			}
			p.pontuacao = u.PontuacaoGymScore
		}
	}
	return p
}

// contextoDe devolve o contexto de quem pede a recomendação, na visão do modo atleta. Com o desafioService
// injetado, é o mesmo contexto do feed (conflito, choque, elegibilidade); sem ele, um contexto mínimo
// (nível/idade). O nível vigente das estatísticas prevalece sobre o cache.
func (s *recomendacaoService) contextoDe(userID uint, perfil perfilRecomendacao) (*contextoUsuario, error) {
	if s.contexto != nil {
		c, err := s.contexto(userID)
		if err != nil {
			return nil, err
		}
		if c.elegibilidade != nil && s.gymScore != nil {
			c.elegibilidade.Nivel = perfil.nivel
		}
		return c, nil
	}
	c := &contextoUsuario{ID: userID, Perfil: models.ModoAtleta, elegibilidade: &PerfilElegibilidade{Nivel: perfil.nivel}}
	if s.usuarioRepo != nil {
		if u, err := s.usuarioRepo.BuscarPorID(userID); err == nil && u != nil {
			c.elegibilidade.DataNascimento = u.DataNascimento
		}
		if p, err := s.usuarioRepo.PapeisDoUsuario(userID); err == nil && p != nil && p.Conta != models.PerfilAtleta {
			c.Perfil = p.Conta
		}
	}
	return c, nil
}

// ─── Score determinístico ────────────────────────────────────────────────────

// Pesos do score (somam com a base; tudo inteiro no fim).
const (
	scoreBase            = 5
	pesoScoreCategoria   = 0.4 // valor 0..100 do eixo da categoria do desafio
	pesoScoreAtributos   = 0.3 // afinidade 0..100: atributos que a categoria exige x valores do atleta
	bonusFaixaNivel      = 10  // desafio com faixa de nível que inclui o atleta (feito para o nível dele)
	bonusValorCompativel = 10  // entrada dentro de ±30% da média histórica
	limiarPontoForte     = 25  // a partir daqui o eixo é citado como "ponto forte"
	maxEixosNaBase       = 2
)

func rotuloEixo(eixos []models.EixoEstatistica, chave string) string {
	for _, e := range eixos {
		if e.Chave == chave {
			return e.Rotulo
		}
	}
	return ""
}

// rotuloCategoria inclui as categorias sem eixo próprio (geral/outros).
func rotuloCategoria(cat string) string {
	if r := rotuloEixo(EixosCategorias, cat); r != "" {
		return r
	}
	switch cat {
	case "outros":
		return "Outros"
	default:
		return "Geral"
	}
}

// pesosDaCategoria é a linha da tabela única de pesos (gymscore.go) que diz o que a categoria exige.
func pesosDaCategoria(cat string) (map[string]float64, bool) {
	if p, ok := pesosCategoria[cat]; ok {
		return p, true
	}
	return pesoCategoriaGenerica, false
}

// pontuar calcula o score e os fatos ("sinais") que justificam o desafio para este atleta.
func pontuar(d *models.Desafio, p perfilRecomendacao) candidatoRecomendacao {
	cat := strings.ToLower(strings.TrimSpace(d.Categoria))
	if cat == "" {
		cat = models.CategoriaPadrao
	}
	pesos, temEixo := pesosDaCategoria(cat)
	rotCat := rotuloCategoria(cat)

	// Afinidade de atributos: média ponderada (pesos somam 1) dos valores do atleta nos atributos exigidos.
	var afinidade float64
	eixos := make([]models.EixoRecomendacao, 0, len(pesos))
	for chave, w := range pesos {
		v := p.atributos[chave]
		afinidade += w * float64(v)
		eixos = append(eixos, models.EixoRecomendacao{Chave: chave, Rotulo: rotuloEixo(EixosAtributos, chave),
			Valor: v, PesoPct: int(math.Round(w * 100))})
	}
	// Eixos mais relevantes primeiro: contribuição (peso x valor), depois peso, depois chave (estável).
	sort.Slice(eixos, func(i, j int) bool {
		ci, cj := eixos[i].PesoPct*eixos[i].Valor, eixos[j].PesoPct*eixos[j].Valor
		if ci != cj {
			return ci > cj
		}
		if eixos[i].PesoPct != eixos[j].PesoPct {
			return eixos[i].PesoPct > eixos[j].PesoPct
		}
		return eixos[i].Chave < eixos[j].Chave
	})
	if len(eixos) > maxEixosNaBase {
		eixos = eixos[:maxEixosNaBase]
	}

	base := models.BaseRecomendacao{Nivel: p.nivel, Categoria: cat, CategoriaRotulo: rotCat, Atributos: eixos,
		NivelMin: d.NivelMin, NivelMax: d.NivelMax, SemDados: p.semDados}
	score := float64(scoreBase) + pesoScoreAtributos*afinidade
	valCat := 0
	if temEixo {
		valCat = p.categorias[cat]
		v := valCat
		base.ValorCategoria = &v
		score += pesoScoreCategoria * float64(valCat)
	}
	faixa := d.NivelMin != nil || d.NivelMax != nil
	if faixa {
		score += bonusFaixaNivel // só chega aqui quem cumpre a faixa (filtro de elegibilidade)
	}
	valor := utils.CentavosParaReais(d.ValorEntradaCentavos)
	if p.valorMedio > 0 && valor >= p.valorMedio*0.7 && valor <= p.valorMedio*1.3 {
		score += bonusValorCompativel
		base.ValorCompativel = true
	}
	base.Score = int(math.Round(score))

	sinais := []string{}
	if temEixo {
		sinais = append(sinais, fmt.Sprintf("categoria %s do atleta: %d/100", rotCat, valCat))
	}
	if len(eixos) > 0 {
		partes := make([]string, len(eixos))
		for i, e := range eixos {
			partes[i] = fmt.Sprintf("%s %d/100", e.Rotulo, e.Valor)
		}
		sinais = append(sinais, "atributos que este desafio mais exige, com o valor do atleta: "+strings.Join(partes, ", "))
	}
	if faixa {
		sinais = append(sinais, fmt.Sprintf("desafio para %s; atleta no nível %d", faixaTexto(d.NivelMin, d.NivelMax, "nível"), p.nivel))
	}
	if base.ValorCompativel {
		sinais = append(sinais, "valor de entrada parecido com o dos desafios anteriores do atleta")
	}
	if p.semDados {
		sinais = append(sinais, "atleta ainda sem dados nos gráficos (últimos 180 dias)")
	}

	return candidatoRecomendacao{d: d, score: base.Score, base: base, sinais: sinais, razao: razaoDeterministica(base)}
}

// razaoDeterministica é o "por que" sem IA: uma frase em pt-BR citando eixo ou nível. Também é o fallback
// de cada item quando o modelo falha ou devolve algo inválido.
func razaoDeterministica(b models.BaseRecomendacao) string {
	catMin := strings.ToLower(b.CategoriaRotulo)
	if b.ValorCategoria != nil && *b.ValorCategoria >= limiarPontoForte {
		return fmt.Sprintf("Você tem %s em %d/100 no seu gráfico: este desafio aproveita um dos seus pontos fortes.",
			b.CategoriaRotulo, *b.ValorCategoria)
	}
	if len(b.Atributos) > 0 && b.Atributos[0].Valor >= limiarPontoForte {
		a := b.Atributos[0]
		if b.ValorCategoria == nil {
			return fmt.Sprintf("Desafio variado que valoriza sua %s (%d/100), um dos seus pontos fortes.", a.Rotulo, a.Valor)
		}
		return fmt.Sprintf("Desafios de %s exigem %s, e a sua está em %d/100.", catMin, a.Rotulo, a.Valor)
	}
	if b.NivelMin != nil || b.NivelMax != nil {
		return fmt.Sprintf("Feito para o seu nível: aceita %s e você está no nível %d.", faixaTexto(b.NivelMin, b.NivelMax, "nível"), b.Nivel)
	}
	if b.SemDados {
		return fmt.Sprintf("Bom ponto de partida no nível %d: participar já começa a preencher seus gráficos.", b.Nivel)
	}
	if b.ValorCategoria != nil {
		return fmt.Sprintf("%s está em %d/100 no seu gráfico: ótimo desafio para evoluir a partir do nível %d.",
			b.CategoriaRotulo, *b.ValorCategoria, b.Nivel)
	}
	return fmt.Sprintf("Desafio aberto ao seu nível %d que trabalha vários atributos ao mesmo tempo.", b.Nivel)
}

// ─── IA (Claude) ─────────────────────────────────────────────────────────────

// razoesDaIA pede o "por que" ao modelo e devolve só as frases válidas (id entre os candidatos, texto
// limpo). Sem IA, erro, timeout ou nada válido → mapa vazio (todos ficam com a determinística).
func (s *recomendacaoService) razoesDaIA(p perfilRecomendacao, cands []candidatoRecomendacao) map[uint]string {
	if s.ia == nil || len(cands) == 0 {
		return nil
	}
	perfil := client.PerfilAtletaIA{Nivel: p.nivel, NivelMax: models.NivelGymScoreMax, Pontuacao: p.pontuacao,
		Atributos: p.atributos, Categorias: p.categorias, SemDados: p.semDados,
		ValorMedioReais: math.Round(p.valorMedio*100) / 100}
	entrada := make([]client.CandidatoIA, len(cands))
	validos := make(map[uint]bool, len(cands))
	for i, c := range cands {
		entrada[i] = client.CandidatoIA{ID: c.d.ID, Titulo: truncar(c.d.Titulo, 120),
			Resumo: truncar(derefStr(c.d.Resumo), 280), Categoria: c.base.Categoria,
			ValorReais: utils.CentavosParaReais(c.d.ValorEntradaCentavos), NivelMin: c.d.NivelMin, NivelMax: c.d.NivelMax,
			Sinais: c.sinais}
		validos[c.d.ID] = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeoutIA)
	defer cancel()
	itens, err := s.ia.GerarRazoesRecomendacao(ctx, perfil, entrada)
	if err != nil {
		log.Printf("[Recomendacao] IA indisponível (%v) — razões determinísticas", err)
		return nil
	}
	return validarRazoesIA(itens, validos)
}

// validarRazoesIA aceita só: id entre os candidatos enviados (sem repetir) e frase limpa (uma linha, sem
// marcação/links, tamanho razoável). O texto vai ao cliente como texto puro — nunca como HTML.
func validarRazoesIA(itens []client.RazaoIA, validos map[uint]bool) map[uint]string {
	out := make(map[uint]string, len(itens))
	vistos := make(map[uint]bool, len(itens))
	for _, it := range itens {
		if !validos[it.ID] || vistos[it.ID] {
			if !validos[it.ID] {
				log.Printf("[Recomendacao] IA devolveu id %d fora dos candidatos — descartado", it.ID)
			}
			vistos[it.ID] = true
			delete(out, it.ID) // id repetido: nenhuma das versões vale
			continue
		}
		vistos[it.ID] = true
		if r, ok := limparRazaoIA(it.PorQue); ok {
			out[it.ID] = r
		}
	}
	return out
}

// limparRazaoIA normaliza espaços, remove caracteres de controle e recusa marcação, links e tamanhos fora
// do esperado.
func limparRazaoIA(s string) (string, bool) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x200b || r == 0xfeff {
			return ' '
		}
		return r
	}, s)
	palavras := strings.Fields(s)
	s = strings.Join(palavras, " ")
	n := utf8.RuneCountInString(s)
	if n < razaoIAMinRunas || n > razaoIAMaxRunas || len(palavras) > razaoIAMaxPalavras {
		return "", false
	}
	if strings.ContainsAny(s, "<>{}[]`*#|\\") {
		return "", false
	}
	l := strings.ToLower(s)
	for _, proibido := range []string{"http:", "https:", "www.", "javascript:", "&lt;", "&#"} {
		if strings.Contains(l, proibido) {
			return "", false
		}
	}
	return s, true
}
