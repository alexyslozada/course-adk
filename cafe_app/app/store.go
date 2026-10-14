package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// InputError es un error causado por datos inválidos del usuario (HTTP 400).
type InputError string

func (e InputError) Error() string { return string(e) }

var ErrNotFound = errors.New("no encontrado")

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

type Product struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	Active   bool   `json:"active"`
}

type InventoryItem struct {
	ProductID   int    `json:"product_id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Quantity    int    `json:"quantity"`
	MinQuantity int    `json:"min_quantity"`
}

func (i InventoryItem) Low() bool { return i.Quantity <= i.MinQuantity }

type Customer struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type SaleItem struct {
	ProductID int    `json:"product_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unit_price"`
}

type Sale struct {
	ID         int    `json:"id"`
	CustomerID *int   `json:"customer_id"`
	Customer   string `json:"customer"`
	Cashier    string `json:"cashier"`
	Total      int    `json:"total"`
	// DiscountPercent es el descuento ya aplicado; Total es el valor final, con el descuento incluido.
	DiscountPercent int        `json:"discount_percent"`
	CreatedAt       time.Time  `json:"created_at"`
	Items           []SaleItem `json:"items"`
}

// SaleLine es lo que se pide al crear una venta: producto y cantidad.
type SaleLine struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type Review struct {
	ID         int       `json:"id"`
	CustomerID *int      `json:"customer_id"`
	Customer   string    `json:"customer"`
	ProductID  *int      `json:"product_id"`
	Product    string    `json:"product"`
	Rating     int       `json:"rating"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store concentra todo el acceso a SQLite. Lo comparten la API JSON y el frontend.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// --- usuarios ---

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, name, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, name, role) VALUES (?, ?, ?, ?)`,
		username, passwordHash, name, role)
	return err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// UserByUsername devuelve el usuario y el hash de su contraseña.
func (s *Store) UserByUsername(ctx context.Context, username string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, name, role, password_hash FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.Name, &u.Role, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, hash, err
}

// --- productos e inventario ---

func (s *Store) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, category, price, active FROM products ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Price, &p.Active); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

// CreateProduct crea el producto y su fila de inventario en cero.
func (s *Store) CreateProduct(ctx context.Context, name, category string, price int) (Product, error) {
	name, category = strings.TrimSpace(name), strings.TrimSpace(category)
	if name == "" || category == "" {
		return Product{}, InputError("el nombre y la categoría son obligatorios")
	}
	if price <= 0 {
		return Product{}, InputError("el precio debe ser mayor a cero")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO products (name, category, price) VALUES (?, ?, ?)`, name, category, price)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Product{}, InputError("ya existe un producto con ese nombre")
		}
		return Product{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Product{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO inventory (product_id) VALUES (?)`, id); err != nil {
		return Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return Product{}, err
	}
	return Product{ID: int(id), Name: name, Category: category, Price: price, Active: true}, nil
}

const inventorySelect = `
	SELECT p.id, p.name, p.category, i.quantity, i.min_quantity
	FROM inventory i JOIN products p ON p.id = i.product_id`

func (s *Store) ListInventory(ctx context.Context) ([]InventoryItem, error) {
	rows, err := s.db.QueryContext(ctx, inventorySelect+` ORDER BY p.category, p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []InventoryItem{}
	for rows.Next() {
		var i InventoryItem
		if err := rows.Scan(&i.ProductID, &i.Name, &i.Category, &i.Quantity, &i.MinQuantity); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func (s *Store) GetInventoryItem(ctx context.Context, productID int) (InventoryItem, error) {
	var i InventoryItem
	err := s.db.QueryRowContext(ctx, inventorySelect+` WHERE p.id = ?`, productID).
		Scan(&i.ProductID, &i.Name, &i.Category, &i.Quantity, &i.MinQuantity)
	if errors.Is(err, sql.ErrNoRows) {
		return InventoryItem{}, ErrNotFound
	}
	return i, err
}

// SetInventory fija la cantidad y el mínimo de un producto.
func (s *Store) SetInventory(ctx context.Context, productID, quantity, minQuantity int) (InventoryItem, error) {
	if quantity < 0 || minQuantity < 0 {
		return InventoryItem{}, InputError("las cantidades no pueden ser negativas")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE inventory SET quantity = ?, min_quantity = ? WHERE product_id = ?`,
		quantity, minQuantity, productID)
	if err != nil {
		return InventoryItem{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return InventoryItem{}, ErrNotFound
	}
	return s.GetInventoryItem(ctx, productID)
}

// --- clientes ---

func (s *Store) ListCustomers(ctx context.Context) ([]Customer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, email FROM customers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	customers := []Customer{}
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

func (s *Store) CreateCustomer(ctx context.Context, name, email string) (Customer, error) {
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	if name == "" {
		return Customer{}, InputError("el nombre es obligatorio")
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO customers (name, email) VALUES (?, ?)`, name, email)
	if err != nil {
		return Customer{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Customer{}, err
	}
	return Customer{ID: int(id), Name: name, Email: email}, nil
}

// --- ventas ---

// CreateSale registra una venta y descuenta el stock en una sola transacción.
func (s *Store) CreateSale(ctx context.Context, userID int, customerID *int, lines []SaleLine) (Sale, error) {
	if len(lines) == 0 {
		return Sale{}, InputError("la venta necesita al menos un producto")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Sale{}, err
	}
	defer tx.Rollback()

	items := make([]SaleItem, 0, len(lines))
	total := 0
	seen := map[int]bool{}
	for _, l := range lines {
		if l.Quantity <= 0 {
			return Sale{}, InputError("las cantidades deben ser mayores a cero")
		}
		if seen[l.ProductID] {
			return Sale{}, InputError("un producto aparece repetido en la venta")
		}
		seen[l.ProductID] = true

		var item SaleItem
		var active bool
		err := tx.QueryRowContext(ctx, `SELECT id, name, price, active FROM products WHERE id = ?`, l.ProductID).
			Scan(&item.ProductID, &item.Name, &item.UnitPrice, &active)
		if errors.Is(err, sql.ErrNoRows) {
			return Sale{}, InputError(fmt.Sprintf("el producto %d no existe", l.ProductID))
		}
		if err != nil {
			return Sale{}, err
		}
		if !active {
			return Sale{}, InputError(fmt.Sprintf("%s no está disponible", item.Name))
		}
		item.Quantity = l.Quantity

		res, err := tx.ExecContext(ctx,
			`UPDATE inventory SET quantity = quantity - ? WHERE product_id = ? AND quantity >= ?`,
			l.Quantity, l.ProductID, l.Quantity)
		if err != nil {
			return Sale{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Sale{}, InputError(fmt.Sprintf("no hay stock suficiente de %s", item.Name))
		}
		total += item.UnitPrice * item.Quantity
		items = append(items, item)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO sales (customer_id, user_id, total) VALUES (?, ?, ?)`, customerID, userID, total)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return Sale{}, InputError("el cliente no existe")
		}
		return Sale{}, err
	}
	saleID, err := res.LastInsertId()
	if err != nil {
		return Sale{}, err
	}
	for _, it := range items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sale_items (sale_id, product_id, quantity, unit_price) VALUES (?, ?, ?, ?)`,
			saleID, it.ProductID, it.Quantity, it.UnitPrice); err != nil {
			return Sale{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Sale{}, err
	}
	return s.GetSale(ctx, int(saleID))
}

const saleSelect = `
	SELECT s.id, s.customer_id, COALESCE(c.name, ''), u.name, s.total, s.discount_percent, s.created_at
	FROM sales s
	JOIN users u ON u.id = s.user_id
	LEFT JOIN customers c ON c.id = s.customer_id`

func scanSale(row interface{ Scan(...any) error }) (Sale, error) {
	var sale Sale
	err := row.Scan(&sale.ID, &sale.CustomerID, &sale.Customer, &sale.Cashier, &sale.Total, &sale.DiscountPercent, &sale.CreatedAt)
	return sale, err
}

func (s *Store) GetSale(ctx context.Context, id int) (Sale, error) {
	sale, err := scanSale(s.db.QueryRowContext(ctx, saleSelect+` WHERE s.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Sale{}, ErrNotFound
	}
	if err != nil {
		return Sale{}, err
	}
	if err := s.loadItems(ctx, []*Sale{&sale}); err != nil {
		return Sale{}, err
	}
	return sale, nil
}

// MaxDiscountPercent es el mayor descuento que se puede aplicar a una venta.
const MaxDiscountPercent = 50

// ApplyDiscount aplica un descuento a una venta ya registrada y recalcula su total.
// Solo se puede una vez por venta.
func (s *Store) ApplyDiscount(ctx context.Context, saleID, percent int) (Sale, error) {
	if percent < 1 || percent > MaxDiscountPercent {
		return Sale{}, InputError(fmt.Sprintf("el descuento debe estar entre 1 y %d por ciento", MaxDiscountPercent))
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Sale{}, err
	}
	defer tx.Rollback()

	var current, subtotal int
	err = tx.QueryRowContext(ctx, `
		SELECT s.discount_percent, COALESCE(SUM(si.quantity * si.unit_price), 0)
		FROM sales s LEFT JOIN sale_items si ON si.sale_id = s.id
		WHERE s.id = ? GROUP BY s.id`, saleID).Scan(&current, &subtotal)
	if errors.Is(err, sql.ErrNoRows) {
		return Sale{}, ErrNotFound
	}
	if err != nil {
		return Sale{}, err
	}
	if current > 0 {
		return Sale{}, InputError(fmt.Sprintf("la venta %d ya tiene un descuento del %d por ciento", saleID, current))
	}
	total := subtotal * (100 - percent) / 100
	if _, err := tx.ExecContext(ctx, `UPDATE sales SET total = ?, discount_percent = ? WHERE id = ?`, total, percent, saleID); err != nil {
		return Sale{}, err
	}
	if err := tx.Commit(); err != nil {
		return Sale{}, err
	}
	return s.GetSale(ctx, saleID)
}

// ListSales devuelve las últimas ventas, la más reciente primero.
func (s *Store) ListSales(ctx context.Context, limit int) ([]Sale, error) {
	rows, err := s.db.QueryContext(ctx, saleSelect+` ORDER BY s.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sales := []Sale{}
	for rows.Next() {
		sale, err := scanSale(rows)
		if err != nil {
			return nil, err
		}
		sales = append(sales, sale)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ptrs := make([]*Sale, len(sales))
	for i := range sales {
		ptrs[i] = &sales[i]
	}
	return sales, s.loadItems(ctx, ptrs)
}

func (s *Store) loadItems(ctx context.Context, sales []*Sale) error {
	for _, sale := range sales {
		rows, err := s.db.QueryContext(ctx,
			`SELECT si.product_id, p.name, si.quantity, si.unit_price
			 FROM sale_items si JOIN products p ON p.id = si.product_id
			 WHERE si.sale_id = ? ORDER BY p.name`, sale.ID)
		if err != nil {
			return err
		}
		sale.Items = []SaleItem{}
		for rows.Next() {
			var it SaleItem
			if err := rows.Scan(&it.ProductID, &it.Name, &it.Quantity, &it.UnitPrice); err != nil {
				rows.Close()
				return err
			}
			sale.Items = append(sale.Items, it)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// --- reseñas ---

const reviewSelect = `
	SELECT r.id, r.customer_id, COALESCE(c.name, ''), r.product_id, COALESCE(p.name, ''),
	       r.rating, r.comment, r.created_at
	FROM reviews r
	LEFT JOIN customers c ON c.id = r.customer_id
	LEFT JOIN products p ON p.id = r.product_id`

func scanReview(row interface{ Scan(...any) error }) (Review, error) {
	var r Review
	err := row.Scan(&r.ID, &r.CustomerID, &r.Customer, &r.ProductID, &r.Product, &r.Rating, &r.Comment, &r.CreatedAt)
	return r, err
}

func (s *Store) ListReviews(ctx context.Context) ([]Review, error) {
	rows, err := s.db.QueryContext(ctx, reviewSelect+` ORDER BY r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := []Review{}
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

func (s *Store) CreateReview(ctx context.Context, customerID, productID *int, rating int, comment string) (Review, error) {
	if rating < 1 || rating > 5 {
		return Review{}, InputError("la calificación debe estar entre 1 y 5")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reviews (customer_id, product_id, rating, comment) VALUES (?, ?, ?, ?)`,
		customerID, productID, rating, strings.TrimSpace(comment))
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return Review{}, InputError("el cliente o el producto no existe")
		}
		return Review{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Review{}, err
	}
	return scanReview(s.db.QueryRowContext(ctx, reviewSelect+` WHERE r.id = ?`, id))
}
