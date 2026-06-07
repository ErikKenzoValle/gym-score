package services_test

import (
	"errors"
	"testing"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/services"
)

// ─── Mock do repositório de usuários ─────────────────────────────────────────

type mockUsuarioRepo struct {
	usuarios map[uint]*models.Usuario
	nextID   uint
}

func novoMockUsuarioRepo() *mockUsuarioRepo {
	return &mockUsuarioRepo{
		usuarios: make(map[uint]*models.Usuario),
		nextID:   1,
	}
}

func (m *mockUsuarioRepo) Criar(u *models.Usuario) error {
	u.ID = m.nextID
	m.nextID++
	m.usuarios[u.ID] = u
	return nil
}

func (m *mockUsuarioRepo) BuscarPorID(id uint) (*models.Usuario, error) {
	u, ok := m.usuarios[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUsuarioRepo) BuscarPorCPF(cpf string) (*models.Usuario, error) {
	for _, u := range m.usuarios {
		if u.CPF == cpf {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUsuarioRepo) BuscarPorEmail(email string) (*models.Usuario, error) {
	for _, u := range m.usuarios {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUsuarioRepo) Atualizar(u *models.Usuario) error {
	if _, ok := m.usuarios[u.ID]; !ok {
		return errors.New("usuário não encontrado")
	}
	m.usuarios[u.ID] = u
	return nil
}

func (m *mockUsuarioRepo) Deletar(id uint) error {
	delete(m.usuarios, id)
	return nil
}

func (m *mockUsuarioRepo) Listar() ([]models.Usuario, error) {
	var lista []models.Usuario
	for _, u := range m.usuarios {
		lista = append(lista, *u)
	}
	return lista, nil
}

// ─── Testes do UsuarioService ─────────────────────────────────────────────────

func TestCriarUsuario_Sucesso(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo)

	req := &models.CriarUsuarioRequest{
		Nome:           "João",
		Sobrenome:      "Silva",
		Email:          "joao@example.com",
		Senha:          "senha123",
		DataNascimento: "1995-03-15",
		Genero:         "M",
	}

	resp, err := svc.CriarUsuario(req)
	if err != nil {
		t.Fatalf("esperava sucesso, obteve erro: %v", err)
	}
	if resp.Email != req.Email {
		t.Errorf("esperava email %s, obteve %s", req.Email, resp.Email)
	}
	if resp.ID == 0 {
		t.Error("esperava ID maior que zero")
	}
}

func TestCriarUsuario_EmailInvalido(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo)

	req := &models.CriarUsuarioRequest{
		Nome:           "João",
		Sobrenome:      "Silva",
		Email:          "email-invalido",
		Senha:          "senha123",
		DataNascimento: "1995-03-15",
		Genero:         "M",
	}

	_, err := svc.CriarUsuario(req)
	if err == nil {
		t.Fatal("esperava erro de e-mail inválido, obteve nil")
	}
	if err.Error() != "e-mail inválido" {
		t.Errorf("esperava 'e-mail inválido', obteve: %v", err)
	}
}

func TestCriarUsuario_EmailDuplicado(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo)

	req := &models.CriarUsuarioRequest{
		Nome:           "João",
		Sobrenome:      "Silva",
		Email:          "joao@example.com",
		Senha:          "senha123",
		DataNascimento: "1995-03-15",
		Genero:         "M",
	}

	// Primeiro cadastro deve funcionar
	if _, err := svc.CriarUsuario(req); err != nil {
		t.Fatalf("primeiro cadastro falhou: %v", err)
	}

	// Segundo cadastro com mesmo e-mail deve falhar
	_, err := svc.CriarUsuario(req)
	if err == nil {
		t.Fatal("esperava erro de e-mail duplicado, obteve nil")
	}
	if err.Error() != "e-mail já cadastrado" {
		t.Errorf("esperava 'e-mail já cadastrado', obteve: %v", err)
	}
}

func TestBuscarUsuarioPorID_NaoEncontrado(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo)

	resp, err := svc.BuscarPorID(999)
	if err != nil {
		t.Fatalf("esperava nil error, obteve: %v", err)
	}
	if resp != nil {
		t.Error("esperava nil para usuário não encontrado")
	}
}
