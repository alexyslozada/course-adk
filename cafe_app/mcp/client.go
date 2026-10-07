package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Config agrupa lo que necesita el servidor MCP para hablar con la app.
type Config struct {
	AppURL      string // dirección de la API de la cafetería
	AppUser     string // usuario de servicio con el que se entra a la API
	AppPassword string
}

// appClient llama a la API de la app. Hace login con el usuario de servicio,
// guarda el JWT y vuelve a entrar si la app responde 401 (token vencido).
type appClient struct {
	cfg  Config
	http *http.Client

	mu    sync.Mutex
	token string
}

func newAppClient(cfg Config) *appClient {
	return &appClient{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
}

// apiError es un error de negocio de la app (por ejemplo, "no hay stock suficiente").
// Su mensaje es el que ve el modelo, así que debe ser legible.
type apiError struct{ msg string }

func (e apiError) Error() string { return e.msg }

func (c *appClient) login(ctx context.Context) error {
	var out struct {
		Token string `json:"token"`
	}
	body := map[string]string{"username": c.cfg.AppUser, "password": c.cfg.AppPassword}
	if err := c.send(ctx, http.MethodPost, "/api/login", "", body, &out); err != nil {
		return fmt.Errorf("login en la app: %w", err)
	}
	c.mu.Lock()
	c.token = out.Token
	c.mu.Unlock()
	return nil
}

func (c *appClient) currentToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// do ejecuta una llamada autenticada; si no hay token o venció, entra de nuevo y reintenta una vez.
func (c *appClient) do(ctx context.Context, method, path string, body, out any) error {
	if c.currentToken() == "" {
		if err := c.login(ctx); err != nil {
			return err
		}
	}
	err := c.send(ctx, method, path, c.currentToken(), body, out)
	if errors.Is(err, errUnauthorized) {
		if err := c.login(ctx); err != nil {
			return err
		}
		err = c.send(ctx, method, path, c.currentToken(), body, out)
	}
	return err
}

var errUnauthorized = errors.New("no autenticado")

func (c *appClient) send(ctx context.Context, method, path, token string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.AppURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo conectar con la app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized && token != "" {
		return errUnauthorized
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return apiError{e.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
