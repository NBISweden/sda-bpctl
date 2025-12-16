package database

import (
	"database/sql"
	"fmt"

	"github.com/NBISweden/sda-bpctl/internal/config"
	_ "github.com/lib/pq"
)

type PostgresDb struct {
	db *sql.DB
}

func (dbs *PostgresDb) Close() {
	dbs.db.Close()
}

func New(cfg *config.Config) (*PostgresDb, error) {
	var err error
	pg := &PostgresDb{db: nil}
	pg.db, err = sql.Open("postgres", dataSourceName(*cfg))

	if err != nil {
		return pg, fmt.Errorf("failed to connect to database: %v", err)
	}

	if err := pg.db.Ping(); err != nil {
		return pg, fmt.Errorf("failed to connect to database: %v", err)
	}

	return pg, nil
}

func dataSourceName(c config.Config) string {
	connInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.DbHost, c.DbPort, c.DbUser, c.DbPassword, c.DbName, c.DbSslMode)

	if c.DbSslMode == "disable" {
		return connInfo
	}

	if c.SslCaCert != "" {
		connInfo = fmt.Sprintf("%s sslrootcert=%s", connInfo, c.SslCaCert)
	}

	if c.DbClientCert != "" {
		connInfo = fmt.Sprintf("%s sslcert=%s", connInfo, c.DbClientCert)
	}

	if c.DbClientKey != "" {
		connInfo = fmt.Sprintf("%s sslkey=%s", connInfo, c.DbClientKey)
	}

	return connInfo
}
