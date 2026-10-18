package controllers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"
)

// AcademiaController gerencia as requisições HTTP relacionadas à gestão de academias
type AcademiaController struct {
	desafioRepo   *repositories.DesafioRepository
	usuarioRepo   repositories.UsuarioRepository
	podioRepo     repositories.PodioRepository
	avaliacaoRepo repositories.AvaliacaoRepository
	academiaSvc   services.AcademiaService
}

// NovoAcademiaController cria uma nova instância do controller de academia
func NovoAcademiaController(
	desafioRepo *repositories.DesafioRepository,
	usuarioRepo repositories.UsuarioRepository,
	podioRepo repositories.PodioRepository,
	avaliacaoRepo repositories.AvaliacaoRepository,
	academiaSvc services.AcademiaService,
) *AcademiaController {
	return &AcademiaController{desafioRepo, usuarioRepo, podioRepo, avaliacaoRepo, academiaSvc}
}

// CriarAcademia godoc
// @Summary     Cadastrar academia
// @Description Cria uma nova conta de perfil "academia" (usuário pessoa jurídica, sem CPF) e o registro de academia vinculado
// @Tags        academia
// @Accept      json
// @Produce     json
// @Param       body body models.CriarAcademiaRequest true "Dados da academia"
// @Success     201 {object} utils.APIResponse
// @Router      /api/academias [post]
func (ctrl *AcademiaController) CriarAcademia(c *fiber.Ctx) error {
	var req models.CriarAcademiaRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.Nome == "" || req.Email == "" || req.Senha == "" || req.RazaoSocial == "" || req.NomeFantasia == "" || req.CNPJ == "" {
		return utils.ValidationError(c, "nome, email, senha, razao_social, nome_fantasia e cnpj são obrigatórios")
	}
	academia, err := ctrl.academiaSvc.Criar(&req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	return utils.Success(c, fiber.StatusCreated, "Academia cadastrada com sucesso", academia)
}

// Dashboard godoc
// @Summary     Dashboard da academia
// @Description Retorna indicadores da academia: total de desafios, participantes, avaliação média
// @Tags        academia
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/academia/dashboard [get]
func (ctrl *AcademiaController) Dashboard(c *fiber.Ctx) error {
	desafios, _, _ := ctrl.desafioRepo.Listar(repositories.FiltroDesafios{Todos: true, Pagina: 1, Limite: 10000})
	totalDesafios := len(desafios)
	totalEncerrados := 0
	for _, d := range desafios {
		if d.Status == models.StatusEncerrado {
			totalEncerrados++
		}
	}

	mediaAval, _ := ctrl.avaliacaoRepo.MediaGlobal()

	return utils.Success(c, fiber.StatusOK, "Dashboard carregado", fiber.Map{
		"total_desafios":      totalDesafios,
		"desafios_encerrados": totalEncerrados,
		"avaliacao_media":     mediaAval,
	})
}

// ListarPresentes godoc
// @Summary     Participantes presentes
// @Description Lista os participantes marcados como presentes no desafio
// @Tags        academia
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/presentes [get]
func (ctrl *AcademiaController) ListarPresentes(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	presentes, err := ctrl.desafioRepo.ListarPresentes(uint(id))
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Presentes listados", presentes)
}

// MarcarPresenca godoc
// @Summary     Marcar presença
// @Description Marca um participante como presente no desafio (requer perfil academia)
// @Tags        academia
// @Accept      json
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/presenca [post]
func (ctrl *AcademiaController) MarcarPresenca(c *fiber.Ctx) error {
	idDesafio, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	var body struct {
		IDUsuario uint `json:"id_usuario"`
	}
	if err := c.BodyParser(&body); err != nil || body.IDUsuario == 0 {
		return utils.ValidationError(c, "id_usuario é obrigatório")
	}
	if err := ctrl.desafioRepo.MarcarPresente(uint(idDesafio), body.IDUsuario); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Presença registrada", nil)
}

// LancarPodio godoc
// @Summary     Lançar pódio
// @Description Registra o Top 5 de um desafio (requer perfil academia). Credita pontos de Elo.
// @Tags        academia
// @Accept      json
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Param       body body models.LancarPodioRequest true "Top 5"
// @Success     201 {object} utils.APIResponse
// @Router      /api/desafios/{id}/podio [post]
func (ctrl *AcademiaController) LancarPodio(c *fiber.Ctx) error {
	idDesafio, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	var req models.LancarPodioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo inválido")
	}
	if len(req.Posicoes) != 5 {
		return utils.ValidationError(c, "O pódio deve conter exatamente 5 posições")
	}

	jaExiste, _ := ctrl.podioRepo.ExistePodio(uint(idDesafio))
	if jaExiste {
		return utils.Error(c, fiber.StatusConflict, "Pódio já foi lançado para este desafio")
	}

	pontosPodio := []int{15, 10, 5, 3, 1}
	for i, pos := range req.Posicoes {
		if pos.IDParticipante == 0 || pos.Marca == "" {
			return utils.ValidationError(c, "Todos os participantes e marcas são obrigatórios")
		}
		entry := &models.PodioDesafio{
			IDDesafio:      uint(idDesafio),
			Posicao:        i + 1,
			IDParticipante: pos.IDParticipante,
			Marca:          pos.Marca,
		}
		if err := ctrl.podioRepo.Criar(entry); err != nil {
			return utils.Error(c, fiber.StatusInternalServerError, "Erro ao salvar posição "+strconv.Itoa(i+1)+": "+err.Error())
		}
		// Creditar pontos de Elo por pódio
		if usuario, err := ctrl.usuarioRepo.BuscarPorID(pos.IDParticipante); err == nil && usuario != nil {
			usuario.Pontos += pontosPodio[i]
			usuario.Elo = calcularFaixaElo(usuario.Pontos)
			_ = ctrl.usuarioRepo.Atualizar(usuario)
		}
	}

	podio, _ := ctrl.podioRepo.ListarPorDesafio(uint(idDesafio))
	return utils.Success(c, fiber.StatusCreated, "Pódio lançado com sucesso", podio)
}

