package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Loader loads and validates backup data
type Loader struct {
	backupDir string
}

// NewLoader creates a new backup loader
func NewLoader(backupDir string) *Loader {
	return &Loader{backupDir: backupDir}
}

// ListBackups lists all available backup directories
func (l *Loader) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(l.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if directory name matches date format YYYY-MM-DD
		if _, err := time.Parse(DateFormat, entry.Name()); err != nil {
			continue
		}

		backupPath := filepath.Join(l.backupDir, entry.Name())

		// Get status.json
		status, err := l.loadStatus(backupPath)
		if err != nil {
			continue // Skip if status.json is not valid
		}

		// Calculate total size
		totalSize, err := l.calculateTotalSize(backupPath)
		if err != nil {
			totalSize = 0
		}

		backupDate, _ := time.Parse(DateFormat, entry.Name())

		backups = append(backups, BackupInfo{
			Date:     backupDate,
			Path:     backupPath,
			Status:   status,
			FileSize: totalSize,
		})
	}

	// Sort by date descending (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Date.After(backups[j].Date)
	})

	return backups, nil
}

// LoadBackup loads a specific backup
func (l *Loader) LoadBackup(date string) (*Backup, error) {
	backupPath := filepath.Join(l.backupDir, date)

	// Verify directory exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("backup directory not found: %s", backupPath)
	}

	// Load status
	status, err := l.loadStatus(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load status: %w", err)
	}

	backup := &Backup{
		Date:        status.StartedAt,
		Path:        backupPath,
		Status:      status,
		Products:    make([]*Product, 0),
		Customers:   make([]*Customer, 0),
		Orders:      make([]*Order, 0),
		Collections: make([]*Collection, 0),
		Metaobjects: &MetaobjectData{
			Definitions: make([]MetaobjectDefinition, 0),
			Entries:     make(map[string][]MetaobjectEntry),
		},
	}

	return backup, nil
}

// LoadEntity loads entity data from backup
func (l *Loader) LoadEntity(date string, entityType EntityType) ([]Item, error) {
	backupPath := filepath.Join(l.backupDir, date)

	var filePath string
	switch entityType {
	case EntityProducts:
		filePath = filepath.Join(backupPath, "products.json")
	case EntityCustomers:
		filePath = filepath.Join(backupPath, "customers.json")
	case EntityOrders:
		filePath = filepath.Join(backupPath, "orders.json")
	case EntityCollections:
		filePath = filepath.Join(backupPath, "collections.json")
	case EntityMetaobjects:
		return l.loadMetaobjectItems(backupPath)
	case EntityPages:
		filePath = filepath.Join(backupPath, "pages.json")
	case EntityThemes:
		return l.loadThemeItems(backupPath)
	default:
		return nil, fmt.Errorf("unsupported entity type: %s", entityType)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Item{}, nil // Empty data is ok
		}
		return nil, fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
	}

	return l.parseItems(data, entityType)
}

// loadMetaobjectItems loads metaobject entries from the metaobjects subdirectory (D6)
func (l *Loader) loadMetaobjectItems(backupPath string) ([]Item, error) {
	metaobjectsDir := filepath.Join(backupPath, "metaobjects")
	if _, err := os.Stat(metaobjectsDir); os.IsNotExist(err) {
		return []Item{}, nil
	}

	// Load definitions first
	defsPath := filepath.Join(metaobjectsDir, "metaobject-definitions.json")
	defsData, err := os.ReadFile(defsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Item{}, nil
		}
		return nil, fmt.Errorf("failed to read metaobject definitions: %w", err)
	}

	var definitions []MetaobjectDefinition
	if err := json.Unmarshal(defsData, &definitions); err != nil {
		return nil, fmt.Errorf("failed to parse metaobject definitions: %w", err)
	}

	var items []Item

	// Create items from definitions (for definition restore)
	for _, def := range definitions {
		items = append(items, Item{
			ID:     def.ID,
			Title:  def.Name,
			Handle: def.Type,
			Status: "active",
			CustomData: map[string]interface{}{
				"definitionType":   def.Type,
				"fieldDefinitions": def.FieldDefinitions,
				"isDefinition":     true,
			},
		})
	}

	// Load entries per type
	entries, err := os.ReadDir(metaobjectsDir)
	if err != nil {
		return items, nil // Return definitions even if entries fail
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if entry.Name() == "metaobject-definitions.json" {
			continue
		}

		// Entry files are named {type}.json
		typeName := strings.TrimSuffix(entry.Name(), ".json")
		entryPath := filepath.Join(metaobjectsDir, entry.Name())

		data, err := os.ReadFile(entryPath)
		if err != nil {
			continue
		}

		var metaobjectEntries []MetaobjectEntry
		if err := json.Unmarshal(data, &metaobjectEntries); err != nil {
			continue
		}

		for _, me := range metaobjectEntries {
			// Convert fields to map for Item
			fields := make(map[string]interface{})
			for _, f := range me.Fields {
				fields[f.Key] = f.Value
			}

			defType := typeName
			items = append(items, Item{
				ID:     me.ID,
				Title:  me.Handle,
				Handle: me.Handle,
				Status: "active",
				CustomData: map[string]interface{}{
					"metaobjectDefinition": defType,
					"metaobjectKey":        me.Handle,
					"metaobjectFields":     fields,
					"isEntry":              true,
				},
			})
		}
	}

	return items, nil
}

