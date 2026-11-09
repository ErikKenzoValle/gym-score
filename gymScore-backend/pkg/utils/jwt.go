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
	// Modo (claim `modo`, rodada 4) é o modo ativo da conta de atleta: "atleta" | "instrutor". Vazio em
	// tokens antigos e nas contas de academia (= atleta). O claim só diz o que o usuário ESCOLHEU: toda
	// rota de instrutor reconfere no banco que o pedido de instrutor continua aprovado.
	Modo string `json:"modo,omitempty"`
	jwt.RegisteredClaims
}

// DuracaoToken é a validade do JWT (e do cookie de sessão, que deve ficar alinhado).
const DuracaoToken = 24 * time.Hour

// GerarToken gera um token JWT assinado para o usuário autenticado (sem claim `modo` = modo atleta).
func GerarToken(userID uint, email string, versao int, secret string) (string, error) {
	return GerarTokenModo(userID, email, versao, "", secret)
}

// GerarTokenModo é GerarToken com o claim `modo` (modo ativo da sessão; "" omite o claim).
func GerarTokenModo(userID uint, email string, versao int, modo, secret string) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		Email:  email,
		Versao: versao,
		Modo:   modo,
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
	// Defesa em profundidade: mesmo com segredos iguais por engano, token do painel admin não vale
	// como token de usuário.
	if ehTokenAdmin(claims.Audience) {
		return nil, errors.New("token de outra audiência")
	}

	return claims, nil
}
