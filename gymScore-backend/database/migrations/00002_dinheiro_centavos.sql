-- +goose Up
-- Dinheiro em centavos (BIGINT). Converte usuarios.saldo (DECIMAL reais) para saldo_centavos.
-- A coluna legada `saldo` é mantida até a 00003, que a usa para criar a linha de abertura no ledger
-- (inclusive o registro de saldos legados negativos, que aqui são zerados por causa do CHECK) e então a remove.
ALTER TABLE `usuarios`
  ADD COLUMN `saldo_centavos` BIGINT NOT NULL DEFAULT 0 COMMENT 'Saldo da carteira em centavos (nunca negativo)' AFTER `genero`;

UPDATE `usuarios` SET `saldo_centavos` = GREATEST(CAST(ROUND(COALESCE(`saldo`, 0) * 100) AS SIGNED), 0);

ALTER TABLE `usuarios`
  ADD CONSTRAINT `chk_usuarios_saldo_centavos` CHECK (`saldo_centavos` >= 0);

-- +goose Down
UPDATE `usuarios` SET `saldo` = `saldo_centavos` / 100;

ALTER TABLE `usuarios`
  DROP CONSTRAINT `chk_usuarios_saldo_centavos`,
  DROP COLUMN `saldo_centavos`;
