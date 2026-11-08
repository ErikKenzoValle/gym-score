package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ExpiracaoToken devolve o `exp` de um JWT que o próprio servidor acabou de assinar (sem validar a
// assinatura: usado só para alinhar a validade do cookie de sessão ao token). Sem `exp` legível,
// devolve agora + padrao.
func ExpiracaoToken(token string, padrao time.Duration) time.Time {
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err == nil && claims.ExpiresAt != nil {
		return claims.ExpiresAt.Time
	}
	return time.Now().Add(padrao)
}
