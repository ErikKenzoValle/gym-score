package repositories

import (
	"errors"
	"strings"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// UsuarioRepository define a interface de acesso a dados para usuários.
// Não há Save genérico: o saldo só muda via CarteiraRepository.
type UsuarioRepository interface {
	Criar(usuario *models.Usuario) error
	BuscarPorID(id uint) (*models.Usuario, error)
	BuscarPorEmail(email string) (*models.Usuario, error)
	BuscarPorCPF(cpf string) (*models.Usuario, error)
	AtualizarPerfil(id uint, campos map[string]any) error
	// AtualizarSenha troca o hash e incrementa token_versao num ÚNICO UPDATE condicionado à
	// versão do token apresentado (`versaoAtual`): só uma de N trocas simultâneas vence.
	// ok=false (nenhuma linha) significa que o token já foi revogado/o usuário não existe.
	AtualizarSenha(id uint, hash string, versaoAtual int) (novaVersao int, ok bool, err error)
	// VersaoToken devolve a versão atual; existe=false se o usuário não existir.
	VersaoToken(id uint) (versao int, existe bool, err error)
	// RevogarTokens incrementa a versão somente se ainda for `versaoAtual` (atômico).
	RevogarTokens(id uint, versaoAtual int) (bool, error)
	Buscar(termo string, excluirID uint, limite int) ([]models.Usuario, error)
}

// usuarioRepository é a implementação concreta usando GORM
type usuarioRepository struct {
	db *gorm.DB
}

// NovoUsuarioRepository cria uma nova instância do repositório de usuários
func NovoUsuarioRepository(db *gorm.DB) UsuarioRepository {
	return &usuarioRepository{db: db}
}

// Criar insere um novo usuário no banco de dados
func (r *usuarioRepository) Criar(usuario *models.Usuario) error {
	return r.db.Create(usuario).Error
}

func (r *usuarioRepository) buscarUm(cond string, arg any) (*models.Usuario, error) {
	var usuario models.Usuario
	err := r.db.Where(cond, arg).First(&usuario).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &usuario, nil
}

// BuscarPorID retorna um usuário pelo seu ID
func (r *usuarioRepository) BuscarPorID(id uint) (*models.Usuario, error) {
	return r.buscarUm("id = ?", id)
}

// BuscarPorEmail retorna um usuário pelo seu e-mail
func (r *usuarioRepository) BuscarPorEmail(email string) (*models.Usuario, error) {
	return r.buscarUm("email = ?", email)
}

// BuscarPorCPF retorna um usuário pelo seu CPF
func (r *usuarioRepository) BuscarPorCPF(cpf string) (*models.Usuario, error) {
	return r.buscarUm("cpf = ?", cpf)
}

// AtualizarPerfil atualiza apenas as colunas informadas (nunca o saldo).
func (r *usuarioRepository) AtualizarPerfil(id uint, campos map[string]any) error {
	if len(campos) == 0 {
		return nil
	}
	return r.db.Model(&models.Usuario{}).Where("id = ?", id).Updates(campos).Error
}

// AtualizarSenha: UPDATE único `... WHERE id = ? AND token_versao = ?`; RowsAffected == 0 => !ok.
func (r *usuarioRepository) AtualizarSenha(id uint, hash string, versaoAtual int) (int, bool, error) {
	res := r.db.Exec("UPDATE usuarios SET senha = ?, token_versao = token_versao + 1 WHERE id = ? AND token_versao = ?",
		hash, id, versaoAtual)
	if res.Error != nil || res.RowsAffected == 0 {
		return 0, false, res.Error
	}
	return versaoAtual + 1, true, nil
}

func (r *usuarioRepository) VersaoToken(id uint) (int, bool, error) {
	var v []int
	if err := r.db.Raw("SELECT token_versao FROM usuarios WHERE id = ?", id).Scan(&v).Error; err != nil || len(v) == 0 {
		return 0, false, err
	}
	return v[0], true, nil
}

func (r *usuarioRepository) RevogarTokens(id uint, versaoAtual int) (bool, error) {
	res := r.db.Exec("UPDATE usuarios SET token_versao = token_versao + 1 WHERE id = ? AND token_versao = ?", id, versaoAtual)
	return res.RowsAffected > 0, res.Error
}

// escapeLike escapa os curingas do LIKE para o termo ser tratado literalmente.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Buscar procura usuários por nome/sobrenome (prefixo de qualquer palavra), exceto excluirID.
func (r *usuarioRepository) Buscar(termo string, excluirID uint, limite int) ([]models.Usuario, error) {
	like := escapeLike(termo) + "%"
	likeMeio := "% " + escapeLike(termo) + "%"
	var usuarios []models.Usuario
	err := r.db.
		Select("id", "nome", "sobrenome").
		Where("id <> ? AND (nome LIKE ? OR sobrenome LIKE ? OR CONCAT(nome, ' ', sobrenome) LIKE ? OR CONCAT(nome, ' ', sobrenome) LIKE ?)",
			excluirID, like, like, like, likeMeio).
		Order("nome ASC, sobrenome ASC").
		Limit(limite).
		Find(&usuarios).Error
	return usuarios, err
}
