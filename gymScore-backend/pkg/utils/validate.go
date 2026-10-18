package utils

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate é a instância compartilhada do validator (thread-safe, com cache de structs).
var validate = func() *validator.Validate {
	v := validator.New()
	// Usa o nome do campo JSON nas mensagens de erro
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		nome := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if nome == "" || nome == "-" {
			return f.Name
		}
		return nome
	})
	return v
}()

// ValidarStruct aplica as tags `validate:` do struct e devolve uma mensagem
// de negócio em português para o primeiro erro (ou "" quando válido).
func ValidarStruct(v any) string {
	err := validate.Struct(v)
	if err == nil {
		return ""
	}
	var erros validator.ValidationErrors
	if !errors.As(err, &erros) || len(erros) == 0 {
		return "Dados inválidos"
	}
	e := erros[0]
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("Campo obrigatório: %s", e.Field())
	case "min":
		if e.Kind() == reflect.String {
			return fmt.Sprintf("Campo %s deve ter no mínimo %s caracteres", e.Field(), e.Param())
		}
		return fmt.Sprintf("Campo %s deve ser no mínimo %s", e.Field(), e.Param())
	case "max":
		if e.Kind() == reflect.String {
			return fmt.Sprintf("Campo %s deve ter no máximo %s caracteres", e.Field(), e.Param())
		}
		return fmt.Sprintf("Campo %s deve ser no máximo %s", e.Field(), e.Param())
	case "email":
		return "E-mail inválido"
	case "oneof":
		return fmt.Sprintf("Campo %s deve ser um de: %s", e.Field(), e.Param())
	case "gt", "gte":
		return fmt.Sprintf("Campo %s deve ser maior que o mínimo permitido", e.Field())
	default:
		return fmt.Sprintf("Campo %s é inválido", e.Field())
	}
}

// CentavosParaReais converte centavos em reais apenas para exibição (nunca para cálculo).
func CentavosParaReais(centavos int64) float64 {
	return float64(centavos) / 100
}
