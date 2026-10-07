package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	name          TEXT NOT NULL,
	role          TEXT NOT NULL CHECK (role IN ('admin', 'cashier'))
);

CREATE TABLE IF NOT EXISTS products (
	id       INTEGER PRIMARY KEY,
	name     TEXT NOT NULL UNIQUE,
	category TEXT NOT NULL,
	price    INTEGER NOT NULL CHECK (price > 0),
	active   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS inventory (
	product_id   INTEGER PRIMARY KEY REFERENCES products(id),
	quantity     INTEGER NOT NULL DEFAULT 0 CHECK (quantity >= 0),
	min_quantity INTEGER NOT NULL DEFAULT 0 CHECK (min_quantity >= 0)
);

CREATE TABLE IF NOT EXISTS customers (
	id    INTEGER PRIMARY KEY,
	name  TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sales (
	id          INTEGER PRIMARY KEY,
	customer_id INTEGER REFERENCES customers(id),
	user_id     INTEGER NOT NULL REFERENCES users(id),
	total       INTEGER NOT NULL,
	discount_percent INTEGER NOT NULL DEFAULT 0 CHECK (discount_percent BETWEEN 0 AND 100),
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sale_items (
	sale_id    INTEGER NOT NULL REFERENCES sales(id),
	product_id INTEGER NOT NULL REFERENCES products(id),
	quantity   INTEGER NOT NULL CHECK (quantity > 0),
	unit_price INTEGER NOT NULL,
	PRIMARY KEY (sale_id, product_id)
);

CREATE TABLE IF NOT EXISTS reviews (
	id          INTEGER PRIMARY KEY,
	customer_id INTEGER REFERENCES customers(id),
	product_id  INTEGER REFERENCES products(id),
	rating      INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
	comment     TEXT NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// openDB abre (o crea) el archivo SQLite y deja el esquema listo.
// _txlock=immediate evita errores de "database is locked" al escribir en transacciones.
func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creando el esquema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrando la base: %w", err)
	}
	return db, nil
}

// migrate agrega las columnas nuevas a las bases creadas por versiones anteriores.
func migrate(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sales') WHERE name = 'discount_percent'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		_, err := db.Exec(`ALTER TABLE sales ADD COLUMN discount_percent INTEGER NOT NULL DEFAULT 0 CHECK (discount_percent BETWEEN 0 AND 100)`)
		return err
	}
	return nil
}
