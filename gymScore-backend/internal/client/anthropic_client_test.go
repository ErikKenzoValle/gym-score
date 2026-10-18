package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// novoClienteFake aponta o SDK para um servidor local, permitindo testar o
// parsing da resposta sem consumir créditos da API real.
func novoClienteFake(t *testing.T, handler http.HandlerFunc) *AnthropicClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := anthropic.NewClient(
		option.WithAPIKey("sk-ant-teste"),
		option.WithBaseURL(srv.URL),
		option.WithMaxRetries(0),
	)
	return &AnthropicClient{client: &c, model: "claude-haiku-4-5-20251001"}
}

func respostaFake(texto string) string {
	b, _ := json.Marshal(map[string]any{
		"id":            "msg_teste",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-haiku-4-5-20251001",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"content":       []map[string]any{{"type": "text", "text": texto}},
		"usage":         map[string]any{"input_tokens": 320, "output_tokens": 90},
	})
	return string(b)
}

func candidatosTeste() []map[string]any {
	return []map[string]any{
		{"id": 10, "titulo": "Corrida 5km", "valor": 50.0},
		{"id": 11, "titulo": "Supino 100kg", "valor": 80.0},
	}
}

func TestNovoAnthropicClient_SemChaveRetornaNil(t *testing.T) {
	if c := NovoAnthropicClient("", "claude-haiku-4-5-20251001"); c != nil {
		t.Fatal("esperava nil quando a chave está vazia")
	}
	if c := NovoAnthropicClient("   ", ""); c != nil {
		t.Fatal("esperava nil quando a chave é apenas espaços")
	}
}

func TestNovoAnthropicClient_ModeloPadrao(t *testing.T) {
	c := NovoAnthropicClient("sk-ant-teste", "")
	if c == nil {
		t.Fatal("esperava cliente não nulo")
	}
	if c.Modelo() == "" {
		t.Error("esperava modelo padrão preenchido quando ANTHROPIC_MODEL está vazio")
	}
}

func TestGerarRazoes_ClienteNilNaoPanica(t *testing.T) {
	var c *AnthropicClient
	if r := c.GerarRazoesRecomendacao(context.Background(), "perfil", candidatosTeste()); r != nil {
		t.Error("esperava nil com cliente nulo")
	}
}

func TestGerarRazoes_RespostaValida(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(corpo), "Corrida 5km") {
			t.Error("prompt deveria conter os candidatos enviados")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respostaFake(`{"razoes":{"10":"Combina com seus treinos de perna","11":"Valor no seu padrão de aposta"}}`)))
	})

	razoes := c.GerarRazoesRecomendacao(context.Background(), "Elo: Ouro", candidatosTeste())
	if len(razoes) != 2 {
		t.Fatalf("esperava 2 razões, obteve %d", len(razoes))
	}
	if razoes[10] != "Combina com seus treinos de perna" {
		t.Errorf("razão do desafio 10 inesperada: %q", razoes[10])
	}
}

func TestGerarRazoes_RespostaComCercaMarkdown(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respostaFake("```json\n{\"razoes\":{\"10\":\"Perfeito para seu nível\"}}\n```")))
	})

	razoes := c.GerarRazoesRecomendacao(context.Background(), "Elo: Bronze", candidatosTeste())
	if razoes[10] != "Perfeito para seu nível" {
		t.Errorf("esperava razão extraída da cerca markdown, obteve %q", razoes[10])
	}
}

func TestGerarRazoes_RespostaInvalidaRetornaNil(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respostaFake("desculpe, não consigo ajudar")))
	})

	if r := c.GerarRazoesRecomendacao(context.Background(), "Elo: Prata", candidatosTeste()); r != nil {
		t.Errorf("esperava nil (fallback heurístico), obteve %v", r)
	}
}

func TestGerarRazoes_ErroDaAPIRetornaNil(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"limite"}}`))
	})

	if r := c.GerarRazoesRecomendacao(context.Background(), "Elo: Elite", candidatosTeste()); r != nil {
		t.Errorf("esperava nil em rate limit, obteve %v", r)
	}
}

func TestGerarRazoes_TimeoutRetornaNil(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respostaFake(`{"razoes":{"10":"tarde demais"}}`)))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if r := c.GerarRazoesRecomendacao(ctx, "Elo: Ouro", candidatosTeste()); r != nil {
		t.Errorf("esperava nil em timeout, obteve %v", r)
	}
}

func TestGerarRazoes_SemCandidatosNaoChamaAPI(t *testing.T) {
	chamou := false
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		chamou = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respostaFake(`{"razoes":{}}`)))
	})

	if r := c.GerarRazoesRecomendacao(context.Background(), "Elo: Ouro", nil); r != nil {
		t.Error("esperava nil sem candidatos")
	}
	if chamou {
		t.Error("não deveria chamar a API sem candidatos (desperdício de tokens)")
	}
}
