package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ResetSenhaRepository persiste tokens de recuperação de senha (somente o hash).
type ResetSenhaRepository interface {
	Criar(t *models.PasswordResetToken) error
	// Consumir marca o token como usado (uso único, não expirado) e troca a senha do
	// dono na MESMA transação. Retorna false se o token for inválido/expirado/usado.
	Consumir(tokenHash string, agora time.Time, novoHashSenha string) (bool, error)
}

type resetSenhaRepository struct {
	db *gorm.DB
}

func NovoResetSenhaRepository(db *gorm.DB) ResetSenhaRepository {
	return &resetSenhaRepository{db}
}

// Criar emite o token invalidando, na mesma transação, os anteriores ainda não usados do usuário.
func (r *resetSenhaRepository) Criar(t *models.PasswordResetToken) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.PasswordResetToken{}).
			Where("id_usuario = ? AND usado_em IS NULL", t.IDUsuario).
			Update("usado_em", time.Now()).Error; err != nil {
			return err
		}
		return tx.Create(t).Error
	})
}

func (r *resetSenhaRepository) Consumir(tokenHash string, agora time.Time, novoHashSenha string) (bool, error) {
	ok := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var t models.PasswordResetToken
		if err := tx.Where("token_hash = ?", tokenHash).First(&t).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		// Uso único: só quem vencer este UPDATE condicional consome o token
		res := tx.Model(&models.PasswordResetToken{}).
			Where("id = ? AND usado_em IS NULL AND expira_em > ?", t.ID, agora).
			Update("usado_em", agora)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		// troca a senha e revoga os JWTs antigos (token_versao + 1) no mesmo UPDATE
		if err := tx.Exec("UPDATE usuarios SET senha = ?, token_versao = token_versao + 1 WHERE id = ?",
			novoHashSenha, t.IDUsuario).Error; err != nil {
			return err
		}
		// Invalida os demais tokens pendentes do mesmo usuário
		if err := tx.Model(&models.PasswordResetToken{}).
			Where("id_usuario = ? AND usado_em IS NULL", t.IDUsuario).
			Update("usado_em", agora).Error; err != nil {
			return err
		}
		ok = true
		return nil
	})
	return ok, err
}
