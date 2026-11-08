package middlewares

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/config"
	"gymScore-backend/pkg/utils"
)

// CSRFMiddleware protege as requisições que mudam estado e se autenticam pelo cookie de sessão (`jwt`/`__Host-jwt`)
// (SameSite=Lax ainda deixa passar alguns casos, ex.: navegação top-level e subdomínios irmãos).
// Regra: método não seguro (POST/PUT/PATCH/DELETE) SEM header Authorization e COM o cookie `jwt` só
// passa se Origin (ou, na falta dele, Referer) for a própria origem da requisição, APP_BASE_URL ou
// uma das CORS_ORIGINS. Requisições com Bearer não dependem do cookie e não são afetadas (um site
// alheio não consegue mandar o header sem passar pelo CORS).
func CSRFMiddleware(cfg *config.Config) fiber.Handler {
	permitidas := origensPermitidas(cfg)
	nomeCookie := NomeCookieUsuario(cfg)
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return c.Next()
		}
		if c.Get(fiber.HeaderAuthorization) != "" || c.Cookies(nomeCookie) == "" {
			return c.Next()
		}
		if origemConfiavel(c, permitidas) {
			return c.Next()
		}
		return utils.Error(c, fiber.StatusForbidden, "Origem da requisição não permitida")
	}
}

// AdminCSRFMiddleware é a checagem de origem do painel admin (/api/admin). O cookie admin_jwt já é
// SameSite=Strict; mesmo assim toda requisição não-GET sem header Authorization (inclusive o login)
// precisa de Origin/Referer confiável.
func AdminCSRFMiddleware(cfg *config.Config) fiber.Handler {
	permitidas := origensPermitidas(cfg)
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return c.Next()
		}
		if c.Get(fiber.HeaderAuthorization) != "" || origemConfiavel(c, permitidas) {
			return c.Next()
		}
		return utils.Error(c, fiber.StatusForbidden, "Origem da requisição não permitida")
	}
}

// origensPermitidas monta o conjunto APP_BASE_URL + CORS_ORIGINS (normalizadas).
func origensPermitidas(cfg *config.Config) map[string]bool {
	permitidas := map[string]bool{}
	if o := origemDe(cfg.AppBaseURL); o != "" {
		permitidas[o] = true
	}
	for _, c := range cfg.CORSOrigins {
		if o := origemDe(c); o != "" {
			permitidas[o] = true
		}
	}
	return permitidas
}

// origemConfiavel: Origin (ou, na falta dele, Referer) é a própria origem da requisição ou uma permitida.
func origemConfiavel(c *fiber.Ctx, permitidas map[string]bool) bool {
	origem := c.Get(fiber.HeaderOrigin)
	if origem == "" || origem == "null" {
		origem = origemDe(c.Get(fiber.HeaderReferer))
	} else {
		origem = origemDe(origem)
	}
	return origem != "" && (permitidas[origem] || origem == origemDe(c.Protocol()+"://"+c.Hostname()))
}

// origemDe normaliza uma URL para "esquema://host[:porta]" (minúsculo); "" se inválida.
func origemDe(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}
