package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// MutationExecutor handles restore mutations
type MutationExecutor struct {
	client    *ShopifyClient
	conflictResolver *ConflictResolver
	imageUploader    *ImageUploader
	logger    *log.Logger
}

// NewMutationExecutor creates a new mutation executor
func NewMutationExecutor(client *ShopifyClient) *MutationExecutor {
	return &MutationExecutor{
		client:          client,
		conflictResolver: NewConflictResolver(),
		imageUploader:    NewImageUploader(client),
		logger:          log.New(os.Stderr),
	}
}

// RestoreItem restores a single item to Shopify
func (e *MutationExecutor) RestoreItem(ctx context.Context, item Item, conflictMode ConflictMode) (*RestoreResult, error) {
	startTime := time.Now()

	result := &RestoreResult{
		ItemID:     item.ID,
		EntityType: item.Type,
		Success:    false,
	}

	// Check if we should skip this item
	if e.client.DryRun {
		result.Success = true
		result.Message = "Dry run - would be restored"
		return result, nil
	}

	var err error
	var restoredID string

	switch item.Type {
	case EntityProducts:
		restoredID, err = e.restoreProduct(ctx, item, conflictMode)
	case EntityCustomers:
		restoredID, err = e.restoreCustomer(ctx, item, conflictMode)
	case EntityOrders:
		restoredID, err = e.restoreOrder(ctx, item, conflictMode)
	case EntityCollections:
		restoredID, err = e.restoreCollection(ctx, item, conflictMode)
	case EntityMetaobjects:
		restoredID, err = e.restoreMetaobject(ctx, item, conflictMode)
	default:
		err = fmt.Errorf("unsupported entity type: %s", item.Type)
	}

	if err != nil {
		result.Message = err.Error()
		return result, err
	}

	result.RestoredID = restoredID
	result.Success = true
	result.Message = "Successfully restored"
	result.Duration = time.Since(startTime)

	return result, nil
}

// RestoreProduct restores a product via GraphQL mutation
func (e *MutationExecutor) restoreProduct(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	// Check for existing product by handle
	existingID, err := e.findProductByHandle(ctx, item.Handle)
	if err != nil {
		return "", fmt.Errorf("check existing product: %w", err)
	}

	if existingID != "" {
		// Handle conflict
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil // Already exists, skip
		case ConflictOverwrite:
			// Delete existing and recreate
			if err := e.deleteProduct(ctx, existingID); err != nil {
				return "", fmt.Errorf("delete existing product: %w", err)
			}
		case ConflictRename:
			// Generate new handle
			item.Handle = generateNewHandle(item.Handle)
		}
	}

	// Create product via GraphQL
	query := `
		mutation productCreate($input: ProductInput!) {
			productCreate(input: $input) {
				product {
					id
					handle
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	input := map[string]interface{}{
		"title":       item.Title,
		"handle":      item.Handle,
		"descriptionHtml": item.Description,
		"productType":  item.ProductType,
		"vendor":      item.Vendor,
		"tags":        item.Tags,
		"status":      "ACTIVE",
	}

	// Add SEO if available
	if item.SEO != nil {
		input["seo"] = map[string]interface{}{
			"title":       item.SEO.Title,
			"description": item.SEO.Description,
		}
	}

	variables := map[string]interface{}{
		"input": input,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create product: %w", err)
	}

	var data struct {
		ProductCreate struct {
			Product struct {
				ID     string `json:"id"`
				Handle string `json:"handle"`
			} `json:"product"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"productCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.ProductCreate.UserErrors) > 0 {
		return "", fmt.Errorf("product creation errors: %v", data.ProductCreate.UserErrors)
	}

	// Upload images if any
	if len(item.Images) > 0 {
		if err := e.imageUploader.UploadProductImages(ctx, data.ProductCreate.Product.ID, item.Images); err != nil {
			e.logger.Warnf("Failed to upload images for product %s: %v", data.ProductCreate.Product.ID, err)
		}
	}

	return data.ProductCreate.Product.ID, nil
}