// ConsultarPodio godoc
// @Summary     Consultar pódio
// @Description Retorna o pódio já lançado de um desafio
// @Tags        academia
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/podio [get]
func (ctrl *AcademiaController) ConsultarPodio(c *fiber.Ctx) error {
	idDesafio, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	podio, err := ctrl.podioRepo.ListarPorDesafio(uint(idDesafio))
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Pódio consultado", podio)
}

// AvaliarDesafio godoc
// @Summary     Avaliar desafio
// @Description Registra nota de 1 a 5 para um desafio (apenas participantes)
// @Tags        academia
// @Accept      json
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Param       body body models.AvaliarDesafioRequest true "Nota"
// @Success     201 {object} utils.APIResponse
// @Router      /api/desafios/{id}/avaliar [post]
func (ctrl *AcademiaController) AvaliarDesafio(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	idDesafio, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	var req models.AvaliarDesafioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo inválido")
	}
	if req.Nota < 1 || req.Nota > 5 {
		return utils.ValidationError(c, "Nota deve estar entre 1 e 5")
	}
	avaliacao := &models.AvaliacaoDesafio{
		IDDesafio: uint(idDesafio),
		IDUsuario: userID,
		Nota:      req.Nota,
	}
	if err := ctrl.avaliacaoRepo.Criar(avaliacao); err != nil {
		return utils.Error(c, fiber.StatusConflict, "Você já avaliou este desafio")
	}
	return utils.Success(c, fiber.StatusCreated, "Avaliação registrada", avaliacao)
}

// calcularFaixaElo delega para utils.CalcularElo para evitar duplicação
func calcularFaixaElo(pontos int) string {
	return utils.CalcularElo(pontos)
}
