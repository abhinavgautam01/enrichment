package enrichment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ecosyste-ms/ecosystems-go"
)

// ancestryBulkLookupResponse mirrors the shape of packages.ecosyste.ms
// bulk_lookup responses, trimmed to the fields BulkLookup reads.
const ancestryBulkLookupResponse = `[
	{
		"name": "@plotly/d3",
		"ecosystem": "npm",
		"purl": "pkg:npm/%40plotly/d3",
		"latest_release_number": "3.8.2",
		"normalized_licenses": ["BSD-3-Clause"],
		"repository_url": "https://github.com/plotly/d3",
		"repo_metadata": {
			"full_name": "plotly/d3",
			"html_url": "https://github.com/plotly/d3",
			"fork": true,
			"source_name": "d3/d3",
			"mirror_url": null,
			"host": {"name": "GitHub", "url": "https://github.com", "kind": "github"},
			"metadata": {"files": {"changelog": "CHANGES.md"}}
		}
	},
	{
		"name": "lodash",
		"ecosystem": "npm",
		"purl": "pkg:npm/lodash",
		"latest_release_number": "4.17.21",
		"normalized_licenses": ["MIT"],
		"repository_url": "https://github.com/lodash/lodash",
		"repo_metadata": {
			"full_name": "lodash/lodash",
			"fork": false,
			"source_name": null,
			"host": {"name": "GitHub", "url": "https://github.com", "kind": "github"}
		}
	},
	{
		"name": "fork-without-source",
		"ecosystem": "npm",
		"purl": "pkg:npm/fork-without-source",
		"repo_metadata": {
			"full_name": "acme/fork-without-source",
			"fork": true,
			"source_name": null,
			"host": {"name": "GitHub", "url": "https://github.com", "kind": "github"}
		}
	},
	{
		"name": "fork-without-host",
		"ecosystem": "npm",
		"purl": "pkg:npm/fork-without-host",
		"repo_metadata": {"full_name": "acme/fork-without-host", "fork": true, "source_name": "acme/upstream"}
	},
	{
		"name": "source-without-fork",
		"ecosystem": "npm",
		"purl": "pkg:npm/source-without-fork",
		"repo_metadata": {"full_name": "acme/source-without-fork", "fork": false, "source_name": "acme/upstream"}
	},
	{
		"name": "no-repository",
		"ecosystem": "npm",
		"purl": "pkg:npm/no-repository",
		"repo_metadata": null
	}
]`

func TestEcosystemsClientBulkLookupRepositoryParent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/packages/bulk_lookup" {
			t.Errorf("request = %s %s, want POST /packages/bulk_lookup", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ancestryBulkLookupResponse))
	}))
	defer srv.Close()

	eco, err := ecosystems.NewClient("test-agent/1.0", ecosystems.WithPackagesServer(srv.URL))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	client := &EcosystemsClient{client: eco}

	purls := []string{
		"pkg:npm/%40plotly/d3", "pkg:npm/lodash", "pkg:npm/fork-without-source",
		"pkg:npm/fork-without-host", "pkg:npm/source-without-fork", "pkg:npm/no-repository",
	}
	result, err := client.BulkLookup(context.Background(), purls)
	if err != nil {
		t.Fatalf("BulkLookup() error = %v", err)
	}
	if len(result) != len(purls) {
		t.Fatalf("BulkLookup() returned %d results, want %d", len(result), len(purls))
	}

	tests := map[string]*RepositoryParent{
		"pkg:npm/%40plotly/d3": {
			FullName: "d3/d3", Evidence: AncestryDeclaredFork,
			HostName: "GitHub", HostKind: "github", HostURL: "https://github.com",
		},
		"pkg:npm/lodash":              nil,
		"pkg:npm/fork-without-source": nil,
		// The declared parent is kept; callers decide whether it is usable
		// without host information.
		"pkg:npm/fork-without-host":   {FullName: "acme/upstream", Evidence: AncestryDeclaredFork},
		"pkg:npm/source-without-fork": nil,
		"pkg:npm/no-repository":       nil,
	}
	for purl, want := range tests {
		info := result[purl]
		if info == nil {
			t.Errorf("%s missing from result", purl)
			continue
		}
		if !reflect.DeepEqual(info.RepositoryParent, want) {
			t.Errorf("%s RepositoryParent = %+v, want %+v", purl, info.RepositoryParent, want)
		}
	}

	d3 := result["pkg:npm/%40plotly/d3"]
	if d3.Name != "@plotly/d3" || d3.LatestVersion != "3.8.2" || d3.License != "BSD-3-Clause" ||
		d3.Repository != "https://github.com/plotly/d3" || d3.ChangelogFilename != "CHANGES.md" ||
		d3.Source != "ecosystems" {
		t.Errorf("@plotly/d3 metadata = %+v", d3)
	}
	lodash := result["pkg:npm/lodash"]
	if lodash.LatestVersion != "4.17.21" || lodash.License != "MIT" ||
		lodash.Repository != "https://github.com/lodash/lodash" {
		t.Errorf("lodash metadata = %+v", lodash)
	}
}

func TestExtractRepositoryParentIgnoresMalformedFields(t *testing.T) {
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(`{"fork":"true","source_name":"acme/upstream"}`), &metadata); err != nil {
		t.Fatal(err)
	}
	if parent := extractRepositoryParent(metadata); parent != nil {
		t.Errorf("string fork flag: RepositoryParent = %+v, want nil", parent)
	}
	metadata = nil
	if err := json.Unmarshal([]byte(`{"fork":true,"source_name":42}`), &metadata); err != nil {
		t.Fatal(err)
	}
	if parent := extractRepositoryParent(metadata); parent != nil {
		t.Errorf("numeric source name: RepositoryParent = %+v, want nil", parent)
	}
	metadata = map[string]interface{}{"fork": true, "source_name": "acme/upstream", "host": "GitHub"}
	want := &RepositoryParent{FullName: "acme/upstream", Evidence: AncestryDeclaredFork}
	if parent := extractRepositoryParent(metadata); !reflect.DeepEqual(parent, want) {
		t.Errorf("string host: RepositoryParent = %+v, want %+v", parent, want)
	}
	if parent := extractRepositoryParent(nil); parent != nil {
		t.Errorf("nil metadata: RepositoryParent = %+v, want nil", parent)
	}
}
