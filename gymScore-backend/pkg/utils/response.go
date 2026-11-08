package utils

import "github.com/gofiber/fiber/v2"

// APIResponse é a estrutura padrão de resposta da API
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	// Codigo identifica erros que o front trata de forma própria (ex.: conta_banida, conta_suspensa).
	Codigo string `json:"codigo,omitempty"`
}

// Success retorna uma resposta de sucesso padronizada
func Success(c *fiber.Ctx, statusCode int, message string, data interface{}) error {
	return c.Status(statusCode).JSON(APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Error retorna uma resposta de erro padronizada
func Error(c *fiber.Ctx, statusCode int, message string) error {
	return c.Status(statusCode).JSON(APIResponse{
		Success: false,
		Error:   message,
	})
}

// ValidationError retorna uma resposta de erro de validação
func ValidationError(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(APIResponse{
		Success: false,
		Error:   message,
	})
}

// ErrorCodigo é Error com um código estável e dados extras (ex.: fim da suspensão, motivo do banimento).
func ErrorCodigo(c *fiber.Ctx, statusCode int, message, codigo string, dados any) error {
	return c.Status(statusCode).JSON(APIResponse{
		Success: false,
		Error:   message,
		Codigo:  codigo,
		Data:    dados,
	})
}
