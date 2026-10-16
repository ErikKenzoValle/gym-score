package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims define os campos do payload do token JWT
type JWTClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	// Versao (claim `tv`) deve bater com usuarios.token_versao; logout, troca e reset de
	// senha incrementam o valor no banco e invalidam os tokens anteriores.
	Versao int `json:"tv"`
	jwt.RegisteredClaims
}

// DuracaoToken é a validade do JWT (e do cookie de sessão, que deve ficar alinhado).
const DuracaoToken = 24 * time.Hour

// GerarToken gera um token JWT assinado para o usuário autenticado
func GerarToken(userID uint, email string, versao int, secret string) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		Email:  email,
		Versao: versao,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(DuracaoToken)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "gymscore-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidarToken valida e decodifica um token JWT
func ValidarToken(tokenString, secret string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inválido")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("token inválido")
	}

	return claims, nil
}