// RestoreCustomer restores a customer via GraphQL mutation
func (e *MutationExecutor) restoreCustomer(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	if item.Email == nil || *item.Email == "" {
		return "", fmt.Errorf("customer email is required")
	}

	// Check for existing customer by email
	existingID, err := e.findCustomerByEmail(ctx, *item.Email)
	if err != nil {
		return "", fmt.Errorf("check existing customer: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
			// Update existing customer
			return e.updateCustomer(ctx, existingID, item)
		case ConflictRename:
			// Cannot rename customer email, skip
			return existingID, nil
		}
	}

	// Create customer via GraphQL
	query := `
		mutation customerCreate($input: CustomerInput!) {
			customerCreate(input: $input) {
				customer {
					id
					email
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	input := map[string]interface{}{
		"email":      *item.Email,
		"firstName":  item.FirstName,
		"lastName":   item.LastName,
		"phone":      item.Phone,
		"acceptsMarketing": false,
	}

	variables := map[string]interface{}{
		"input": input,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create customer: %w", err)
	}

	var data struct {
		CustomerCreate struct {
			Customer struct {
				ID    string `json:"id"`
				Email string `json:"email"`
			} `json:"customer"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"customerCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.CustomerCreate.UserErrors) > 0 {
		// Check if customer already exists error
		for _, userErr := range data.CustomerCreate.UserErrors {
			if contains(userErr.Field, "email") && strings.Contains(userErr.Message, "already taken") {
				// Customer exists, find and return
				existingID, _ := e.findCustomerByEmail(ctx, *item.Email)
				if existingID != "" {
					return existingID, nil
				}
			}
		}
		return "", fmt.Errorf("customer creation errors: %v", data.CustomerCreate.UserErrors)
	}

	return data.CustomerCreate.Customer.ID, nil
}

// RestoreOrder restores an order via REST API
// Note: Orders cannot be fully restored via API due to payment processing limitations
func (e *MutationExecutor) restoreOrder(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	// Orders can only be created via REST, and with significant limitations
	// This is a best-effort attempt - orders will likely need manual payment processing

	if item.OrderNumber == nil {
		return "", fmt.Errorf("order number is required")
	}

	// Check for existing order by order number
	existingID, err := e.findOrderByNumber(ctx, *item.OrderNumber)
	if err != nil {
		return "", fmt.Errorf("check existing order: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite, ConflictRename:
			// Orders cannot be overwritten or renamed
			return existingID, nil
		}
	}

	// Create order via REST
	req := Request{
		Method: "POST",
		Path:   "/orders.json",
		Body: map[string]interface{}{
			"order": map[string]interface{}{
				"line_items":        item.LineItems,
				"customer":          item.Customer,
				"billing_address":   item.BillingAddress,
				"shipping_address":  item.ShippingAddress,
				"financial_status":  item.FinancialStatus,
				"fulfillment_status": item.FulfillmentStatus,
				"tags":              item.Tags,
				"note":              item.Note,
			},
		},
	}

	resp, err := e.client.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create order: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("order creation failed: %d - %s", resp.StatusCode, string(resp.Body))
	}

	var data struct {
		Order struct {
			ID string `json:"id"`
		} `json:"order"`
	}

	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return data.Order.ID, nil
}

// RestoreCollection restores a collection via GraphQL mutation
func (e *MutationExecutor) restoreCollection(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	// Check for existing collection by handle
	existingID, err := e.findCollectionByHandle(ctx, item.Handle)
	if err != nil {
		return "", fmt.Errorf("check existing collection: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
			// Delete and recreate
			if err := e.deleteCollection(ctx, existingID); err != nil {
				return "", fmt.Errorf("delete existing collection: %w", err)
			}
		case ConflictRename:
			item.Handle = generateNewHandle(item.Handle)
		}
	}

	query := `
		mutation collectionCreate($input: CollectionInput!) {
			collectionCreate(input: $input) {
				collection {
					id
					handle
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	input := map[string]interface{}{
		"title":   item.Title,
		"handle":  item.Handle,
		"descriptionHtml": item.Description,
		"rules":   item.CollectionRules,
	}

	variables := map[string]interface{}{
		"input": input,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create collection: %w", err)
	}

	var data struct {
		CollectionCreate struct {
			Collection struct {
				ID     string `json:"id"`
				Handle string `json:"handle"`
			} `json:"collection"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"collectionCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.CollectionCreate.UserErrors) > 0 {
		return "", fmt.Errorf("collection creation errors: %v", data.CollectionCreate.UserErrors)
	}

	return data.CollectionCreate.Collection.ID, nil
}

