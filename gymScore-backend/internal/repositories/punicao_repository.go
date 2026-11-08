package repositories

import (
	"errors"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EstadoConta é o estado efetivo do usuário derivado das punições vigentes (colunas de `usuarios`).
type EstadoConta struct {
	BanidoEm       *time.Time
	SuspensoAte    *time.Time
	MotivoBloqueio *string
}

// FiltroPunicoes parametriza a listagem de punições (página >= 1, limite 1..100).
// Vigentes = só não revogadas e não vencidas em `Agora` (obrigatório quando Vigentes = true).
type FiltroPunicoes struct {
	IDUsuario  *uint
	IDAcademia *uint
	IDDesafio  *uint
	IDDenuncia *uint
	Tipo       string
	Escopo     string
	Vigentes   bool
	Agora      time.Time
	Pagina     int
	Limite     int
}

// PunicaoRepository acessa punições e recursos. Punições não são apagadas nem editadas: só INSERT e
// revogação (UPDATE condicional). O estado efetivo em `usuarios` (banido_em/suspenso_ate/motivo_bloqueio)
// é SEMPRE recalculado a partir das punições vigentes por RecalcularEstadoConta, na mesma transação.
type PunicaoRepository interface {
	// Aplicar insere a punição (Inicio zero = agora no banco). Os CHECKs do schema validam escopo,
	// aplicador (academia × admin), fim de suspensão e anulação com desafio.
	Aplicar(tx *gorm.DB, p *models.Punicao) error
	// Buscar devolve a punição (nil se não existir). travar = FOR UPDATE.
	Buscar(tx *gorm.DB, id uint, travar bool) (*models.Punicao, error)
	// Revogar marca revogada (só admin). false se não existe ou já estava revogada.
	Revogar(tx *gorm.DB, id, idAdmin uint, motivo string, em time.Time) (bool, error)
	// RecalcularEstadoConta trava a linha do usuário, recalcula banido_em (banimento vigente mais antigo),
	// suspenso_ate (maior fim entre suspensões vigentes) e motivo_bloqueio (justificativa do banimento ou
	// da suspensão que define o fim, até 255 caracteres) e grava. Se o estado ENDURECEU (banimento novo ou
	// suspensão mais longa), incrementa token_versao no mesmo UPDATE (derruba as sessões).
	RecalcularEstadoConta(tx *gorm.DB, idUsuario uint, agora time.Time) (estado EstadoConta, endureceu bool, err error)
	// UsuariosComSuspensaoVencida devolve ids com suspenso_ate <= agora (job fim-suspensao).
	UsuariosComSuspensaoVencida(agora time.Time, limite int) ([]uint, error)
	// TemBloqueioAcademia diz se há bloqueio_academia vigente do usuário na academia.
	TemBloqueioAcademia(tx *gorm.DB, idUsuario, idAcademia uint, agora time.Time) (bool, error)
	// AnulacaoDoDesafio devolve a punição anulacao_resultado não revogada do desafio (nil se não houver).
	AnulacaoDoDesafio(tx *gorm.DB, idDesafio uint) (*models.Punicao, error)
	// ContarDoUsuarioDesde conta punições de um tipo desde `desde` (reincidência).
	ContarDoUsuarioDesde(idUsuario uint, tipo string, desde time.Time) (int64, error)
	Listar(f FiltroPunicoes) ([]models.Punicao, int64, error)

	// CriarRecurso insere o recurso. 1062 = já existe recurso para a punição (uq_recurso_punicao).
	CriarRecurso(tx *gorm.DB, rc *models.Recurso) error
	// BuscarRecurso devolve o recurso (nil se não existir). travar = FOR UPDATE.
	BuscarRecurso(tx *gorm.DB, id uint, travar bool) (*models.Recurso, error)
	// RecursoDaPunicao devolve o recurso da punição (nil se não houver).
	RecursoDaPunicao(tx *gorm.DB, idPunicao uint) (*models.Recurso, error)
	// JulgarRecurso: pendente -> deferido | indeferido (só admin). false se não estava pendente.
	JulgarRecurso(tx *gorm.DB, id uint, status string, idAdmin uint, texto string, em time.Time) (bool, error)
	// ListarRecursos devolve a fila (status vazio = todos), mais antigos primeiro.
	ListarRecursos(status string, pagina, limite int) ([]models.Recurso, int64, error)
}

type punicaoRepository struct{ db *gorm.DB }

// NovoPunicaoRepository cria o repositório de punições e recursos.
func NovoPunicaoRepository(db *gorm.DB) PunicaoRepository { return &punicaoRepository{db} }

const condVigente = "revogada_em IS NULL AND inicio <= ? AND (fim IS NULL OR fim > ?)"

func (r *punicaoRepository) Aplicar(tx *gorm.DB, p *models.Punicao) error {
	q := dbOuTx(r.db, tx)
	if p.Inicio.IsZero() {
		q = q.Omit("Inicio") // DEFAULT CURRENT_TIMESTAMP(3)
		if err := q.Create(p).Error; err != nil {
			return err
		}
		return dbOuTx(r.db, tx).Raw("SELECT inicio FROM punicoes WHERE id = ?", p.ID).Row().Scan(&p.Inicio)
	}
	return q.Create(p).Error
}

func (r *punicaoRepository) Buscar(tx *gorm.DB, id uint, travar bool) (*models.Punicao, error) {
	q := dbOuTx(r.db, tx)
	if travar {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var p models.Punicao
	err := q.First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

func (r *punicaoRepository) Revogar(tx *gorm.DB, id, idAdmin uint, motivo string, em time.Time) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE punicoes SET revogada_em = ?, revogada_por = ?, revogada_motivo = ? WHERE id = ? AND revogada_em IS NULL",
		em, idAdmin, motivo, id))
}

