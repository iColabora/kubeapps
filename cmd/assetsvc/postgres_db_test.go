/*
Copyright (c) 2020 Bitnami

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

// Currently these tests will be skipped entirely unless the
// ENABLE_PG_INTEGRATION_TESTS env var is set.
// Run the local postgres with
// docker run --publish 5432:5432 -e ALLOW_EMPTY_PASSWORD=yes bitnami/postgresql:11.13.0-debian-10-r0
// in another terminal.
package main

import (
	"database/sql"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeapps/kubeapps/cmd/assetsvc/pkg/utils"
	"github.com/kubeapps/kubeapps/pkg/chart/models"
	"github.com/kubeapps/kubeapps/pkg/dbutils/dbutilstest"
	"github.com/kubeapps/kubeapps/pkg/dbutils/dbutilstest/pgtest"
	_ "github.com/lib/pq"
)

func getInitializedManager(t *testing.T) (*utils.PostgresAssetManager, func()) {
	pam, cleanup := pgtest.GetInitializedManager(t)
	return &utils.PostgresAssetManager{pam}, cleanup
}

func TestGetChart(t *testing.T) {
	pgtest.SkipIfNoDB(t)
	const repoName = "repo-name"

	testCases := []struct {
		name string
		// existingCharts is a map of charts per namespace
		existingCharts map[string][]models.Chart
		chartId        string
		namespace      string
		expectedChart  string
		expectedErr    error
	}{
		{
			name:        "it returns an error if the chart does not exist",
			chartId:     "doesnt-exist-1",
			namespace:   "doesnt-exist",
			expectedErr: sql.ErrNoRows,
		},
		{
			name: "it returns an error if the chart does not exist in that repo",
			existingCharts: map[string][]models.Chart{
				"namespace-1": []models.Chart{
					models.Chart{ID: "chart-1", Name: "my-chart"},
				},
			},
			chartId:     "chart-1",
			namespace:   "other-namespace",
			expectedErr: sql.ErrNoRows,
		},
		{
			name: "it returns the chart matching the chartid",
			existingCharts: map[string][]models.Chart{
				"namespace-1": []models.Chart{
					models.Chart{ID: "chart-1", Name: "my-chart"},
				},
			},
			chartId:       "chart-1",
			namespace:     "namespace-1",
			expectedErr:   nil,
			expectedChart: "my-chart",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pam, cleanup := getInitializedManager(t)
			defer cleanup()
			for namespace, charts := range tc.existingCharts {
				pgtest.EnsureChartsExist(t, pam, charts, models.Repo{Name: repoName, Namespace: namespace})
			}

			chart, err := pam.GetChart(tc.namespace, tc.chartId)

			if got, want := err, tc.expectedErr; got != want {
				t.Fatalf("In '"+tc.name+"': "+"got: %+v, want: %+v", got, want)
			}
			if got, want := chart.Name, tc.expectedChart; got != want {
				t.Errorf("In '"+tc.name+"': "+"got: %q, want: %q", got, want)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	pgtest.SkipIfNoDB(t)
	const repoName = "repo-name"

	testCases := []struct {
		name string
		// existingCharts is a map of charts per namespace
		existingCharts   map[string][]models.Chart
		chartId          string
		namespace        string
		requestedVersion string
		expectedVersion  string
		expectedErr      error
	}{
		{
			name:        "it returns an error if the chart does not exist",
			chartId:     "doesnt-exist-1",
			namespace:   "doesnt-exist",
			expectedErr: sql.ErrNoRows,
		},
		{
			name: "it returns an error if the chart version does not exist",
			existingCharts: map[string][]models.Chart{
				"namespace-1": []models.Chart{
					models.Chart{ID: "chart-1", ChartVersions: []models.ChartVersion{
						models.ChartVersion{Version: "1.2.3"},
					}},
				},
			},
			chartId:          "chart-1",
			namespace:        "namespace-1",
			requestedVersion: "doesnt-exist",
			expectedErr:      utils.ErrChartVersionNotFound,
		},
		{
			name: "it returns an error if the chart version does not exist in that namespace",
			existingCharts: map[string][]models.Chart{
				"namespace-1": []models.Chart{
					models.Chart{ID: "chart-1", ChartVersions: []models.ChartVersion{
						models.ChartVersion{Version: "1.2.3"},
					}},
				},
			},
			chartId:          "chart-1",
			namespace:        "other-namespace",
			requestedVersion: "1.2.3",
			expectedErr:      sql.ErrNoRows,
		},
		{
			name: "it returns the chart version matching the chartid and version",
			existingCharts: map[string][]models.Chart{
				"namespace-1": []models.Chart{
					models.Chart{ID: "chart-1", ChartVersions: []models.ChartVersion{
						models.ChartVersion{Version: "1.2.3"},
						models.ChartVersion{Version: "4.5.6"},
					}},
				},
			},
			chartId:          "chart-1",
			namespace:        "namespace-1",
			requestedVersion: "1.2.3",
			expectedVersion:  "1.2.3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pam, cleanup := getInitializedManager(t)
			defer cleanup()
			for namespace, charts := range tc.existingCharts {
				pgtest.EnsureChartsExist(t, pam, charts, models.Repo{Name: repoName, Namespace: namespace})
			}

			chart, err := pam.GetChartVersion(tc.namespace, tc.chartId, tc.requestedVersion)

			if got, want := err, tc.expectedErr; got != want {
				t.Fatalf("got: %+v, want: %+v", got, want)
			}
			if tc.expectedErr != nil {
				return
			}
			// The function just returns the chart with only the one version.
			if got, want := len(chart.ChartVersions), 1; got != want {
				t.Fatalf("got: %d, want: %d", got, want)
			}
			if got, want := chart.ChartVersions[0].Version, tc.expectedVersion; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}

func TestGetPaginatedChartList(t *testing.T) {
	pgtest.SkipIfNoDB(t)
	const (
		repoName      = "repo-name"
		namespaceName = "namespace-name"
	)

	chartVersions := []models.ChartVersion{
		models.ChartVersion{
			Digest: "abc-123",
		},
	}

	testCases := []struct {
		name string
		// existingCharts is a map of charts per namespace and repo
		existingCharts   map[string]map[string][]models.Chart
		namespace        string
		repo             string
		expectedCharts   []*models.Chart
		expectedErr      error
		expectedNumPages int
	}{
		{
			name:             "it returns an empty list if the repo or namespace do not exist",
			repo:             "repo-doesnt-exist",
			namespace:        "doesnt-exist",
			expectedCharts:   []*models.Chart{},
			expectedNumPages: 0,
		},
		{
			name:      "it returns charts from a specific repo in a specific namespace",
			repo:      repoName,
			namespace: namespaceName,
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
					},
					"other-repo": []models.Chart{
						models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
					},
				},
				"other-namespace": map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
					},
				},
			},
			expectedCharts: []*models.Chart{
				&models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
			},
			expectedNumPages: 1,
		},
		{
			name: "it returns charts from multiple repos in a specific namespace",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
					},
					"other-repo": []models.Chart{
						models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
					},
				},
				"other-namespace": map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
					},
				},
			},
			repo:      "",
			namespace: namespaceName,
			expectedCharts: []*models.Chart{
				&models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
				&models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
			},
			expectedNumPages: 1,
		},
		{
			name: "it includes charts from global repositories and the specific namespace",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
					},
					"other-repo": []models.Chart{
						models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
					},
				},
				"other-namespace": map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
					},
				},
				dbutilstest.KubeappsTestNamespace: map[string][]models.Chart{
					"global-repo": []models.Chart{
						models.Chart{ID: "global-repo/global-chart", Name: "global-chart"},
					},
				},
			},
			repo:      "",
			namespace: namespaceName,
			expectedCharts: []*models.Chart{
				&models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
				&models.Chart{ID: "global-repo/global-chart", Name: "global-chart"},
				&models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
			},
			expectedNumPages: 1,
		},
		{
			name: "it returns charts from multiple repos across all namespaces",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
					},
					"other-repo": []models.Chart{
						models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
					},
				},
				"other-namespace": map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
					},
				},
			},
			repo:      "",
			namespace: "_all",
			expectedCharts: []*models.Chart{
				&models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
				&models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
				&models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
			},
			expectedNumPages: 1,
		},
		{
			name: "it returns charts from a single repo across all namespaces",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
					},
					"other-repo": []models.Chart{
						models.Chart{ID: "other-repo/other-chart", Name: "other-chart"},
					},
				},
				"other-namespace": map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
					},
				},
			},
			repo:      repoName,
			namespace: "_all",
			expectedCharts: []*models.Chart{
				&models.Chart{ID: repoName + "/chart-1", Name: "chart-1"},
				&models.Chart{ID: repoName + "/chart-in-other-namespace", Name: "chart-in-other-namespace"},
			},
			expectedNumPages: 1,
		},
		{
			name: "it does not remove duplicates",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: map[string][]models.Chart{
					repoName: []models.Chart{
						models.Chart{ID: repoName + "/chart-1", Name: "chart-1", ChartVersions: chartVersions},
					},
					"other-repo": []models.Chart{
						models.Chart{ID: "other-repo/same-chart-different-repo", Name: "same-chart-different-repo", ChartVersions: chartVersions},
					},
				},
			},
			repo:      "",
			namespace: namespaceName,
			expectedCharts: []*models.Chart{
				&models.Chart{ID: repoName + "/chart-1", Name: "chart-1", ChartVersions: chartVersions},
				&models.Chart{ID: "other-repo/same-chart-different-repo", Name: "same-chart-different-repo", ChartVersions: chartVersions},
			},
			expectedNumPages: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pam, cleanup := getInitializedManager(t)
			defer cleanup()
			for namespace, chartsPerRepo := range tc.existingCharts {
				for repo, charts := range chartsPerRepo {
					pgtest.EnsureChartsExist(t, pam, charts, models.Repo{Name: repo, Namespace: namespace})
				}
			}

			charts, numPages, err := pam.GetPaginatedChartListWithFilters(utils.ChartQuery{Namespace: tc.namespace, Repos: []string{tc.repo}}, 1, 10)

			if got, want := err, tc.expectedErr; got != want {
				t.Fatalf("In '"+tc.name+"': "+"got err: %+v, want: %+v", got, want)
			}
			if got, want := numPages, tc.expectedNumPages; got != want {
				t.Fatalf("In '"+tc.name+"': "+"got numPages: %+v, want: %+v", got, want)
			}
			if got, want := charts, tc.expectedCharts; !cmp.Equal(want, got) {
				t.Errorf("In '"+tc.name+"': "+"mismatch (-want +got):\n%s", cmp.Diff(want, got))
			}
		})
	}
}

// ByID implements sort.Interface for []models.Chart based on
// the ID field.
type byID []*models.Chart

func (a byID) Len() int           { return len(a) }
func (a byID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byID) Less(i, j int) bool { return a[i].ID < a[j].ID }

func TestGetChartsWithFilters(t *testing.T) {
	pgtest.SkipIfNoDB(t)
	const (
		repoName1     = "repo-name-1"
		repoName2     = "repo-name-2"
		namespaceName = "namespace-name"
	)

	chartVersion := models.ChartVersion{
		Digest:     "abc-123",
		Version:    "1.0chart",
		AppVersion: "2.0app",
	}

	chartVersions := []models.ChartVersion{chartVersion}

	chartWithVersionRepo1 := models.Chart{ID: repoName1 + "/chart-1", Name: "chart-1", ChartVersions: chartVersions}
	chartWithVersionRepo2 := models.Chart{ID: repoName2 + "/chart-1", Name: "chart-1", ChartVersions: chartVersions}

	testCases := []struct {
		name string
		// existingCharts is a map of charts per namespace and repo
		existingCharts map[string]map[string][]models.Chart
		namespace      string
		chartName      string
		chartVersion   string
		appVersion     string
		expectedCharts []*models.Chart
		expectedErr    error
	}{
		{
			name: "returns charts in the specific namespace",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: {
					repoName1: {chartWithVersionRepo1},
					"other-repo": []models.Chart{
						{ID: "other-repo/other-chart", Name: "other-chart"},
					},
				},
				"other-namespace": {
					repoName1: {chartWithVersionRepo1},
				},
			},
			namespace:    namespaceName,
			chartName:    chartWithVersionRepo1.Name,
			chartVersion: chartWithVersionRepo1.ChartVersions[0].Version,
			appVersion:   chartWithVersionRepo1.ChartVersions[0].AppVersion,
			expectedCharts: []*models.Chart{
				&chartWithVersionRepo1,
			},
		},
		{
			name: "returns charts from different repos in the specific namespace",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: {
					repoName1:    {chartWithVersionRepo1},
					"other-repo": {chartWithVersionRepo2},
				},
			},
			namespace:    namespaceName,
			chartName:    chartWithVersionRepo1.Name,
			chartVersion: chartWithVersionRepo1.ChartVersions[0].Version,
			appVersion:   chartWithVersionRepo1.ChartVersions[0].AppVersion,
			expectedCharts: []*models.Chart{
				&chartWithVersionRepo1,
				&chartWithVersionRepo2,
			},
		},
		{
			name: "includes charts from global repositories",
			existingCharts: map[string]map[string][]models.Chart{
				namespaceName: {
					repoName1: {chartWithVersionRepo1},
				},
				dbutilstest.KubeappsTestNamespace: {
					"other-repo": {chartWithVersionRepo2},
				},
			},
			namespace:    namespaceName,
			chartName:    chartWithVersionRepo1.Name,
			chartVersion: chartWithVersionRepo1.ChartVersions[0].Version,
			appVersion:   chartWithVersionRepo1.ChartVersions[0].AppVersion,
			expectedCharts: []*models.Chart{
				&chartWithVersionRepo1,
				&chartWithVersionRepo2,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pam, cleanup := getInitializedManager(t)
			defer cleanup()
			for namespace, chartsPerRepo := range tc.existingCharts {
				for repo, charts := range chartsPerRepo {
					pgtest.EnsureChartsExist(t, pam, charts, models.Repo{Name: repo, Namespace: namespace})
				}
			}

			charts, _, err := pam.GetPaginatedChartListWithFilters(utils.ChartQuery{Namespace: tc.namespace, ChartName: tc.chartName, Version: tc.chartVersion, AppVersion: tc.appVersion}, 1, 0)
			if err != nil {
				t.Fatalf("%+v", err)
			}

			sort.Sort(byID(charts))
			sort.Sort(byID(tc.expectedCharts))
			if got, want := charts, tc.expectedCharts; !cmp.Equal(want, got) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(want, got))
			}
		})

	}
}
