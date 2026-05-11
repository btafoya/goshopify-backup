package main

import "time"

// EntityType represents the types of entities to restore
type EntityType string

const (
	EntityProducts    EntityType = "products"
	EntityCustomers   EntityType = "customers"
	EntityOrders      EntityType = "orders"
	EntityCollections EntityType = "collections"
	EntityMetaobjects EntityType = "metaobjects"
	EntityPages       EntityType = "pages"
	EntityThemes      EntityType = "themes"
)

func (e EntityType) String() string {
	return string(e)
}

// State represents the TUI state machine
type State int

const (
	StateConfig       State = iota // Configure target store
	StateBackupSelect              // Select backup directory
	StateEntitySelect              // Select entity type
	StateItemSelect                // Select items to restore
	StatePreview                   // Preview changes
	StateConfirm                   // Confirm restore
	StateRunning                   // Restore in progress
	StateComplete                  // Restore completed
	StateError                     // Error state
	StateAbort                     // Abort confirmation
)

func (s State) String() string {
	switch s {
	case StateConfig:
		return "Config"
	case StateBackupSelect:
		return "Backup Select"
	case StateEntitySelect:
		return "Entity Select"
	case StateItemSelect:
		return "Item Select"
	case StatePreview:
		return "Preview"
	case StateConfirm:
		return "Confirm"
	case StateRunning:
		return "Running"
	case StateComplete:
		return "Complete"
	case StateError:
		return "Error"
	case StateAbort:
		return "Abort"
	default:
		return "Unknown"
	}
}

// Config holds restore configuration
type Config struct {
	// Source
	BackupDir  string // Backup directory (default: /backups/shopify)
	BackupDate string // Specific backup date (empty = latest)

	// Target Store
	Store        string // Target store URL
	AccessToken  string // Shopify access token (direct)
	ClientID     string // Shopify app client ID (for client credentials flow)
	ClientSecret string // Shopify app client secret (for client credentials flow)
	APIVersion   string // Shopify API version

	// Behavior
	DryRun        bool        // Validate only, don't restore
	Force         bool        // Override conflicts without prompt
	RestoreImages ImagePolicy // How to handle images
	Resume        bool        // Resume from interrupted restore

	// Output
	LogDir      string // Log directory
	RollbackDir string // Rollback script directory
	Verbose     bool   // Verbose logging
}

// ImagePolicy defines how to handle product images
type ImagePolicy string

// BackupInfo represents a backup directory
type BackupInfo struct {
	Date     time.Time
	Path     string
	Status   *BackupStatus
	FileSize int64
}

// BackupStatus represents the status of a backup
type BackupStatus struct {
	StartedAt   time.Time               `json:"startedAt"`
	CompletedAt time.Time               `json:"completedAt,omitempty"`
	Duration    string                  `json:"duration,omitempty"`
	Modules     map[string]ModuleStatus `json:"modules"`
	TotalSize   int64                   `json:"totalSize,omitempty"`
}

// ModuleStatus represents the status of a single backup module
type ModuleStatus struct {
	Status      string    `json:"status"` // "pending", "running", "completed", "failed"
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Count       int       `json:"count"`
	Error       string    `json:"error,omitempty"`
	FileSize    int64     `json:"fileSize,omitempty"`
}

// Item represents a single entity item
type Item struct {
	ID         string
	Title      string
	Handle     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Status     string
	Tags       []string
	CustomData map[string]interface{}
	Type       EntityType

	// Product-specific fields
	Description  string
	ProductType  string
	Vendor       string
	Price        *string
	VariantCount *int
	Variants     []ProductVariant
	Images       []Image
	Metafields   []Metafield
	SEO          *SEOInfo

	// Customer-specific fields
	Email      *string
	FirstName  string
	LastName   string
	Phone      string
	OrderCount *int
	Addresses  []CustomerAddress

	// Order-specific fields
	OrderNumber       *string
	FinancialStatus   *string
	FulfillmentStatus *string
	LineItems         []interface{}
	Customer          interface{}
	BillingAddress    interface{}
	ShippingAddress   interface{}
	Note              string

	// Collection-specific fields
	ProductsCount      *int
	CollectionProducts []string // Product GIDs from backup
	CollectionRules    []interface{}

	// Metaobject-specific fields
	Key                  string
	MetaobjectDefinition *string
	MetaobjectFields     map[string]interface{}

	// Page-specific fields
	BodyHTML       string
	TemplateSuffix string
	Author         string
	PublishedAt    *time.Time
}

