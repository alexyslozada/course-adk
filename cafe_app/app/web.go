package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed templates static
var assets embed.FS

type App struct {
	store     *Store
	jwtSecret []byte
	agents    *agentsClient
	templates map[string]*template.Template
}

// page es lo que reciben todas las plantillas: el layout usa Title, Active y User; el contenido, Data.
type page struct {
	Title  string
	Active string
	User   *User
	Data   any
}

var funcs = template.FuncMap{
	"money": func(n int) string {
		s := strconv.Itoa(n)
		for i := len(s) - 3; i > 0; i -= 3 {
			s = s[:i] + "." + s[i:]
		}
		return "$ " + s
	},
	"date":  func(t time.Time) string { return t.Local().Format("02/01/2006 15:04") },
	"stars": func(n int) string { return strings.Repeat("★", n) + strings.Repeat("☆", 5-n) },
	"deref": func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	},
}

func NewApp(store *Store, jwtSecret []byte, agents *agentsClient) (*App, error) {
	a := &App{store: store, jwtSecret: jwtSecret, agents: agents, templates: map[string]*template.Template{}}
	for _, name := range []string{"login", "inventario", "productos", "ventas", "clientes", "resenas", "asistente"} {
		t, err := template.New("").Funcs(funcs).ParseFS(assets, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("plantilla %s: %w", name, err)
		}
		a.templates[name] = t
	}
	return a, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginSubmit)
	mux.HandleFunc("POST /logout", a.logoutSubmit)

	mux.HandleFunc("GET /{$}", a.requireWeb(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/inventario", http.StatusSeeOther)
	}))
	mux.HandleFunc("GET /inventario", a.requireWeb(a.inventoryPage))
	mux.HandleFunc("POST /inventario/{product_id}", a.requireWeb(a.inventorySave))
	mux.HandleFunc("GET /productos", a.requireWeb(a.productsPage))
	mux.HandleFunc("POST /productos", a.requireWeb(a.productCreate))
	mux.HandleFunc("GET /ventas", a.requireWeb(a.salesPage))
	mux.HandleFunc("POST /ventas", a.requireWeb(a.saleCreate))
	mux.HandleFunc("GET /clientes", a.requireWeb(a.customersPage))
	mux.HandleFunc("POST /clientes", a.requireWeb(a.customerCreate))
	mux.HandleFunc("GET /resenas", a.requireWeb(a.reviewsPage))
	mux.HandleFunc("POST /resenas", a.requireWeb(a.reviewCreate))

	mux.HandleFunc("GET /asistente", a.requireWeb(a.assistantPage))
	mux.HandleFunc("POST /asistente/mensaje", a.requireWeb(a.assistantMessage))
	mux.HandleFunc("POST /asistente/nueva", a.requireWeb(a.assistantReset))

	a.registerAPI(mux)
	return mux
}

// render dibuja una página completa; el buffer evita enviar HTML a medias si la plantilla falla.
func (a *App) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	p := page{Title: title, Active: name, Data: data}
	if u, ok := a.currentUser(r); ok {
		p.User = &u
	}
	a.execute(w, name, "layout", p)
}

// fragment dibuja una pieza de HTML (una fila) que htmx inserta en la página.
func (a *App) fragment(w http.ResponseWriter, pageName, block string, data any) {
	a.execute(w, pageName, block, data)
}

func (a *App) execute(w http.ResponseWriter, pageName, block string, data any) {
	var buf bytes.Buffer
	if err := a.templates[pageName].ExecuteTemplate(&buf, block, data); err != nil {
		log.Printf("plantilla %s/%s: %v", pageName, block, err)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// htmxError muestra el mensaje en el <p id="error"> de la página sin tocar el resto.
func (a *App) htmxError(w http.ResponseWriter, err error) {
	msg := "error interno"
	if input, ok := errors.AsType[InputError](err); ok {
		msg = input.Error()
	} else if errors.Is(err, ErrNotFound) {
		msg = err.Error()
	} else {
		log.Printf("error interno: %v", err)
	}
	w.Header().Set("HX-Retarget", "#error")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Write([]byte(template.HTMLEscapeString(msg)))
}

// optionalID convierte un campo de formulario opcional ("" = sin valor) en *int.
func optionalID(v string) (*int, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, InputError("identificador inválido")
	}
	return &n, nil
}

func formInt(r *http.Request, field string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue(field)))
	if err != nil {
		return 0, InputError("el campo " + field + " debe ser un número entero")
	}
	return n, nil
}