func cortarMotivo(s string) *string {
	rs := []rune(s)
	if len(rs) > models.MotivoBloqueioMaxCaracter {
		rs = rs[:models.MotivoBloqueioMaxCaracter]
	}
	out := string(rs)
	return &out
}

func (r *punicaoRepository) RecalcularEstadoConta(tx *gorm.DB, idUsuario uint, agora time.Time) (EstadoConta, bool, error) {
	db := dbOuTx(r.db, tx)
	var atual []struct {
		BanidoEm    *time.Time
		SuspensoAte *time.Time
	}
	if err := db.Raw("SELECT banido_em, suspenso_ate FROM usuarios WHERE id = ? FOR UPDATE", idUsuario).Scan(&atual).Error; err != nil {
		return EstadoConta{}, false, err
	}
	if len(atual) == 0 {
		return EstadoConta{}, false, gorm.ErrRecordNotFound
	}
	var ban, susp []models.Punicao
	if err := db.Where("id_usuario = ? AND tipo = ? AND "+condVigente, idUsuario, models.PunicaoBanimento, agora, agora).
		Order("inicio ASC, id ASC").Limit(1).Find(&ban).Error; err != nil {
		return EstadoConta{}, false, err
	}
	if err := db.Where("id_usuario = ? AND tipo = ? AND "+condVigente, idUsuario, models.PunicaoSuspensao, agora, agora).
		Order("fim DESC, id DESC").Limit(1).Find(&susp).Error; err != nil {
		return EstadoConta{}, false, err
	}
	var novo EstadoConta
	if len(susp) == 1 {
		// usuarios.suspenso_ate é DATETIME sem fração (schema do RDS): arredonda para cima, nunca encurta.
		fim := susp[0].Fim.Truncate(time.Second)
		if fim.Before(*susp[0].Fim) {
			fim = fim.Add(time.Second)
		}
		novo.SuspensoAte = &fim
		novo.MotivoBloqueio = cortarMotivo(susp[0].Justificativa)
	}
	if len(ban) == 1 {
		inicio := ban[0].Inicio.Truncate(time.Second)
		novo.BanidoEm = &inicio
		novo.MotivoBloqueio = cortarMotivo(ban[0].Justificativa)
	}
	velho := atual[0]
	endureceu := (velho.BanidoEm == nil && novo.BanidoEm != nil) ||
		(novo.SuspensoAte != nil && (velho.SuspensoAte == nil || novo.SuspensoAte.After(*velho.SuspensoAte)))
	incremento := 0
	if endureceu {
		incremento = 1
	}
	err := db.Exec("UPDATE usuarios SET banido_em = ?, suspenso_ate = ?, motivo_bloqueio = ?, token_versao = token_versao + ? WHERE id = ?",
		novo.BanidoEm, novo.SuspensoAte, novo.MotivoBloqueio, incremento, idUsuario).Error
	return novo, endureceu, err
}

