package utils

import (
	"regexp"
	"strings"
)

// emailRegex é a expressão regular para validação de e-mail
// Equivalente ao regex utilizado no servidor Java original
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,6}$`)

// ValidarEmail verifica se o e-mail fornecido possui formato válido
func ValidarEmail(email string) bool {
	email = strings.TrimSpace(email)
	return emailRegex.MatchString(email)
}

// ValidarSaldo verifica se o usuário possui saldo suficiente para participar do desafio
// Equivalente à lógica do endpoint /validar-saldo do servidor Java original
func ValidarSaldo(saldoUsuario, valorDesafio float64) bool {
	return saldoUsuario >= valorDesafio
}

// CalcularSaldosAposDesafio calcula os novos saldos após o encerramento de um desafio
// Equivalente à lógica do endpoint /finalizar-desafio do servidor Java original:
// - Vencedor recebe o valor apostado integralmente
// - Perdedor perde metade do valor apostado
func CalcularSaldosAposDesafio(saldoVencedor, saldoPerdedor, valorApostado float64) (novoSaldoVencedor, novoSaldoPerdedor float64) {
	novoSaldoVencedor = saldoVencedor + valorApostado
	novoSaldoPerdedor = saldoPerdedor - (valorApostado / 2)
	return
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
	if secondDigit != int(cpf[10]-'0') {
		return false
	}

	return true
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