// --- login ---

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, r, "login", "Entrar", nil)
}

func (a *App) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if !a.login(w, r, r.PostFormValue("username"), r.PostFormValue("password")) {
		w.WriteHeader(http.StatusUnauthorized)
		a.render(w, r, "login", "Entrar", "Usuario o contraseña incorrectos")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	a.logout(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- inventario ---

func (a *App) inventoryPage(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListInventory(r.Context())
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.render(w, r, "inventario", "Inventario", items)
}

func (a *App) inventorySave(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.Atoi(r.PathValue("product_id"))
	if err != nil {
		a.htmxError(w, InputError("producto inválido"))
		return
	}
	quantity, err := formInt(r, "quantity")
	if err != nil {
		a.htmxError(w, err)
		return
	}
	minQuantity, err := formInt(r, "min_quantity")
	if err != nil {
		a.htmxError(w, err)
		return
	}
	item, err := a.store.SetInventory(r.Context(), productID, quantity, minQuantity)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "inventario", "inventory_row", item)
}

// --- productos ---

func (a *App) productsPage(w http.ResponseWriter, r *http.Request) {
	products, err := a.store.ListProducts(r.Context())
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.render(w, r, "productos", "Productos", products)
}

func (a *App) productCreate(w http.ResponseWriter, r *http.Request) {
	price, err := formInt(r, "price")
	if err != nil {
		a.htmxError(w, err)
		return
	}
	p, err := a.store.CreateProduct(r.Context(), r.PostFormValue("name"), r.PostFormValue("category"), price)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "productos", "product_row", p)
}

// --- ventas ---

type salesData struct {
	Sales     []Sale
	Products  []Product
	Customers []Customer
}

func (a *App) salesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sales, err := a.store.ListSales(ctx, 50)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	products, err := a.store.ListProducts(ctx)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	customers, err := a.store.ListCustomers(ctx)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.render(w, r, "ventas", "Ventas", salesData{Sales: sales, Products: products, Customers: customers})
}

// saleCreate arma la venta con los campos qty_<id> del formulario que tengan cantidad.
func (a *App) saleCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	customerID, err := optionalID(r.PostFormValue("customer_id"))
	if err != nil {
		a.htmxError(w, err)
		return
	}
	products, err := a.store.ListProducts(ctx)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	var lines []SaleLine
	for _, p := range products {
		v := strings.TrimSpace(r.PostFormValue("qty_" + strconv.Itoa(p.ID)))
		if v == "" || v == "0" {
			continue
		}
		qty, err := strconv.Atoi(v)
		if err != nil {
			a.htmxError(w, InputError("cantidad inválida para "+p.Name))
			return
		}
		lines = append(lines, SaleLine{ProductID: p.ID, Quantity: qty})
	}
	sale, err := a.store.CreateSale(ctx, userFrom(ctx).ID, customerID, lines)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "ventas", "sale_row", sale)
}

// --- clientes ---

func (a *App) customersPage(w http.ResponseWriter, r *http.Request) {
	customers, err := a.store.ListCustomers(r.Context())
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.render(w, r, "clientes", "Clientes", customers)
}

func (a *App) customerCreate(w http.ResponseWriter, r *http.Request) {
	c, err := a.store.CreateCustomer(r.Context(), r.PostFormValue("name"), r.PostFormValue("email"))
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "clientes", "customer_row", c)
}

// --- reseñas ---

type reviewsData struct {
	Reviews   []Review
	Products  []Product
	Customers []Customer
}

func (a *App) reviewsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reviews, err := a.store.ListReviews(ctx)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	products, err := a.store.ListProducts(ctx)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	customers, err := a.store.ListCustomers(ctx)
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.render(w, r, "resenas", "Reseñas", reviewsData{Reviews: reviews, Products: products, Customers: customers})
}

func (a *App) reviewCreate(w http.ResponseWriter, r *http.Request) {
	customerID, err := optionalID(r.PostFormValue("customer_id"))
	if err != nil {
		a.htmxError(w, err)
		return
	}
	productID, err := optionalID(r.PostFormValue("product_id"))
	if err != nil {
		a.htmxError(w, err)
		return
	}
	rating, err := formInt(r, "rating")
	if err != nil {
		a.htmxError(w, err)
		return
	}
	review, err := a.store.CreateReview(r.Context(), customerID, productID, rating, r.PostFormValue("comment"))
	if err != nil {
		a.htmxError(w, err)
		return
	}
	a.fragment(w, "resenas", "review_row", review)
}
