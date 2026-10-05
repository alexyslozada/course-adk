package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

const (
	jiraMCPURL        = "https://mcp.atlassian.com/v1/mcp"
	oauthCallbackAddr = "localhost:3142"
)

// MCPJiraHTTP es el MCP de Atlassian (Jira y Confluence). No usa token fijo:
// el usuario inicia sesión en el navegador (OAuth 2.1 con PKCE).
type MCPJiraHTTP struct {
	ClientName string
}

func NewMCPJiraHTTP(clientName string) MCPJiraHTTP {
	if clientName == "" {
		panic("El nombre del cliente OAuth no puede estar vacío")
	}

	return MCPJiraHTTP{ClientName: clientName}
}

// codeReceiver recibe el ?code=... cuando Atlassian redirige al terminar el login.
type codeReceiver struct {
	callbacks chan callbackResult
}

type callbackResult struct {
	result *auth.AuthorizationResult
	err    error
}

// handleCallback atiende la redirección de Atlassian a http://localhost:3142/callback.
func (r *codeReceiver) handleCallback(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()

	var cb callbackResult
	if errCode := query.Get("error"); errCode != "" {
		cb.err = fmt.Errorf("la autorización fue rechazada por Atlassian: %s", errCode)
	} else {
		cb.result = &auth.AuthorizationResult{
			Code:  query.Get("code"),
			State: query.Get("state"),
			Iss:   query.Get("iss"),
		}
	}

	// Solo entregamos el resultado si hay un login esperando.
	select {
	case r.callbacks <- cb:
		fmt.Fprint(w, "Listo, ya puedes cerrar esta ventana y volver al agente.")
	default:
		http.Error(w, "No hay ningún login pendiente", http.StatusConflict)
	}
}

// fetchAuthorizationCode lo llama el SDK de MCP cuando el servidor responde 401.
// Muestra la URL de login y espera a que el navegador vuelva con el code.
func (r *codeReceiver) fetchAuthorizationCode(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
	fmt.Println("Abre esta URL en tu navegador para iniciar sesión en Atlassian:")
	fmt.Println(args.URL)

	select {
	case cb := <-r.callbacks:
		return cb.result, cb.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (this MCPJiraHTTP) ToolSet() (tool.Toolset, error) {
	redirectURL := "http://" + oauthCallbackAddr + "/callback"

	// Servidor local que recibe la redirección del login
	receiver := &codeReceiver{
		callbacks: make(chan callbackResult),
	}

	listener, err := net.Listen("tcp", oauthCallbackAddr)
	if err != nil {
		return nil, fmt.Errorf("no pude escuchar en %s: %w", oauthCallbackAddr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", receiver.handleCallback)
	go http.Serve(listener, mux)

	// Manejador OAuth: descubre el servidor, registra el cliente y hace el login
	authConfig := auth.AuthorizationCodeHandlerConfig{
		RedirectURL:              redirectURL,
		AuthorizationCodeFetcher: receiver.fetchAuthorizationCode,
		// Atlassian permite registrar el cliente dinámicamente,
		// así que no hace falta crear una app en Atlassian.
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:   this.ClientName,
				RedirectURIs: []string{redirectURL},
			},
		},
	}

	authHandler, err := auth.NewAuthorizationCodeHandler(&authConfig)
	if err != nil {
		return nil, fmt.Errorf("no pude crear el manejador OAuth: %w", err)
	}

	transport := mcp.StreamableClientTransport{
		Endpoint:     jiraMCPURL,
		OAuthHandler: authHandler,
	}

	// No uses mcptoolset.Config.Auth aquí: pisaría el token obtenido con el login.
	toolsetConfig := mcptoolset.Config{
		Transport: &transport,
	}

	return mcptoolset.New(toolsetConfig)
}
