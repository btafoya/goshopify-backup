package main

import "time"

// API Configuration
const (
	APIVersion            = "2025-07"
	GraphQLEndpointFormat = "https://%s/admin/api/%s/graphql.json"
	RESTEndpointFormat    = "https://%s/admin/api/%s/"
)

// Rate Limiting
const (
	RequestsPerSecond  = 40
	MinRequestInterval = 25 * time.Millisecond
)

// Retry Configuration
const (
	RestoreRetryCount = 3
	RestoreRetryDelay = 2 * time.Second
)

// TUI Configuration
const (
	RefreshInterval = 500 * time.Millisecond
	PageSize        = 50
	MaxDisplayItems = 1000
)

// File Configuration
const (
	RestoreTag   = "restored_from_backup"
	StateFile    = ".restore_state.json"
	RollbackFile = "rollback_%s.sh"
	LogFile      = "restore_%s.log"
)

// Date Formats
const (
	DateFormat     = "2006-01-02"
	DateTimeFormat = "2006-01-02T15:04:05Z"
)

// Limits
const (
	MaxConcurrentUploads = 5
	MaxBatchSize         = 250
)

// Credentials
const (
	CredentialsDir  = "~/.config/goshopify"
	CredentialsFile = "credentials.json"
)

// Store URL Validation
const (
	StoreDomainPattern = `^https://[a-zA-Z0-9][a-zA-Z0-9\-]*\.myshopify\.com$`
)

// Entity Types
var EntityTypes = []EntityType{
	EntityProducts,
	EntityCustomers,
	EntityOrders,
	EntityCollections,
	EntityMetaobjects,
}

// EntityDisplayNames maps entity types to display names
var EntityDisplayNames = map[EntityType]string{
	EntityProducts:    "Products",
	EntityCustomers:   "Customers",
	EntityOrders:      "Orders",
	EntityCollections: "Collections",
	EntityMetaobjects: "Metaobjects",
}

// Status Values
const (
	StatusActive   = "active"
	StatusDraft    = "draft"
	StatusArchived = "archived"
	StatusAny      = "any"
)

// Available Status Filters
var StatusFilters = []string{
	StatusAny,
	StatusActive,
	StatusDraft,
	StatusArchived,
}

// Change Actions
const (
	ActionCreate   = "create"
	ActionUpdate   = "update"
	ActionSkip     = "skip"
	ActionConflict = "conflict"
)

// Conflict Resolutions
const (
	ResolutionSkip     = "skip"
	ResolutionOverwrite = "overwrite"
	ResolutionRename   = "rename"
)

// Error Codes
const (
	ErrCodeNotFound   = "NOT_FOUND"
	ErrCodeConflict   = "CONFLICT"
	ErrCodeValidation = "VALIDATION"
	ErrCodeRateLimit  = "RATE_LIMIT"
	ErrCodeNetwork    = "NETWORK"
	ErrCodeAuth       = "AUTH"
	ErrCodePermission = "PERMISSION"
	ErrCodeUnknown    = "UNKNOWN"
)

// Exit Codes
const (
	ExitSuccess       = 0
	ExitFailed        = 1
	ExitConfigError   = 2
	ExitUserAborted   = 3
	ExitValidationError = 4
	ExitNetworkError  = 5
)

// Image Policies
const (
	ImageRestore     = "restore"
	ImageSkip        = "skip"
	ImageInteractive = "interactive"
)