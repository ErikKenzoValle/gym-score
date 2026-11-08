package repositories

import (
	"errors"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Paginação das listagens do painel admin.
const (
	adminLimitePadrao = 20
	adminLimiteMaximo = 100
)

// paginarAdmin normaliza página (>= 1) e limite (1..100, padrão 20) e devolve o offset.
func paginarAdmin(pagina, limite int) (offset, lim int) {
	if pagina < 1 {
		pagina = 1
	}
	if limite < 1 {
		limite = adminLimitePadrao
	}
	if limite > adminLimiteMaximo {
		limite = adminLimiteMaximo
	}
	return (pagina - 1) * limite, limite
}

// dbOuTx devolve tx quando informado; senão o db do repositório.
func dbOuTx(db, tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return db
}

// ─── Administradores ─────────────────────────────────────────────────────────

// FiltroAdmins parametriza a listagem de administradores.
type FiltroAdmins struct {
	Papel  string // vazio = todos
	Ativo  *bool  // nil = todos
	Pagina int
	Limite int
}

// AdminRepository acesso a dados da tabela `administradores`.
// Métodos de escrita recebem `tx` (pode ser nil) para rodarem na mesma transação da auditoria.
// Toda mudança que afeta a autorização (ativo, papel, senha) incrementa token_versao no MESMO
// UPDATE, revogando os tokens já emitidos.
type AdminRepository interface {
	Criar(tx *gorm.DB, a *models.Administrador) error
	BuscarPorID(id uint) (*models.Administrador, error)
	BuscarPorEmail(email string) (*models.Administrador, error)
	// TravarPorID lê com SELECT ... FOR UPDATE (usar dentro de transação). nil se não existir.
	TravarPorID(tx *gorm.DB, id uint) (*models.Administrador, error)
	Listar(f FiltroAdmins) ([]models.Administrador, int64, error)
	// ContarSuperadminsAtivos conta superadmins ativos travando essas linhas (FOR UPDATE), para
	// que duas operações concorrentes não removam/rebaixem juntas o último superadmin.
	ContarSuperadminsAtivos(tx *gorm.DB) (int64, error)
	// AlterarAtivo muda `ativo` e incrementa token_versao. false se o admin não existe ou já estava assim.
	AlterarAtivo(tx *gorm.DB, id uint, ativo bool) (bool, error)
	// AlterarPapel muda `papel` e incrementa token_versao. false se não existe ou já estava assim.
	AlterarPapel(tx *gorm.DB, id uint, papel string) (bool, error)
	// AlterarSenha grava o novo hash e incrementa token_versao. false se o admin não existe.
	AlterarSenha(tx *gorm.DB, id uint, hash string) (bool, error)
	// RevogarTokens incrementa token_versao (logout global do admin).
	RevogarTokens(tx *gorm.DB, id uint) error
	RegistrarLogin(tx *gorm.DB, id uint, em time.Time) error
	// EstadoToken devolve o necessário para validar um JWT admin: versão, ativo e papel.
	// existe=false se o admin não existir.
	EstadoToken(id uint) (versao int, ativo bool, papel string, existe bool, err error)
}

type adminRepository struct{ db *gorm.DB }

// NovoAdminRepository cria uma nova instância do repositório de administradores
func NovoAdminRepository(db *gorm.DB) AdminRepository { return &adminRepository{db} }

func (r *adminRepository) Criar(tx *gorm.DB, a *models.Administrador) error {
	return dbOuTx(r.db, tx).Create(a).Error
}

func (r *adminRepository) buscarUm(db *gorm.DB, cond string, arg any) (*models.Administrador, error) {
	var a models.Administrador
	err := db.Where(cond, arg).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *adminRepository) BuscarPorID(id uint) (*models.Administrador, error) {
	return r.buscarUm(r.db, "id = ?", id)
}

func (r *adminRepository) BuscarPorEmail(email string) (*models.Administrador, error) {
	return r.buscarUm(r.db, "email = ?", email)
}

func (r *adminRepository) TravarPorID(tx *gorm.DB, id uint) (*models.Administrador, error) {
	return r.buscarUm(dbOuTx(r.db, tx).Clauses(clause.Locking{Strength: "UPDATE"}), "id = ?", id)
}

func (r *adminRepository) Listar(f FiltroAdmins) ([]models.Administrador, int64, error) {
	q := r.db.Model(&models.Administrador{})
	if f.Papel != "" {
		q = q.Where("papel = ?", f.Papel)
	}
	if f.Ativo != nil {
		q = q.Where("ativo = ?", *f.Ativo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limite := paginarAdmin(f.Pagina, f.Limite)
	var lista []models.Administrador
	err := q.Order("id ASC").Offset(offset).Limit(limite).Find(&lista).Error
	return lista, total, err
}

func (r *adminRepository) ContarSuperadminsAtivos(tx *gorm.DB) (int64, error) {
	var ids []uint
	err := dbOuTx(r.db, tx).Model(&models.Administrador{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("papel = ? AND ativo = 1", models.PapelSuperadmin).
		Pluck("id", &ids).Error
	return int64(len(ids)), err
}

func (r *adminRepository) AlterarAtivo(tx *gorm.DB, id uint, ativo bool) (bool, error) {
	res := dbOuTx(r.db, tx).Exec(
		"UPDATE administradores SET ativo = ?, token_versao = token_versao + 1 WHERE id = ? AND ativo <> ?",
		ativo, id, ativo)
	return res.RowsAffected > 0, res.Error
}

func (r *adminRepository) AlterarPapel(tx *gorm.DB, id uint, papel string) (bool, error) {
	res := dbOuTx(r.db, tx).Exec(
		"UPDATE administradores SET papel = ?, token_versao = token_versao + 1 WHERE id = ? AND papel <> ?",
		papel, id, papel)
	return res.RowsAffected > 0, res.Error
}

func (r *adminRepository) AlterarSenha(tx *gorm.DB, id uint, hash string) (bool, error) {
	res := dbOuTx(r.db, tx).Exec(
		"UPDATE administradores SET senha_hash = ?, token_versao = token_versao + 1 WHERE id = ?", hash, id)
	return res.RowsAffected > 0, res.Error
}

func (r *adminRepository) RevogarTokens(tx *gorm.DB, id uint) error {
	return dbOuTx(r.db, tx).Exec("UPDATE administradores SET token_versao = token_versao + 1 WHERE id = ?", id).Error
}

func (r *adminRepository) RegistrarLogin(tx *gorm.DB, id uint, em time.Time) error {
	return dbOuTx(r.db, tx).Model(&models.Administrador{}).Where("id = ?", id).UpdateColumn("ultimo_login_em", em).Error
}

func (r *adminRepository) EstadoToken(id uint) (int, bool, string, bool, error) {
	var linhas []struct {
		TokenVersao int
		Ativo       bool
		Papel       string
	}
	err := r.db.Raw("SELECT token_versao, ativo, papel FROM administradores WHERE id = ?", id).Scan(&linhas).Error
	if err != nil || len(linhas) == 0 {
		return 0, false, "", false, err
	}
	l := linhas[0]
	return l.TokenVersao, l.Ativo, l.Papel, true, nil
}

// ─── Auditoria ───────────────────────────────────────────────────────────────

// FiltroAuditoria parametriza a listagem da auditoria. Campos zerados = sem filtro.
type FiltroAuditoria struct {
	IDAdmin  uint
	Acao     string
	AlvoTipo string
	AlvoID   uint
	De       *time.Time // criado_em >= De
	Ate      *time.Time // criado_em <  Ate
	Pagina   int
	Limite   int
}

// AuditoriaRepository é append-only: só insere e lista (não há update/delete).
type AuditoriaRepository interface {
	// Registrar grava a linha na transação `tx` da ação auditada (tx nil só fora de transação).
	Registrar(tx *gorm.DB, a *models.AdminAuditoria) error
	// Listar devolve a página (mais recentes primeiro) com o admin carregado (id, nome, email, papel).
	Listar(f FiltroAuditoria) ([]models.AdminAuditoria, int64, error)
}

type auditoriaRepository struct{ db *gorm.DB }

// NovoAuditoriaRepository cria uma nova instância do repositório de auditoria
func NovoAuditoriaRepository(db *gorm.DB) AuditoriaRepository { return &auditoriaRepository{db} }

func (r *auditoriaRepository) Registrar(tx *gorm.DB, a *models.AdminAuditoria) error {
	if a.Motivo == "" {
		return errors.New("auditoria: motivo é obrigatório")
	}
	return dbOuTx(r.db, tx).Omit("Admin").Create(a).Error
}

func (r *auditoriaRepository) Listar(f FiltroAuditoria) ([]models.AdminAuditoria, int64, error) {
	q := r.db.Model(&models.AdminAuditoria{})
	if f.IDAdmin != 0 {
		q = q.Where("id_admin = ?", f.IDAdmin)
	}
	if f.Acao != "" {
		q = q.Where("acao = ?", f.Acao)
	}
	if f.AlvoTipo != "" {
		q = q.Where("alvo_tipo = ?", f.AlvoTipo)
	}
	if f.AlvoID != 0 {
		q = q.Where("alvo_id = ?", f.AlvoID)
	}
	if f.De != nil {
		q = q.Where("criado_em >= ?", *f.De)
	}
	if f.Ate != nil {
		q = q.Where("criado_em < ?", *f.Ate)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, limite := paginarAdmin(f.Pagina, f.Limite)
	var lista []models.AdminAuditoria
	err := q.Preload("Admin", func(db *gorm.DB) *gorm.DB { return db.Select("id", "nome", "email", "papel") }).
		Order("criado_em DESC, id DESC").Offset(offset).Limit(limite).Find(&lista).Error
	return lista, total, err
}
