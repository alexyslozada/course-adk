package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const agentsAppName = "cafe_coordinator"

var errSessionNotFound = errors.New("sesión de agentes no encontrada")

// agentsClient habla con la API REST del ADK. El navegador nunca la llama directo:
// el backend de la app reenvía el chat con el usuario logueado como user_id.
type agentsClient struct {
	baseURL string
	http    *http.Client
}

func newAgentsClient(baseURL string) *agentsClient {
	// el timeout es amplio porque un turno puede incluir varias llamadas al modelo y a las tools
	return &agentsClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 2 * time.Minute}}
}

func (c *agentsClient) post(ctx context.Context, path string, body any) ([]byte, int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("no se pudo conectar con los agentes: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// CreateSession abre una conversación nueva para el usuario y devuelve su id.
func (c *agentsClient) CreateSession(ctx context.Context, userID string) (string, error) {
	path := fmt.Sprintf("/apps/%s/users/%s/sessions", agentsAppName, url.PathEscape(userID))
	data, status, err := c.post(ctx, path, map[string]any{})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("creando la sesión de agentes: %s", strings.TrimSpace(string(data)))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("respuesta inesperada al crear la sesión: %s", data)
	}
	return out.ID, nil
}

// Reply es lo que los agentes respondieron en un turno: texto y, si quedaron esperando
// a una persona, la confirmación pendiente.
type Reply struct {
	Text         string
	Confirmation *Confirmation
}

// Confirmation es una petición de aprobación del ADK (Human in the Loop).
type Confirmation struct {
	CallID string
	Hint   string
	Fields []ConfirmationField // lo que la persona puede modificar antes de aprobar
}

// ConfirmationField es un valor del payload de la confirmación que la persona puede cambiar.
type ConfirmationField struct {
	Name    string
	Label   string
	Value   string
	Numeric bool
}

var fieldLabels = map[string]string{"percent": "Porcentaje de descuento"}

// Send envía el mensaje del usuario y devuelve lo que respondieron los agentes.
func (c *agentsClient) Send(ctx context.Context, userID, sessionID, text string) (Reply, error) {
	return c.run(ctx, userID, sessionID, map[string]any{"text": text})
}

// Confirm responde a una confirmación pendiente: aprobada o rechazada, con el payload modificado.
func (c *agentsClient) Confirm(ctx context.Context, userID, sessionID, callID string, confirmed bool, payload map[string]any) (Reply, error) {
	response := map[string]any{"confirmed": confirmed}
	if confirmed && len(payload) > 0 {
		response["payload"] = payload
	}
	return c.run(ctx, userID, sessionID, map[string]any{
		"functionResponse": map[string]any{
			"id":       callID,
			"name":     "adk_request_confirmation",
			"response": response,
		},
	})
}

func (c *agentsClient) run(ctx context.Context, userID, sessionID string, part map[string]any) (Reply, error) {
	data, status, err := c.post(ctx, "/run", map[string]any{
		"appName":   agentsAppName,
		"userId":    userID,
		"sessionId": sessionID,
		"newMessage": map[string]any{
			"role":  "user",
			"parts": []map[string]any{part},
		},
	})
	if err != nil {
		return Reply{}, err
	}
	if status == http.StatusNotFound && strings.Contains(string(data), "session not found") {
		return Reply{}, errSessionNotFound
	}
	if status != http.StatusOK {
		return Reply{}, fmt.Errorf("los agentes respondieron %d: %s", status, strings.TrimSpace(string(data)))
	}
	return parseReply(data)
}

func parseReply(data []byte) (Reply, error) {
	var events []struct {
		Content *struct {
			Role  string `json:"role"`
			Parts []struct {
				Text         string `json:"text"`
				Thought      bool   `json:"thought"`
				FunctionCall *struct {
					ID   string         `json:"id"`
					Name string         `json:"name"`
					Args map[string]any `json:"args"`
				} `json:"functionCall"`
				FunctionResponse *struct {
					ID string `json:"id"`
				} `json:"functionResponse"`
			} `json:"parts"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &events); err != nil {
		return Reply{}, fmt.Errorf("respuesta inesperada de los agentes: %w", err)
	}

	var texts []string
	var pending []*Confirmation
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			switch {
			case p.FunctionCall != nil && p.FunctionCall.Name == "adk_request_confirmation":
				pending = append(pending, confirmationFrom(p.FunctionCall.ID, p.FunctionCall.Args))
			case p.FunctionResponse != nil:
				// una confirmación que ya tiene respuesta deja de estar pendiente
				pending = slices.DeleteFunc(pending, func(c *Confirmation) bool { return c.CallID == p.FunctionResponse.ID })
			case p.Text != "" && !p.Thought && e.Content.Role == "model":
				// se juntan los textos del modelo en orden (sin los pensamientos internos)
				texts = append(texts, p.Text)
			}
		}
	}
	reply := Reply{Text: strings.Join(texts, "\n\n")}
	if len(pending) > 0 {
		reply.Confirmation = pending[len(pending)-1]
	}
	return reply, nil
}

// confirmationFrom lee los argumentos de adk_request_confirmation: la pista para la
// persona y el payload, de donde salen los campos que puede modificar.
func confirmationFrom(callID string, args map[string]any) *Confirmation {
	c := &Confirmation{CallID: callID}
	tc, _ := args["toolConfirmation"].(map[string]any)
	c.Hint, _ = tc["hint"].(string)
	if c.Hint == "" {
		if original, ok := args["originalFunctionCall"].(map[string]any); ok {
			c.Hint = fmt.Sprintf("El asistente quiere ejecutar %v con %v.", original["name"], original["args"])
		}
	}
	payload, _ := tc["payload"].(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(payload)) {
		field := ConfirmationField{Name: name, Label: fieldLabels[name]}
		if field.Label == "" {
			field.Label = name
		}
		switch v := payload[name].(type) {
		case float64:
			field.Numeric = true
			field.Value = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			field.Value = fmt.Sprint(v)
		}
		c.Fields = append(c.Fields, field)
	}
	return c
}
