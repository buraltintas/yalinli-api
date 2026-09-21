package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	// Nothing is held open while nobody is asking. The managed Postgres behind
	// this suspends itself only after five minutes with no connection at all
	// and is billed by the hour it stays awake, so a floor of even one
	// connection kept it awake for as long as an instance was alive.
	cfg.MinConns = 0
	// Long enough that a burst of requests shares one connection, short enough
	// that a single query does not hold the database up past the window it
	// needs to suspend. The cost is a fresh connection on the first request
	// after a quiet spell, about a second.
	cfg.MaxConnIdleTime = 90 * time.Second
	cfg.MaxConnLifetime = 30 * time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}
