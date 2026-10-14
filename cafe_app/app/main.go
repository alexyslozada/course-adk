// App base de la cafetería: SQLite, API REST y un frontend de plantillas Go + htmx.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	db, err := openDB(getenv("CAFE_DB", "cafe.db"))
	if err != nil {
		log.Fatalf("abriendo la base de datos: %v", err)
	}
	defer db.Close()

	store := NewStore(db)
	if err := seed(ctx, store); err != nil {
		log.Fatalf("cargando datos de ejemplo: %v", err)
	}

	secret := os.Getenv("CAFE_JWT_SECRET")
	if secret == "" {
		secret = "secreto-de-desarrollo-cambiar-en-produccion"
		log.Print("CAFE_JWT_SECRET no está definido: usando el secreto de desarrollo")
	}
	app, err := NewApp(store, []byte(secret))
	if err != nil {
		log.Fatalf("iniciando la app: %v", err)
	}

	addr := getenv("CAFE_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("cafetería escuchando en http://localhost%s", addr)
	log.Fatal(srv.ListenAndServe())
}
