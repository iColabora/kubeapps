/*
Copyright (c) Bitnami

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/go-cmp/cmp"
	"github.com/kubeapps/kubeapps/pkg/chart/models"
	"github.com/kubeapps/kubeapps/pkg/dbutils"
)

func getMockManager(t *testing.T) (*postgresAssetManager, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	pgManager := &postgresAssetManager{&dbutils.PostgresAssetManager{DB: db, KubeappsNamespace: "kubeapps"}}

	return pgManager, mock, func() { db.Close() }
}

func Test_PGgetChart(t *testing.T) {
	pgManager, mock, cleanup := getMockManager(t)
	defer cleanup()

	icon := []byte("test")
	iconB64 := base64.StdEncoding.EncodeToString(icon)
	dbChart := models.ChartIconString{
		Chart:   models.Chart{ID: "foo"},
		RawIcon: iconB64,
	}
	dbChartJSON, err := json.Marshal(dbChart)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	mock.ExpectQuery("SELECT info FROM charts*").
		WithArgs("namespace", "foo").
		WillReturnRows(sqlmock.NewRows([]string{"info"}).AddRow(string(dbChartJSON)))

	chart, err := pgManager.getChart("namespace", "foo")
	if err != nil {
		t.Errorf("Found error %v", err)
	}
	expectedChart := models.Chart{
		ID:      "foo",
		RawIcon: icon,
	}
	if !cmp.Equal(chart, expectedChart) {
		t.Errorf("Unexpected result %v", cmp.Diff(chart, expectedChart))
	}
}

func Test_PGgetChartVersion(t *testing.T) {
	pgManager, mock, cleanup := getMockManager(t)
	defer cleanup()

	dbChart := models.Chart{
		ID: "foo",
		ChartVersions: []models.ChartVersion{
			{Version: "1.0.0"},
			{Version: "2.0.0"},
		},
	}
	dbChartJSON, err := json.Marshal(dbChart)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	mock.ExpectQuery("SELECT info FROM charts*").
		WithArgs("namespace", "foo").
		WillReturnRows(sqlmock.NewRows([]string{"info"}).AddRow(string(dbChartJSON)))

	chart, err := pgManager.getChartVersion("namespace", "foo", "1.0.0")
	if err != nil {
		t.Errorf("Found error %v", err)
	}
	expectedChart := models.Chart{
		ID: "foo",
		ChartVersions: []models.ChartVersion{
			{Version: "1.0.0"},
		},
	}
	if !cmp.Equal(chart, expectedChart) {
		t.Errorf("Unexpected result %v", cmp.Diff(chart, expectedChart))
	}
}

func Test_getChartFiles(t *testing.T) {
	pgManager, mock, cleanup := getMockManager(t)
	defer cleanup()

	expectedFiles := models.ChartFiles{ID: "foo"}
	filesJSON, err := json.Marshal(expectedFiles)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	mock.ExpectQuery("SELECT info FROM files*").
		WithArgs("namespace", "foo").
		WillReturnRows(sqlmock.NewRows([]string{"info"}).AddRow(string(filesJSON)))

	files, err := pgManager.getChartFiles("namespace", "foo")
	if err != nil {
		t.Errorf("Found error %v", err)
	}
	if !cmp.Equal(files, expectedFiles) {
		t.Errorf("Unexpected result %v", cmp.Diff(files, expectedFiles))
	}
}

func Test_getChartFiles_withSlashes(t *testing.T) {
	pgManager, mock, cleanup := getMockManager(t)
	defer cleanup()

	expectedFiles := models.ChartFiles{ID: "fo%2Fo"}
	filesJSON, err := json.Marshal(expectedFiles)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	mock.ExpectQuery("SELECT info FROM files*").
		WithArgs("namespace", "fo%2Fo").
		WillReturnRows(sqlmock.NewRows([]string{"info"}).AddRow(string(filesJSON)))

	files, err := pgManager.getChartFiles("namespace", "fo%2Fo")
	if err != nil {
		t.Errorf("Found error %v", err)
	}
	if !cmp.Equal(files, expectedFiles) {
		t.Errorf("Unexpected result %v", cmp.Diff(files, expectedFiles))
	}
}

func Test_getChartsWithFilters(t *testing.T) {
	pgManager, mock, cleanup := getMockManager(t)
	defer cleanup()

	dbChart := models.Chart{
		Name: "foo",
		ChartVersions: []models.ChartVersion{
			{Version: "2.0.0", AppVersion: "2.0.2"},
			{Version: "1.0.0", AppVersion: "1.0.1"},
		},
	}
	dbChartJSON, err := json.Marshal(dbChart)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	version := "1.0.0"
	appVersion := "1.0.1"
	parametrizedJsonbLiteral := fmt.Sprintf(`[{"version":"%s","app_version":"%s"}]`, version, appVersion)

	mock.ExpectQuery("SELECT info FROM charts WHERE *").
		WithArgs("namespace", "kubeapps", "foo", parametrizedJsonbLiteral).
		WillReturnRows(sqlmock.NewRows([]string{"info"}).AddRow(dbChartJSON))

	charts, _, err := pgManager.getPaginatedChartListWithFilters(ChartQuery{namespace: "namespace", chartName: "foo", version: version, appVersion: appVersion}, 1, 0)
	if err != nil {
		t.Errorf("Found error %v", err)
	}
	expectedCharts := []*models.Chart{&models.Chart{
		Name: "foo",
		ChartVersions: []models.ChartVersion{
			{Version: "2.0.0", AppVersion: "2.0.2"},
			{Version: "1.0.0", AppVersion: "1.0.1"},
		},
	}}
	if !cmp.Equal(charts, expectedCharts) {
		t.Errorf("Unexpected result %v", cmp.Diff(charts, expectedCharts))
	}
}

func Test_getChartsWithFilters_withSlashes(t *testing.T) {
	pgManager, mock, cleanup := getMockManager(t)
	defer cleanup()

	dbChart := models.Chart{
		Name: "fo%2Fo",
		ChartVersions: []models.ChartVersion{
			{Version: "2.0.0", AppVersion: "2.0.2"},
			{Version: "1.0.0", AppVersion: "1.0.1"},
		},
	}
	dbChartJSON, err := json.Marshal(dbChart)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	version := "1.0.0"
	appVersion := "1.0.1"
	parametrizedJsonbLiteral := fmt.Sprintf(`[{"version":"%s","app_version":"%s"}]`, version, appVersion)

	mock.ExpectQuery("SELECT info FROM charts WHERE *").
		WithArgs("namespace", "kubeapps", "fo%2Fo", parametrizedJsonbLiteral).
		WillReturnRows(sqlmock.NewRows([]string{"info"}).AddRow(dbChartJSON))

	charts, _, err := pgManager.getPaginatedChartListWithFilters(ChartQuery{namespace: "namespace", chartName: "fo%2Fo", version: version, appVersion: appVersion}, 1, 0)
	if err != nil {
		t.Errorf("Found error %v", err)
	}
	expectedCharts := []*models.Chart{&models.Chart{
		Name: "fo%2Fo",
		ChartVersions: []models.ChartVersion{
			{Version: "2.0.0", AppVersion: "2.0.2"},
			{Version: "1.0.0", AppVersion: "1.0.1"},
		},
	}}
	if !cmp.Equal(charts, expectedCharts) {
		t.Errorf("Unexpected result %v", cmp.Diff(charts, expectedCharts))
	}
}

func Test_getAllChartCategories(t *testing.T) {

	tests := []struct {
		name                    string
		namespace               string
		repo                    string
		expectedChartCategories []*models.ChartCategory
	}{
		{
			name:      "without repo",
			namespace: "other-namespace",
			repo:      "",
			expectedChartCategories: []*models.ChartCategory{
				{Name: "cat1", Count: 1},
				{Name: "cat2", Count: 2},
				{Name: "cat3", Count: 3},
			},
		},
		{
			name:      "with repo",
			namespace: "other-namespace",
			repo:      "bitnami",
			expectedChartCategories: []*models.ChartCategory{
				{Name: "cat1", Count: 1},
				{Name: "cat2", Count: 2},
				{Name: "cat3", Count: 3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgManager, mock, cleanup := getMockManager(t)
			defer cleanup()

			rows := sqlmock.NewRows([]string{"name", "count"})
			for _, chartCategories := range tt.expectedChartCategories {
				rows.AddRow(chartCategories.Name, chartCategories.Count)
			}

			expectedParams := []driver.Value{"other-namespace", "kubeapps"}
			if tt.repo != "" {
				expectedParams = append(expectedParams, tt.repo)
			}
			mock.ExpectQuery("SELECT (info ->> 'category')*").
				WithArgs(expectedParams...).
				WillReturnRows(rows)

			chartCategories, err := pgManager.getAllChartCategories(ChartQuery{namespace: tt.namespace, repos: []string{tt.repo}})
			if err != nil {
				t.Fatalf("Found error %v", err)
			}
			if !cmp.Equal(chartCategories, tt.expectedChartCategories) {
				t.Errorf("Unexpected result %v", cmp.Diff(chartCategories, tt.expectedChartCategories))
			}
		})
	}
}
func Test_getPaginatedChartList(t *testing.T) {
	availableCharts := []*models.Chart{
		{ID: "bar", ChartVersions: []models.ChartVersion{{Digest: "456"}}},
		{ID: "copyFoo", ChartVersions: []models.ChartVersion{{Digest: "123"}}},
		{ID: "foo", ChartVersions: []models.ChartVersion{{Digest: "123"}}},
		{ID: "fo%2Fo", ChartVersions: []models.ChartVersion{{Digest: "321"}}},
	}
	tests := []struct {
		name               string
		namespace          string
		repo               string
		pageNumber         int
		pageSize           int
		expectedCharts     []*models.Chart
		expectedTotalPages int
	}{
		{
			name:               "one page with duplicates with repo",
			namespace:          "other-namespace",
			repo:               "bitnami",
			pageNumber:         1,
			pageSize:           100,
			expectedCharts:     availableCharts,
			expectedTotalPages: 1,
		},
		{
			name:               "one page with duplicates",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         1,
			pageSize:           100,
			expectedCharts:     availableCharts,
			expectedTotalPages: 1,
		},
		{
			name:               "repo has many charts with pagination (2 pages)",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         2,
			pageSize:           2,
			expectedCharts:     []*models.Chart{availableCharts[2], availableCharts[3]},
			expectedTotalPages: 2,
		},
		{
			name:               "repo has many charts with pagination (non existing page)",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         3,
			pageSize:           2,
			expectedCharts:     []*models.Chart{},
			expectedTotalPages: 2,
		},
		{
			name:               "repo has many charts with pagination (out of range size)",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         1,
			pageSize:           100,
			expectedCharts:     availableCharts,
			expectedTotalPages: 1,
		},
		{
			name:               "repo has many charts with pagination (w/ page, w size)",
			namespace:          "other-namespace",
			repo:               "",
			pageSize:           3,
			expectedCharts:     []*models.Chart{availableCharts[0], availableCharts[1], availableCharts[2]},
			expectedTotalPages: 2,
		},
		{
			name:               "repo has many charts with pagination (w/ page, w zero size)",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         2,
			pageSize:           0,
			expectedCharts:     availableCharts,
			expectedTotalPages: 1,
		},
		{
			name:               "repo has many charts with pagination (w/ wrong page, w/ size)",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         -2,
			pageSize:           2,
			expectedCharts:     []*models.Chart{availableCharts[0], availableCharts[1]},
			expectedTotalPages: 2,
		},
		{
			name:               "repo has many charts with pagination (w/ page, w/o size)",
			namespace:          "other-namespace",
			repo:               "",
			pageNumber:         2,
			expectedCharts:     availableCharts,
			expectedTotalPages: 1,
		},
		{
			name:               "repo has many charts with pagination (w/o page, w/ size)",
			namespace:          "other-namespace",
			repo:               "",
			pageSize:           2,
			expectedCharts:     []*models.Chart{availableCharts[0], availableCharts[1]},
			expectedTotalPages: 2,
		},
		{
			name:               "repo has many charts with pagination (w/o page, w/o size)",
			namespace:          "other-namespace",
			repo:               "",
			expectedCharts:     availableCharts,
			expectedTotalPages: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgManager, mock, cleanup := getMockManager(t)
			defer cleanup()

			rows := sqlmock.NewRows([]string{"info"})
			rowCount := sqlmock.NewRows([]string{"count"}).AddRow(len(availableCharts))

			for _, chart := range tt.expectedCharts {
				chartJSON, err := json.Marshal(chart)
				if err != nil {
					t.Fatalf("%+v", err)
				}
				rows.AddRow(string(chartJSON))
			}
			expectedParams := []driver.Value{"other-namespace", "kubeapps"}
			if tt.repo != "" {
				expectedParams = append(expectedParams, "bitnami")
			}

			mock.ExpectQuery("SELECT info FROM *").
				WithArgs(expectedParams...).
				WillReturnRows(rows)

			mock.ExpectQuery("^SELECT count(.+) FROM").
				WillReturnRows(rowCount)

			charts, totalPages, err := pgManager.getPaginatedChartListWithFilters(ChartQuery{namespace: tt.namespace, repos: []string{tt.repo}}, tt.pageNumber, tt.pageSize)
			if err != nil {
				t.Fatalf("Found error %v", err)
			}
			if totalPages != tt.expectedTotalPages {
				t.Errorf("Unexpected number of pages, got %d expecting %d", totalPages, tt.expectedTotalPages)
			}
			if tt.pageSize > 0 {
				if len(charts) > tt.pageSize {
					t.Errorf("Unexpected number of charts, got %d expecting %d", len(charts), tt.pageSize)
				}
			}
			if !cmp.Equal(charts, tt.expectedCharts) {
				t.Errorf("Unexpected result %v", cmp.Diff(tt.expectedCharts, charts))
			}
		})
	}
}

func Test_generateWhereClause(t *testing.T) {
	tests := []struct {
		name           string
		namespace      string
		chartName      string
		version        string
		appVersion     string
		repos          []string
		categories     []string
		query          string
		expectedClause string
		expectedParams []interface{}
	}{
		{
			name:           "returns where clause - no params",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: "WHERE (repo_namespace = $1 OR repo_namespace = $2)",
			expectedParams: []interface{}{string(""), string("kubeapps")},
		},
		{
			name:           "returns where clause - single param - namespace",
			namespace:      "my-ns",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: "WHERE (repo_namespace = $1 OR repo_namespace = $2)",
			expectedParams: []interface{}{string("my-ns"), string("kubeapps")},
		},
		{
			name:           "returns where clause - single param - name",
			namespace:      "",
			chartName:      "my-chart",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: "WHERE (repo_namespace = $1 OR repo_namespace = $2) AND (info->>'name' = $3)",
			expectedParams: []interface{}{string(""), string("kubeapps"), string("my-chart")},
		},
		{
			name:           "returns where clause - single param - version",
			namespace:      "",
			chartName:      "",
			version:        "1.0.0",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: "WHERE (repo_namespace = $1 OR repo_namespace = $2)", //needs both version and appVersion
			expectedParams: []interface{}{string(""), string("kubeapps")},
		},
		{
			name:           "returns where clause - single param - appVersion",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "0.1.0",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: "WHERE (repo_namespace = $1 OR repo_namespace = $2)", //needs both version and appVersion
			expectedParams: []interface{}{string(""), string("kubeapps")},
		},
		{
			name:           "returns where clause - single param - version AND appVersion",
			namespace:      "",
			chartName:      "",
			version:        "1.0.0",
			appVersion:     "0.1.0",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND (info->'chartVersions' @> $3::jsonb)`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string(`[{"version":"1.0.0","app_version":"0.1.0"}]`)},
		},
		{
			name:           "returns where clause - no params",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "",
			expectedClause: "WHERE (repo_namespace = $1 OR repo_namespace = $2)",
			expectedParams: []interface{}{string(""), string("kubeapps")},
		},
		{
			name:           "returns where clause - single param - single repo",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{"my-repo1"},
			categories:     []string{""},
			query:          "",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND ((repo_name = $3))`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("my-repo1")},
		},
		{
			name:           "returns where clause - single param - multiple repos",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{"my-repo1", "my-repo2"},
			categories:     []string{""},
			query:          "",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND ((repo_name = $3) OR (repo_name = $4))`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("my-repo1"), string("my-repo2")},
		},
		{
			name:           "returns where clause - single param - single category",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{"my-category1"},
			query:          "",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND (info->>'category' = $3)`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("my-category1")},
		},
		{
			name:           "returns where clause - single param - multiple categories",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{"my-category1", "my-category2"},
			query:          "",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND (info->>'category' = $3 OR info->>'category' = $4)`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("my-category1"), string("my-category2")},
		},
		{
			name:           "returns where clause - single param - query (one word)",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "chart",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND ((info ->> 'name' ILIKE $3) OR (info ->> 'description' ILIKE $3) OR (info -> 'repo' ->> 'name' ILIKE $3) OR (info ->> 'keywords' ILIKE $3) OR (info ->> 'sources' ILIKE $3) OR (info -> 'maintainers' ->> 'name' ILIKE $3))`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("%chart%")},
		},
		{
			name:           "returns where clause - single param - query (two words)",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "my chart",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND ((info ->> 'name' ILIKE $3) OR (info ->> 'description' ILIKE $3) OR (info -> 'repo' ->> 'name' ILIKE $3) OR (info ->> 'keywords' ILIKE $3) OR (info ->> 'sources' ILIKE $3) OR (info -> 'maintainers' ->> 'name' ILIKE $3))`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("%my chart%")},
		},
		{
			name:           "returns where clause - single param - query (with slash)",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "my/chart",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND ((info ->> 'name' ILIKE $3) OR (info ->> 'description' ILIKE $3) OR (info -> 'repo' ->> 'name' ILIKE $3) OR (info ->> 'keywords' ILIKE $3) OR (info ->> 'sources' ILIKE $3) OR (info -> 'maintainers' ->> 'name' ILIKE $3))`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("%my/chart%")},
		},
		{
			name:           "returns where clause - single param - query (encoded)",
			namespace:      "",
			chartName:      "",
			version:        "",
			appVersion:     "",
			repos:          []string{""},
			categories:     []string{""},
			query:          "my%2Fchart",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND ((info ->> 'name' ILIKE $3) OR (info ->> 'description' ILIKE $3) OR (info -> 'repo' ->> 'name' ILIKE $3) OR (info ->> 'keywords' ILIKE $3) OR (info ->> 'sources' ILIKE $3) OR (info -> 'maintainers' ->> 'name' ILIKE $3))`,
			expectedParams: []interface{}{string(""), string("kubeapps"), string("%my%2Fchart%")},
		},
		{
			name:           "returns where clause - every param",
			namespace:      "my-ns",
			chartName:      "my-chart",
			version:        "1.0.0",
			appVersion:     "0.1.0",
			repos:          []string{"my-repo1", "my-repo2"},
			categories:     []string{"my-category1", "my-category2"},
			query:          "best chart",
			expectedClause: `WHERE (repo_namespace = $1 OR repo_namespace = $2) AND (info->>'name' = $3) AND (info->'chartVersions' @> $4::jsonb) AND ((repo_name = $5) OR (repo_name = $6)) AND (info->>'category' = $7 OR info->>'category' = $8) AND ((info ->> 'name' ILIKE $9) OR (info ->> 'description' ILIKE $9) OR (info -> 'repo' ->> 'name' ILIKE $9) OR (info ->> 'keywords' ILIKE $9) OR (info ->> 'sources' ILIKE $9) OR (info -> 'maintainers' ->> 'name' ILIKE $9))`,
			expectedParams: []interface{}{string("my-ns"), string("kubeapps"), string("my-chart"), string(`[{"version":"1.0.0","app_version":"0.1.0"}]`), string("my-repo1"), string("my-repo2"), string("my-category1"), string("my-category2"), string("%best chart%")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgManager, _, cleanup := getMockManager(t)
			defer cleanup()

			cq := ChartQuery{
				namespace:   tt.namespace,
				chartName:   tt.chartName,
				version:     tt.version,
				appVersion:  tt.appVersion,
				searchQuery: tt.query,
				repos:       tt.repos,
				categories:  tt.categories,
			}
			whereQuery, whereQueryParams := pgManager.generateWhereClause(cq)

			if tt.expectedClause != whereQuery {
				t.Errorf("Expecting query:\n'%s'\nreceived query:\n'%s'\nin '%s'", tt.expectedClause, whereQuery, tt.name)
			}

			if !cmp.Equal(tt.expectedParams, whereQueryParams) {
				t.Errorf("Param mismatch in '%s': %s", tt.name, cmp.Diff(tt.expectedParams, whereQueryParams))
			}
		})
	}
}