// ThemeBackupEntry is the JSON shape written by the backup tool's themes.json.
type ThemeBackupEntry struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	UpdatedAt string `json:"updated_at"`
}

// ThemeMetaEntry mirrors backup/themes.go ThemeMeta written to .meta.json.
type ThemeMetaEntry struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	UpdatedAt     string `json:"updated_at"`
	SchemaVersion string `json:"schema_version"`
	OS2           bool   `json:"online_store_2"`
	CLIVersion    string `json:"shopify_cli_version"`
}

// loadThemeItems reads the themes/ directory from a backup. Each subdirectory
// under themes/ is a theme; themes.json is the list snapshot from the backup
// run. Each theme's .meta.json is merged when present for richer Item metadata.
func (l *Loader) loadThemeItems(backupPath string) ([]Item, error) {
	themesDir := filepath.Join(backupPath, "themes")
	if _, err := os.Stat(themesDir); os.IsNotExist(err) {
		return []Item{}, nil
	}

	// Load list snapshot (best-effort)
	listByID := make(map[int64]ThemeBackupEntry)
	if data, err := os.ReadFile(filepath.Join(themesDir, "themes.json")); err == nil {
		var entries []ThemeBackupEntry
		if err := json.Unmarshal(data, &entries); err == nil {
			for _, e := range entries {
				listByID[e.ID] = e
			}
		}
	}

	dirs, err := os.ReadDir(themesDir)
	if err != nil {
		return nil, fmt.Errorf("read themes dir: %w", err)
	}

	var items []Item
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		// Theme dir names are stringified theme IDs
		themeIDStr := d.Name()
		themeDir := filepath.Join(themesDir, themeIDStr)

		var themeID int64
		fmt.Sscanf(themeIDStr, "%d", &themeID)

		name := themeIDStr
		role := ""
		updatedAt := ""
		schemaVersion := ""
		os2 := false
		cliVersion := ""

		if entry, ok := listByID[themeID]; ok {
			name = entry.Name
			role = entry.Role
			updatedAt = entry.UpdatedAt
		}

		if metaData, err := os.ReadFile(filepath.Join(themeDir, ".meta.json")); err == nil {
			var meta ThemeMetaEntry
			if err := json.Unmarshal(metaData, &meta); err == nil {
				if meta.Name != "" {
					name = meta.Name
				}
				if meta.Role != "" {
					role = meta.Role
				}
				if meta.UpdatedAt != "" {
					updatedAt = meta.UpdatedAt
				}
				schemaVersion = meta.SchemaVersion
				os2 = meta.OS2
				cliVersion = meta.CLIVersion
			}
		}

		items = append(items, Item{
			ID:     themeIDStr,
			Title:  fmt.Sprintf("%s (%s)", name, role),
			Handle: themeIDStr,
			Status: role,
			CustomData: map[string]interface{}{
				"isTheme":        true,
				"themePath":      themeDir,
				"themeName":      name,
				"themeRole":      role,
				"sourceID":       themeID,
				"updatedAt":      updatedAt,
				"schemaVersion":  schemaVersion,
				"onlineStore2":   os2,
				"cliVersion":     cliVersion,
			},
		})
	}

	return items, nil
}

// parseItems parses JSON data into Item slice
func (l *Loader) parseItems(data []byte, entityType EntityType) ([]Item, error) {
	var items []Item

	switch entityType {
	case EntityProducts:
		var products []Product
		if err := json.Unmarshal(data, &products); err != nil {
			return nil, fmt.Errorf("failed to parse products: %w", err)
		}
		for _, p := range products {
			items = append(items, p.ToItem())
		}

	case EntityCustomers:
		var customers []Customer
		if err := json.Unmarshal(data, &customers); err != nil {
			return nil, fmt.Errorf("failed to parse customers: %w", err)
		}
		for _, c := range customers {
			items = append(items, c.ToItem())
		}

	case EntityOrders:
		var orders []Order
		if err := json.Unmarshal(data, &orders); err != nil {
			return nil, fmt.Errorf("failed to parse orders: %w", err)
		}
		for _, o := range orders {
			items = append(items, o.ToItem())
		}

	case EntityCollections:
		var collections []Collection
		if err := json.Unmarshal(data, &collections); err != nil {
			return nil, fmt.Errorf("failed to parse collections: %w", err)
		}
		for _, c := range collections {
			items = append(items, c.ToItem())
		}

	case EntityPages:
		var pages []Page
		if err := json.Unmarshal(data, &pages); err != nil {
			return nil, fmt.Errorf("failed to parse pages: %w", err)
		}
		for _, p := range pages {
			items = append(items, p.ToItem())
		}
	}

	return items, nil
}

