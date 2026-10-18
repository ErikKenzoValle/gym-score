//go:build integration

package integration

import (
	"fmt"
	"testing"
)

func criarTreino(t *testing.T, u *user) uint {
	t.Helper()
	r := esperar(t, api(t, "POST", "/api/treinos", u.Token, map[string]any{
		"nome": "Treino A", "exercicios": []map[string]any{{"nome": "Supino", "grupo_muscular": "peito", "series": 3, "repeticoes": 10}},
	}), 201)
	return uint(r.data()["id"].(float64))
}

func TestTreino_IsolamentoEntreUsuarios(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	id := criarTreino(t, a)
	path := fmt.Sprintf("/api/treinos/%d", id)

	esperar(t, api(t, "GET", path, a.Token, nil), 200)
	esperar(t, api(t, "GET", path, b.Token, nil), 404)    // não lê treino alheio
	esperar(t, api(t, "DELETE", path, b.Token, nil), 403) // não apaga treino alheio
	esperar(t, api(t, "GET", path, a.Token, nil), 200)

	lista := esperar(t, api(t, "GET", "/api/treinos", b.Token, nil), 200)
	if len(lista.Body["data"].([]any)) != 0 {
		t.Errorf("b não deveria ver treinos de a: %s", lista.Raw)
	}
	esperar(t, api(t, "DELETE", path, a.Token, nil), 200)
	esperar(t, api(t, "GET", path, a.Token, nil), 404)
}

func TestTreino_CriarIgnoraIdUsuarioDoCorpo(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/treinos", a.Token, map[string]any{
		"id_usuario": b.ID, "nome": "Meu",
		"exercicios": []map[string]any{{"nome": "Remada", "grupo_muscular": "costas"}},
	}), 201)
	if uint(r.data()["id_usuario"].(float64)) != a.ID {
		t.Errorf("treino deveria pertencer a quem tem o token: %v", r.data())
	}
	esperar(t, api(t, "POST", "/api/treinos", a.Token, map[string]any{"nome": "Sem exercícios"}), 400)
}

func TestTreino_ConcluirExercicioValidaDonoDoTreino(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	id := criarTreino(t, a)

	esperar(t, api(t, "POST", "/api/treinos/concluir", a.Token, map[string]any{"id_treino": id, "grupo_muscular": "peito"}), 200)
	esperar(t, api(t, "POST", "/api/treinos/concluir", b.Token, map[string]any{"id_treino": id, "grupo_muscular": "peito"}), 404)
	esperar(t, api(t, "POST", "/api/treinos/concluir", b.Token, map[string]any{"grupo_muscular": "peito"}), 200) // sem treino: ok
	esperar(t, api(t, "POST", "/api/treinos/concluir", b.Token, map[string]any{}), 400)

	radar := esperar(t, api(t, "GET", "/api/treinos/radar", b.Token, nil), 200).Body["data"].([]any)
	if len(radar) != 1 || radar[0].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("radar de b deveria ter só a conclusão dele: %v", radar)
	}
}

func TestTreino_ImportarPorCodigo(t *testing.T) {
	a, b := novoUsuario(t), novoUsuario(t)
	r := esperar(t, api(t, "POST", "/api/treinos", a.Token, map[string]any{
		"nome": "Compartilhado", "exercicios": []map[string]any{{"nome": "Agachamento", "grupo_muscular": "pernas"}},
	}), 201)
	codigo := r.data()["codigo"].(string)

	esperar(t, api(t, "POST", "/api/treinos/importar", a.Token, map[string]any{"codigo": codigo}), 409) // já é seu
	esperar(t, api(t, "POST", "/api/treinos/importar", b.Token, map[string]any{"codigo": "NAOEXISTE"}), 404)
	imp := esperar(t, api(t, "POST", "/api/treinos/importar", b.Token, map[string]any{"codigo": codigo}), 200).data()
	if uint(imp["id_usuario"].(float64)) != b.ID {
		t.Errorf("treino importado deve ser de b: %v", imp)
	}
}
