package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// DesafioDenuncia é o que a abertura de denúncia precisa saber do desafio (jurisdição e vínculo).
type DesafioDenuncia struct {
	ID         uint
	Titulo     string
	IDAcademia *uint
	// IDUsuarioAcademia é a conta (usuarios.id) da academia dona; nil sem academia.
	IDUsuarioAcademia *uint
}

// ModeracaoConsultaRepository reúne as consultas auxiliares do serviço de moderação que não pertencem
// a um agregado só (vínculo com desafio, travas da linha do usuário, punições vigentes dentro de tx).
// Só leitura (as travas são SELECT ... FOR UPDATE).
type ModeracaoConsultaRepository interface {
	// TravarUsuario trava a linha do usuário (serializa limite de denúncias, bloqueio pelo painel e
	// fim de suspensão). Devolve suspenso_ate atual; existe=false se o usuário não existir.
	TravarUsuario(tx *gorm.DB, id uint) (suspensoAte *time.Time, existe bool, err error)
	// DesafioParaDenuncia devolve o desafio com a conta da academia dona (nil se não existir).
	DesafioParaDenuncia(id uint) (*DesafioDenuncia, error)
	// VinculadoAoDesafio: participante, instrutor designado ou conta da academia dona.
	VinculadoAoDesafio(idDesafio, idUsuario uint) (bool, error)
	// PunicoesVigentes lista (dentro da tx, SEM trava) as punições vigentes do usuário de um tipo. Usar
	// depois de TravarUsuario: a leitura consistente começa após a trava e enxerga o que os concorrentes
	// já confirmaram; a revogação continua sendo CAS (sem travar punições aqui, evitando inverter a ordem
	// punição -> usuário usada por RevogarPunicao).
	PunicoesVigentes(tx *gorm.DB, idUsuario uint, tipo string, agora time.Time) ([]models.Punicao, error)
	// SuspensoesEncerradas lista suspensões não revogadas do usuário com fim em (de, ate] (job fim-suspensao).
	SuspensoesEncerradas(tx *gorm.DB, idUsuario uint, de, ate time.Time) ([]models.Punicao, error)
	// UsuarioDaAcademia devolve a conta (usuarios.id) da academia; nil se a academia não existir.
	UsuarioDaAcademia(idAcademia uint) (*uint, error)
	// UsuariosPorID carrega id, nome, sobrenome, username e email dos usuários pedidos.
	UsuariosPorID(ids []uint) (map[uint]models.Usuario, error)
	// DenunciasPorID carrega denúncias (sem evidências) pelos ids.
	DenunciasPorID(ids []uint) (map[uint]models.Denuncia, error)
	// PunicoesDaDenuncia conta punições geradas pela denúncia.
	PunicoesDaDenuncia(idDenuncia uint) (int64, error)
	// PunicoesPorID carrega punições pelos ids (fila de recursos).
	PunicoesPorID(ids []uint) (map[uint]models.Punicao, error)
	// RecursosDasPunicoes devolve o recurso de cada punição (no máximo um por punição).
	RecursosDasPunicoes(ids []uint) (map[uint]models.Recurso, error)
	// DenunciasDoDenunciado lista as denúncias contra o usuário (mais recentes primeiro), exceto `exceto`.
	DenunciasDoDenunciado(idDenunciado, exceto uint, limite int) ([]models.Denuncia, error)
}

type moderacaoConsultaRepository struct{ db *gorm.DB }

// NovoModeracaoConsultaRepository cria o repositório de consultas auxiliares da moderação.
func NovoModeracaoConsultaRepository(db *gorm.DB) ModeracaoConsultaRepository {
	return &moderacaoConsultaRepository{db}
}

func (r *moderacaoConsultaRepository) TravarUsuario(tx *gorm.DB, id uint) (*time.Time, bool, error) {
	var v []struct{ SuspensoAte *time.Time }
	if err := dbOuTx(r.db, tx).Raw("SELECT suspenso_ate FROM usuarios WHERE id = ? FOR UPDATE", id).Scan(&v).Error; err != nil {
		return nil, false, err
	}
	if len(v) == 0 {
		return nil, false, nil
	}
	return v[0].SuspensoAte, true, nil
}

func (r *moderacaoConsultaRepository) DesafioParaDenuncia(id uint) (*DesafioDenuncia, error) {
	var v []DesafioDenuncia
	err := r.db.Raw(`SELECT d.id, d.titulo, d.id_academia, a.id_usuario AS id_usuario_academia
	                   FROM desafios d LEFT JOIN academias a ON a.id = d.id_academia WHERE d.id = ?`, id).Scan(&v).Error
	if err != nil || len(v) == 0 {
		return nil, err
	}
	return &v[0], nil
}

