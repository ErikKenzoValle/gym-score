// Package database embute as migrations goose e as executa no boot da aplicação.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var arquivos embed.FS

// Migrar aplica todas as migrations pendentes (goose "up") usando os SQLs embutidos.
func Migrar(ctx context.Context, db *sql.DB) error {
	return migrar(ctx, db, -1)
}

// MigrarAte aplica as migrations até a versão informada (usado para testar a conversão de
// dados legados: baseline -> dados antigos -> demais migrations).
func MigrarAte(ctx context.Context, db *sql.DB, versao int64) error {
	return migrar(ctx, db, versao)
}

// Trava entre instâncias: réplicas que sobem juntas (MIGRATE_ON_BOOT=true em todas) migram uma de
// cada vez. Sem ela, duas instâncias executavam a mesma migration em paralelo e uma falhava com
// "Error 1304 PROCEDURE _goose_000NN_add already exists" / "Error 1305 ... does not exist".
// A trava é uma linha na tabela `goose_lock` (criada pelo goose se não existir) com lease renovado por
// heartbeat: se a instância que migra morrer, outra assume depois que o lease expira (30 s).
const (
	travaIntervalo   = 2 * time.Second // espera entre tentativas de pegar a trava
	travaTentativas  = 300             // 300 × 2 s = até 10 min esperando a outra instância migrar
	travaLeaseMaximo = 30 * time.Second
)

func migrar(ctx context.Context, db *sql.DB, ate int64) error {
	sub, err := fs.Sub(arquivos, "migrations")
	if err != nil {
		return fmt.Errorf("migrations embutidas: %w", err)
	}
	trava, err := lock.NewMySQLTableLocker(
		lock.WithTableLockTimeout(travaIntervalo, travaTentativas),
		lock.WithTableLeaseDuration(travaLeaseMaximo),
	)
	if err != nil {
		return fmt.Errorf("criar trava das migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectMySQL, db, sub, goose.WithLocker(trava))
	if err != nil {
		return fmt.Errorf("criar provider goose: %w", err)
	}
	var errUp error
	if ate >= 0 {
		_, errUp = provider.UpTo(ctx, ate)
	} else {
		_, errUp = provider.Up(ctx)
	}
	if err := errUp; err != nil {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}
