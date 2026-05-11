package auth

// BackupScopes lists the Shopify Admin API scopes required by the backup tool.
// Aggregated across all backup modules: products, customers, orders,
// collections, content (pages/blogs/articles), metaobjects, redirects, metafields.
var BackupScopes = []string{
	"read_products",
	"read_customers",
	"read_orders",
	"read_content",
	"read_metaobjects",
	"read_online_store_pages",
}

// RestoreScopes lists write scopes required by the restore tool.
var RestoreScopes = []string{
	"write_products",
	"write_customers",
	"write_orders",
	"write_content",
	"write_metaobjects",
	"write_online_store_pages",
}
