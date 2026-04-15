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
	client           *ShopifyClient
	conflictResolver *ConflictResolver
	imageUploader    *ImageUploader
	logger           *log.Logger
}

// NewMutationExecutor creates a new mutation executor
func NewMutationExecutor(client *ShopifyClient) *MutationExecutor {
	return &MutationExecutor{
		client:           client,
		conflictResolver: NewConflictResolver(),
		imageUploader:    NewImageUploader(client),
		logger:           log.New(os.Stderr),
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

	if e.client.DryRun {
		result.Success = true
		result.Message = "Dry run - would be restored"
		return result, nil
	}

	// Apply restore tag (D7)
	item = e.applyRestoreTag(item)

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

// applyRestoreTag appends the restore tag to item tags (D7)
func (e *MutationExecutor) applyRestoreTag(item Item) Item {
	if len(item.Tags) == 0 {
		item.Tags = []string{}
	}
	for _, tag := range item.Tags {
		if tag == RestoreTag {
			return item // Already tagged
		}
	}
	item.Tags = append(item.Tags, RestoreTag)
	return item
}

// restoreProduct restores a product via GraphQL mutation
func (e *MutationExecutor) restoreProduct(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	existingID, err := e.findProductByHandle(ctx, item.Handle)
	if err != nil {
		return "", fmt.Errorf("check existing product: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
			if err := e.deleteProduct(ctx, existingID); err != nil {
				return "", fmt.Errorf("delete existing product: %w", err)
			}
		case ConflictRename:
			item.Handle = generateNewHandle(item.Handle)
		}
	}

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
		"title":          item.Title,
		"handle":         item.Handle,
		"descriptionHtml": item.Description,
		"productType":    item.ProductType,
		"vendor":         item.Vendor,
		"tags":           item.Tags,
		"status":         "ACTIVE",
	}

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

	productID := data.ProductCreate.Product.ID

	// Upload images
	if len(item.Images) > 0 {
		if err := e.imageUploader.UploadProductImages(ctx, productID, item.Images); err != nil {
			e.logger.Warnf("Failed to upload images for product %s: %v", productID, err)
		}
	}

	// Restore product variants (D1)
	if len(item.Variants) > 0 {
		if err := e.restoreProductVariants(ctx, productID, item.Variants); err != nil {
			e.logger.Warnf("Failed to restore variants for product %s: %v", productID, err)
		}
	}

	// Restore metafields (D2)
	if len(item.Metafields) > 0 {
		if err := e.restoreMetafields(ctx, "PRODUCT", productID, item.Metafields); err != nil {
			e.logger.Warnf("Failed to restore metafields for product %s: %v", productID, err)
		}
	}

	return productID, nil
}

// restoreCustomer restores a customer via GraphQL mutation
func (e *MutationExecutor) restoreCustomer(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	if item.Email == nil || *item.Email == "" {
		return "", fmt.Errorf("customer email is required")
	}

	existingID, err := e.findCustomerByEmail(ctx, *item.Email)
	if err != nil {
		return "", fmt.Errorf("check existing customer: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
			return e.updateCustomer(ctx, existingID, item)
		case ConflictRename:
			return existingID, nil
		}
	}

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
		"email":            *item.Email,
		"firstName":        item.FirstName,
		"lastName":         item.LastName,
		"phone":           item.Phone,
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
		for _, userErr := range data.CustomerCreate.UserErrors {
			if contains(userErr.Field, "email") && strings.Contains(userErr.Message, "already taken") {
				existingID, _ := e.findCustomerByEmail(ctx, *item.Email)
				if existingID != "" {
					return existingID, nil
				}
			}
		}
		return "", fmt.Errorf("customer creation errors: %v", data.CustomerCreate.UserErrors)
	}

	customerID := data.CustomerCreate.Customer.ID

	// Restore customer addresses (D3)
	if len(item.Addresses) > 0 {
		if err := e.restoreCustomerAddresses(ctx, customerID, item.Addresses); err != nil {
			e.logger.Warnf("Failed to restore addresses for customer %s: %v", customerID, err)
		}
	}

	// Restore metafields (D2)
	if len(item.Metafields) > 0 {
		if err := e.restoreMetafields(ctx, "CUSTOMER", customerID, item.Metafields); err != nil {
			e.logger.Warnf("Failed to restore metafields for customer %s: %v", customerID, err)
		}
	}

	return customerID, nil
}

