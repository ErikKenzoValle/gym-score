-- +goose Up
-- Versão do token JWT: incrementada em logout, troca e reset de senha para revogar tokens antigos (claim `tv`).
ALTER TABLE `usuarios`
  ADD COLUMN `token_versao` INT NOT NULL DEFAULT 0 COMMENT 'Versão dos tokens JWT válidos do usuário' AFTER `saldo_centavos`;

-- +goose Down
ALTER TABLE `usuarios`
  DROP COLUMN `token_versao`;
