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

// adkEvent es un evento de la conversación, como lo devuelve la API del ADK.
type adkEvent struct {
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

func parseReply(data []byte) (Reply, error) {
	var events []adkEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return Reply{}, fmt.Errorf("respuesta inesperada de los agentes: %w", err)
	}
	return summarize(events), nil
}

// summarize toma la respuesta final del turno y la confirmación que haya quedado pendiente (una que
// ya tiene respuesta deja de estarlo).
//
// Solo se muestra el ÚLTIMO mensaje del modelo: en un turno hablan varios agentes (por ejemplo, el de
// reseñas dice "quedó guardada" y luego el coordinador lo confirma), y mostrarlos todos repetía la
// misma respuesta. Pedirle al coordinador "no repitas" no bastó; esto sí lo garantiza.
func summarize(events []adkEvent) Reply {
	var last string
	var pending []*Confirmation
	for _, e := range events {
		if e.Content == nil {
			continue
		}
		var text []string
		for _, p := range e.Content.Parts {
			switch {
			case p.FunctionCall != nil && p.FunctionCall.Name == "adk_request_confirmation":
				pending = append(pending, confirmationFrom(p.FunctionCall.ID, p.FunctionCall.Args))
			case p.FunctionResponse != nil:
				pending = slices.DeleteFunc(pending, func(c *Confirmation) bool { return c.CallID == p.FunctionResponse.ID })
			case p.Text != "" && !p.Thought && e.Content.Role == "model":
				text = append(text, p.Text)
			}
		}
		if len(text) > 0 {
			last = strings.Join(text, "")
		}
	}
	reply := Reply{Text: strings.TrimSpace(last)}
	if len(pending) > 0 {
		reply.Confirmation = pending[len(pending)-1]
	}
	return reply
}

// Conversation es una conversación guardada del usuario, para listarla.
type Conversation struct {
	ID      string
	Title   string
	Updated time.Time
}

// Conversations lista las conversaciones del usuario, de la más reciente a la más antigua. El
// título lo guarda el coordinador en el estado de cada sesión; las que no tienen ningún mensaje
// no se listan.
func (c *agentsClient) Conversations(ctx context.Context, userID string) ([]Conversation, error) {
	path := fmt.Sprintf("%s/apps/%s/users/%s/sessions", c.baseURL, agentsAppName, url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar con los agentes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("los agentes respondieron %d al listar las conversaciones", resp.StatusCode)
	}
	var sessions []struct {
		ID             string         `json:"id"`
		LastUpdateTime int64          `json:"lastUpdateTime"`
		State          map[string]any `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("respuesta inesperada de los agentes: %w", err)
	}

	var out []Conversation
	for _, s := range sessions {
		title, _ := s.State["title"].(string)
		if title == "" {
			continue
		}
		out = append(out, Conversation{ID: s.ID, Title: title, Updated: time.Unix(s.LastUpdateTime, 0)})
	}
	slices.SortFunc(out, func(a, b Conversation) int { return b.Updated.Compare(a.Updated) })
	return out, nil
}

// History devuelve la conversación guardada de una sesión, como pares pregunta/respuesta.
// Una sesión que los agentes no conocen da una conversación vacía.
func (c *agentsClient) History(ctx context.Context, userID, sessionID string) ([]chatExchange, error) {
	path := fmt.Sprintf("%s/apps/%s/users/%s/sessions/%s", c.baseURL, agentsAppName, url.PathEscape(userID), url.PathEscape(sessionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar con los agentes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("los agentes respondieron %d al pedir la conversación", resp.StatusCode)
	}
	var session struct {
		Events []adkEvent `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("respuesta inesperada de los agentes: %w", err)
	}

	// cada mensaje de texto del usuario abre un intercambio; lo que sigue es la respuesta
	var exchanges []chatExchange
	var group []adkEvent
	question := ""
	flush := func() {
		if question != "" {
			exchanges = append(exchanges, chatExchange{Question: question, Reply: summarize(group)})
		}
	}
	for _, e := range session.Events {
		if e.Content != nil && e.Content.Role == "user" {
			var text strings.Builder
			for _, p := range e.Content.Parts {
				text.WriteString(p.Text)
			}
			if text.Len() > 0 {
				flush()
				question, group = strings.TrimSpace(text.String()), nil
				continue
			}
		}
		group = append(group, e)
	}
	flush()
	// Una tarjeta vieja ya no es accionable: el agente siguió con otra. Solo queda la última.
	for i := range exchanges[:max(len(exchanges)-1, 0)] {
		exchanges[i].Confirmation = nil
	}
	return exchanges, nil
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
