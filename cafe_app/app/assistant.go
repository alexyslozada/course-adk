package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const chatCookie = "cafe_chat"

type chatMessage struct {
	Question string
	Answer   string
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
	answer, err := a.agents.Send(ctx, userID, sessionID, text)
	if errors.Is(err, errSessionNotFound) {
		if err = newSession(); err == nil {
			answer, err = a.agents.Send(ctx, userID, sessionID, text)
		}
	}
	if err != nil {
		a.htmxError(w, err)
		return
	}
	if answer == "" {
		answer = "(sin respuesta)"
	}
	a.fragment(w, "asistente", "chat_exchange", chatMessage{Question: text, Answer: answer})
}

// assistantReset olvida la conversación actual: el próximo mensaje abre una sesión nueva.
func (a *App) assistantReset(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: chatCookie, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/asistente", http.StatusSeeOther)
}
