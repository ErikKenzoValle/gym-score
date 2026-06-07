package controllers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gynScore-backend/internal/config"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
	"gynScore-backend/pkg/utils"
)

// UsuarioController gerencia as requisições HTTP relacionadas a usuários
type UsuarioController struct {
	service services.UsuarioService
	cfg     *config.Config
}

// NovoUsuarioController cria uma nova instância do controller de usuários
func NovoUsuarioController(service services.UsuarioService, cfg *config.Config) *UsuarioController {
	return &UsuarioController{service: service, cfg: cfg}
}

// CriarUsuario godoc
// @Summary     Criar novo usuário
// @Description Registra um novo usuário no sistema após validação de e-mail e dados
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.CriarUsuarioRequest true "Dados do usuário"
// @Success     201 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Failure     500 {object} utils.APIResponse
// @Router      /api/usuarios [post]
func (ctrl *UsuarioController) CriarUsuario(c *fiber.Ctx) error {
	var req models.CriarUsuarioRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	if req.Nome == "" || req.Sobrenome == "" || req.Email == "" || req.Senha == "" || req.DataNascimento == "" || req.Genero == "" {
		return utils.ValidationError(c, "Todos os campos são obrigatórios: nome, sobrenome, email, senha, data_nascimento, genero")
	}

	usuario, err := ctrl.service.CriarUsuario(&req)
	if err != nil {
		if err.Error() == "e-mail inválido" || err.Error() == "e-mail já cadastrado" {
			return utils.ValidationError(c, err.Error())
		}
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao criar usuário: "+err.Error())
	}

	return utils.Success(c, fiber.StatusCreated, "Usuário criado com sucesso", usuario)
}

// Login godoc
// @Summary     Autenticar usuário
// @Description Valida credenciais e retorna token JWT via JSON e Cookie
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.LoginRequest true "Credenciais de acesso"
// @Success     200 {object} utils.APIResponse
// @Failure     401 {object} utils.APIResponse
// @Router      /api/login [post]
func (ctrl *UsuarioController) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	if req.Email == "" || req.Senha == "" {
		return utils.ValidationError(c, "E-mail e senha são obrigatórios")
	}

	resp, err := ctrl.service.Login(&req, ctrl.cfg.JWTSecret)
	if err != nil {
		return utils.Error(c, fiber.StatusUnauthorized, err.Error())
	}

	// Configurar Cookie com o Token JWT
	c.Cookie(&fiber.Cookie{
		Name:     "jwt",
		Value:    resp.Token,
		Expires:  time.Now().Add(time.Hour * 72), // 3 dias (mesmo tempo do token)
		HTTPOnly: true,
		Secure:   false, // Em produção deve ser true se usar HTTPS
		SameSite: "Lax",
		Path:     "/",
	})

	return utils.Success(c, fiber.StatusOK, "Usuário autenticado com sucesso", resp)
}

// BuscarUsuario godoc
// @Summary     Buscar perfil do usuário
// @Description Retorna os dados públicos de um usuário pelo ID
// @Tags        usuarios
// @Produce     json
// @Param       id path int true "ID do usuário"
// @Success     200 {object} utils.APIResponse
// @Failure     404 {object} utils.APIResponse
// @Router      /api/usuarios/{id} [get]
func (ctrl *UsuarioController) BuscarUsuario(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		return utils.ValidationError(c, "ID inválido")
	}

	usuario, err := ctrl.service.BuscarPorID(uint(id))
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao buscar usuário: "+err.Error())
	}
	if usuario == nil {
		return utils.Error(c, fiber.StatusNotFound, "Usuário não encontrado")
	}

	return utils.Success(c, fiber.StatusOK, "Usuário encontrado", usuario)
}

// AlterarSenha godoc
// @Summary     Alterar senha
// @Description Altera a senha do usuário autenticado após validar a senha atual
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.AlterarSenhaRequest true "Senhas"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/usuarios/senha [patch]
func (ctrl *UsuarioController) AlterarSenha(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}

	var req models.AlterarSenhaRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.SenhaAtual == "" || req.NovaSenha == "" {
		return utils.ValidationError(c, "Campos obrigatórios: senha_atual, nova_senha")
	}

	if err := ctrl.service.AlterarSenha(userID, &req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Senha alterada com sucesso", nil)
}

// RecuperarSenha godoc
// @Summary     Recuperar senha
// @Description Redefine a senha validando e-mail + CPF sem necessidade de e-mail de reset
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.RecuperarSenhaRequest true "Dados de recuperação"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/usuarios/recuperar-senha [post]
func (ctrl *UsuarioController) RecuperarSenha(c *fiber.Ctx) error {
	var req models.RecuperarSenhaRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}
	if req.Email == "" || req.CPF == "" || req.NovaSenha == "" {
		return utils.ValidationError(c, "Campos obrigatórios: email, cpf, nova_senha")
	}

	if err := ctrl.service.RecuperarSenha(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Senha redefinida com sucesso", nil)
}

// AtualizarPerfil godoc
// @Summary     Atualizar perfil
// @Description Atualiza nome, sobrenome e gênero do usuário autenticado
// @Tags        usuarios
// @Accept      json
// @Produce     json
// @Param       body body models.AtualizarPerfilRequest true "Dados do perfil"
// @Success     200 {object} utils.APIResponse
// @Failure     400 {object} utils.APIResponse
// @Router      /api/usuarios/perfil [patch]
func (ctrl *UsuarioController) AtualizarPerfil(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uint)
	if !ok || userID == 0 {
		return utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
	}

	var req models.AtualizarPerfilRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ValidationError(c, "Corpo da requisição inválido")
	}

	usuario, err := ctrl.service.AtualizarPerfil(userID, &req)
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Perfil atualizado com sucesso", usuario)
}

// ListarUsuarios godoc
// @Summary     Listar usuários
// @Description Retorna todos os usuários cadastrados
// @Tags        usuarios
// @Produce     json
// @Success     200 {object} utils.APIResponse
// @Router      /api/usuarios [get]
func (ctrl *UsuarioController) ListarUsuarios(c *fiber.Ctx) error {
	usuarios, err := ctrl.service.Listar()
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "Erro ao listar usuários: "+err.Error())
	}

	return utils.Success(c, fiber.StatusOK, "Usuários listados com sucesso", usuarios)
}