// SEOInfo represents SEO information
type SEOInfo struct {
	Title       string
	Description string
}

// FilterCriteria represents active filters
type FilterCriteria struct {
	SearchText string
	DateFrom   *time.Time
	DateTo     *time.Time
	Statuses   []string
	Tags       []string
}

// PreviewChange represents a change that will be made
type PreviewChange struct {
	EntityType EntityType
	ItemID     string
	ItemTitle  string
	Action     string
	Conflict   *ConflictInfo
}

// ConflictInfo represents a conflict with existing data
type ConflictInfo struct {
	ExistingID   string
	ExistingData map[string]interface{}
	BackupData   map[string]interface{}
	Diff         string
}

// RestoreProgress tracks restore progress
type RestoreProgress struct {
	TotalItems     int
	CompletedItems int
	FailedItems    int
	SkippedItems   int
	CurrentEntity  EntityType
	CurrentItem    string
	Logs           []LogEntry
	StartTime      time.Time
	CompletedAt    time.Time
	Status         string
	Duration       time.Duration
	CurrentItems   map[string]bool
}

// RestoreResult represents the result of a single item restore
type RestoreResult struct {
	EntityType EntityType
	ItemID     string
	Success    bool
	Message    string
	Error      string
	RestoredID string // New ID in target store
	Duration   time.Duration
	CreatedAt  time.Time
}

// LogEntry represents a log entry
type LogEntry struct {
	Level     string
	Message   string
	Timestamp time.Time
}

// RollbackScript contains commands to revert a restore
type RollbackScript struct {
	GeneratedAt  time.Time
	TargetStore  string
	BackupDate   string
	CreatedAt    time.Time
	Instructions []string
	Commands     []string
	Actions      []RollbackAction
}

// RollbackAction represents a single rollback action
type RollbackAction struct {
	EntityType  EntityType
	Action      string // "delete", "update", "untag"
	ID          string
	Data        map[string]interface{}
	Description string
}

// RestoreState represents the state for resume functionality
type RestoreState struct {
	StartedAt      time.Time               `json:"startedAt"`
	BackupDate     string                  `json:"backupDate"`
	TargetStore    string                  `json:"targetStore"`
	SelectedItems  map[EntityType][]string `json:"selectedItems"`
	CompletedItems []CompletedItem         `json:"completedItems"`
	FailedItems    []FailedItem            `json:"failedItems"`
	Progress       ProgressState           `json:"progress"`
}

// CompletedItem represents a successfully restored item
type CompletedItem struct {
	EntityType  EntityType `json:"entityType"`
	SourceID    string     `json:"sourceId"`
	TargetID    string     `json:"targetId"`
	CompletedAt time.Time  `json:"completedAt"`
}

// FailedItem represents a failed restore item
type FailedItem struct {
	EntityType EntityType `json:"entityType"`
	SourceID   string     `json:"sourceId"`
	Error      string     `json:"error"`
	FailedAt   time.Time  `json:"failedAt"`
}