func (r *punicaoRepository) UsuariosComSuspensaoVencida(agora time.Time, limite int) ([]uint, error) {
	if limite < 1 {
		limite = 100
	}
	var ids []uint
	err := r.db.Model(&models.Usuario{}).Where("suspenso_ate <= ?", agora).Order("suspenso_ate ASC").Limit(limite).Pluck("id", &ids).Error
	return ids, err
}

func (r *punicaoRepository) TemBloqueioAcademia(tx *gorm.DB, idUsuario, idAcademia uint, agora time.Time) (bool, error) {
	var n int64
	err := dbOuTx(r.db, tx).Model(&models.Punicao{}).
		Where("id_academia = ? AND id_usuario = ? AND tipo = ? AND "+condVigente, idAcademia, idUsuario, models.PunicaoBloqueioAcademia, agora, agora).
		Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *punicaoRepository) AnulacaoDoDesafio(tx *gorm.DB, idDesafio uint) (*models.Punicao, error) {
	var p []models.Punicao
	err := dbOuTx(r.db, tx).Where("id_desafio = ? AND tipo = ? AND revogada_em IS NULL", idDesafio, models.PunicaoAnulacaoResultado).
		Order("id DESC").Limit(1).Find(&p).Error
	if err != nil || len(p) == 0 {
		return nil, err
	}
	return &p[0], nil
}

func (r *punicaoRepository) ContarDoUsuarioDesde(idUsuario uint, tipo string, desde time.Time) (int64, error) {
	var n int64
	err := r.db.Model(&models.Punicao{}).Where("id_usuario = ? AND tipo = ? AND inicio >= ?", idUsuario, tipo, desde).Count(&n).Error
	return n, err
}

func (r *punicaoRepository) Listar(f FiltroPunicoes) ([]models.Punicao, int64, error) {
	q := r.db.Model(&models.Punicao{})
	if f.IDUsuario != nil {
		q = q.Where("id_usuario = ?", *f.IDUsuario)
	}
	if f.IDAcademia != nil {
		q = q.Where("id_academia = ?", *f.IDAcademia)
	}
	if f.IDDesafio != nil {
		q = q.Where("id_desafio = ?", *f.IDDesafio)
	}
	if f.IDDenuncia != nil {
		q = q.Where("id_denuncia = ?", *f.IDDenuncia)
	}
	if f.Tipo != "" {
		q = q.Where("tipo = ?", f.Tipo)
	}
	if f.Escopo != "" {
		q = q.Where("escopo = ?", f.Escopo)
	}
	if f.Vigentes {
		q = q.Where(condVigente, f.Agora, f.Agora)
	}
	return paginarContando[models.Punicao](q, f.Pagina, f.Limite, "id DESC", nil)
}

func (r *punicaoRepository) CriarRecurso(tx *gorm.DB, rc *models.Recurso) error {
	return dbOuTx(r.db, tx).Create(rc).Error
}

func (r *punicaoRepository) BuscarRecurso(tx *gorm.DB, id uint, travar bool) (*models.Recurso, error) {
	q := dbOuTx(r.db, tx)
	if travar {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rc models.Recurso
	err := q.First(&rc, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rc, err
}

func (r *punicaoRepository) RecursoDaPunicao(tx *gorm.DB, idPunicao uint) (*models.Recurso, error) {
	var rc []models.Recurso
	err := dbOuTx(r.db, tx).Where("id_punicao = ?", idPunicao).Limit(1).Find(&rc).Error
	if err != nil || len(rc) == 0 {
		return nil, err
	}
	return &rc[0], nil
}

func (r *punicaoRepository) JulgarRecurso(tx *gorm.DB, id uint, status string, idAdmin uint, texto string, em time.Time) (bool, error) {
	if status != models.RecursoDeferido && status != models.RecursoIndeferido {
		return false, gorm.ErrInvalidValue
	}
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE recursos SET status = ?, julgado_por = ?, julgado_em = ?, decisao_texto = ? WHERE id = ? AND status = ?",
		status, idAdmin, em, texto, id, models.RecursoPendente))
}

func (r *punicaoRepository) ListarRecursos(status string, pagina, limite int) ([]models.Recurso, int64, error) {
	q := r.db.Model(&models.Recurso{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return paginarContando[models.Recurso](q, pagina, limite, "criado_em ASC, id ASC", nil)
}
