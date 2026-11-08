package controllers

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// DesafioController gerencia as requisições HTTP de desafios.
type DesafioController struct {
	service services.DesafioService
}

// NovoDesafioController cria uma nova instância do controller de desafios
func NovoDesafioController(service services.DesafioService) *DesafioController {
	return &DesafioController{service: service}
}

func statusValido(s string) (models.StatusDesafio, bool) {
	switch st := models.StatusDesafio(s); st {
	case "", models.StatusAberto, models.StatusEmAndamento, models.StatusEncerrado, models.StatusCancelado:
		return st, true
	}
	return "", false
}

// filtroDaQuery lê e valida os filtros de listagem (status, categoria, q, id_academia, paginação).
func filtroDaQuery(c *fiber.Ctx) (services.FiltroListagem, bool) {
	status, valido := statusValido(c.Query("status"))
	if !valido {
		_ = utils.ValidationError(c, "status inválido")
		return services.FiltroListagem{}, false
	}
	categoria := strings.ToLower(strings.TrimSpace(c.Query("categoria")))
	if categoria != "" && !models.CategoriaValida(categoria) {
		_ = utils.ValidationError(c, "categoria inválida")
		return services.FiltroListagem{}, false
	}
	busca := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(busca) > 100 {
		_ = utils.ValidationError(c, "q deve ter no máximo 100 caracteres")
		return services.FiltroListagem{}, false
	}
	var idAcademia uint
	if v := c.Query("id_academia"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n == 0 {
			_ = utils.ValidationError(c, "id_academia inválido")
			return services.FiltroListagem{}, false
		}
		idAcademia = uint(n)
	}
	return services.FiltroListagem{
		Status: status, Categoria: categoria, Busca: busca, IDAcademia: idAcademia,
		Pagina: c.QueryInt("pagina", 1), Limite: c.QueryInt("limite", 20),
	}, true
}

func (ctrl *DesafioController) listar(c *fiber.Ctx, meus bool) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	f, ok := filtroDaQuery(c)
	if !ok {
		return nil
	}
	var (
		lista *models.ListaDesafiosResponse
		err   error
	)
	if meus {
		lista, err = ctrl.service.ListarMeus(userID, f)
	} else {
		lista, err = ctrl.service.Listar(userID, f)
	}
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Desafios listados", lista)
}

// ListarDesafios godoc
// @Summary     Feed de desafios
// @Description Por padrão só desafios abertos com data_evento futura, do mais próximo ao mais distante. Com status diferente de aberto, só os desafios desse status em que o usuário está envolvido. Paginação e filtros no SQL.
// @Tags        desafios
// @Produce     json
// @Param       status      query string false "aberto|em_andamento|encerrado|cancelado"
// @Param       categoria   query string false "geral|corrida|musculacao|crossfit|funcional|ciclismo|natacao|calistenia|luta|outros"
// @Param       q           query string false "Texto livre (título, local, nome da academia)"
// @Param       id_academia query int    false "Só desafios desta academia"
// @Param       pagina      query int    false "Página (1..)"
// @Param       limite      query int    false "Itens por página (máx. 100)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios [get]
func (ctrl *DesafioController) ListarDesafios(c *fiber.Ctx) error { return ctrl.listar(c, false) }

// ListarMeus godoc
// @Summary     Meus desafios
// @Description Desafios em que o usuário do token participa (atleta, com seu_codigo_ingresso e status), é a academia dona ou o instrutor designado (qualquer status)
// @Tags        desafios
// @Produce     json
// @Param       status    query string false "aberto|em_andamento|encerrado|cancelado"
// @Param       categoria query string false "Categoria"
// @Param       q         query string false "Texto livre"
// @Param       pagina    query int    false "Página (1..)"
// @Param       limite    query int    false "Itens por página (máx. 100)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/meus [get]
func (ctrl *DesafioController) ListarMeus(c *fiber.Ctx) error { return ctrl.listar(c, true) }

// Categorias godoc
// @Summary     Categorias de desafio
// @Description Lista as categorias aceitas (chips do feed)
// @Tags        desafios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/categorias [get]
func (ctrl *DesafioController) Categorias(c *fiber.Ctx) error {
	return utils.Success(c, fiber.StatusOK, "Categorias", models.CategoriasDesafio)
}

