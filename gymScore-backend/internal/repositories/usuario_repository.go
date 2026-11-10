package repositories

import (
	"errors"
	"strings"
	"time"

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
	// AtualizarUltimaVez grava só `ultima_vez` (UPDATE de uma coluna; heartbeat). Não existe Save da
	// linha inteira: ele regravaria pontos/senha/perfil lidos antes e desfaria UPDATEs concorrentes.
	AtualizarUltimaVez(id uint, em time.Time) error
	AtualizarPerfil(id uint, campos map[string]any) error
	// DefinirDataNascimento grava a data só se ainda for NULL (UPDATE condicional, atômico).
	// ok=false (nenhuma linha) significa que a data já estava definida (ou o usuário não existe).
	DefinirDataNascimento(id uint, nasc time.Time) (ok bool, err error)
	// AtualizarSenha troca o hash e incrementa token_versao num ÚNICO UPDATE condicionado à
	// versão do token apresentado (`versaoAtual`): só uma de N trocas simultâneas vence.
	// ok=false (nenhuma linha) significa que o token já foi revogado/o usuário não existe.
	AtualizarSenha(id uint, hash string, versaoAtual int) (novaVersao int, ok bool, err error)
	// EstadoSessao devolve a versão atual do token e o estado de moderação (banido_em, suspenso_ate);
	// Existe=false se o usuário não existir. Uma única consulta por requisição autenticada.
	EstadoSessao(id uint) (models.EstadoSessao, error)
	// PapeisDoUsuario devolve o tipo da conta (atleta | academia) e os registros de instrutor/academia
	// com status, numa consulta (migration 00020: instrutor é modo da conta de atleta, não perfil).
	// nil se o usuário não existir.
	PapeisDoUsuario(id uint) (*models.PapeisUsuario, error)
	// RevogarTokens incrementa a versão somente se ainda for `versaoAtual` (atômico).
	RevogarTokens(id uint, versaoAtual int) (bool, error)
	// Buscar procura usuários por nome/sobrenome (prefixo), excluindo excluirID — usado para adicionar amigos.
	Buscar(termo string, excluirID uint, limite int) ([]models.Usuario, error)
	// BuscarPorTermo procura por nome, sobrenome ou username (substring, case-insensitive) — busca geral.
	BuscarPorTermo(q string) ([]models.Usuario, error)
	// IncrementarPontos soma delta (pode ser negativo) aos pontos do usuário num UPDATE atômico
	// (`pontos = pontos + ?`), na transação `tx` informada pelo chamador — nunca lê/regrava o valor
	// antigo em memória (evita lost update quando dois eventos pontuam o mesmo usuário em paralelo).
	IncrementarPontos(tx *gorm.DB, id uint, delta int) error
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

// AtualizarUltimaVez: UPDATE usuarios SET ultima_vez = ? WHERE id = ? (sem tocar atualizado_em).
func (r *usuarioRepository) AtualizarUltimaVez(id uint, em time.Time) error {
	return r.db.Exec("UPDATE usuarios SET ultima_vez = ? WHERE id = ?", em, id).Error
}

// AtualizarPerfil atualiza apenas as colunas informadas (nunca o saldo).
func (r *usuarioRepository) AtualizarPerfil(id uint, campos map[string]any) error {
	if len(campos) == 0 {
		return nil
	}
	return r.db.Model(&models.Usuario{}).Where("id = ?", id).Updates(campos).Error
}

// DefinirDataNascimento: `UPDATE ... WHERE id = ? AND data_nascimento IS NULL`; RowsAffected == 0 => !ok.
func (r *usuarioRepository) DefinirDataNascimento(id uint, nasc time.Time) (bool, error) {
	res := r.db.Exec("UPDATE usuarios SET data_nascimento = ? WHERE id = ? AND data_nascimento IS NULL", nasc, id)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
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

func (r *usuarioRepository) EstadoSessao(id uint) (models.EstadoSessao, error) {
	var v []struct {
		TokenVersao int
		BanidoEm    *time.Time
		SuspensoAte *time.Time
	}
	// Estado efetivo da moderação (00013/00019): banido_em e suspenso_ate, recalculados das punições.
	if err := r.db.Raw("SELECT token_versao, banido_em, suspenso_ate FROM usuarios WHERE id = ?", id).Scan(&v).Error; err != nil || len(v) == 0 {
		return models.EstadoSessao{}, err
	}
	return models.EstadoSessao{Versao: v[0].TokenVersao, Existe: true, BanidoEm: v[0].BanidoEm, SuspensoAte: v[0].SuspensoAte}, nil
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

// BuscarPorTermo retorna até 20 usuários cujo nome, sobrenome ou username contenham a string q (case-insensitive)
func (r *usuarioRepository) BuscarPorTermo(q string) ([]models.Usuario, error) {
	var usuarios []models.Usuario
	like := "%" + q + "%"
	err := r.db.Where("LOWER(nome) LIKE LOWER(?) OR LOWER(sobrenome) LIKE LOWER(?) OR LOWER(username) LIKE LOWER(?)", like, like, like).
		Limit(20).Find(&usuarios).Error
	return usuarios, err
}

// IncrementarPontos soma delta aos pontos do usuário num único UPDATE (`pontos = pontos + ?`).
// Deve ser chamado dentro da MESMA transação do evento que concede os pontos (ex.: apuração de
// desafio), para que pontuação e prêmio em dinheiro vivam ou morram juntos.
func (r *usuarioRepository) IncrementarPontos(tx *gorm.DB, id uint, delta int) error {
	return tx.Exec("UPDATE usuarios SET pontos = pontos + ? WHERE id = ?", delta, id).Error
}

func (r *usuarioRepository) PapeisDoUsuario(id uint) (*models.PapeisUsuario, error) {
	var linhas []struct {
		Perfil          string
		InstrutorID     *uint
		InstrutorStatus *string
		InstrutorAcad   *uint
		InstrutorMotivo *string
		AcademiaID      *uint
		AcademiaStatus  *string
	}
	err := r.db.Raw(`SELECT u.perfil, i.id AS instrutor_id, i.status AS instrutor_status,
			i.id_academia AS instrutor_acad, i.motivo_rejeicao AS instrutor_motivo,
			a.id AS academia_id, a.status AS academia_status
		FROM usuarios u
		LEFT JOIN instrutores i ON i.id_usuario = u.id
		LEFT JOIN academias a ON a.id_usuario = u.id
		WHERE u.id = ?`, id).Scan(&linhas).Error
	if err != nil || len(linhas) == 0 {
		return nil, err
	}
	l := linhas[0]
	p := &models.PapeisUsuario{IDUsuario: id, Conta: l.Perfil}
	if l.InstrutorID != nil {
		p.InstrutorID = *l.InstrutorID
	}
	if l.InstrutorStatus != nil {
		p.InstrutorStatus = *l.InstrutorStatus
	}
	if l.InstrutorAcad != nil {
		p.InstrutorAcademiaID = *l.InstrutorAcad
	}
	p.InstrutorMotivoRejeicao = l.InstrutorMotivo
	if l.AcademiaID != nil {
		p.AcademiaID = *l.AcademiaID
	}
	if l.AcademiaStatus != nil {
		p.AcademiaStatus = *l.AcademiaStatus
	}
	return p, nil
}
