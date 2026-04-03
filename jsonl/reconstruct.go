package jsonl

import (
	"encoding/json"
	"fmt"
	"io"
)

// GID type prefixes for routing children to parents
const (
	GIDLineItem         = "gid://shopify/LineItem/"
	GIDOrderTransaction = "gid://shopify/OrderTransaction/"
	GIDOrderFulfillment = "gid://shopify/Fulfillment/"
	GIDOrderRefund      = "gid://shopify/Refund/"
	GIDProductVariant   = "gid://shopify/ProductVariant/"
	GIDProductImage     = "gid://shopify/ProductImage/"
	GIDMetafield        = "gid://shopify/Metafield/"
	GIDCustomerAddress  = "gid://shopify/MailingAddress/"
	GIDCollection       = "gid://shopify/Collection/"
)

// ReconstructBulkData takes flat JSONL records and builds nested structures
// Shopify bulk operations return flat JSONL; this rebuilds the hierarchy
func ReconstructBulkData(records []map[string]interface{}) ([]map[string]interface{}, error) {
	rootObjects := make(map[string]interface{})
	rootOrder := make([]string, 0)

	for _, record := range records {
		if isRootObject(record) {
			id, _ := record["id"].(string)
			if id == "" {
				continue
			}
			rootObjects[id] = record
			rootOrder = append(rootOrder, id)
		}
	}

	// Second pass: attach children to parents
	for _, record := range records {
		parentID, hasParent := record["__parentId"].(string)
		if !hasParent {
			continue
		}

		parent, exists := rootObjects[parentID]
		if !exists {
			// Parent not found, could be a separate root object not in this batch
			continue
		}

		attachToParent(parent.(map[string]interface{}), record)
	}

	// Build result in original order
	result := make([]map[string]interface{}, 0, len(rootOrder))
	for _, id := range rootOrder {
		if obj, exists := rootObjects[id]; exists {
			result = append(result, obj.(map[string]interface{}))
		}
	}

	return result, nil
}

// isRootObject checks if a record is a root-level object (no __parentId or root type)
func isRootObject(record map[string]interface{}) bool {
	if _, hasParent := record["__parentId"]; hasParent {
		return false
	}

	typename, _ := record["__typename"].(string)
	if typename == "" {
		return true
	}

	// Root types
	switch typename {
	case "Product", "Customer", "Order", "Collection":
		return true
	default:
		return false
	}
}

// attachToParent attaches a child record to its parent based on GID prefix or __typename
func attachToParent(parent, child map[string]interface{}) {
	childID, _ := child["id"].(string)
	typename, _ := child["__typename"].(string)

	// Route by GID prefix if __typename is not available
	if typename == "" && childID != "" {
		switch {
		case hasPrefix(childID, GIDProductVariant):
			typename = "ProductVariant"
		case hasPrefix(childID, GIDProductImage):
			typename = "ProductImage"
		case hasPrefix(childID, GIDLineItem):
			typename = "LineItem"
		case hasPrefix(childID, GIDOrderTransaction):
			typename = "OrderTransaction"
		case hasPrefix(childID, GIDOrderFulfillment):
			typename = "Fulfillment"
		case hasPrefix(childID, GIDOrderRefund):
			typename = "Refund"
		case hasPrefix(childID, GIDCustomerAddress):
			typename = "MailingAddress"
		case hasPrefix(childID, GIDMetafield):
			typename = "Metafield"
		}
	}

	// Remove __parentId from child
	delete(child, "__parentId")

	switch typename {
	case "ProductVariant":
		attachToSlice(parent, child, "variants")
	case "ProductImage":
		attachToSlice(parent, child, "images")
	case "Metafield":
		attachToSlice(parent, child, "metafields")
	case "LineItem":
		attachToSlice(parent, child, "lineItems")
	case "OrderTransaction":
		attachToSlice(parent, child, "transactions")
	case "Fulfillment":
		attachToSlice(parent, child, "fulfillments")
	case "Refund":
		attachToSlice(parent, child, "refunds")
	case "MailingAddress":
		attachAsSingle(parent, child, "addresses")
	default:
		// Unknown type, skip
	}
}

// attachToSlice attaches child to a slice field in parent, creating it if needed
func attachToSlice(parent, child map[string]interface{}, field string) {
	slice, exists := parent[field]
	if !exists {
		parent[field] = []map[string]interface{}{child}
		return
	}

	sliceVal, ok := slice.([]map[string]interface{})
	if !ok {
		// Field exists but is wrong type, replace
		parent[field] = []map[string]interface{}{child}
		return
	}

	parent[field] = append(sliceVal, child)
}

// attachAsSingle attaches child as a single value (for addresses which can be multiple)
func attachAsSingle(parent, child map[string]interface{}, field string) {
	slice, exists := parent[field]
	if !exists {
		parent[field] = []map[string]interface{}{child}
		return
	}

	sliceVal, ok := slice.([]map[string]interface{})
	if !ok {
		parent[field] = []map[string]interface{}{child}
		return
	}

	parent[field] = append(sliceVal, child)
}

// hasPrefix checks if a string has the given prefix
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// NormalizeJSONL converts reconstructed data to a clean JSON structure
// Removes internal Shopify fields like __typename
func NormalizeJSONL(data []map[string]interface{}) ([]byte, error) {
	cleaned := make([]map[string]interface{}, len(data))
	for i, item := range data {
		cleaned[i] = removeInternalFields(item)
	}
	return json.MarshalIndent(cleaned, "", "  ")
}

// removeInternalFields recursively removes __typename and other internal fields
func removeInternalFields(obj map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range obj {
		if k == "__typename" || k == "__parentId" {
			continue
		}

		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = removeInternalFields(val)
		case []interface{}:
			result[k] = cleanSlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// cleanSlice recursively cleans a slice
func cleanSlice(slice []interface{}) []interface{} {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = removeInternalFields(val)
		case []interface{}:
			result[i] = cleanSlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// StreamReconstruct reconstructs JSONL data from a reader without loading all records into memory
// Writes reconstructed JSON directly to the provided writer
func StreamReconstruct(records []map[string]interface{}, output io.Writer) error {
	result, err := ReconstructBulkData(records)
	if err != nil {
		return fmt.Errorf("reconstruction failed: %w", err)
	}

	jsonData, err := NormalizeJSONL(result)
	if err != nil {
		return fmt.Errorf("JSON normalization failed: %w", err)
	}

	_, err = output.Write(jsonData)
	return err
}
