package client

import (
	"context"
	"encoding/json"
	"errors"
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

func candidatosTeste() []CandidatoIA {
	return []CandidatoIA{
		{ID: 10, Titulo: "Corrida 5km", Categoria: "corrida", ValorReais: 50, Sinais: []string{"categoria Corrida do atleta: 60/100"}},
		{ID: 11, Titulo: "Supino 100kg", Categoria: "musculacao", ValorReais: 80, Sinais: []string{"categoria Musculação do atleta: 10/100"}},
	}
}

func perfilTeste() PerfilAtletaIA {
	return PerfilAtletaIA{Nivel: 4, NivelMax: 10, Pontuacao: 120,
		Atributos: map[string]int{"forca": 10, "resistencia": 70}, Categorias: map[string]int{"corrida": 60}}
}

func respostaComStop(texto, stop string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_teste", "type": "message", "role": "assistant", "model": "claude-haiku-4-5-20251001",
		"stop_reason": stop, "stop_sequence": nil,
		"content": []map[string]any{{"type": "text", "text": texto}},
		"usage":   map[string]any{"input_tokens": 320, "output_tokens": 90},
	})
	return string(b)
}

func servir(texto string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respostaFake(texto)))
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
	if c.Modelo() != string(anthropic.ModelClaudeHaiku4_5) {
		t.Errorf("modelo padrão deveria ser o Haiku 4.5, obteve %q", c.Modelo())
	}
}

func TestGerarRazoes_ClienteNilDevolveErroSemPanico(t *testing.T) {
	var c *AnthropicClient
	r, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), candidatosTeste())
	if r != nil || !errors.Is(err, ErrIADesabilitada) {
		t.Errorf("esperava ErrIADesabilitada com cliente nulo, obteve %v %v", r, err)
	}
}

func TestGerarRazoes_RespostaValidaEPedidoEstruturado(t *testing.T) {
	var corpo map[string]any
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &corpo)
		if !strings.Contains(string(b), "Corrida 5km") || !strings.Contains(string(b), "resistencia") {
			t.Error("prompt deveria conter os candidatos e os eixos do atleta")
		}
		servir(`{"recomendacoes":[{"id":10,"por_que":"Sua Corrida em 60/100 combina com esta prova."},{"id":11,"por_que":"Bom para subir sua Força, hoje em 10/100."}]}`)(w, r)
	})
	razoes, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), candidatosTeste())
	if err != nil || len(razoes) != 2 {
		t.Fatalf("esperava 2 razões, obteve %v %v", razoes, err)
	}
	if razoes[0].ID != 10 || razoes[0].PorQue != "Sua Corrida em 60/100 combina com esta prova." {
		t.Errorf("razão do desafio 10 inesperada: %+v", razoes[0])
	}
	// saída estruturada: output_config.format = json_schema com o campo por_que; teto de tokens mantido
	oc, _ := corpo["output_config"].(map[string]any)
	f, _ := oc["format"].(map[string]any)
	if f["type"] != "json_schema" || !strings.Contains(fmtJSON(f["schema"]), "por_que") {
		t.Errorf("pedido deveria usar structured outputs com o schema das razões: %v", corpo["output_config"])
	}
	if corpo["max_tokens"] != float64(maxTokensRazoes) || corpo["model"] != "claude-haiku-4-5-20251001" {
		t.Errorf("modelo/max_tokens alterados: %v %v", corpo["model"], corpo["max_tokens"])
	}
}

func fmtJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestGerarRazoes_RespostaComCercaMarkdown(t *testing.T) {
	c := novoClienteFake(t, servir("```json\n{\"recomendacoes\":[{\"id\":10,\"por_que\":\"Perfeito para seu nível 4.\"}]}\n```"))
	razoes, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), candidatosTeste())
	if err != nil || len(razoes) != 1 || razoes[0].PorQue != "Perfeito para seu nível 4." {
		t.Errorf("esperava razão extraída da cerca markdown, obteve %v %v", razoes, err)
	}
}

func TestGerarRazoes_JSONInvalidoOuForaDoFormato(t *testing.T) {
	for nome, texto := range map[string]string{
		"texto livre":    "desculpe, não consigo ajudar",
		"formato antigo": `{"razoes":{"10":"Combina com você"}}`,
		"campo extra":    `{"recomendacoes":[{"id":10,"por_que":"ok ok ok ok","html":"<b>x</b>"}]}`,
		"id texto":       `{"recomendacoes":[{"id":"10","por_que":"Combina com você hoje"}]}`,
		"lista vazia":    `{"recomendacoes":[]}`,
		"cortado":        `{"recomendacoes":[{"id":10,"por_que":"Comb`,
	} {
		c := novoClienteFake(t, servir(texto))
		r, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), candidatosTeste())
		if err == nil || r != nil {
			t.Errorf("%s: esperava erro (fallback), obteve %v", nome, r)
		}
	}
}

func TestGerarRazoes_StopMaxTokensOuRecusa(t *testing.T) {
	for _, stop := range []string{"max_tokens", "refusal"} {
		c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respostaComStop(`{"recomendacoes":[]}`, stop)))
		})
		r, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), candidatosTeste())
		if !errors.Is(err, ErrRespostaCortada) || r != nil {
			t.Errorf("stop_reason=%s: esperava ErrRespostaCortada, obteve %v %v", stop, r, err)
		}
	}
}

func TestGerarRazoes_ErroDaAPIDevolveErro(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"limite"}}`))
	})
	if r, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), candidatosTeste()); err == nil || r != nil {
		t.Errorf("esperava erro em rate limit, obteve %v", r)
	}
}

func TestGerarRazoes_Timeout(t *testing.T) {
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		servir(`{"recomendacoes":[{"id":10,"por_que":"tarde demais para isso"}]}`)(w, r)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if r, err := c.GerarRazoesRecomendacao(ctx, perfilTeste(), candidatosTeste()); err == nil || r != nil {
		t.Errorf("esperava erro em timeout, obteve %v", r)
	}
}

func TestGerarRazoes_SemCandidatosNaoChamaAPI(t *testing.T) {
	chamou := false
	c := novoClienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		chamou = true
		servir(`{"recomendacoes":[]}`)(w, r)
	})
	if r, err := c.GerarRazoesRecomendacao(context.Background(), perfilTeste(), nil); r != nil || err != nil {
		t.Error("esperava nil, nil sem candidatos")
	}
	if chamou {
		t.Error("não deveria chamar a API sem candidatos (desperdício de tokens)")
	}
}
