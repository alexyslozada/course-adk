package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const chatCookie = "cafe_chat"

// chatExchange es una pregunta del usuario con la respuesta de los agentes.
type chatExchange struct {
	Question string
	Reply
}

// chatDecision es la respuesta a una confirmación: lo que decidió la persona y lo que siguió.
type chatDecision struct {
	Approved bool
	Reply
}

func (a *App) assistantPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, "asistente", "Asistente", nil)
}

// assistantMessage reenvía el mensaje a los agentes y devuelve la pareja pregunta/respuesta.
// La conversación (sesión del ADK) se recuerda en una cookie; si los agentes no la conocen
// (por ejemplo, se reiniciaron), se abre una nueva y se reintenta una vez.
func (a *App) assistantMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	text := strings.TrimSpace(r.PostFormValue("message"))
	if text == "" {
		a.htmxError(w, InputError("escribe un mensaje"))
		return
	}
	userID := strconv.Itoa(userFrom(ctx).ID)

	sessionID := ""
	if c, err := r.Cookie(chatCookie); err == nil {
		sessionID = c.Value
	}
	newSession := func() error {
		id, err := a.agents.CreateSession(ctx, userID)
		if err != nil {
			return err
		}
		sessionID = id
		http.SetCookie(w, &http.Cookie{Name: chatCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		return nil
	}

	if sessionID == "" {
		if err := newSession(); err != nil {
			a.htmxError(w, err)
			return
		}
	}
	reply, err := a.agents.Send(ctx, userID, sessionID, text)
	if errors.Is(err, errSessionNotFound) {
		if err = newSession(); err == nil {
			reply, err = a.agents.Send(ctx, userID, sessionID, text)
		}
	}
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "asistente", "chat_exchange", chatExchange{Question: text, Reply: reply})
}

// assistantConfirm responde a una confirmación pendiente (aprobar o rechazar, con el payload
// que la persona haya modificado) y devuelve lo que siguió en la conversación.
func (a *App) assistantConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := r.Cookie(chatCookie)
	callID := r.PostFormValue("call_id")
	if err != nil || callID == "" {
		a.htmxError(w, InputError("no hay una conversación con una confirmación pendiente"))
		return
	}
	approved := r.PostFormValue("decision") == "approve"

	// los campos del payload llegan como n.<nombre> (numéricos) o t.<nombre> (texto)
	payload := map[string]any{}
	for key, values := range r.PostForm {
		if len(values) == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(key, "n."):
			n, err := strconv.ParseFloat(strings.TrimSpace(values[0]), 64)
			if err != nil {
				a.htmxError(w, InputError("el campo "+strings.TrimPrefix(key, "n.")+" debe ser un número"))
				return
			}
			payload[strings.TrimPrefix(key, "n.")] = n
		case strings.HasPrefix(key, "t."):
			payload[strings.TrimPrefix(key, "t.")] = values[0]
		}
	}

	userID := strconv.Itoa(userFrom(ctx).ID)
	reply, err := a.agents.Confirm(ctx, userID, c.Value, callID, approved, payload)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			err = InputError("la conversación ya no existe; empieza una nueva")
		}
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "asistente", "chat_decision", chatDecision{Approved: approved, Reply: reply})
}

// assistantReset olvida la conversación actual: el próximo mensaje abre una sesión nueva.
func (a *App) assistantReset(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: chatCookie, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/asistente", http.StatusSeeOther)
}
