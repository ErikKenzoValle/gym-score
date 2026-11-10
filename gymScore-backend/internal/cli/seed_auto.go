package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Seed automático na inicialização (B5-6): o servidor roda o mesmo `seed-teste` logo depois das
// migrations, para as contas de teste existirem sempre em desenvolvimento/homologação.
//   - Lista de permissão (QA56-2): só roda com APP_ENV definido EXPLICITAMENTE no ambiente/.env com
//     development, dev, local ou test (sem diferenciar maiúsculas, com trim). Sem APP_ENV (o default do
//     config não conta), vazio ou qualquer outro valor (staging, production…): não roda, só um log.
//   - SEED_TESTE_AUTO=false desliga.
//   - Idempotente (é o próprio SeedTeste: contas pelo e-mail, desafio de exemplo e crédito inicial uma vez só).
//   - Duas instâncias subindo juntas: trava nomeada do MySQL (GET_LOCK) numa conexão dedicada; quem não
//     conseguir a trava em LockSeedTimeout pula o seed (a outra instância já está semeando).
//   - Falha só é logada: o servidor sobe mesmo assim.

// LockSeedNome é a trava nomeada (GET_LOCK) do seed automático.
const LockSeedNome = "gymscore_seed_teste_auto"

// LockSeedTimeout é quanto uma instância espera pela trava antes de desistir do seed.
const LockSeedTimeout = 30 * time.Second

// ErrSeedTravado: outra instância está com a trava do seed (esta pulou).
var ErrSeedTravado = errors.New("seed automático pulado: outra instância está semeando (GET_LOCK ocupado)")

// ResultadoSeedAuto diz o que o seed automático fez.
type ResultadoSeedAuto struct {
	Executado bool           // false = pulado (produção, desligado ou trava ocupada)
	Motivo    string         // por que foi pulado
	Seed      *ResultadoSeed // preenchido quando executado
}

// ambientesSeedAuto são os únicos APP_ENV em que o seed automático roda.
var ambientesSeedAuto = map[string]bool{"development": true, "dev": true, "local": true, "test": true}

// PermiteSeedAuto diz se o seed automático pode rodar para o APP_ENV cru (appEnvExplicito = "" quando a
// variável não existe). Quando não pode, devolve o motivo para o log.
func PermiteSeedAuto(appEnvExplicito string) (bool, string) {
	env := strings.ToLower(strings.TrimSpace(appEnvExplicito))
	if env == "" {
		return false, "APP_ENV não definido explicitamente; o seed automático só roda com APP_ENV=development|dev|local|test"
	}
	if !ambientesSeedAuto[env] {
		return false, fmt.Sprintf("APP_ENV=%q não está na lista development|dev|local|test", appEnvExplicito)
	}
	return true, ""
}

// SeedTesteAuto roda o seed de teste com as regras do boot. Devolve erro só quando o seed tentou rodar e
// falhou (ou não conseguiu a trava); quem chama decide só logar (o servidor nunca cai por isto).
// appEnvExplicito é o APP_ENV cru do ambiente ("" se ausente), não o valor com default do config.
func SeedTesteAuto(ctx context.Context, appEnvExplicito string, habilitado bool, db *gorm.DB, taxaPct int, saida io.Writer) (*ResultadoSeedAuto, error) {
	if ok, motivo := PermiteSeedAuto(appEnvExplicito); !ok {
		return &ResultadoSeedAuto{Motivo: motivo}, nil
	}
	appEnv := strings.TrimSpace(appEnvExplicito)
	if !habilitado {
		return &ResultadoSeedAuto{Motivo: "SEED_TESTE_AUTO=false"}, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var ok *int64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", LockSeedNome, int(LockSeedTimeout/time.Second)).Scan(&ok); err != nil {
		return nil, fmt.Errorf("GET_LOCK do seed: %w", err)
	}
	if ok == nil || *ok != 1 {
		return &ResultadoSeedAuto{Motivo: "trava ocupada"}, ErrSeedTravado
	}
	defer func() {
		var liberou *int64
		if err := conn.QueryRowContext(context.Background(), "SELECT RELEASE_LOCK(?)", LockSeedNome).Scan(&liberou); err != nil {
			log.Printf("[SEED] erro ao liberar a trava do seed: %v", err)
		}
	}()
	res, err := SeedTeste(appEnv, db, taxaPct, saida)
	if err != nil {
		return nil, err
	}
	return &ResultadoSeedAuto{Executado: true, Seed: res}, nil
}
