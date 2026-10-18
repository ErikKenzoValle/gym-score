package controllers

import (
	"log"
	"strconv"

	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// usuarioAutenticado retorna o ID do usuário do contexto JWT (0 se ausente).
// É a ÚNICA fonte de identidade: nenhum DTO aceita id de quem age.
func usuarioAutenticado(c *fiber.Ctx) uint {
	if uid := c.Locals("user_id"); uid != nil {
		if id, ok := uid.(uint); ok {
			return id
		}
	}
	return 0
}

// exigirUsuario devolve o ID do JWT ou responde 401.
func exigirUsuario(c *fiber.Ctx) (uint, bool) {
	id := usuarioAutenticado(c)
	if id == 0 {
		_ = utils.Error(c, fiber.StatusUnauthorized, "Usuário não autenticado")
		return 0, false
	}
	return id, true
}

// lerCorpo faz o parse do JSON e aplica as tags `validate:`. Em caso de falha já
// responde 400 e devolve false.
func lerCorpo(c *fiber.Ctx, dst any) bool {
	if err := c.BodyParser(dst); err != nil {
		_ = utils.ValidationError(c, "Corpo da requisição inválido")
		return false
	}
	if msg := utils.ValidarStruct(dst); msg != "" {
		_ = utils.ValidationError(c, msg)
		return false
	}
	return true
}

// idDaRota lê o parâmetro numérico :nome da rota; responde 400 se inválido.
func idDaRota(c *fiber.Ctx, nome string) (uint, bool) {
	id, err := strconv.ParseUint(c.Params(nome), 10, 32)
	if err != nil || id == 0 {
		_ = utils.ValidationError(c, "ID inválido")
		return 0, false
	}
	return uint(id), true
}

// responderErro mapeia erros de serviço para HTTP. Erros de negócio mostram a
// mensagem; qualquer outro erro vai só para o log e o cliente recebe mensagem genérica.
func responderErro(c *fiber.Ctx, err error) error {
	if e, ok := services.AsErroNegocio(err); ok {
		return utils.Error(c, e.Status, e.Mensagem)
	}
	log.Printf("[ERRO] %s %s: %v", c.Method(), c.Path(), err)
	return utils.Error(c, fiber.StatusInternalServerError, "Erro interno do servidor")
}

// versaoDoToken devolve o claim `tv` do token que autenticou a requisição.
func versaoDoToken(c *fiber.Ctx) int {
	v, _ := c.Locals("token_versao").(int)
	return v
}
