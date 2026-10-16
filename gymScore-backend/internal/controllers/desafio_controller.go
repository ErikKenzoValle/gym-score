package controllers

import (
	"io"
	"strconv"

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
	case "", models.StatusAberto, models.StatusEmAndamento, models.StatusEmVotacao,
		models.StatusEncerrado, models.StatusCancelado:
		return st, true
	}
	return "", false
}

func (ctrl *DesafioController) listar(c *fiber.Ctx, meus bool) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	status, valido := statusValido(c.Query("status"))
	if !valido {
		return utils.ValidationError(c, "status inválido")
	}
	pagina, limite := c.QueryInt("pagina", 1), c.QueryInt("limite", 20)
	var (
		lista *models.ListaDesafiosResponse
		err   error
	)
	if meus {
		lista, err = ctrl.service.ListarMeus(userID, status, pagina, limite)
	} else {
		lista, err = ctrl.service.Listar(userID, status, pagina, limite)
	}
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Desafios listados", lista)
}

// ListarDesafios godoc
// @Summary     Listar desafios
// @Description Desafios abertos com vaga + os do próprio usuário. Paginação e filtro no SQL.
// @Tags        desafios
// @Produce     json
// @Param       status query string false "aberto|em_andamento|em_votacao|encerrado|cancelado"
// @Param       pagina query int false "Página (1..)"
// @Param       limite query int false "Itens por página (máx. 100)"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios [get]
func (ctrl *DesafioController) ListarDesafios(c *fiber.Ctx) error { return ctrl.listar(c, false) }

// ListarMeus godoc
// @Summary     Listar meus desafios
// @Description Desafios que o usuário do token criou ou participa (qualquer status)
// @Tags        desafios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/meus [get]
func (ctrl *DesafioController) ListarMeus(c *fiber.Ctx) error { return ctrl.listar(c, true) }

// BuscarDesafio godoc
// @Summary     Detalhe do desafio
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
// @Summary     Criar desafio
// @Description O criador é o usuário do token e paga a entrada ao criar (débito atômico).
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
// @Summary Entrar no desafio (debita a entrada)
// @Tags    desafios
// @Router  /api/desafios/{id}/entrar [post]
func (ctrl *DesafioController) Entrar() fiber.Handler {
	return ctrl.acao("Você entrou no desafio", ctrl.service.Entrar)
}

// Sair godoc
// @Summary Sair do desafio aberto (reembolsa a entrada)
// @Tags    desafios
// @Router  /api/desafios/{id}/sair [post]
func (ctrl *DesafioController) Sair() fiber.Handler {
	return ctrl.acao("Você saiu do desafio e a entrada foi reembolsada", ctrl.service.Sair)
}

// Cancelar godoc
// @Summary Cancelar desafio aberto (só criador; reembolsa todos)
// @Tags    desafios
// @Router  /api/desafios/{id}/cancelar [post]
func (ctrl *DesafioController) Cancelar() fiber.Handler {
	return ctrl.acao("Desafio cancelado e participantes reembolsados", ctrl.service.Cancelar)
}

// Iniciar godoc
// @Summary Iniciar desafio (só criador, mínimo 2 participantes)
// @Tags    desafios
// @Router  /api/desafios/{id}/iniciar [post]
func (ctrl *DesafioController) Iniciar() fiber.Handler {
	return ctrl.acao("Desafio iniciado", ctrl.service.Iniciar)
}

// AbrirVotacao godoc
// @Summary Abrir votação (só criador)
// @Tags    desafios
// @Router  /api/desafios/{id}/abrir-votacao [post]
func (ctrl *DesafioController) AbrirVotacao() fiber.Handler {
	return ctrl.acao("Votação aberta", ctrl.service.AbrirVotacao)
}

// Apurar godoc
// @Summary Apurar votação antecipadamente (só criador)
// @Tags    desafios
// @Router  /api/desafios/{id}/apurar [post]
func (ctrl *DesafioController) Apurar() fiber.Handler {
	return ctrl.acao("Desafio apurado", ctrl.service.Apurar)
}

// Votar godoc
// @Summary Votar em outro participante
// @Tags    desafios
// @Accept  json
// @Param   body body models.VotarRequest true "Participante votado"
// @Router  /api/desafios/{id}/votar [post]
func (ctrl *DesafioController) Votar(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	var req models.VotarRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	d, err := ctrl.service.Votar(userID, id, req.IDVotado)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Voto registrado", d)
}

// Convidar godoc
// @Summary Convidar um amigo para o desafio aberto (só criador)
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

// Checkin godoc
// @Summary     Registrar check-in (prova de treino)
// @Description multipart/form-data: texto?, latitude?, longitude?, foto? (JPG/PNG/WEBP até 3 MB). Só participante, só em_andamento, 1 por dia.
// @Tags        desafios
// @Accept      mpfd
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Param       texto formData string false "Descrição (até 500 caracteres)"
// @Param       latitude formData number false "Latitude"
// @Param       longitude formData number false "Longitude"
// @Param       foto formData file false "Foto"
// @Success     201 {object} utils.APIResponse
// @Router      /api/desafios/{id}/checkins [post]
func (ctrl *DesafioController) Checkin(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	in := services.CheckinInput{Texto: c.FormValue("texto")}
	for campo, dst := range map[string]**float64{"latitude": &in.Latitude, "longitude": &in.Longitude} {
		if v := c.FormValue(campo); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return utils.ValidationError(c, "Campo "+campo+" inválido")
			}
			*dst = &f
		}
	}
	if fh, err := c.FormFile("foto"); err == nil {
		if fh.Size > services.TamanhoMaxFoto {
			return utils.Error(c, fiber.StatusRequestEntityTooLarge, "A foto deve ter no máximo 3 MB")
		}
		f, err := fh.Open()
		if err != nil {
			return utils.ValidationError(c, "Não foi possível ler a foto")
		}
		defer f.Close()
		// lê no máximo 3 MB + 1 byte (não confia no tamanho declarado)
		dados, err := io.ReadAll(io.LimitReader(f, services.TamanhoMaxFoto+1))
		if err != nil {
			return utils.ValidationError(c, "Não foi possível ler a foto")
		}
		in.Foto = dados
	}
	r, err := ctrl.service.Checkin(userID, id, in)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Check-in registrado", r)
}

// ListarCheckins godoc
// @Summary     Listar check-ins do desafio
// @Tags        desafios
// @Produce     json
// @Param       id path int true "ID do desafio"
// @Success     200 {object} utils.APIResponse
// @Router      /api/desafios/{id}/checkins [get]
func (ctrl *DesafioController) ListarCheckins(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	lista, err := ctrl.service.ListarCheckins(userID, id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Check-ins listados", lista)
}
