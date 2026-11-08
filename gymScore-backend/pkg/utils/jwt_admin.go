package utils

import (
	"errors"
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AudienciaAdmin é o claim `aud` dos tokens do painel admin. Tokens de usuário não têm `aud` e são
// assinados com outro segredo (JWT_SECRET x ADMIN_JWT_SECRET): um nunca vale no lugar do outro.
const AudienciaAdmin = "gymscore-admin"

// AdminClaims é o payload do JWT do painel admin.
type AdminClaims struct {
	AdminID uint `json:"admin_id"`
	// Versao (claim `tv`) deve bater com administradores.token_versao (desativar, trocar papel,
	// trocar senha e logout incrementam o valor no banco).
	Versao int `json:"tv"`
	jwt.RegisteredClaims
}

// GerarTokenAdmin assina o token do admin com o segredo do painel e a audiência gymscore-admin.
func GerarTokenAdmin(adminID uint, versao int, secret string, ttl time.Duration) (string, error) {
	agora := time.Now()
	claims := AdminClaims{
		AdminID: adminID,
		Versao:  versao,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(agora.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(agora),
			Issuer:    "gymscore-api",
			Audience:  jwt.ClaimStrings{AudienciaAdmin},
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ValidarTokenAdmin valida assinatura (HMAC), expiração e audiência gymscore-admin.
func ValidarTokenAdmin(tokenString, secret string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inválido")
		}
		return []byte(secret), nil
	}, jwt.WithAudience(AudienciaAdmin), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid || claims.AdminID == 0 {
		return nil, errors.New("token inválido")
	}
	return claims, nil
}

// ehTokenAdmin diz se as claims trazem a audiência do painel admin.
func ehTokenAdmin(aud jwt.ClaimStrings) bool { return slices.Contains(aud, AudienciaAdmin) }