// restoreOrder restores an order via REST API
func (e *MutationExecutor) restoreOrder(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	if item.OrderNumber == nil {
		return "", fmt.Errorf("order number is required")
	}

	existingID, err := e.findOrderByNumber(ctx, *item.OrderNumber)
	if err != nil {
		return "", fmt.Errorf("check existing order: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite, ConflictRename:
			return existingID, nil
		}
	}

	req := Request{
		Method: "POST",
		Path:   "/orders.json",
		Body: map[string]interface{}{
			"order": map[string]interface{}{
				"line_items":        item.LineItems,
				"customer":         item.Customer,
				"billing_address":  item.BillingAddress,
				"shipping_address": item.ShippingAddress,
				"financial_status":  item.FinancialStatus,
				"fulfillment_status": item.FulfillmentStatus,
				"tags":             item.Tags,
				"note":             item.Note,
			},
		},
	}

	resp, err := e.client.DoWithRetry(ctx, req)
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

	// Restore metafields (D2)
	if len(item.Metafields) > 0 {
		if err := e.restoreMetafields(ctx, "ORDER", data.Order.ID, item.Metafields); err != nil {
			e.logger.Warnf("Failed to restore metafields for order %s: %v", data.Order.ID, err)
		}
	}

	return data.Order.ID, nil
}

