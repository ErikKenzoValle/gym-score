package utils

import (
	"regexp"
	"strings"
)

// emailRegex é a expressão regular para validação de e-mail
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,6}$`)

// ValidarEmail verifica se o e-mail fornecido possui formato válido
func ValidarEmail(email string) bool {
	email = strings.TrimSpace(email)
	return emailRegex.MatchString(email)
}

// ValidarCPF verifica se o CPF informado é válido (algoritmo de dígitos verificadores)
func ValidarCPF(cpf string) bool {
	// Remover caracteres não numéricos
	cpf = strings.ReplaceAll(cpf, ".", "")
	cpf = strings.ReplaceAll(cpf, "-", "")
	cpf = strings.ReplaceAll(cpf, " ", "")

	if len(cpf) != 11 {
		return false
	}
	for i := 0; i < 11; i++ {
		if cpf[i] < '0' || cpf[i] > '9' {
			return false
		}
	}

	// CPFs com todos os dígitos iguais são inválidos
	isAllEqual := true
	for i := 1; i < 11; i++ {
		if cpf[i] != cpf[0] {
			isAllEqual = false
			break
		}
	}
	if isAllEqual {
		return false
	}

	// Validação do primeiro dígito
	sum := 0
	for i := 0; i < 9; i++ {
		digit := int(cpf[i] - '0')
		sum += digit * (10 - i)
	}
	firstDigit := (sum * 10) % 11
	if firstDigit == 10 {
		firstDigit = 0
	}
	if firstDigit != int(cpf[9]-'0') {
		return false
	}

	// Validação do segundo dígito
	sum = 0
	for i := 0; i < 10; i++ {
		digit := int(cpf[i] - '0')
		sum += digit * (11 - i)
	}
	secondDigit := (sum * 10) % 11
	if secondDigit == 10 {
		secondDigit = 0
	}
	return secondDigit == int(cpf[10]-'0')
}

// CalcularElo retorna a faixa de Elo baseada nos pontos acumulados pelo usuário.
// Escala unificada com o frontend (perfil.html, array _ELOS) para evitar divergência entre telas.
func CalcularElo(pontos int) string {
	switch {
	case pontos >= 8000:
		return "Elite"
	case pontos >= 3000:
		return "Diamante"
	case pontos >= 1000:
		return "Ouro"
	case pontos >= 500:
		return "Prata"
	case pontos >= 100:
		return "Bronze"
	default:
		return "Iniciante"
	}
}