// ProgressState represents progress for resume
type ProgressState struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// Credential represents saved store credentials
type Credential struct {
	Store        string    `json:"store"`
	AccessToken  string    `json:"access_token,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	ClientSecret string    `json:"client_secret,omitempty"`
	APIVersion   string    `json:"api_version"`
	LastUsed     time.Time `json:"last_used"`
	Nickname     string    `json:"nickname,omitempty"`
}

// Entity interface for all entity types
type Entity interface {
	GetID() string
	GetTitle() string
	GetHandle() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetStatus() string
	ToItem() Item
}

// Product represents a product from backup
type Product struct {
	ID          string
	Title       string
	Handle      string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Vendor      string
	ProductType string
	Tags        []string
	Status      string
	Variants    []ProductVariant
	Images      []ProductImage
	Metafields  []Metafield
}

// ProductVariant represents a product variant
type ProductVariant struct {
	ID                string
	Title             string
	Price             string
	SKU               string
	InventoryQuantity int
	CompareAtPrice    string
	Metafields        []Metafield
}

// ProductImage represents a product image
type ProductImage struct {
	ID       string
	Src      string
	AltText  string
	Width    int
	Height   int
	Position int
}

// Customer represents a customer from backup
type Customer struct {
	ID         string
	Email      string
	FirstName  string
	LastName   string
	Phone      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	State      string
	Addresses  []CustomerAddress
	Metafields []Metafield
}

// CustomerAddress represents a customer address
type CustomerAddress struct {
	ID       string
	Address1 string
	Address2 string
	City     string
	Province string
	Country  string
	Zip      string
	Phone    string
}

// Order represents an order from backup
type Order struct {
	ID                 string
	Name               string
	OrderNumber        int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ProcessedAt        time.Time
	FinancialStatus    string
	FulfillmentStatus  string
	TotalPrice         string
	SubtotalPrice      string
	TotalTax           string
	TotalDiscounts     string
	TotalShippingPrice string
	LineItems          []LineItem
	Transactions       []OrderTransaction
	Fulfillments       []OrderFulfillment
	Refunds            []OrderRefund
	Customer           *OrderCustomer
	BillingAddress     *CustomerAddress
	ShippingAddress    *CustomerAddress
	Metafields         []Metafield
}

// LineItem represents an order line item
type LineItem struct {
	ID       string
	Title    string
	Quantity int
	Price    string
	SKU      string
	Variant  *LineItemVariant
}

// LineItemVariant represents a line item variant
type LineItemVariant struct {
	ID    string
	Title string
}

// OrderTransaction represents an order transaction
type OrderTransaction struct {
	ID     string
	Kind   string
	Status string
	Amount string
}

// OrderFulfillment represents an order fulfillment
type OrderFulfillment struct {
	ID              string
	Status          string
	TrackingCompany string
	TrackingNumber  string
	TrackingInfoURL string
}

// OrderRefund represents an order refund
type OrderRefund struct {
	ID        string
	CreatedAt time.Time
	Amount    string
}

// OrderCustomer represents order customer info
type OrderCustomer struct {
	ID    int
	Email string
}

// Collection represents a collection from backup
type Collection struct {
	ID          string
	Title       string
	Handle      string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	SortOrder   string
	Products    []CollectionProduct
	Rules       []CollectionRule
	Metafields  []Metafield
}

// CollectionProduct represents a product in a collection
type CollectionProduct struct {
	ID string
}

// CollectionRule represents a smart collection rule
type CollectionRule struct {
	Column   string
	Relation string
	Value    string
}

// Metafield represents a metafield
type Metafield struct {
	ID        string
	Namespace string
	Key       string
	Value     interface{}
	Type      string
}

// MetaobjectDefinition represents a metaobject definition
type MetaobjectDefinition struct {
	ID               string
	Type             string
	Name             string
	FieldDefinitions []FieldDefinition
}

// FieldDefinition represents a metaobject field definition
type FieldDefinition struct {
	Key  string
	Name string
	Type FieldDefinitionType
}

// FieldDefinitionType represents a field definition type
type FieldDefinitionType struct {
	Name string
}

// MetaobjectEntry represents a metaobject entry
type MetaobjectEntry struct {
	ID     string
	Handle string
	Type   string
	Fields []MetaobjectField
}

// MetaobjectField represents a metaobject field value
type MetaobjectField struct {
	Key   string
	Value interface{}
}

// MetaobjectData contains definitions and entries
type MetaobjectData struct {
	Definitions []MetaobjectDefinition
	Entries     map[string][]MetaobjectEntry // Type -> Entries
}

// Page represents a page from backup
type Page struct {
	ID             string
	Title          string
	Handle         string
	BodyHTML       string
	Author         string
	TemplateSuffix string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	PublishedAt    *time.Time
}