// loadStatus loads status.json from backup directory
func (l *Loader) loadStatus(backupPath string) (*BackupStatus, error) {
	statusPath := filepath.Join(backupPath, "status.json")

	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, err
	}

	var status BackupStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status.json: %w", err)
	}

	return &status, nil
}

// calculateTotalSize calculates total size of backup files
func (l *Loader) calculateTotalSize(backupPath string) (int64, error) {
	var total int64

	err := filepath.Walk(backupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	return total, err
}

// Backup represents a loaded backup
type Backup struct {
	Date        time.Time
	Path        string
	Status      *BackupStatus
	Products    []*Product
	Customers   []*Customer
	Orders      []*Order
	Collections []*Collection
	Metaobjects *MetaobjectData
}

// BackupInfo represents a backup directory listing
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

// EntityType for backup package
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

// Item for backup package
type Item struct {
	ID                string
	Title             string
	Handle            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Status            string
	Tags              []string
	CustomData        map[string]interface{}
	Price             *string
	VariantCount      *int
	Email             *string
	OrderCount        *int
	OrderNumber       *string
	FinancialStatus   *string
	FulfillmentStatus *string
	ProductsCount     *int
	Type              *string
	// Rich data fields for restore
	Variants           []ProductVariant
	Metafields         []Metafield
	Addresses          []CustomerAddress
	CollectionProducts []string // Product GIDs
	Images             []ProductImage
}

// Product entity types
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

func (p *Product) ToItem() Item {
	item := Item{
		ID:         p.ID,
		Title:      p.Title,
		Handle:     p.Handle,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
		Status:     p.Status,
		Tags:       p.Tags,
		CustomData: make(map[string]interface{}),
		Variants:   p.Variants,
		Metafields: p.Metafields,
		Images:     p.Images,
	}

	if len(p.Variants) > 0 {
		price := p.Variants[0].Price
		item.Price = &price
		count := len(p.Variants)
		item.VariantCount = &count
	}

	return item
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

// Customer entity
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

func (c *Customer) ToItem() Item {
	return Item{
		ID:         c.ID,
		Title:      fmt.Sprintf("%s %s", c.FirstName, c.LastName),
		Handle:     strings.ToLower(fmt.Sprintf("%s-%s", c.FirstName, c.LastName)),
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
		Status:     c.State,
		Email:      &c.Email,
		Tags:       c.Tags(),
		CustomData: make(map[string]interface{}),
		Addresses:  c.Addresses,
		Metafields: c.Metafields,
	}
}

func (c *Customer) Tags() []string {
	// Customers don't have tags, return empty slice
	return []string{}
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

// Order entity
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

func (o *Order) ToItem() Item {
	name := o.Name
	orderNumber := fmt.Sprintf("%d", o.OrderNumber)
	return Item{
		ID:                o.ID,
		Title:             name,
		Handle:            strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		CreatedAt:         o.CreatedAt,
		UpdatedAt:         o.UpdatedAt,
		Status:            o.FulfillmentStatus,
		OrderNumber:       &orderNumber,
		FinancialStatus:   &o.FinancialStatus,
		FulfillmentStatus: &o.FulfillmentStatus,
		Tags:              []string{},
		CustomData:        make(map[string]interface{}),
	}
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

// Collection entity
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

func (c *Collection) ToItem() Item {
	count := len(c.Products)
	productIDs := make([]string, len(c.Products))
	for i, p := range c.Products {
		productIDs[i] = p.ID
	}
	return Item{
		ID:                 c.ID,
		Title:              c.Title,
		Handle:             c.Handle,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
		Status:             "active",
		ProductsCount:      &count,
		Tags:               []string{},
		CustomData:         make(map[string]interface{}),
		CollectionProducts: productIDs,
		Metafields:         c.Metafields,
	}
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

// MetaobjectData contains definitions and entries
type MetaobjectData struct {
	Definitions []MetaobjectDefinition
	Entries     map[string][]MetaobjectEntry
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

// Page represents a page from backup (Shopify REST API format)
type Page struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	BodyHTML       string `json:"body_html"`
	Handle         string `json:"handle"`
	Author         string `json:"author"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	TemplateSuffix string `json:"template_suffix"`
	PublishedAt    string `json:"published_at"`
	ShopifyThemeID int    `json:"shopify_theme_id"`
}

// ToItem converts a Page to an Item
func (p *Page) ToItem() Item {
	return Item{
		ID:     fmt.Sprintf("%d", p.ID),
		Title:  p.Title,
		Handle: p.Handle,
		Status: "active",
		CustomData: map[string]interface{}{
			"body_html":       p.BodyHTML,
			"template_suffix": p.TemplateSuffix,
			"author":          p.Author,
			"published_at":    p.PublishedAt,
		},
	}
}

// DateFormat constant
const DateFormat = "2006-01-02"
