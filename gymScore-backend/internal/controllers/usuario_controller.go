package controllers

import (
	"strconv"
	"strings"
	"time"

	"gymScore-backend/internal/config"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// UsuarioController gerencia as requisições HTTP relacionadas a usuários e autenticação
type UsuarioController struct {
	service      services.UsuarioService
	senhaService services.SenhaService
	cfg          *config.Config
	desafioRepo  *repositories.DesafioRepository
}

// NovoUsuarioController cria uma nova instância do controller de usuários
func NovoUsuarioController(service services.UsuarioService, senhaService services.SenhaService, cfg *config.Config, desafioRepo *repositories.DesafioRepository) *UsuarioController {
	return &UsuarioController{service: service, senhaService: senhaService, cfg: cfg, desafioRepo: desafioRepo}
}

// CriarUsuario godoc
// @Summary     Criar novo usuário
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.CriarUsuarioRequest true "Dados do usuário"
// @Success     201 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/usuarios [post]
func (ctrl *UsuarioController) CriarUsuario(c *fiber.Ctx) error {
	var req models.CriarUsuarioRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	usuario, err := ctrl.service.CriarUsuario(&req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, "Usuário criado com sucesso", usuario)
}

// cookieSessao monta o cookie JWT: HttpOnly, Secure em produção, validade igual à do token.
func (ctrl *UsuarioController) cookieSessao(valor string, expira time.Time) *fiber.Cookie {
	return &fiber.Cookie{
		Name:     "jwt",
		Value:    valor,
		Expires:  expira,
		HTTPOnly: true,
		Secure:   ctrl.cfg.IsProduction(),
		SameSite: "Lax",
		Path:     "/",
	}
}

// Login godoc
// @Summary     Autenticar usuário
// @Description Valida credenciais e retorna token JWT (24h) via JSON e Cookie HttpOnly
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.LoginRequest true "Credenciais de acesso"
// @Success     200 {object} utils.APIResponse
// @Failure     401 {object} utils.APIResponse
// @Router      /api/login [post]
func (ctrl *UsuarioController) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	resp, err := ctrl.service.Login(&req, ctrl.cfg.JWTSecret)
	if err != nil {
		return responderErro(c, err)
	}
	c.Cookie(ctrl.cookieSessao(resp.Token, time.Now().Add(utils.DuracaoToken)))
	return utils.Success(c, fiber.StatusOK, "Usuário autenticado com sucesso", resp)
}

// Logout godoc
// @Summary     Encerrar sessão
// @Description Revoga o token atual (passa a dar 401) e limpa o cookie de sessão
// @Tags        usuarios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/logout [post]
func (ctrl *UsuarioController) Logout(c *fiber.Ctx) error {
	// Revoga o token apresentado (se válido): ele passa a dar 401 em qualquer rota protegida.
	if token := middlewares.ExtrairToken(c); token != "" {
		if claims, err := utils.ValidarToken(token, ctrl.cfg.JWTSecret); err == nil {
			if err := ctrl.service.Logout(claims.UserID, claims.Versao); err != nil {
				return responderErro(c, err)
			}
		}
	}
	c.Cookie(ctrl.cookieSessao("", time.Now().Add(-time.Hour)))
	return utils.Success(c, fiber.StatusOK, "Sessão encerrada", nil)
}

// Me godoc
// @Summary     Perfil do usuário logado
// @Description Perfil completo (inclui e-mail, CPF e saldo) — somente do dono do token
// @Tags        usuarios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/me [get]
func (ctrl *UsuarioController) Me(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	perfil, err := ctrl.service.Perfil(userID)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Perfil encontrado", perfil)
}

// BuscarUsuario godoc
// @Summary     Dados públicos de um usuário
// @Description Retorna somente id, nome e sobrenome
// @Tags        usuarios
// @Produce     json
// @Param       id path int true "ID do usuário"
// @Success     200 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Router      /api/usuarios/{id} [get]
func (ctrl *UsuarioController) BuscarUsuario(c *fiber.Ctx) error {
	id, ok := idDaRota(c, "id")
	if !ok {
		return nil
	}
	pub, err := ctrl.service.Publico(id)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Usuário encontrado", pub)
}

// BuscarUsuarios godoc
// @Summary     Buscar usuários por nome
// @Description Mínimo 2 caracteres, máximo 20 resultados, apenas id/nome/sobrenome
// @Tags        usuarios
// @Produce     json
// @Param       q query string true "Termo de busca"
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/busca [get]
func (ctrl *UsuarioController) BuscarUsuarios(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	lista, err := ctrl.service.Buscar(userID, c.Query("q"))
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Busca concluída", lista)
}

// AlterarSenha godoc
// @Summary     Alterar senha
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.AlterarSenhaRequest true "Senhas"
// @Description Revoga os tokens antigos e devolve `data.token` (novo) — o cliente deve substituí-lo
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/senha [patch]
func (ctrl *UsuarioController) AlterarSenha(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.AlterarSenhaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	token, err := ctrl.service.AlterarSenha(userID, versaoDoToken(c), &req, ctrl.cfg.JWTSecret)
	if err != nil {
		return responderErro(c, err)
	}
	// Os tokens antigos foram revogados: devolve um novo para manter ESTA sessão logada
	c.Cookie(ctrl.cookieSessao(token, time.Now().Add(utils.DuracaoToken)))
	return utils.Success(c, fiber.StatusOK, "Senha alterada com sucesso", fiber.Map{"token": token})
}

