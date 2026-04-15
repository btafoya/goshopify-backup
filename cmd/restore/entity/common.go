package entity

import (
	"time"

	"github.com/btafoya/goshopify-restore/backup"
)

// Entity interface for all entity types
type Entity interface {
	GetID() string
	GetTitle() string
	GetHandle() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetStatus() string
	ToItem() backup.Item
}

// ProductAdapter adapts backup.Product to Entity interface
type ProductAdapter struct {
	product *backup.Product
}

func NewProductAdapter(product *backup.Product) Entity {
	return &ProductAdapter{product: product}
}

func (a *ProductAdapter) GetID() string       { return a.product.ID }
func (a *ProductAdapter) GetTitle() string    { return a.product.Title }
func (a *ProductAdapter) GetHandle() string   { return a.product.Handle }
func (a *ProductAdapter) GetCreatedAt() time.Time { return a.product.CreatedAt }
func (a *ProductAdapter) GetUpdatedAt() time.Time { return a.product.UpdatedAt }
func (a *ProductAdapter) GetStatus() string   { return a.product.Status }
func (a *ProductAdapter) ToItem() backup.Item { return a.product.ToItem() }

// CustomerAdapter adapts backup.Customer to Entity interface
type CustomerAdapter struct {
	customer *backup.Customer
}

func NewCustomerAdapter(customer *backup.Customer) Entity {
	return &CustomerAdapter{customer: customer}
}

func (a *CustomerAdapter) GetID() string       { return a.customer.ID }
func (a *CustomerAdapter) GetTitle() string {
	return a.customer.FirstName + " " + a.customer.LastName
}
func (a *CustomerAdapter) GetHandle() string {
	// Generate handle from name
	return (a.customer.FirstName + "-" + a.customer.LastName)
}
func (a *CustomerAdapter) GetCreatedAt() time.Time { return a.customer.CreatedAt }
func (a *CustomerAdapter) GetUpdatedAt() time.Time { return a.customer.UpdatedAt }
func (a *CustomerAdapter) GetStatus() string   { return a.customer.State }
func (a *CustomerAdapter) ToItem() backup.Item { return a.customer.ToItem() }

// OrderAdapter adapts backup.Order to Entity interface
type OrderAdapter struct {
	order *backup.Order
}

func NewOrderAdapter(order *backup.Order) Entity {
	return &OrderAdapter{order: order}
}

func (a *OrderAdapter) GetID() string       { return a.order.ID }
func (a *OrderAdapter) GetTitle() string    { return a.order.Name }
func (a *OrderAdapter) GetHandle() string   { return "" }
func (a *OrderAdapter) GetCreatedAt() time.Time { return a.order.CreatedAt }
func (a *OrderAdapter) GetUpdatedAt() time.Time { return a.order.UpdatedAt }
func (a *OrderAdapter) GetStatus() string   { return a.order.FulfillmentStatus }
func (a *OrderAdapter) ToItem() backup.Item { return a.order.ToItem() }

// CollectionAdapter adapts backup.Collection to Entity interface
type CollectionAdapter struct {
	collection *backup.Collection
}

func NewCollectionAdapter(collection *backup.Collection) Entity {
	return &CollectionAdapter{collection: collection}
}

func (a *CollectionAdapter) GetID() string       { return a.collection.ID }
func (a *CollectionAdapter) GetTitle() string    { return a.collection.Title }
func (a *CollectionAdapter) GetHandle() string   { return a.collection.Handle }
func (a *CollectionAdapter) GetCreatedAt() time.Time { return a.collection.CreatedAt }
func (a *CollectionAdapter) GetUpdatedAt() time.Time { return a.collection.UpdatedAt }
func (a *CollectionAdapter) GetStatus() string   { return "active" }
func (a *CollectionAdapter) ToItem() backup.Item { return a.collection.ToItem() }

// MetaobjectEntryAdapter adapts backup.MetaobjectEntry to Entity interface
type MetaobjectEntryAdapter struct {
	entry     *backup.MetaobjectEntry
	typeName  string
}

func NewMetaobjectEntryAdapter(entry *backup.MetaobjectEntry, typeName string) Entity {
	return &MetaobjectEntryAdapter{entry: entry, typeName: typeName}
}

func (a *MetaobjectEntryAdapter) GetID() string       { return a.entry.ID }
func (a *MetaobjectEntryAdapter) GetTitle() string    { return a.entry.Handle }
func (a *MetaobjectEntryAdapter) GetHandle() string   { return a.entry.Handle }
func (a *MetaobjectEntryAdapter) GetCreatedAt() time.Time { return time.Time{} }
func (a *MetaobjectEntryAdapter) GetUpdatedAt() time.Time { return time.Time{} }
func (a *MetaobjectEntryAdapter) GetStatus() string   { return "active" }
func (a *MetaobjectEntryAdapter) ToItem() backup.Item {
	fields := make(map[string]interface{})
	for _, f := range a.entry.Fields {
		fields[f.Key] = f.Value
	}
	return backup.Item{
		ID:    a.entry.ID,
		Title: a.entry.Handle,
		Handle: a.entry.Handle,
		Status: "active",
		CustomData: map[string]interface{}{
			"metaobjectDefinition": a.typeName,
			"metaobjectKey":       a.entry.Handle,
			"metaobjectFields":    fields,
			"isEntry":             true,
		},
	}
}

// MetaobjectDefinitionAdapter adapts backup.MetaobjectDefinition to Entity interface
type MetaobjectDefinitionAdapter struct {
	def *backup.MetaobjectDefinition
}

func NewMetaobjectDefinitionAdapter(def *backup.MetaobjectDefinition) Entity {
	return &MetaobjectDefinitionAdapter{def: def}
}

func (a *MetaobjectDefinitionAdapter) GetID() string       { return a.def.ID }
func (a *MetaobjectDefinitionAdapter) GetTitle() string    { return a.def.Name }
func (a *MetaobjectDefinitionAdapter) GetHandle() string   { return a.def.Type }
func (a *MetaobjectDefinitionAdapter) GetCreatedAt() time.Time { return time.Time{} }
func (a *MetaobjectDefinitionAdapter) GetUpdatedAt() time.Time { return time.Time{} }
func (a *MetaobjectDefinitionAdapter) GetStatus() string   { return "active" }
func (a *MetaobjectDefinitionAdapter) ToItem() backup.Item {
	return backup.Item{
		ID:     a.def.ID,
		Title:  a.def.Name,
		Handle: a.def.Type,
		Status: "active",
		CustomData: map[string]interface{}{
			"definitionType":   a.def.Type,
			"fieldDefinitions": a.def.FieldDefinitions,
			"isDefinition":     true,
		},
	}
}