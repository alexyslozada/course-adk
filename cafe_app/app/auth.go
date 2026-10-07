package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName = "cafe_token"
	tokenTTL   = 24 * time.Hour
)

type ctxKey struct{}

// claims es lo que viaja firmado en el JWT: el id del usuario va en "sub".
type claims struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// authenticate valida usuario y contraseña y devuelve el usuario si son correctos.
func (a *App) authenticate(r *http.Request, username, password string) (User, bool) {
	user, hash, err := a.store.UserByUsername(r.Context(), username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, false
	}
	return user, true
}

// issueToken firma un JWT (HS256) que expira a las 24 horas.
func (a *App) issueToken(u User) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Username: u.Username,
		Name:     u.Name,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.Itoa(u.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}).SignedString(a.jwtSecret)
}

// login deja el JWT en una cookie HttpOnly para el navegador.
func (a *App) login(w http.ResponseWriter, r *http.Request, username, password string) bool {
	user, ok := a.authenticate(r, username, password)
	if !ok {
		return false
	}
	token, err := a.issueToken(user)
	if err != nil {
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(tokenTTL.Seconds()),
	})
	return true
}

func (a *App) logout(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
}

// currentUser lee el JWT de "Authorization: Bearer ..." o de la cookie y lo valida (firma y expiración).
func (a *App) currentUser(r *http.Request) (User, bool) {
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		c, err := r.Cookie(cookieName)
		if err != nil {
			return User{}, false
		}
		raw = c.Value
	}
	var c claims
	token, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return a.jwtSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return User{}, false
	}
	id, err := strconv.Atoi(c.Subject)
	if err != nil {
		return User{}, false
	}
	return User{ID: id, Username: c.Username, Name: c.Name, Role: c.Role}, true
}

func userFrom(ctx context.Context) User {
	u, _ := ctx.Value(ctxKey{}).(User)
	return u
}

// requireWeb protege las páginas: sin token válido redirige al login.
func (a *App) requireWeb(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.currentUser(r)
		if !ok {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

// requireAPI protege la API: sin token válido responde 401 en JSON.
func (a *App) requireAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.currentUser(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no autenticado"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}
