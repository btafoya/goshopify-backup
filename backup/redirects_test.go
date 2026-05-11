package backup

import (
	"testing"
)

func TestNewRedirectsModule(t *testing.T) {
	mod := NewRedirectsModule()
	if mod == nil {
		t.Fatal("NewRedirectsModule() returned nil")
	}
	if mod.Name() != "redirects" {
		t.Errorf("Name() = %v, want redirects", mod.Name())
	}
}

func TestParseURLRedirect(t *testing.T) {
	raw := map[string]interface{}{
		"id":     "gid://shopify/UrlRedirect/12345",
		"path":   "/old-page",
		"target": "/new-page",
	}

	redirect, ok := parseURLRedirect(raw)
	if !ok {
		t.Fatal("parseURLRedirect() returned false")
	}

	if redirect.ID != "gid://shopify/UrlRedirect/12345" {
		t.Errorf("ID = %v, want gid://shopify/UrlRedirect/12345", redirect.ID)
	}
	if redirect.Path != "/old-page" {
		t.Errorf("Path = %v, want /old-page", redirect.Path)
	}
	if redirect.Target != "/new-page" {
		t.Errorf("Target = %v, want /new-page", redirect.Target)
	}
}

func TestParseURLRedirect_MissingID(t *testing.T) {
	raw := map[string]interface{}{
		"path":   "/old-page",
		"target": "/new-page",
	}

	_, ok := parseURLRedirect(raw)
	if ok {
		t.Error("parseURLRedirect() should return false for missing ID")
	}
}

func TestParseURLRedirect_MissingPath(t *testing.T) {
	raw := map[string]interface{}{
		"id":     "gid://shopify/UrlRedirect/12345",
		"target": "/new-page",
	}

	_, ok := parseURLRedirect(raw)
	if ok {
		t.Error("parseURLRedirect() should return false for missing path")
	}
}

func TestParseURLRedirect_MissingTarget(t *testing.T) {
	raw := map[string]interface{}{
		"id":   "gid://shopify/UrlRedirect/12345",
		"path": "/old-page",
	}

	_, ok := parseURLRedirect(raw)
	if ok {
		t.Error("parseURLRedirect() should return false for missing target")
	}
}

func TestConvertRESTIDToGID(t *testing.T) {
	tests := []struct {
		name string
		id   int64
		want string
	}{
		{
			name: "positive ID",
			id:   12345,
			want: "gid://shopify/UrlRedirect/12345",
		},
		{
			name: "zero ID",
			id:   0,
			want: "gid://shopify/UrlRedirect/0",
		},
		{
			name: "large ID",
			id:   9999999999,
			want: "gid://shopify/UrlRedirect/9999999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertRESTIDToGID(tt.id)
			if result != tt.want {
				t.Errorf("convertRESTIDToGID(%v) = %v, want %v", tt.id, result, tt.want)
			}
		})
	}
}

func TestParseRedirectID(t *testing.T) {
	tests := []struct {
		name    string
		gid     string
		want    int64
		wantErr bool
	}{
		{
			name:    "valid GID",
			gid:     "gid://shopify/UrlRedirect/12345",
			want:    12345,
			wantErr: false,
		},
		{
			name:    "large ID",
			gid:     "gid://shopify/UrlRedirect/9999999999",
			want:    9999999999,
			wantErr: false,
		},
		{
			name:    "invalid format",
			gid:     "invalid",
			want:    0,
			wantErr: true,
		},
		{
			name:    "missing parts",
			gid:     "gid://shopify/UrlRedirect",
			want:    0,
			wantErr: true,
		},
		{
			name:    "non-numeric ID",
			gid:     "gid://shopify/UrlRedirect/abc",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRedirectID(tt.gid)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRedirectID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseRedirectID() = %v, want %v", got, tt.want)
			}
		})
	}
}
