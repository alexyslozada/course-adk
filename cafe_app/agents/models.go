package main

import (
	"context"
	"log"
	"net/http"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
)

// newAnthropicModel crea un modelo de Anthropic. El ADK Go no trae un paquete propio para Anthropic,
// pero su endpoint es compatible con la Chat Completions API de OpenAI: basta con apuntar
// openaimodel a esa dirección (ver my_agent_with_other_models).
func newAnthropicModel(ctx context.Context, apiKey, modelName string) (model.LLM, error) {
	clientConfig := openaimodel.ClientConfig{
		APIKey:  apiKey,
		BaseURL: "https://api.anthropic.com/v1/",
		API:     openaimodel.APIChatCompletions,
	}
	return openaimodel.NewModel(ctx, modelName, &clientConfig)
}

// loggingTransport anota cada llamada al modelo (sin la clave ni el cuerpo): sirve para saber cuántas
// peticiones hace cada acción y por qué se agota la cuota por minuto. Se activa con CAFE_LOG_MODEL_CALLS=1.
type loggingTransport struct {
	base http.RoundTripper
}

func (t loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	log.Printf("llamada al modelo: %s -> %d", r.URL.Path, status)
	return resp, err
}

// modelHTTPClient devuelve el cliente HTTP del modelo: con registro de llamadas si se pidió, o el de por defecto.
func modelHTTPClient(logCalls bool) *http.Client {
	if !logCalls {
		return nil
	}
	return &http.Client{Transport: loggingTransport{base: http.DefaultTransport}}
}