// RestoreMetaobject restores a metaobject via GraphQL mutation
func (e *MutationExecutor) restoreMetaobject(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	if item.MetaobjectDefinition == nil {
		return "", fmt.Errorf("metaobject definition is required")
	}

	// Check for existing metaobject by key
	existingID, err := e.findMetaobjectByKey(ctx, *item.MetaobjectDefinition, item.Key)
	if err != nil {
		return "", fmt.Errorf("check existing metaobject: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
			// Delete and recreate
			if err := e.deleteMetaobject(ctx, existingID); err != nil {
				return "", fmt.Errorf("delete existing metaobject: %w", err)
			}
		case ConflictRename:
			item.Key = generateNewKey(item.Key)
		}
	}

	query := `
		mutation metaobjectCreate($metaobject: MetaobjectCreateInput!) {
			metaobjectCreate(metaobject: $metaobject) {
				metaobject {
					id
					key
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	input := map[string]interface{}{
		"type":   *item.MetaobjectDefinition,
		"key":    item.Key,
		"fields": item.MetaobjectFields,
		"capabilities": map[string]interface{}{
			"publishable": map[string]interface{}{
				"status": "ACTIVE",
			},
		},
	}

	variables := map[string]interface{}{
		"metaobject": input,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create metaobject: %w", err)
	}

	var data struct {
		MetaobjectCreate struct {
			Metaobject struct {
				ID  string `json:"id"`
				Key string `json:"key"`
			} `json:"metaobject"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"metaobjectCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.MetaobjectCreate.UserErrors) > 0 {
		return "", fmt.Errorf("metaobject creation errors: %v", data.MetaobjectCreate.UserErrors)
	}

	return data.MetaobjectCreate.Metaobject.ID, nil
}

// Helper methods for finding existing entities

func (e *MutationExecutor) findProductByHandle(ctx context.Context, handle string) (string, error) {
	query := `
		query findProduct($handle: String!) {
			productByHandle(handle: $handle) {
				id
			}
		}
	`

	variables := map[string]interface{}{
		"handle": handle,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", err
	}

	var data struct {
		ProductByHandle *struct {
			ID string `json:"id"`
		} `json:"productByHandle"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}

	if data.ProductByHandle != nil {
		return data.ProductByHandle.ID, nil
	}

	return "", nil
}

func (e *MutationExecutor) findCustomerByEmail(ctx context.Context, email string) (string, error) {
	query := `
		query findCustomer($email: String!) {
			customer(email: $email) {
				id
			}
		}
	`

	variables := map[string]interface{}{
		"email": email,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", err
	}

	var data struct {
		Customer *struct {
			ID string `json:"id"`
		} `json:"customer"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}

	if data.Customer != nil {
		return data.Customer.ID, nil
	}

	return "", nil
}

func (e *MutationExecutor) findOrderByNumber(ctx context.Context, orderNumber string) (string, error) {
	// Use REST API for order lookup
	req := Request{
		Method: "GET",
		Path:   fmt.Sprintf("/orders.json?name=%s", orderNumber),
	}

	resp, err := e.client.Do(ctx, req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 404 {
		return "", nil
	}

	var data struct {
		Orders []struct {
			ID string `json:"id"`
		} `json:"orders"`
	}

	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return "", err
	}

	if len(data.Orders) > 0 {
		return data.Orders[0].ID, nil
	}

	return "", nil
}

