package repositories

import (
	"errors"
	"strings"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// VinculoInstrutorRepository cuida do vínculo academia <-> instrutor (instrutores.id_academia) e do
// cadastro de instrutores. Os métodos operam sobre r.db; dentro de uma transação use ComTx(tx).
type VinculoInstrutorRepository struct {
	db *gorm.DB
}

// NovoVinculoInstrutorRepository cria o repositório de vínculos de instrutores.
func NovoVinculoInstrutorRepository(db *gorm.DB) *VinculoInstrutorRepository {
	return &VinculoInstrutorRepository{db: db}
}

// ComTx devolve um repositório que opera dentro da transação informada.
func (r *VinculoInstrutorRepository) ComTx(tx *gorm.DB) *VinculoInstrutorRepository {
	return &VinculoInstrutorRepository{db: tx}
}

// Criar insere o registro de instrutor.
func (r *VinculoInstrutorRepository) Criar(i *models.Instrutor) error {
	return r.db.Create(i).Error
}

// ListarTodos devolve o diretório de instrutores (com usuário), por nome.
func (r *VinculoInstrutorRepository) ListarTodos() ([]models.Instrutor, error) {
	var lista []models.Instrutor
	err := r.db.Preload("Usuario").
		Joins("JOIN usuarios u ON u.id = instrutores.id_usuario").
		Order("u.nome ASC, instrutores.id ASC").Find(&lista).Error
	return lista, err
}

// ListarPorAcademia devolve uma página dos instrutores vinculados à academia (com usuário) e o total.
func (r *VinculoInstrutorRepository) ListarPorAcademia(idAcademia uint, pagina, limite int) ([]models.Instrutor, int64, error) {
	q := r.db.Model(&models.Instrutor{}).Where("instrutores.id_academia = ?", idAcademia)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var lista []models.Instrutor
	err := q.Preload("Usuario").
		Joins("JOIN usuarios u ON u.id = instrutores.id_usuario").
		Order("u.nome ASC, instrutores.id ASC").
		Offset((pagina - 1) * limite).Limit(limite).Find(&lista).Error
	return lista, total, err
}

// ContarPorAcademia conta os instrutores vinculados à academia.
func (r *VinculoInstrutorRepository) ContarPorAcademia(idAcademia uint) (int64, error) {
	var total int64
	err := r.db.Model(&models.Instrutor{}).Where("id_academia = ?", idAcademia).Count(&total).Error
	return total, err
}

// BuscarPorEmail devolve o instrutor (com usuário) cuja conta tem esse e-mail, ou nil.
func (r *VinculoInstrutorRepository) BuscarPorEmail(email string) (*models.Instrutor, error) {
	var i models.Instrutor
	err := r.db.Preload("Usuario").
		Joins("JOIN usuarios u ON u.id = instrutores.id_usuario").
		Where("u.email = ?", strings.TrimSpace(email)).First(&i).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &i, err
}

// TravarPorID lê o instrutor (com usuário) com SELECT ... FOR UPDATE. Só faz sentido numa transação.
func (r *VinculoInstrutorRepository) TravarPorID(id uint) (*models.Instrutor, error) {
	var i models.Instrutor
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&i, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var u models.Usuario
	if err := r.db.First(&u, i.IDUsuario).Error; err == nil {
		i.Usuario = &u
	}
	return &i, nil
}

// Vincular liga o instrutor à academia só se ele estiver sem vínculo (UPDATE condicional atômico).
func (r *VinculoInstrutorRepository) Vincular(idInstrutor, idAcademia uint) (bool, error) {
	res := r.db.Model(&models.Instrutor{}).Where("id = ? AND id_academia IS NULL", idInstrutor).
		Update("id_academia", idAcademia)
	return res.RowsAffected > 0, res.Error
}

// Desvincular remove o vínculo só se o instrutor ainda for DESTA academia.
func (r *VinculoInstrutorRepository) Desvincular(idInstrutor, idAcademia uint) (bool, error) {
	res := r.db.Model(&models.Instrutor{}).Where("id = ? AND id_academia = ?", idInstrutor, idAcademia).
		Update("id_academia", nil)
	return res.RowsAffected > 0, res.Error
}

// Convidar grava o convite da academia, só se o instrutor estiver sem vínculo e sem convite pendente de
// OUTRA academia (convite repetido da mesma academia renova convite_em). UPDATE condicional atômico.
func (r *VinculoInstrutorRepository) Convidar(idInstrutor, idAcademia uint, em time.Time) (bool, error) {
	res := r.db.Model(&models.Instrutor{}).
		Where("id = ? AND id_academia IS NULL AND (id_academia_convite IS NULL OR id_academia_convite = ?)", idInstrutor, idAcademia).
		Updates(map[string]any{"id_academia_convite": idAcademia, "convite_em": em})
	return res.RowsAffected > 0, res.Error
}

// AceitarConvite transforma o convite pendente desta academia em vínculo (id_academia) e limpa o convite,
// só se o instrutor ainda estiver sem vínculo.
func (r *VinculoInstrutorRepository) AceitarConvite(idInstrutor, idAcademia uint) (bool, error) {
	res := r.db.Model(&models.Instrutor{}).
		Where("id = ? AND id_academia IS NULL AND id_academia_convite = ?", idInstrutor, idAcademia).
		Updates(map[string]any{"id_academia": idAcademia, "id_academia_convite": nil, "convite_em": nil})
	return res.RowsAffected > 0, res.Error
}

// LimparConvite apaga o convite pendente desta academia (recusa do instrutor ou cancelamento pela academia).
func (r *VinculoInstrutorRepository) LimparConvite(idInstrutor, idAcademia uint) (bool, error) {
	res := r.db.Model(&models.Instrutor{}).
		Where("id = ? AND id_academia_convite = ?", idInstrutor, idAcademia).
		Updates(map[string]any{"id_academia_convite": nil, "convite_em": nil})
	return res.RowsAffected > 0, res.Error
}

// ListarConvitesDaAcademia devolve os instrutores (com usuário) com convite pendente desta academia,
// do mais recente para o mais antigo.
func (r *VinculoInstrutorRepository) ListarConvitesDaAcademia(idAcademia uint) ([]models.Instrutor, error) {
	var lista []models.Instrutor
	err := r.db.Preload("Usuario").Where("id_academia_convite = ?", idAcademia).
		Order("convite_em DESC, id DESC").Find(&lista).Error
	return lista, err
}

// DesafiosAtivosDe devolve (id e título) dos desafios abertos ou em andamento em que o instrutor é
// membro da equipe (rodada 5: qualquer origem), no máximo `limite`.
func (r *VinculoInstrutorRepository) DesafiosAtivosDe(idInstrutor uint, limite int) ([]models.Desafio, error) {
	var ds []models.Desafio
	err := r.db.Select("id", "titulo", "status").
		Where("id IN (SELECT di.id_desafio FROM desafio_instrutores di WHERE di.id_instrutor = ?) AND status IN ?", idInstrutor,
			[]models.StatusDesafio{models.StatusAberto, models.StatusEmAndamento}).
		Order("id ASC").Limit(limite).Find(&ds).Error
	return ds, err
}

// ContarDesafiosAtivos devolve, por id de instrutor, em quantos desafios abertos/em andamento ele está
// na equipe.
func (r *VinculoInstrutorRepository) ContarDesafiosAtivos(ids []uint) (map[uint]int, error) {
	out := make(map[uint]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var linhas []struct {
		IDInstrutor uint
		Total       int
	}
	err := r.db.Table("desafio_instrutores di").Select("di.id_instrutor, COUNT(*) AS total").
		Joins("JOIN desafios d ON d.id = di.id_desafio").
		Where("di.id_instrutor IN ? AND d.status IN ?", ids,
			[]models.StatusDesafio{models.StatusAberto, models.StatusEmAndamento}).
		Group("di.id_instrutor").Scan(&linhas).Error
	for _, l := range linhas {
		out[l.IDInstrutor] = l.Total
	}
	return out, err
}

// UsuarioDaAcademia devolve o id do usuário dono da academia (0 se não existir).
func (r *VinculoInstrutorRepository) UsuarioDaAcademia(idAcademia uint) (uint, error) {
	var ids []uint
	err := r.db.Model(&models.Academia{}).Where("id = ?", idAcademia).Pluck("id_usuario", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return ids[0], nil
}

// NomeAcademia devolve o nome fantasia da academia ("" se não existir).
func (r *VinculoInstrutorRepository) NomeAcademia(idAcademia uint) (string, error) {
	var nomes []string
	err := r.db.Model(&models.Academia{}).Where("id = ?", idAcademia).Pluck("nome_fantasia", &nomes).Error
	if err != nil || len(nomes) == 0 {
		return "", err
	}
	return nomes[0], nil
}