// restoreCollection restores a collection via GraphQL mutation
func (e *MutationExecutor) restoreCollection(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	existingID, err := e.findCollectionByHandle(ctx, item.Handle)
	if err != nil {
		return "", fmt.Errorf("check existing collection: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
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
		"title":          item.Title,
		"handle":         item.Handle,
		"descriptionHtml": item.Description,
		"rules":          item.CollectionRules,
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

	collectionID := data.CollectionCreate.Collection.ID

	// Add products to collection (D4)
	if len(item.CollectionProducts) > 0 {
		if err := e.addProductsToCollection(ctx, collectionID, item.CollectionProducts); err != nil {
			e.logger.Warnf("Failed to add products to collection %s: %v", collectionID, err)
		}
	}

	// Restore metafields (D2)
	if len(item.Metafields) > 0 {
		if err := e.restoreMetafields(ctx, "COLLECTION", collectionID, item.Metafields); err != nil {
			e.logger.Warnf("Failed to restore metafields for collection %s: %v", collectionID, err)
		}
	}

	return collectionID, nil
}

// restoreMetaobject restores a metaobject via GraphQL mutation
func (e *MutationExecutor) restoreMetaobject(ctx context.Context, item Item, conflictMode ConflictMode) (string, error) {
	if item.MetaobjectDefinition == nil {
		return "", fmt.Errorf("metaobject definition is required")
	}

	existingID, err := e.findMetaobjectByKey(ctx, *item.MetaobjectDefinition, item.Key)
	if err != nil {
		return "", fmt.Errorf("check existing metaobject: %w", err)
	}

	if existingID != "" {
		switch conflictMode {
		case ConflictSkip:
			return existingID, nil
		case ConflictOverwrite:
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
		"type": *item.MetaobjectDefinition,
		"key":  item.Key,
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

// --- Phase 2: New mutation helpers (D1-D5, D7) ---

// restoreProductVariants creates variants for a product (D1)
func (e *MutationExecutor) restoreProductVariants(ctx context.Context, productID string, variants []ProductVariant) error {
	for _, variant := range variants {
		query := `
			mutation productVariantCreate($input: ProductVariantInput!) {
				productVariantCreate(input: $input) {
					productVariant {
						id
					}
					userErrors {
						field
						message
					}
				}
			}
		`

		input := map[string]interface{}{
			"productId": productID,
			"title":     variant.Title,
			"price":     variant.Price,
			"sku":       variant.SKU,
		}

		if variant.CompareAtPrice != "" {
			input["compareAtPrice"] = variant.CompareAtPrice
		}

		if variant.InventoryQuantity != 0 {
			input["inventoryQuantities"] = []map[string]interface{}{
				{
					"availableQuantity": variant.InventoryQuantity,
					"locationId":       nil, // Will use default location
				},
			}
		}

		variables := map[string]interface{}{
			"input": input,
		}

		resp, err := e.client.DoGraphQL(ctx, query, variables)
		if err != nil {
			e.logger.Warnf("Failed to create variant %s: %v", variant.Title, err)
			continue
		}

		var data struct {
			ProductVariantCreate struct {
				UserErrors []struct {
					Field   []string `json:"field"`
					Message string   `json:"message"`
				} `json:"userErrors"`
			} `json:"productVariantCreate"`
		}

		if err := json.Unmarshal(resp.Data, &data); err != nil {
			continue
		}

		if len(data.ProductVariantCreate.UserErrors) > 0 {
			e.logger.Warnf("Variant creation errors for %s: %v", variant.Title, data.ProductVariantCreate.UserErrors)
		}
	}
	return nil
}

// restoreMetafields creates metafields on an entity (D2)
func (e *MutationExecutor) restoreMetafields(ctx context.Context, ownerType, ownerID string, metafields []Metafield) error {
	if len(metafields) == 0 {
		return nil
	}

	// Build metafields input for metafieldsSet mutation
	var metafieldInputs []map[string]interface{}
	for _, mf := range metafields {
		if mf.Namespace == "" || mf.Key == "" {
			continue
		}
		input := map[string]interface{}{
			"ownerId":   ownerID,
			"namespace": mf.Namespace,
			"key":       mf.Key,
			"type":      mf.Type,
		}

		// Convert value to string representation
		switch v := mf.Value.(type) {
		case string:
			input["value"] = v
		default:
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				input["value"] = fmt.Sprintf("%v", v)
			} else {
				input["value"] = string(jsonBytes)
			}
		}

		metafieldInputs = append(metafieldInputs, input)
	}

	if len(metafieldInputs) == 0 {
		return nil
	}

	// Use metafieldsSet mutation (bulk set)
	query := `
		mutation metafieldsSet($metafields: [MetafieldsSetInput!]!) {
			metafieldsSet(metafields: $metafields) {
				userErrors {
					field
					message
				}
			}
		}
	`

	variables := map[string]interface{}{
		"metafields": metafieldInputs,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return fmt.Errorf("set metafields: %w", err)
	}

	var data struct {
		MetafieldsSet struct {
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"metafieldsSet"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("unmarshal metafields response: %w", err)
	}

	if len(data.MetafieldsSet.UserErrors) > 0 {
		return fmt.Errorf("metafield errors: %v", data.MetafieldsSet.UserErrors)
	}

	return nil
}

// restoreCustomerAddresses creates addresses for a customer (D3)
func (e *MutationExecutor) restoreCustomerAddresses(ctx context.Context, customerID string, addresses []CustomerAddress) error {
	for _, addr := range addresses {
		query := `
			mutation customerAddressCreate($address: CustomerAddressInput!, $customerId: ID!) {
				customerAddressCreate(address: $address, customerId: $customerId) {
					customerAddress {
						id
					}
					userErrors {
						field
						message
					}
				}
			}
		`

		input := map[string]interface{}{
			"address1": addr.Address1,
			"address2": addr.Address2,
			"city":     addr.City,
			"province": addr.Province,
			"country":  addr.Country,
			"zip":      addr.Zip,
			"phone":    addr.Phone,
		}

		variables := map[string]interface{}{
			"address":   input,
			"customerId": customerID,
		}

		resp, err := e.client.DoGraphQL(ctx, query, variables)
		if err != nil {
			e.logger.Warnf("Failed to create address for customer %s: %v", customerID, err)
			continue
		}

		var data struct {
			CustomerAddressCreate struct {
				UserErrors []struct {
					Field   []string `json:"field"`
					Message string   `json:"message"`
				} `json:"userErrors"`
			} `json:"customerAddressCreate"`
		}

		if err := json.Unmarshal(resp.Data, &data); err != nil {
			continue
		}

		if len(data.CustomerAddressCreate.UserErrors) > 0 {
			e.logger.Warnf("Address creation errors: %v", data.CustomerAddressCreate.UserErrors)
		}
	}
	return nil
}

// addProductsToCollection adds products to a collection (D4)
func (e *MutationExecutor) addProductsToCollection(ctx context.Context, collectionID string, productIDs []string) error {
	if len(productIDs) == 0 {
		return nil
	}

	query := `
		mutation collectionAddProducts($id: ID!, $productIds: [ID!]!) {
			collectionAddProducts(id: $id, productIds: $productIds) {
				job {
					id
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	// Batch products in groups of MaxBatchSize
	for i := 0; i < len(productIDs); i += MaxBatchSize {
		end := i + MaxBatchSize
		if end > len(productIDs) {
			end = len(productIDs)
		}

		batch := productIDs[i:end]

		variables := map[string]interface{}{
			"id":         collectionID,
			"productIds": batch,
		}

		resp, err := e.client.DoGraphQL(ctx, query, variables)
		if err != nil {
			e.logger.Warnf("Failed to add products batch to collection %s: %v", collectionID, err)
			continue
		}

		var data struct {
			CollectionAddProducts struct {
				UserErrors []struct {
					Field   []string `json:"field"`
					Message string   `json:"message"`
				} `json:"userErrors"`
			} `json:"collectionAddProducts"`
		}

		if err := json.Unmarshal(resp.Data, &data); err != nil {
			continue
		}

		if len(data.CollectionAddProducts.UserErrors) > 0 {
			e.logger.Warnf("Collection add products errors: %v", data.CollectionAddProducts.UserErrors)
		}
	}
	return nil
}

// RestoreMetaobjectDefinition creates a metaobject definition (D5)
func (e *MutationExecutor) RestoreMetaobjectDefinition(ctx context.Context, def MetaobjectDefinition) (string, error) {
	query := `
		mutation metaobjectDefinitionCreate($input: MetaobjectDefinitionCreateInput!) {
			metaobjectDefinitionCreate(input: $input) {
				metaobjectDefinition {
					id
					type
				}
				userErrors {
					field
					message
				}
			}
		}
	`

	fieldDefs := make([]map[string]interface{}, len(def.FieldDefinitions))
	for i, fd := range def.FieldDefinitions {
		fieldDefs[i] = map[string]interface{}{
			"key":  fd.Key,
			"name": fd.Name,
			"type": map[string]interface{}{
				"name": fd.Type.Name,
			},
		}
	}

	input := map[string]interface{}{
		"type":             def.Type,
		"name":            def.Name,
		"fieldDefinitions": fieldDefs,
	}

	variables := map[string]interface{}{
		"input": input,
	}

	resp, err := e.client.DoGraphQL(ctx, query, variables)
	if err != nil {
		return "", fmt.Errorf("create metaobject definition: %w", err)
	}

	var data struct {
		MetaobjectDefinitionCreate struct {
			MetaobjectDefinition struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"metaobjectDefinition"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"metaobjectDefinitionCreate"`
	}

	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(data.MetaobjectDefinitionCreate.UserErrors) > 0 {
		return "", fmt.Errorf("metaobject definition errors: %v", data.MetaobjectDefinitionCreate.UserErrors)
	}

	return data.MetaobjectDefinitionCreate.MetaobjectDefinition.ID, nil
}

// --- Helper methods for finding existing entities ---

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
	req := Request{
		Method: "GET",
		Path:   fmt.Sprintf("/orders.json?name=%s", orderNumber),
	}

	resp, err := e.client.DoWithRetry(ctx, req)
	if err != nil {
		return "", err
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

// --- Delete and update helpers ---

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
			DeletedID  string `json:"deletedId"`
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
			DeletedID  string `json:"deletedId"`
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
			DeletedID  string `json:"deletedId"`
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
		"id":        id,
		"email":     *item.Email,
		"firstName": item.FirstName,
		"lastName":  item.LastName,
		"phone":     item.Phone,
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

// --- Utility functions ---

func generateNewHandle(handle string) string {
	return fmt.Sprintf("%s-%d", handle, time.Now().Unix())
}

func generateNewKey(key string) string {
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