// BuscarDesafio godoc
// @Summary     Detalhe do desafio
// @Description Desafio completo com participantes, flags pode_* e, se encerrado, o pódio. Desafios não abertos só para participantes, academia dona e instrutor designado.
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id} [get]
func (ctrl *DesafioController) BuscarDesafio(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	d, err := ctrl.service.Detalhe(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Desafio encontrado", d)
}

// CriarDesafio godoc
// @Summary     Criar desafio (perfil academia)
// @Description Só a academia cria. A academia dona é a do token; id_instrutor precisa estar vinculado a ela. A academia não paga entrada nem vira participante.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       body body models.CriarDesafioRequest true "Dados do desafio"
// @Success     201 {object} utils.APIResponse
// @Router      /api/desafios [post]
func (ctrl *DesafioController) CriarDesafio(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.CriarDesafioRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	d, err := ctrl.service.Criar(userID, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Desafio criado com sucesso", d)
}

// EditarDesafio godoc
// @Summary     Editar desafio aberto (academia dona)
// @Description Campos opcionais. Não altera a entrada se já há inscritos; vagas não podem ficar abaixo dos inscritos. Trocar id_instrutor notifica o antigo e o novo.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                         true "ID do desafio"
// @Param       body body models.EditarDesafioRequest true "Campos a alterar"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id} [patch]
func (ctrl *DesafioController) EditarDesafio(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	if err := ctrl.service.AutorizarEdicao(userID, id); err != nil {
		return responderErro(c, err)
	}
	var req models.EditarDesafioRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	d, err := ctrl.service.Editar(userID, id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Desafio atualizado", d)
}

// acao cria um handler para as ações POST /desafios/:id/<acao> sem corpo.
func (ctrl *DesafioController) acao(msg string, fn func(userID, id uint) (*models.DesafioResponse, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := exigirUsuario(c)
		if !ok {
			return nil
		}
		id, ok := idDaRota(c, "id")
		if !ok {
			return nil
		}
		d, err := fn(userID, id)
		if err != nil {
			return responderErro(c, err)
		}
		return utils.Success(c, fiber.StatusOK, msg, d)
	}
}

// Entrar godoc
// @Summary Inscrever-se no desafio (só atleta; debita a entrada e gera o ingresso)
// @Tags    desafios
// @Router  /api/desafios/{id}/entrar [post]
func (ctrl *DesafioController) Entrar() fiber.Handler {
	return ctrl.acao("Você entrou no desafio", ctrl.service.Entrar)
}

// Sair godoc
// @Summary Sair do desafio aberto (só atleta; reembolsa a entrada)
// @Tags    desafios
// @Router  /api/desafios/{id}/sair [post]
func (ctrl *DesafioController) Sair() fiber.Handler {
	return ctrl.acao("Você saiu do desafio e a entrada foi reembolsada", ctrl.service.Sair)
}

// Cancelar godoc
// @Summary Cancelar desafio aberto ou em andamento (só academia dona; reembolsa todos)
// @Tags    desafios
// @Router  /api/desafios/{id}/cancelar [post]
func (ctrl *DesafioController) Cancelar() fiber.Handler {
	return ctrl.acao("Desafio cancelado e participantes reembolsados", ctrl.service.Cancelar)
}

// Iniciar godoc
// @Summary Iniciar desafio (instrutor designado ou academia dona; fecha as inscrições)
// @Tags    desafios
// @Router  /api/desafios/{id}/iniciar [post]
func (ctrl *DesafioController) Iniciar() fiber.Handler {
	return ctrl.acao("Desafio iniciado", ctrl.service.Iniciar)
}

// MarcarPresenca godoc
// @Summary     Marcar presença (instrutor designado ou academia dona)
// @Description Corpo: {"codigo_ingresso": "<QR>"} OU {"id_usuario": N}. Idempotente. Em aberto só no dia do evento; em andamento a qualquer momento.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                    true "ID do desafio"
// @Param       body body models.PresencaRequest true "Ingresso ou usuário"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/presenca [post]
func (ctrl *DesafioController) MarcarPresenca(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.PresencaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	r, err := ctrl.service.MarcarPresenca(userID, id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	msg := "Presença registrada"
	if r.JaPresente {
		msg = "Presença já estava registrada"
	}
	return utils.Success(c, fiber.StatusOK, msg, r)
}

// ListarPresentes godoc
// @Summary     Atletas presentes (instrutor designado ou academia dona)
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/presentes [get]
func (ctrl *DesafioController) ListarPresentes(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	lista, err := ctrl.service.ListarPresentes(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Presentes listados", lista)
}

// LancarPodio godoc
// @Summary     Lançar pódio (instrutor designado ou academia dona)
// @Description Exatamente min(posicoes_podio, presentes) posições, só presentes, sem repetir, marca obrigatória. Divide o pote, credita os prêmios, aplica pontos e encerra o desafio. Devolve o desafio encerrado.
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                       true "ID do desafio"
// @Param       body body models.LancarPodioRequest true "Posições (índice 0 = 1º lugar)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/podio [post]
func (ctrl *DesafioController) LancarPodio(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.LancarPodioRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	d, err := ctrl.service.LancarPodio(userID, id, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Pódio lançado e desafio encerrado", d)
}

// ConsultarPodio godoc
// @Summary     Pódio do desafio
// @Description Lista vazia enquanto o desafio não estiver encerrado
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/podio [get]
func (ctrl *DesafioController) ConsultarPodio(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	podio, err := ctrl.service.Podio(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Pódio consultado", podio)
}

// AvaliarDesafio godoc
// @Summary     Avaliar desafio
// @Description Nota de 1 a 5, só de quem esteve presente num desafio encerrado; uma por atleta
// @Tags        desafios
// @Accept      json
// @Produce     json
// @Param       id   path int                         true "ID do desafio"
// @Param       body body models.AvaliarDesafioRequest true "Nota"
// @Success     201 {object} utils.APIResponse
// @Router      /api/desafios/{id}/avaliar [post]
func (ctrl *DesafioController) AvaliarDesafio(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.AvaliarDesafioRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	a, err := ctrl.service.Avaliar(userID, id, req.Nota)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Avaliação registrada", a)
}

// Convidar godoc
// @Summary Convidar um amigo para o desafio aberto (atleta)
// @Tags    desafios
// @Accept  json
// @Param   body body models.ConvidarRequest true "Amigo convidado"
// @Router  /api/desafios/{id}/convidar [post]
func (ctrl *DesafioController) Convidar(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.ConvidarRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	if err := ctrl.service.Convidar(userID, id, req.IDAmigo); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Convite enviado", nil)
}
