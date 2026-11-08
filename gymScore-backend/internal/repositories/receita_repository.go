package repositories

import (
	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ReceitaRepository acessa a receita da plataforma (taxa de inscrição e tarifa de saque).
// A plataforma não tem carteira de usuário: a receita vive nesta tabela, ligada ao desafio ou à
// linha do ledger que a gerou. O registro é feito na mesma transação do fato gerador.
type ReceitaRepository struct {
	db *gorm.DB
}

// NovoReceitaRepository cria o repositório de receitas da plataforma.
func NovoReceitaRepository(db *gorm.DB) *ReceitaRepository {
	return &ReceitaRepository{db: db}
}

// RegistrarTaxaInscricao grava a taxa do desafio. A UK (id_desafio, origem) impede lançar duas vezes.
func (r *ReceitaRepository) RegistrarTaxaInscricao(tx *gorm.DB, idDesafio uint, valor int64) error {
	return tx.Create(&models.PlataformaReceita{
		Origem: models.ReceitaTaxaInscricao, ValorCentavos: valor, IDDesafio: &idDesafio,
	}).Error
}

// TotalPorOrigem soma a receita por origem (taxa_inscricao, tarifa_saque).
func (r *ReceitaRepository) TotalPorOrigem() (map[string]int64, error) {
	var linhas []struct {
		Origem string
		Total  int64
	}
	err := r.db.Model(&models.PlataformaReceita{}).Select("origem, COALESCE(SUM(valor_centavos),0) AS total").
		Group("origem").Scan(&linhas).Error
	out := map[string]int64{}
	for _, l := range linhas {
		out[l.Origem] = l.Total
	}
	return out, err
}
