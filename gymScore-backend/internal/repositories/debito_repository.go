package repositories

import (
	"fmt"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Abatimento descreve um desconto de débito feito por AbaterDebitos.
type Abatimento struct {
	IDDebito    uint
	IDDesafio   uint
	Valor       int64 // centavos abatidos (positivo)
	Quitado     bool
	IDTransacao uint // linha abatimento_debito do ledger
}

// DebitoRepository acessa usuario_debitos. Com débito aberto o saque é bloqueado (checar com
// TotalAberto DENTRO da transação do saque, com a linha do usuário travada).
type DebitoRepository interface {
	// Criar registra o débito (valor = saldo_restante = o que faltou estornar).
	Criar(tx *gorm.DB, idUsuario, idDesafio uint, valor int64, motivo string) (*models.UsuarioDebito, error)
	// TotalAberto soma o saldo restante dos débitos abertos do usuário, travando-os (FOR UPDATE).
	TotalAberto(tx *gorm.DB, idUsuario uint) (int64, error)
	// AbaterDebitos desconta do SALDO do usuário até `limite` centavos (o crédito que acabou de entrar),
	// abatendo os débitos mais antigos primeiro: para cada um grava `abatimento_debito` (negativo, com o
	// id_desafio do débito) via CarteiraRepository.Debitar e reduz saldo_restante (quitado_em quando 0).
	// Tudo na transação `tx` do crédito. Sem débito aberto ou limite <= 0 = nada.
	AbaterDebitos(tx *gorm.DB, carteira *CarteiraRepository, idUsuario uint, limite int64, agora time.Time) ([]Abatimento, error)
	// DoUsuario lista os débitos do usuário (abertos primeiro).
	DoUsuario(idUsuario uint) ([]models.UsuarioDebito, error)
	// Listar devolve uma página (abertos = só quitado_em NULL).
	Listar(abertos bool, idUsuario *uint, pagina, limite int) ([]models.UsuarioDebito, int64, error)
}

type debitoRepository struct{ db *gorm.DB }

// NovoDebitoRepository cria o repositório de débitos de usuário.
func NovoDebitoRepository(db *gorm.DB) DebitoRepository { return &debitoRepository{db} }

func (r *debitoRepository) Criar(tx *gorm.DB, idUsuario, idDesafio uint, valor int64, motivo string) (*models.UsuarioDebito, error) {
	if valor <= 0 {
		return nil, fmt.Errorf("valor de débito deve ser positivo")
	}
	d := &models.UsuarioDebito{IDUsuario: idUsuario, IDDesafio: idDesafio, ValorCentavos: valor,
		SaldoRestanteCentavos: valor, Motivo: motivo}
	return d, dbOuTx(r.db, tx).Create(d).Error
}

func (r *debitoRepository) TotalAberto(tx *gorm.DB, idUsuario uint) (int64, error) {
	var restantes []int64
	err := dbOuTx(r.db, tx).Model(&models.UsuarioDebito{}).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id_usuario = ? AND quitado_em IS NULL", idUsuario).Pluck("saldo_restante_centavos", &restantes).Error
	var total int64
	for _, v := range restantes {
		total += v
	}
	return total, err
}

func (r *debitoRepository) AbaterDebitos(tx *gorm.DB, carteira *CarteiraRepository, idUsuario uint, limite int64, agora time.Time) ([]Abatimento, error) {
	if limite <= 0 {
		return nil, nil
	}
	db := dbOuTx(r.db, tx)
	var abertos []models.UsuarioDebito
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id_usuario = ? AND quitado_em IS NULL", idUsuario).Order("id ASC").Find(&abertos).Error; err != nil {
		return nil, err
	}
	out := []Abatimento{}
	for _, d := range abertos {
		if limite <= 0 {
			break
		}
		valor := min(d.SaldoRestanteCentavos, limite)
		idDesafio := d.IDDesafio
		t, err := carteira.Debitar(db, Lancamento{IDUsuario: idUsuario, Tipo: models.TipoAbatimentoDebito, ValorCentavos: valor,
			IDDesafio: &idDesafio, Descricao: fmt.Sprintf("Abatimento do débito #%d", d.ID)})
		if err != nil {
			return out, err
		}
		quitado := valor == d.SaldoRestanteCentavos
		var quitadoEm *time.Time
		if quitado {
			quitadoEm = &agora
		}
		res := db.Exec(`UPDATE usuario_debitos SET saldo_restante_centavos = saldo_restante_centavos - ?, quitado_em = ?
			WHERE id = ? AND quitado_em IS NULL AND saldo_restante_centavos >= ?`, valor, quitadoEm, d.ID, valor)
		if res.Error != nil {
			return out, res.Error
		}
		if res.RowsAffected == 0 {
			return out, fmt.Errorf("débito %d mudou durante o abatimento", d.ID)
		}
		limite -= valor
		out = append(out, Abatimento{IDDebito: d.ID, IDDesafio: d.IDDesafio, Valor: valor, Quitado: quitado, IDTransacao: t.ID})
	}
	return out, nil
}

func (r *debitoRepository) DoUsuario(idUsuario uint) ([]models.UsuarioDebito, error) {
	lista := []models.UsuarioDebito{}
	err := r.db.Where("id_usuario = ?", idUsuario).Order("quitado_em IS NOT NULL, id ASC").Find(&lista).Error
	return lista, err
}

func (r *debitoRepository) Listar(abertos bool, idUsuario *uint, pagina, limite int) ([]models.UsuarioDebito, int64, error) {
	q := r.db.Model(&models.UsuarioDebito{})
	if abertos {
		q = q.Where("quitado_em IS NULL")
	}
	if idUsuario != nil {
		q = q.Where("id_usuario = ?", *idUsuario)
	}
	return paginarContando[models.UsuarioDebito](q, pagina, limite, "id DESC", nil)
}
