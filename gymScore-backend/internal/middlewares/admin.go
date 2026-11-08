package middlewares

import (
	"log"
	"net/url"
	"path"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/config"
	"gymScore-backend/pkg/utils"
)

// CookieAdmin é o nome do cookie de sessão do painel admin em desenvolvimento HTTP (Path /api/admin,
// SameSite=Strict). Com cookies Secure o nome é CookieAdminSeguro (ver NomeCookieAdmin).
const CookieAdmin = "admin_jwt"

// Chaves de c.Locals preenchidas pelo AdminAuthMiddleware.
const (
	LocalAdminID    = "admin_id"
	LocalAdminPapel = "admin_papel"
)

// papelSuperadmin espelha models.PapelSuperadmin (evita importar models no middleware).
const papelSuperadmin = "superadmin"

// AdminEstadoReader devolve o estado vigente do admin para validar o token (versão, ativo e papel).
type AdminEstadoReader interface {
	EstadoToken(id uint) (versao int, ativo bool, papel string, existe bool, err error)
}

// extrairTokenAdmin lê o JWT admin do header Authorization (Bearer) ou do cookie admin
// (`admin_jwt`/`__Secure-admin_jwt` conforme o ambiente). Nunca lê o cookie de sessão dos usuários.
func extrairTokenAdmin(c *fiber.Ctx, nomeCookie string) string {
	if h := c.Get(fiber.HeaderAuthorization); h != "" {
		partes := strings.SplitN(h, " ", 2)
		if len(partes) == 2 && strings.EqualFold(partes[0], "bearer") {
			return strings.TrimSpace(partes[1])
		}
		return ""
	}
	return c.Cookies(nomeCookie)
}

// AdminAuthMiddleware autentica o painel admin: assinatura com ADMIN_JWT_SECRET, aud=gymscore-admin,
// admin existente e ativo, e claim `tv` igual a administradores.token_versao. O papel vem do banco
// (não do token), então rebaixar um superadmin vale na hora (o UPDATE também incrementa a versão).
func AdminAuthMiddleware(cfg *config.Config, estados AdminEstadoReader) fiber.Handler {
	segredo := cfg.SegredoAdmin()
	nomeCookie := NomeCookieAdmin(cfg)
	return func(c *fiber.Ctx) error {
		token := extrairTokenAdmin(c, nomeCookie)
		if token == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "Token de administrador não fornecido")
		}
		claims, err := utils.ValidarTokenAdmin(token, segredo)
		if err != nil {
			return utils.Error(c, fiber.StatusUnauthorized, "Token inválido ou expirado")
		}
		versao, ativo, papel, existe, err := estados.EstadoToken(claims.AdminID)
		if err != nil {
			log.Printf("[ADMIN] erro ao consultar o estado do admin %d: %v", claims.AdminID, err)
			return utils.Error(c, fiber.StatusInternalServerError, "Erro interno do servidor")
		}
		if !existe || !ativo || versao != claims.Versao {
			return utils.Error(c, fiber.StatusUnauthorized, "Sessão encerrada. Faça login novamente")
		}
		c.Locals(LocalAdminID, claims.AdminID)
		c.Locals(LocalAdminPapel, papel)
		return c.Next()
	}
}

// RequireSuperadmin libera só superadmin (usar depois do AdminAuthMiddleware); admin comum recebe 403.
func RequireSuperadmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if papel, _ := c.Locals(LocalAdminPapel).(string); papel != papelSuperadmin {
			return utils.Error(c, fiber.StatusForbidden, "Ação restrita a superadministradores")
		}
		return c.Next()
	}
}

// CSPAdmin é a Content-Security-Policy das páginas do painel: só recursos da própria origem, sem
// script/estilo inline (o front do admin não usa onclick nem <script> inline), sem frames. blob: em
// img-src serve às evidências de denúncia (fetch autenticado → object URL).
const CSPAdmin = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; " +
	"font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// AdminPaginasHeaders aplica os headers de segurança às páginas e arquivos de /admin.
func AdminPaginasHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Security-Policy", CSPAdmin)
		c.Set(fiber.HeaderXFrameOptions, "DENY")
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		c.Set(fiber.HeaderReferrerPolicy, "same-origin")
		return c.Next()
	}
}

// AdminPaginasHeadersPorCaminho aplica os headers de AdminPaginasHeaders a QUALQUER forma de caminho
// que chegue ao painel: `/%61dmin/...` (o Static decodifica, o roteamento do Fiber não), `/ADMIN/`,
// `/admin/../admin/index.html`, `//admin/...`. Registrar como middleware global (sem prefixo) antes do
// app.Static("/"): a decisão é tomada pelo caminho decodificado, normalizado e em minúsculas, que é o
// mesmo que o Static usa para achar o arquivo.
func AdminPaginasHeadersPorCaminho() fiber.Handler {
	aplicar := AdminPaginasHeaders()
	return func(c *fiber.Ctx) error {
		if EhCaminhoAdmin(string(c.Context().URI().Path())) || EhCaminhoAdmin(c.Path()) {
			return aplicar(c)
		}
		return c.Next()
	}
}

// EhCaminhoAdmin diz se o caminho, depois de decodificado (inclusive codificação dupla), normalizado
// (`.`/`..`/barras repetidas/contrabarra) e em minúsculas, é /admin ou está sob /admin/.
func EhCaminhoAdmin(caminho string) bool {
	for i := 0; i < 3; i++ { // %2561 -> %61 -> a
		dec, err := url.PathUnescape(caminho)
		if err != nil || dec == caminho {
			break
		}
		caminho = dec
	}
	caminho = strings.ToLower(strings.ReplaceAll(caminho, "\\", "/"))
	if !strings.HasPrefix(caminho, "/") {
		caminho = "/" + caminho
	}
	caminho = path.Clean(caminho)
	return caminho == "/admin" || strings.HasPrefix(caminho, "/admin/")
}

// AdminAPIHeaders: respostas da API admin nunca vão para cache.
func AdminAPIHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		return c.Next()
	}
}
