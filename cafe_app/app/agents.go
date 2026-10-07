package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Send envía el mensaje del usuario y devuelve lo que respondieron los agentes.
func (c *agentsClient) Send(ctx context.Context, userID, sessionID, text string) (string, error) {
	data, status, err := c.post(ctx, "/run", map[string]any{
		"appName":   agentsAppName,
		"userId":    userID,
		"sessionId": sessionID,
		"newMessage": map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"text": text}},
		},
	})
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound && strings.Contains(string(data), "session not found") {
		return "", errSessionNotFound
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("los agentes respondieron %d: %s", status, strings.TrimSpace(string(data)))
	}

	var events []struct {
		Content *struct {
			Role  string `json:"role"`
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &events); err != nil {
		return "", fmt.Errorf("respuesta inesperada de los agentes: %w", err)
	}
	// se juntan los textos del modelo en orden (sin los pensamientos internos)
	var texts []string
	for _, e := range events {
		if e.Content == nil || e.Content.Role != "model" {
			continue
		}
		for _, p := range e.Content.Parts {
			if p.Text != "" && !p.Thought {
				texts = append(texts, p.Text)
			}
		}
	}
	return strings.Join(texts, "\n\n"), nil
}