func (e *MutationExecutor) findCollectionByHandle(ctx context.Context, handle string) (string, error) {
	query := `
		query findCollection($handle: String!) {
			collectionByHandle(handle: $handle) {
				id
			}
		}
	`

	variables := map[string]interface{}{
		"handle": handle,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", err
	}

	var data struct {
		CollectionByHandle *struct {
			ID string `json:"id"`
		} `json:"collectionByHandle"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}

	if data.CollectionByHandle != nil {
		return data.CollectionByHandle.ID, nil
	}

	return "", nil
}

func (e *MutationExecutor) findMetaobjectByKey(ctx context.Context, definition, key string) (string, error) {
	query := `
		query findMetaobject($type: String!, $key: String!) {
			metaobject(type: $type, key: $key) {
				id
			}
		}
	`

	variables := map[string]interface{}{
		"type": definition,
		"key":  key,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", err
	}

	var data struct {
		Metaobject *struct {
			ID string `json:"id"`
		} `json:"metaobject"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}

	if data.Metaobject != nil {
		return data.Metaobject.ID, nil
	}

	return "", nil
}

// Helper methods for delete/update operations

func (e *MutationExecutor) deleteProduct(ctx context.Context, id string) error {
	query := `
		mutation productDelete($input: ProductDeleteInput!) {
			productDelete(input: $input) {
				deletedId
				userErrors {
					field
					message
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"id": id,
		},
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return err
	}

	var data struct {
		ProductDelete struct {
			DeletedID string `json:"deletedId"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"productDelete"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return err
	}

	if len(data.ProductDelete.UserErrors) > 0 {
		return fmt.Errorf("delete errors: %v", data.ProductDelete.UserErrors)
	}

	return nil
}

func (e *MutationExecutor) deleteCollection(ctx context.Context, id string) error {
	query := `
		mutation collectionDelete($input: CollectionDeleteInput!) {
			collectionDelete(input: $input) {
				deletedId
				userErrors {
					field
					message
				}
			}
		}
	`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"id": id,
		},
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return err
	}

	var data struct {
		CollectionDelete struct {
			DeletedID string `json:"deletedId"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"collectionDelete"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return err
	}

	if len(data.CollectionDelete.UserErrors) > 0 {
		return fmt.Errorf("delete errors: %v", data.CollectionDelete.UserErrors)
	}

	return nil
}

func (e *MutationExecutor) deleteMetaobject(ctx context.Context, id string) error {
	query := `
		mutation metaobjectDelete($id: ID!) {
			metaobjectDelete(id: $id) {
				deletedId
				userErrors {
					field
					message
				}
			}
		}
	`

	variables := map[string]interface{}{
		"id": id,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return err
	}

	var data struct {
		MetaobjectDelete struct {
			DeletedID string `json:"deletedId"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"metaobjectDelete"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return err
	}

	if len(data.MetaobjectDelete.UserErrors) > 0 {
		return fmt.Errorf("delete errors: %v", data.MetaobjectDelete.UserErrors)
	}

	return nil
}

func (e *MutationExecutor) updateCustomer(ctx context.Context, id string, item Item) (string, error) {
	query := `
		mutation customerUpdate($input: CustomerUpdateInput!) {
			customerUpdate(input: $input) {
				customer {
					id
					email
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	input := map[string]interface{}{
		"id":         id,
		"email":      *item.Email,
		"firstName":  item.FirstName,
		"lastName":   item.LastName,
		"phone":      item.Phone,
	}

	variables := map[string]interface{}{
		"input": input,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("update customer: %w", err)
	}

	var data struct {
		CustomerUpdate struct {
			Customer struct {
				ID    string `json:"id"`
				Email string `json:"email"`
			} `json:"customer"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"customerUpdate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.CustomerUpdate.UserErrors) > 0 {
		return "", fmt.Errorf("customer update errors: %v", data.CustomerUpdate.UserErrors)
	}

	return data.CustomerUpdate.Customer.ID, nil
}

// Helper functions

func generateNewHandle(handle string) string {
	// Simple approach: append timestamp
	return fmt.Sprintf("%s-%d", handle, time.Now().Unix())
}

func generateNewKey(key string) string {
	// Simple approach: append timestamp
	return fmt.Sprintf("%s-%d", key, time.Now().Unix())
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}