// AtualizarPerfil godoc
// @Summary     Atualizar perfil
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.AtualizarPerfilRequest true "Dados do perfil"
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/perfil [patch]
func (ctrl *UsuarioController) AtualizarPerfil(c *fiber.Ctx) error {
	userID, ok := exigirUsuario(c)
	if !ok {
		return nil
	}
	var req models.AtualizarPerfilRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	usuario, err := ctrl.service.AtualizarPerfil(userID, &req)
	if err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Perfil atualizado com sucesso", usuario)
}

// EsqueciSenha godoc
// @Summary     Solicitar link de redefinição de senha
// @Description Sempre responde 200 genérico (não revela se o e-mail existe)
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body models.EsqueciSenhaRequest true "E-mail"
// @Success     200 {object} utils.APIResponse
// @Router      /api/auth/esqueci-senha [post]
func (ctrl *UsuarioController) EsqueciSenha(c *fiber.Ctx) error {
	var req models.EsqueciSenhaRequest
	if err := c.BodyParser(&req); err == nil && strings.TrimSpace(req.Email) != "" {
		ctrl.senhaService.EsqueciSenha(req.Email)
	}
	return utils.Success(c, fiber.StatusOK, "Se o e-mail estiver cadastrado, você receberá um link para redefinir a senha", nil)
}

// RedefinirSenha godoc
// @Summary     Redefinir senha com o token recebido por e-mail
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body models.RedefinirSenhaRequest true "Token e nova senha"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/auth/redefinir-senha [post]
func (ctrl *UsuarioController) RedefinirSenha(c *fiber.Ctx) error {
	var req models.RedefinirSenhaRequest
	if !lerCorpo(c, &req) {
		return nil
	}
	if err := ctrl.senhaService.RedefinirSenha(&req); err != nil {
		return responderErro(c, err)
	}
	return utils.Success(c, fiber.StatusOK, "Senha redefinida com sucesso", nil)
}

// BuscarPublico godoc
// @Summary     Perfil público
// @Description Retorna dados públicos de um usuário pelo ID (sem CPF, email, saldo)
// @Tags        usuarios
// @Produce     json
// @Param       id path int true "ID do usuário"
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/{id}/publico [get]
func (ctrl *UsuarioController) BuscarPublico(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	resp, err := ctrl.service.BuscarPublico(uint(id))
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	if resp == nil {
		return utils.Error(c, fiber.StatusNotFound, "Usuário não encontrado")
	}
	return utils.Success(c, fiber.StatusOK, "Perfil encontrado", resp)
}

// HistoricoDesafios godoc
// @Summary     Histórico de desafios
// @Description Retorna os desafios encerrados de outro usuário (perfil público)
// @Tags        usuarios
// @Produce     json
// @Param       id path int true "ID do usuário"
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/{id}/historico [get]
func (ctrl *UsuarioController) HistoricoDesafios(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}
	desafios, err := ctrl.service.HistoricoDesafios(uint(id), ctrl.desafioRepo)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Histórico encontrado", desafios)
}

// BuscarPorTermo godoc
// @Summary     Buscar usuários
// @Description Busca usuários por nome ou username (tempo real)
// @Tags        usuarios
// @Produce     json
// @Param       q query string true "Texto de busca"
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/buscar [get]
func (ctrl *UsuarioController) BuscarPorTermo(c *fiber.Ctx) error {
	q := c.Query("q")
	if len(q) < 2 {
		return utils.ValidationError(c, "Parâmetro 'q' deve ter pelo menos 2 caracteres")
	}
	resp, err := ctrl.service.BuscarPorTermo(q)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	if resp == nil {
		resp = []models.UsuarioPublicoResponse{}
	}
	return utils.Success(c, fiber.StatusOK, "Busca realizada", resp)
}

// Ingressos godoc
// @Summary     Listar ingressos do usuário
// @Description Retorna todos os desafios em que o usuário logado é participante, com código de ingresso para check-in
// @Tags        usuarios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Failure     500 {object} utils.APIResponse
// @Router      /api/usuarios/ingressos [get]
func (ctrl *UsuarioController) Ingressos(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	lista, err := ctrl.service.Ingressos(userID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao buscar ingressos: "+err.Error())
	}
	if lista == nil {
		lista = []models.IngressoResponse{}
	}
	return utils.Success(c, fiber.StatusOK, "Ingressos encontrados", lista)
}

// AtualizarUltimaVez godoc
// @Summary     Heartbeat de presença
// @Description Atualiza o timestamp de última vez online do usuário autenticado
// @Tags        usuarios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios/ultima-vez [patch]
func (ctrl *UsuarioController) AtualizarUltimaVez(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}
	if err := ctrl.service.AtualizarUltimaVez(userID); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, "Presença atualizada", nil)
}
