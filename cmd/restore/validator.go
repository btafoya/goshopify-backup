package main

import (
	"fmt"
	"strings"
)

// ValidateRestoreItems checks items for potential issues before restore
type RestoreValidator struct {
	warnings []string
	errors   []string
}

// NewRestoreValidator creates a new validator
func NewRestoreValidator() *RestoreValidator {
	return &RestoreValidator{}
}

// ValidateItem validates a single item before restore
func (v *RestoreValidator) ValidateItem(item Item) {
	switch item.Type {
	case EntityProducts:
		v.validateProduct(item)
	case EntityCustomers:
		v.validateCustomer(item)
	case EntityOrders:
		v.validateOrder(item)
	case EntityCollections:
		v.validateCollection(item)
	case EntityMetaobjects:
		v.validateMetaobject(item)
	}
}

// validateProduct checks product data for issues
func (v *RestoreValidator) validateProduct(item Item) {
	if item.Title == "" {
		v.errors = append(v.errors, fmt.Sprintf("Product %s: missing title", item.ID))
	}
	if item.Handle == "" {
		v.warnings = append(v.warnings, fmt.Sprintf("Product %s: missing handle, will be auto-generated", item.ID))
	}
	if len(item.Variants) > 100 {
		v.warnings = append(v.warnings, fmt.Sprintf("Product %s: has %d variants (Shopify limit is 100 per product)", item.ID, len(item.Variants)))
	}
	for _, variant := range item.Variants {
		if variant.Price == "" {
			v.errors = append(v.errors, fmt.Sprintf("Product %s variant %s: missing price", item.ID, variant.Title))
		}
	}
}

// validateCustomer checks customer data for issues
func (v *RestoreValidator) validateCustomer(item Item) {
	if item.Email == nil || *item.Email == "" {
		v.errors = append(v.errors, fmt.Sprintf("Customer %s: missing email (required)", item.ID))
	}
	for _, addr := range item.Addresses {
		if addr.Address1 == "" || addr.City == "" || addr.Country == "" {
			v.warnings = append(v.warnings, fmt.Sprintf("Customer %s: address missing required fields (address1, city, country)", item.ID))
			break
		}
	}
}

// validateOrder checks order data for issues
func (v *RestoreValidator) validateOrder(item Item) {
	if item.OrderNumber == nil {
		v.errors = append(v.errors, fmt.Sprintf("Order %s: missing order number", item.ID))
	}
	if len(item.LineItems) == 0 {
		v.warnings = append(v.warnings, fmt.Sprintf("Order %s: no line items, restore may be incomplete", item.ID))
	}
	v.warnings = append(v.warnings, fmt.Sprintf("Order %s: orders cannot be fully restored via API (payment processing limitation)", item.ID))
}

// validateCollection checks collection data for issues
func (v *RestoreValidator) validateCollection(item Item) {
	if item.Title == "" {
		v.errors = append(v.errors, fmt.Sprintf("Collection %s: missing title", item.ID))
	}
	if len(item.CollectionProducts) > 0 {
		v.warnings = append(v.warnings, fmt.Sprintf("Collection %s: product links will need ID mapping (source IDs differ from target)", item.ID))
	}
}

// validateMetaobject checks metaobject data for issues
func (v *RestoreValidator) validateMetaobject(item Item) {
	defType, _ := item.CustomData["metaobjectDefinition"].(string)
	if defType == "" {
		v.errors = append(v.errors, fmt.Sprintf("Metaobject %s: missing definition type", item.ID))
	}
	fields, _ := item.CustomData["metaobjectFields"].(map[string]interface{})
	if len(fields) == 0 {
		v.warnings = append(v.warnings, fmt.Sprintf("Metaobject %s: no fields defined", item.ID))
	}
}

// ValidateRelationships checks for cross-entity dependency issues
func (v *RestoreValidator) ValidateRelationships(items []Item) {
	// Count entities by type
	productIDs := make(map[string]bool)
	collectionProductRefs := make(map[string][]string) // collectionID -> referenced product IDs

	for _, item := range items {
		switch item.Type {
		case EntityProducts:
			productIDs[item.ID] = true
		case EntityCollections:
			for _, pid := range item.CollectionProducts {
				collectionProductRefs[item.ID] = append(collectionProductRefs[item.ID], pid)
			}
		}
	}

	// Check collection product references
	for collectionID, pids := range collectionProductRefs {
		for _, pid := range pids {
			if !productIDs[pid] {
				v.warnings = append(v.warnings, fmt.Sprintf("Collection %s references product %s not in restore set - link will be skipped", collectionID, pid))
			}
		}
	}
}

// HasErrors returns true if any validation errors were found
func (v *RestoreValidator) HasErrors() bool {
	return len(v.errors) > 0
}

// HasWarnings returns true if any warnings were found
func (v *RestoreValidator) HasWarnings() bool {
	return len(v.warnings) > 0
}

// GetErrors returns all validation errors
func (v *RestoreValidator) GetErrors() []string {
	return v.errors
}

// GetWarnings returns all validation warnings
func (v *RestoreValidator) GetWarnings() []string {
	return v.warnings
}

// Summary returns a formatted summary of all validation issues
func (v *RestoreValidator) Summary() string {
	var b strings.Builder

	if len(v.errors) > 0 {
		b.WriteString("VALIDATION ERRORS (must fix before restore):\n")
		for _, e := range v.errors {
			b.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	if len(v.warnings) > 0 {
		b.WriteString("\nWARNINGS (review before proceeding):\n")
		for _, w := range v.warnings {
			b.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}

	if len(v.errors) == 0 && len(v.warnings) == 0 {
		b.WriteString("All items passed validation.\n")
	}

	return b.String()
}