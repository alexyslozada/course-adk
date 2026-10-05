package main

import "google.golang.org/genai"

// ----------------------------------------------
// Salida del paso 1 (invoice_reader): los ítems de la factura
// ----------------------------------------------
var description = genai.Schema{
	Type:        genai.TypeString,
	Description: "Descripción del producto tal como aparece en la factura",
}

var quantity = genai.Schema{
	Type:        genai.TypeNumber,
	Description: "Cantidad comprada",
}

var unitPrice = genai.Schema{
	Type:        genai.TypeNumber,
	Description: "Precio unitario",
}

var total = genai.Schema{
	Type:        genai.TypeNumber,
	Description: "Total del ítem (cantidad por precio unitario)",
}

var item = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"description": &description,
		"quantity":    &quantity,
		"unit_price":  &unitPrice,
		"total":       &total,
	},
	Required: []string{"description", "quantity", "unit_price", "total"},
}

var invoiceItemsSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"items": {
			Type:  genai.TypeArray,
			Items: &item,
		},
	},
	Required: []string{"items"},
}

// ----------------------------------------------
// Salida del paso 2 (item_categorizer): los ítems agrupados por categoría
// ----------------------------------------------
var category = genai.Schema{
	Type:        genai.TypeString,
	Description: "Categoría de los ítems",
	Enum:        []string{"Café", "Lácteos", "Desechables", "Limpieza", "Otros"},
}

var subtotal = genai.Schema{
	Type:        genai.TypeNumber,
	Description: "Suma de los totales de los ítems de la categoría",
}

var categoryGroup = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"category": &category,
		"items": {
			Type:  genai.TypeArray,
			Items: &item,
		},
		"subtotal": &subtotal,
	},
	Required: []string{"category", "items", "subtotal"},
}

var categoriesSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"categories": {
			Type:  genai.TypeArray,
			Items: &categoryGroup,
		},
	},
	Required: []string{"categories"},
}
