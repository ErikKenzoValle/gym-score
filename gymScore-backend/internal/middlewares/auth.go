package middlewares

import (
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/pkg/utils"
)

// VersaoTokenReader consulta a versão de token vigente do usuário (revogação de JWT) e o estado de
// moderação (banimento/suspensão).
type VersaoTokenReader interface {
	EstadoSessao(id uint) (models.EstadoSessao, error)
}

// LocalSuspensoAte é a chave de c.Locals com o fim da suspensão vigente (*time.Time; ausente = não suspenso).
const LocalSuspensoAte = "suspenso_ate"

// Códigos/mensagens iguais aos do pacote services (erros.go); repetidos aqui para não criar ciclo.
const (
	codigoContaBanida     = "conta_banida"
	codigoContaSuspensa   = "conta_suspensa"
	mensagemContaBanida   = "Conta banida. Você pode recorrer da decisão pela tela de login"
	mensagemContaSuspensa = "Conta suspensa: esta ação fica bloqueada até o fim da suspensão"
)

// LocalModo é a chave de c.Locals com o claim `modo` do token (string; "" em tokens antigos).
const LocalModo = "modo"

// ModoDaSessao devolve o claim `modo` da requisição autenticada ("" = token sem claim = atleta).
func ModoDaSessao(c *fiber.Ctx) string {
	m, _ := c.Locals(LocalModo).(string)
	return m
}

// agoraAuth é o relógio do middleware (substituível em teste).
var agoraAuth = time.Now

// ExtrairToken lê o JWT do header Authorization (Bearer) ou do cookie de sessão do usuário
// (`jwt` em desenvolvimento HTTP, `__Host-jwt` quando os cookies são Secure — ver NomeCookieUsuario).
func ExtrairToken(c *fiber.Ctx, cfg *config.Config) string {
	var token string

	// 1. Tentar obter o token do cabeçalho Authorization
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			token = parts[1]
		}
	}

	// 2. Se não houver token no header, tentar obter do cookie de sessão
	if token == "" {
		token = c.Cookies(NomeCookieUsuario(cfg))
	}
	return token
}

// AuthMiddleware valida o JWT (header ou cookie) e confere o claim `tv` com o banco: tokens
// emitidos antes de um logout / troca / reset de senha deixam de valer imediatamente.
func AuthMiddleware(cfg *config.Config, versoes VersaoTokenReader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := ExtrairToken(c, cfg)
		if token == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "Token de autenticação não fornecido")
		}

		// 3. Validar o token encontrado
		claims, err := utils.ValidarToken(token, cfg.JWTSecret)
		if err != nil {
			return utils.Error(c, fiber.StatusUnauthorized, "Token inválido ou expirado")
		}

		estado, err := versoes.EstadoSessao(claims.UserID)
		if err != nil {
			log.Printf("[AUTH] erro ao consultar versão do token do usuário %d: %v", claims.UserID, err)
			return utils.Error(c, fiber.StatusInternalServerError, "Erro interno do servidor")
		}
		if !estado.Existe || estado.Versao != claims.Versao {
			return utils.Error(c, fiber.StatusUnauthorized, "Sessão encerrada. Faça login novamente")
		}
		// Banido: nenhuma rota autenticada (o recurso do banido é pela rota pública — Q1).
		if estado.BanidoEm != nil {
			return utils.ErrorCodigo(c, fiber.StatusForbidden, mensagemContaBanida, codigoContaBanida, nil)
		}
		// Suspenso (Q2): segue autenticado; as rotas de participação/gestão usam BloquearSuspenso.
		// Comparação em tempo real: o acesso não depende do job fim-suspensao.
		if estado.SuspensoAte != nil && estado.SuspensoAte.After(agoraAuth()) {
			c.Locals(LocalSuspensoAte, *estado.SuspensoAte)
		}

		// Armazena os dados do usuário autenticado no contexto da requisição
		c.Locals("user_id", claims.UserID)
		c.Locals("user_email", claims.Email)
		c.Locals("token_versao", claims.Versao)
		// Modo escolhido pelo usuário (claim `modo`; vazio em tokens antigos = atleta). É só a escolha: as
		// rotas de instrutor reconferem no banco (RequirePerfil / contexto dos services).
		c.Locals(LocalModo, claims.Modo)

		return c.Next()
	}
}

// BloquearSuspenso recusa (403 conta_suspensa, com o fim) as ações de participação/gestão de quem está
// suspenso (Q2). Vai DEPOIS do AuthMiddleware. Leitura, carteira, notificações, recurso e as próprias
// denúncias não usam este middleware.
func BloquearSuspenso() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if fim, ok := c.Locals(LocalSuspensoAte).(time.Time); ok {
			return utils.ErrorCodigo(c, fiber.StatusForbidden, mensagemContaSuspensa, codigoContaSuspensa,
				fiber.Map{"suspenso_ate": fim})
		}
		return c.Next()
	}
}

// LimiteCorpo recusa (413) corpos acima de `padrao` bytes em todas as rotas, exceto nas que `excecao`
// aceitar (que têm limite próprio, ex.: upload de evidência). O BodyLimit do Fiber fica no maior limite
// (o da exceção); este middleware devolve o limite de 1 MiB a todo o resto.
func LimiteCorpo(padrao int, excecao func(c *fiber.Ctx) bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if len(c.Request().Body()) > padrao && (excecao == nil || !excecao(c)) {
			return utils.Error(c, fiber.StatusRequestEntityTooLarge, "Corpo da requisição grande demais")
		}
		return c.Next()
	}
}
