// Servidor MCP de la cafetería: expone la app como tools (menú, stock, ventas, reseñas).
// Se protege con un token bearer y habla con la app a través de su API REST.
package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requireToken deja pasar solo las peticiones con "Authorization: Bearer <token>".
func requireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "token inválido", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	token := os.Getenv("CAFE_MCP_TOKEN")
	if token == "" {
		log.Fatal("Debes configurar CAFE_MCP_TOKEN (el token que deben enviar los agentes)")
	}

	config := Config{
		AppURL:      getenv("CAFE_APP_URL", "http://localhost:8080"),
		AppUser:     getenv("CAFE_APP_USER", "asistente"),
		AppPassword: getenv("CAFE_APP_PASSWORD", "asistente123"),
	}
	app := newAppClient(config)

	server := mcp.NewServer(&mcp.Implementation{Name: "cafe-mcp", Version: "1.0.0"}, nil)
	registerTools(server, app)

	// Stateless: cada petición es independiente, sin sesión que guardar.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true})

	addr := getenv("CAFE_MCP_ADDR", ":8090")
	srv := &http.Server{
		Addr:              addr,
		Handler:           requireToken(token, handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("MCP de la cafetería en http://localhost%s (app: %s)", addr, config.AppURL)
	log.Fatal(srv.ListenAndServe())
}
