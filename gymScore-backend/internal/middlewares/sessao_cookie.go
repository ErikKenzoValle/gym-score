package middlewares

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"gymScore-backend/internal/config"
	"gymScore-backend/pkg/utils"
)

// Cookies de sessão (R7). O JWT nunca vai no corpo das respostas: a sessão do navegador é só o cookie.
//
//	                 | desenvolvimento (HTTP)       | Secure (produção ou APP_BASE_URL https)
//	usuário  nome    | jwt                          | __Host-jwt
//	         attrs   | Path=/; HttpOnly; Lax        | Path=/; HttpOnly; Secure; Lax (sem Domain)
//	admin    nome    | admin_jwt                    | __Secure-admin_jwt
//	         attrs   | Path=/api/admin; HttpOnly;   | Path=/api/admin; HttpOnly; Secure; Strict
//	                 | Strict                       |
//
// Expires = `exp` do JWT. Para apagar: mesmo nome/Path/Secure/SameSite, valor vazio e Expires no passado.
const (
	CookieUsuario        = "jwt"
	CookieUsuarioSeguro  = "__Host-jwt"
	CookieAdminSeguro    = "__Secure-admin_jwt"
	caminhoCookieUsuario = "/"
	caminhoCookieAdmin   = "/api/admin"
)

// expiraApagar é a data usada para apagar o cookie (Expires no passado).
var expiraApagar = time.Unix(0, 0).UTC()

// NomeCookieUsuario é o nome do cookie de sessão do usuário conforme o ambiente.
func NomeCookieUsuario(cfg *config.Config) string {
	if cfg.CookieSeguro() {
		return CookieUsuarioSeguro
	}
	return CookieUsuario
}

// NomeCookieAdmin é o nome do cookie de sessão do painel admin conforme o ambiente.
func NomeCookieAdmin(cfg *config.Config) string {
	if cfg.CookieSeguro() {
		return CookieAdminSeguro
	}
	return CookieAdmin
}

// CookieSessaoUsuario grava o JWT de usuário; validade = `exp` do token.
func CookieSessaoUsuario(cfg *config.Config, token string) *fiber.Cookie {
	return cookieUsuario(cfg, token, utils.ExpiracaoToken(token, utils.DuracaoToken))
}

// CookieSessaoUsuarioApagar apaga o cookie de usuário com os mesmos atributos com que foi criado.
func CookieSessaoUsuarioApagar(cfg *config.Config) *fiber.Cookie {
	return cookieUsuario(cfg, "", expiraApagar)
}

func cookieUsuario(cfg *config.Config, valor string, expira time.Time) *fiber.Cookie {
	return &fiber.Cookie{
		Name: NomeCookieUsuario(cfg), Value: valor, Path: caminhoCookieUsuario, Expires: expira,
		HTTPOnly: true, Secure: cfg.CookieSeguro(), SameSite: fiber.CookieSameSiteLaxMode,
	}
}

// CookieSessaoAdmin grava o JWT do painel admin; validade = `exp` do token.
func CookieSessaoAdmin(cfg *config.Config, token string) *fiber.Cookie {
	return cookieAdmin(cfg, token, utils.ExpiracaoToken(token, cfg.TTLAdmin()))
}

// CookieSessaoAdminApagar apaga o cookie admin com os mesmos atributos com que foi criado.
func CookieSessaoAdminApagar(cfg *config.Config) *fiber.Cookie {
	return cookieAdmin(cfg, "", expiraApagar)
}

func cookieAdmin(cfg *config.Config, valor string, expira time.Time) *fiber.Cookie {
	return &fiber.Cookie{
		Name: NomeCookieAdmin(cfg), Value: valor, Path: caminhoCookieAdmin, Expires: expira,
		HTTPOnly: true, Secure: cfg.CookieSeguro(), SameSite: fiber.CookieSameSiteStrictMode,
	}
}

// SemCache marca a resposta como não armazenável (respostas de autenticação e de sessão).
func SemCache() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		c.Set(fiber.HeaderPragma, "no-cache")
		return c.Next()
	}
}
