package main

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// seed carga datos de ejemplo la primera vez (si no hay usuarios).
func seed(ctx context.Context, s *Store) error {
	n, err := s.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	users := []struct{ username, password, name, role string }{
		{"admin", "admin123", "Alexys", "admin"},
		{"cajero", "cajero123", "Camila", "cashier"},
	}
	for _, u := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := s.CreateUser(ctx, u.username, string(hash), u.name, u.role); err != nil {
			return err
		}
	}

	// stock y mínimo por producto; el pan de queso arranca bajo a propósito
	// (en la fase 8 el administrador se lo pide a la panadería).
	products := []struct {
		name, category string
		price, stock   int
		min            int
	}{
		{"Espresso", "Bebidas", 5000, 80, 20},
		{"Americano", "Bebidas", 6000, 80, 20},
		{"Cappuccino", "Bebidas", 8000, 60, 15},
		{"Latte", "Bebidas", 8500, 60, 15},
		{"Chocolate caliente", "Bebidas", 9000, 40, 10},
		{"Croissant", "Panadería", 6500, 25, 8},
		{"Pan de queso", "Panadería", 4000, 3, 10},
		{"Sándwich de jamón y queso", "Comida", 12000, 15, 5},
		{"Torta de zanahoria", "Postres", 9500, 12, 4},
		{"Brownie", "Postres", 7000, 20, 6},
	}
	ids := map[string]int{}
	for _, p := range products {
		created, err := s.CreateProduct(ctx, p.name, p.category, p.price)
		if err != nil {
			return fmt.Errorf("producto %s: %w", p.name, err)
		}
		if _, err := s.SetInventory(ctx, created.ID, p.stock, p.min); err != nil {
			return err
		}
		ids[p.name] = created.ID
	}

	customerIDs := []int{}
	for _, c := range []struct{ name, email string }{
		{"Laura Gómez", "laura@example.com"},
		{"Carlos Pérez", "carlos@example.com"},
		{"Marta Ríos", "marta@example.com"},
		{"Andrés Vélez", ""},
	} {
		created, err := s.CreateCustomer(ctx, c.name, c.email)
		if err != nil {
			return err
		}
		customerIDs = append(customerIDs, created.ID)
	}

	sales := []struct {
		customer int
		lines    []SaleLine
	}{
		{0, []SaleLine{{ids["Cappuccino"], 2}, {ids["Croissant"], 1}}},
		{1, []SaleLine{{ids["Americano"], 1}, {ids["Brownie"], 1}}},
		{2, []SaleLine{{ids["Latte"], 1}, {ids["Torta de zanahoria"], 1}}},
		{0, []SaleLine{{ids["Espresso"], 2}}},
	}
	for _, sale := range sales {
		if _, err := s.CreateSale(ctx, 2, &customerIDs[sale.customer], sale.lines); err != nil {
			return fmt.Errorf("venta de ejemplo: %w", err)
		}
	}

	reviews := []struct {
		customer, rating int
		product, comment string
	}{
		{0, 5, "Cappuccino", "El mejor cappuccino del barrio."},
		{1, 4, "Brownie", "Muy bueno, un poco dulce."},
		{2, 2, "Latte", "Llegó tibio."},
		{0, 5, "Croissant", "Recién horneado, delicioso."},
	}
	for _, r := range reviews {
		pid := ids[r.product]
		if _, err := s.CreateReview(ctx, &customerIDs[r.customer], &pid, r.rating, r.comment); err != nil {
			return err
		}
	}
	return nil
}