func (r *moderacaoConsultaRepository) VinculadoAoDesafio(idDesafio, idUsuario uint) (bool, error) {
	var ok bool
	err := r.db.Raw(`SELECT
	    EXISTS(SELECT 1 FROM desafio_participantes p WHERE p.id_desafio = ? AND p.id_usuario = ?)
	 OR EXISTS(SELECT 1 FROM desafios d JOIN instrutores i ON i.id = d.id_instrutor WHERE d.id = ? AND i.id_usuario = ?)
	 OR EXISTS(SELECT 1 FROM desafios d JOIN academias a ON a.id = d.id_academia WHERE d.id = ? AND a.id_usuario = ?)`,
		idDesafio, idUsuario, idDesafio, idUsuario, idDesafio, idUsuario).Row().Scan(&ok)
	return ok, err
}

func (r *moderacaoConsultaRepository) PunicoesVigentes(tx *gorm.DB, idUsuario uint, tipo string, agora time.Time) ([]models.Punicao, error) {
	lista := []models.Punicao{}
	err := dbOuTx(r.db, tx).
		Where("id_usuario = ? AND tipo = ? AND "+condVigente, idUsuario, tipo, agora, agora).
		Order("id ASC").Find(&lista).Error
	return lista, err
}

func (r *moderacaoConsultaRepository) SuspensoesEncerradas(tx *gorm.DB, idUsuario uint, de, ate time.Time) ([]models.Punicao, error) {
	lista := []models.Punicao{}
	err := dbOuTx(r.db, tx).
		Where("id_usuario = ? AND tipo = ? AND revogada_em IS NULL AND fim > ? AND fim <= ?", idUsuario, models.PunicaoSuspensao, de, ate).
		Order("id ASC").Find(&lista).Error
	return lista, err
}

func (r *moderacaoConsultaRepository) UsuarioDaAcademia(idAcademia uint) (*uint, error) {
	var ids []uint
	if err := r.db.Model(&models.Academia{}).Where("id = ?", idAcademia).Limit(1).Pluck("id_usuario", &ids).Error; err != nil || len(ids) == 0 {
		return nil, err
	}
	return &ids[0], nil
}

func (r *moderacaoConsultaRepository) UsuariosPorID(ids []uint) (map[uint]models.Usuario, error) {
	out := map[uint]models.Usuario{}
	if len(ids) == 0 {
		return out, nil
	}
	var us []models.Usuario
	if err := r.db.Select("id", "nome", "sobrenome", "username", "email").Where("id IN ?", ids).Find(&us).Error; err != nil {
		return nil, err
	}
	for _, u := range us {
		out[u.ID] = u
	}
	return out, nil
}

func (r *moderacaoConsultaRepository) DenunciasPorID(ids []uint) (map[uint]models.Denuncia, error) {
	out := map[uint]models.Denuncia{}
	if len(ids) == 0 {
		return out, nil
	}
	var ds []models.Denuncia
	if err := r.db.Where("id IN ?", ids).Find(&ds).Error; err != nil {
		return nil, err
	}
	for _, d := range ds {
		out[d.ID] = d
	}
	return out, nil
}

func (r *moderacaoConsultaRepository) PunicoesDaDenuncia(idDenuncia uint) (int64, error) {
	var n int64
	err := r.db.Model(&models.Punicao{}).Where("id_denuncia = ?", idDenuncia).Count(&n).Error
	return n, err
}

func (r *moderacaoConsultaRepository) PunicoesPorID(ids []uint) (map[uint]models.Punicao, error) {
	out := map[uint]models.Punicao{}
	if len(ids) == 0 {
		return out, nil
	}
	var ps []models.Punicao
	if err := r.db.Where("id IN ?", ids).Find(&ps).Error; err != nil {
		return nil, err
	}
	for _, p := range ps {
		out[p.ID] = p
	}
	return out, nil
}

func (r *moderacaoConsultaRepository) RecursosDasPunicoes(ids []uint) (map[uint]models.Recurso, error) {
	out := map[uint]models.Recurso{}
	if len(ids) == 0 {
		return out, nil
	}
	var rs []models.Recurso
	if err := r.db.Where("id_punicao IN ?", ids).Find(&rs).Error; err != nil {
		return nil, err
	}
	for _, rc := range rs {
		out[rc.IDPunicao] = rc
	}
	return out, nil
}

func (r *moderacaoConsultaRepository) DenunciasDoDenunciado(idDenunciado, exceto uint, limite int) ([]models.Denuncia, error) {
	lista := []models.Denuncia{}
	err := r.db.Where("id_denunciado = ? AND id <> ?", idDenunciado, exceto).Order("id DESC").Limit(limite).Find(&lista).Error
	return lista, err
}
