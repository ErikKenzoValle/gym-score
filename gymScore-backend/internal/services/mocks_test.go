package services_test

import (
	"sync"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
)

// ─── Mock do repositório de usuários ─────────────────────────────────────────

type mockUsuarioRepo struct {
	usuarios map[uint]*models.Usuario
	nextID   uint
}

func novoMockUsuarioRepo() *mockUsuarioRepo {
	return &mockUsuarioRepo{usuarios: map[uint]*models.Usuario{}, nextID: 1}
}

func (m *mockUsuarioRepo) Criar(u *models.Usuario) error {
	u.ID = m.nextID
	m.nextID++
	m.usuarios[u.ID] = u
	return nil
}
func (m *mockUsuarioRepo) BuscarPorID(id uint) (*models.Usuario, error) { return m.usuarios[id], nil }
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
func (m *mockUsuarioRepo) AtualizarPerfil(id uint, campos map[string]any) error {
	if u := m.usuarios[id]; u != nil {
		if v, ok := campos["nome"]; ok {
			u.Nome = v.(string)
		}
	}
	return nil
}
func (m *mockUsuarioRepo) AtualizarSenha(id uint, hash string, versao int) (int, bool, error) {
	u := m.usuarios[id]
	if u == nil || u.TokenVersao != versao {
		return 0, false, nil
	}
	u.Senha = hash
	u.TokenVersao++
	return u.TokenVersao, true, nil
}
func (m *mockUsuarioRepo) VersaoToken(id uint) (int, bool, error) {
	u := m.usuarios[id]
	if u == nil {
		return 0, false, nil
	}
	return u.TokenVersao, true, nil
}
func (m *mockUsuarioRepo) RevogarTokens(id uint, versao int) (bool, error) {
	if u := m.usuarios[id]; u != nil && u.TokenVersao == versao {
		u.TokenVersao++
		return true, nil
	}
	return false, nil
}
func (m *mockUsuarioRepo) Buscar(termo string, excluirID uint, limite int) ([]models.Usuario, error) {
	return nil, nil
}

// ─── Mock do repositório de amizades ─────────────────────────────────────────

type mockAmizadeRepo struct {
	mu   sync.Mutex
	rels []*models.Amizade
}

func (m *mockAmizadeRepo) Criar(a *models.Amizade) error {
	for _, r := range m.rels {
		if (r.IDUsuario == a.IDUsuario && r.IDAmigo == a.IDAmigo) || (r.IDUsuario == a.IDAmigo && r.IDAmigo == a.IDUsuario) {
			return repositories.ErrAmizadeDuplicada
		}
	}
	a.ID = uint(len(m.rels) + 1)
	m.rels = append(m.rels, a)
	return nil
}
func (m *mockAmizadeRepo) BuscarRelacao(a, b uint) (*models.Amizade, error) {
	for _, r := range m.rels {
		if (r.IDUsuario == a && r.IDAmigo == b) || (r.IDUsuario == b && r.IDAmigo == a) {
			return r, nil
		}
	}
	return nil, nil
}
func (m *mockAmizadeRepo) ListarAmigos(id uint) ([]models.Amizade, error) { return nil, nil }
func (m *mockAmizadeRepo) Reenviar(id, rem, dest uint) (bool, error) {
	for _, r := range m.rels {
		if r.ID == id && r.Status == models.StatusAmizadeRecusada {
			r.IDUsuario, r.IDAmigo, r.Status = rem, dest, models.StatusAmizadePendente
			return true, nil
		}
	}
	return false, nil
}
func (m *mockAmizadeRepo) Responder(rem, dest uint, novo models.StatusAmizade) (bool, error) {
	for _, r := range m.rels {
		if r.IDUsuario == rem && r.IDAmigo == dest && r.Status == models.StatusAmizadePendente {
			r.Status = novo
			return true, nil
		}
	}
	return false, nil
}
func (m *mockAmizadeRepo) Remover(a, b uint) (bool, error) { return false, nil }
func (m *mockAmizadeRepo) SaoAmigos(a, b uint) (bool, error) {
	r, _ := m.BuscarRelacao(a, b)
	return r != nil && r.Status == models.StatusAmizadeAceita, nil
}

// ─── Mock de reset de senha (reproduz a regra de uso único/expiração do SQL) ──

type mockResetRepo struct {
	tokens []*models.PasswordResetToken
	senhas map[uint]string
}

func (m *mockResetRepo) Criar(t *models.PasswordResetToken) error {
	m.tokens = append(m.tokens, t)
	return nil
}
func (m *mockResetRepo) Consumir(hash string, agora time.Time, novo string) (bool, error) {
	for _, t := range m.tokens {
		if t.TokenHash == hash && t.UsadoEm == nil && t.ExpiraEm.After(agora) {
			t.UsadoEm = &agora
			m.senhas[t.IDUsuario] = novo
			return true, nil
		}
	}
	return false, nil
}

type mailerCaptura struct{ corpos []string }

func (m *mailerCaptura) Enviar(para, assunto, corpo string) error {
	m.corpos = append(m.corpos, corpo)
	return nil
}
