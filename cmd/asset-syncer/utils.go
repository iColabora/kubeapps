/*
Copyright (c) 2018 The Helm Authors

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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/disintegration/imaging"
	"github.com/ghodss/yaml"
	"github.com/itchyny/gojq"
	"github.com/kubeapps/common/datastore"
	apprepov1alpha1 "github.com/kubeapps/kubeapps/cmd/apprepository-controller/pkg/apis/apprepository/v1alpha1"
	"github.com/kubeapps/kubeapps/pkg/chart/models"
	"github.com/kubeapps/kubeapps/pkg/helm"
	httpclient "github.com/kubeapps/kubeapps/pkg/http-client"
	"github.com/kubeapps/kubeapps/pkg/tarutil"
	log "github.com/sirupsen/logrus"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	h3chart "helm.sh/helm/v3/pkg/chart"
	"k8s.io/helm/pkg/proto/hapi/chart"
)

const (
	additionalCAFile = "/usr/local/share/ca-certificates/ca.crt"
	numWorkers       = 10
)

type importChartFilesJob struct {
	Name         string
	Repo         *models.Repo
	ChartVersion models.ChartVersion
}

type pullChartJob struct {
	AppName string
	Tag     string
}

type pullChartResult struct {
	Chart *models.Chart
	Error error
}

type checkTagJob struct {
	AppName string
	Tag     string
}

type checkTagResult struct {
	checkTagJob
	isHelmChart bool
	Error       error
}

func parseRepoURL(repoURL string) (*url.URL, error) {
	repoURL = strings.TrimSpace(repoURL)
	return url.ParseRequestURI(repoURL)
}

type assetManager interface {
	Delete(repo models.Repo) error
	Sync(repo models.Repo, charts []models.Chart) error
	LastChecksum(repo models.Repo) string
	UpdateLastCheck(repoNamespace, repoName, checksum string, now time.Time) error
	Init() error
	Close() error
	InvalidateCache() error
	updateIcon(repo models.Repo, data []byte, contentType, ID string) error
	filesExist(repo models.Repo, chartFilesID, digest string) bool
	insertFiles(chartID string, files models.ChartFiles) error
}

func newManager(config datastore.Config, kubeappsNamespace string) (assetManager, error) {
	return newPGManager(config, kubeappsNamespace)
}

func getSha256(src []byte) (string, error) {
	f := bytes.NewReader(src)
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// Repo defines the methods to retrive information from the given repository
type Repo interface {
	Checksum() (string, error)
	Repo() *models.RepoInternal
	FilterIndex()
	Charts(fetchLatestOnly bool) ([]models.Chart, error)
	FetchFiles(name string, cv models.ChartVersion) (map[string]string, error)
}

// HelmRepo implements the Repo interface for chartmuseum-like repositories
type HelmRepo struct {
	content []byte
	*models.RepoInternal
	netClient httpclient.Client
	filter    *apprepov1alpha1.FilterRuleSpec
}

// Checksum returns the sha256 of the repo
func (r *HelmRepo) Checksum() (string, error) {
	return getSha256(r.content)
}

// Repo returns the repo information
func (r *HelmRepo) Repo() *models.RepoInternal {
	return r.RepoInternal
}

// FilterRepo is a no-op for a Helm repo
func (r *HelmRepo) FilterIndex() {
	// no-op
}

func compileJQ(rule *apprepov1alpha1.FilterRuleSpec) (*gojq.Code, []interface{}, error) {
	query, err := gojq.Parse(rule.JQ)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to parse jq query: %v", err)
	}
	varNames := []string{}
	varValues := []interface{}{}
	for name, val := range rule.Variables {
		varNames = append(varNames, name)
		varValues = append(varValues, val)
	}
	code, err := gojq.Compile(
		query,
		gojq.WithVariables(varNames),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to compile jq: %v", err)
	}
	return code, varValues, nil
}

func satisfy(chartInput map[string]interface{}, code *gojq.Code, vars []interface{}) (bool, error) {
	res, _ := code.Run(chartInput, vars...).Next()
	if err, ok := res.(error); ok {
		return false, fmt.Errorf("Unable to run jq: %v", err)
	}

	satisfied, ok := res.(bool)
	if !ok {
		return false, fmt.Errorf("Unable to convert jq result to boolean. Got: %v", res)
	}
	return satisfied, nil
}

func filterCharts(charts []models.Chart, filterRule *apprepov1alpha1.FilterRuleSpec) ([]models.Chart, error) {
	if filterRule == nil || filterRule.JQ == "" {
		// No filter
		return charts, nil
	}
	jqCode, vars, err := compileJQ(filterRule)
	if err != nil {
		return nil, err
	}
	result := []models.Chart{}
	for _, chart := range charts {
		// Convert the chart to a map[interface]{}
		chartBytes, err := json.Marshal(chart)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse chart: %v", err)
		}
		chartInput := map[string]interface{}{}
		err = json.Unmarshal(chartBytes, &chartInput)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse chart: %v", err)
		}

		satisfied, err := satisfy(chartInput, jqCode, vars)
		if err != nil {
			return nil, err
		}
		if satisfied {
			// All rules have been checked and matched
			result = append(result, chart)
		}
	}
	return result, nil
}

// Charts retrieve the list of charts exposed in the repo
func (r *HelmRepo) Charts(fetchLatestOnly bool) ([]models.Chart, error) {
	repo := &models.Repo{
		Namespace: r.Namespace,
		Name:      r.Name,
		URL:       r.URL,
		Type:      r.Type,
	}
	charts, err := helm.ChartsFromIndex(r.content, repo, fetchLatestOnly)
	if err != nil {
		return []models.Chart{}, err
	}
	if len(charts) == 0 {
		return []models.Chart{}, fmt.Errorf("no charts in repository index")
	}

	return filterCharts(charts, r.filter)
}

// FetchFiles retrieves the important files of a chart and version from the repo
func (r *HelmRepo) FetchFiles(name string, cv models.ChartVersion) (map[string]string, error) {
	authorizationHeader := ""
	chartTarballURL := chartTarballURL(r.RepoInternal, cv)

	if passCredentials || len(r.AuthorizationHeader) > 0 && isURLDomainEqual(chartTarballURL, r.URL) {
		authorizationHeader = r.AuthorizationHeader
	}

	return tarutil.FetchChartDetailFromTarball(
		name,
		chartTarballURL,
		userAgent(),
		authorizationHeader,
		r.netClient)
}

// TagList represents a list of tags as specified at
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#content-discovery
type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// OCIRegistry implements the Repo interface for OCI repositories
type OCIRegistry struct {
	repositories []string
	*models.RepoInternal
	tags   map[string]TagList
	puller helm.ChartPuller
	ociCli ociAPI
	filter *apprepov1alpha1.FilterRuleSpec
}

func doReq(url string, cli httpclient.Client, headers map[string]string) ([]byte, error) {
	headers["User-Agent"] = userAgent()
	return httpclient.Get(url, cli, headers)
}

// OCILayer represents a single OCI layer
type OCILayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

// OCIManifest representation
type OCIManifest struct {
	Schema int        `json:"schema"`
	Config OCILayer   `json:"config"`
	Layers []OCILayer `json:"layers"`
}

type ociAPI interface {
	TagList(appName string) (*TagList, error)
	IsHelmChart(appName, tag string) (bool, error)
}

type ociAPICli struct {
	authHeader string
	url        *url.URL
	netClient  httpclient.Client
}

// TagList retrieves the list of tags for an asset
func (o *ociAPICli) TagList(appName string) (*TagList, error) {
	url := *o.url
	url.Path = path.Join("v2", url.Path, appName, "tags", "list")
	data, err := doReq(url.String(), o.netClient, map[string]string{"Authorization": o.authHeader})
	if err != nil {
		return nil, err
	}

	var appTags TagList
	err = json.Unmarshal(data, &appTags)
	if err != nil {
		return nil, err
	}
	return &appTags, nil
}

func (o *ociAPICli) IsHelmChart(appName, tag string) (bool, error) {
	repoURL := *o.url
	repoURL.Path = path.Join("v2", repoURL.Path, appName, "manifests", tag)
	log.Debugf("getting tag %s", repoURL.String())
	manifestData, err := doReq(
		repoURL.String(),
		o.netClient,
		map[string]string{
			"Authorization": o.authHeader,
			"Accept":        "application/vnd.oci.image.manifest.v1+json",
		})
	if err != nil {
		return false, err
	}
	var manifest OCIManifest
	err = json.Unmarshal(manifestData, &manifest)
	if err != nil {
		return false, err
	}
	return manifest.Config.MediaType == helm.HelmChartConfigMediaType, nil
}

func tagCheckerWorker(o ociAPI, tagJobs <-chan checkTagJob, resultChan chan checkTagResult) {
	for j := range tagJobs {
		isHelmChart, err := o.IsHelmChart(j.AppName, j.Tag)
		resultChan <- checkTagResult{j, isHelmChart, err}
	}
}

// Checksum returns the sha256 of the repo by concatenating tags for
// all repositories within the registry and returning the sha256.
// Caveat: Mutated image tags won't be detected as new
func (r *OCIRegistry) Checksum() (string, error) {
	r.tags = map[string]TagList{}
	for _, appName := range r.repositories {
		tags, err := r.ociCli.TagList(appName)
		if err != nil {
			return "", err
		}
		r.tags[appName] = *tags
	}

	content, err := json.Marshal(r.tags)
	if err != nil {
		return "", err
	}

	return getSha256(content)
}

// Repo returns the repo information
func (r *OCIRegistry) Repo() *models.RepoInternal {
	return r.RepoInternal
}

type artifactFiles struct {
	Metadata string
	Readme   string
	Values   string
	Schema   string
}

func extractFilesFromBuffer(buf *bytes.Buffer) (*artifactFiles, error) {
	result := &artifactFiles{}
	gzf, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	tarReader := tar.NewReader(gzf)
	importantFiles := map[string]bool{
		"chart.yaml":         true,
		"readme.md":          true,
		"values.yaml":        true,
		"values.schema.json": true,
	}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		compressedFileName := header.Name
		if len(strings.Split(compressedFileName, "/")) > 2 {
			// We are only interested on files within the root directory
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Ignore directories
		case tar.TypeReg:
			filename := strings.ToLower(path.Base(compressedFileName))
			if importantFiles[filename] {
				// Read content
				data, err := ioutil.ReadAll(tarReader)
				if err != nil {
					return nil, err
				}
				switch filename {
				case "chart.yaml":
					result.Metadata = string(data)
				case "readme.md":
					result.Readme = string(data)
				case "values.yaml":
					result.Values = string(data)
				case "values.schema.json":
					result.Schema = string(data)
				}
			}
		default:
			// Unknown type, ignore
		}
	}
	return result, nil
}

func pullAndExtract(repoURL *url.URL, appName, tag string, puller helm.ChartPuller, r *OCIRegistry) (*models.Chart, error) {
	ref := path.Join(repoURL.Host, repoURL.Path, fmt.Sprintf("%s:%s", appName, tag))

	chartBuffer, digest, err := puller.PullOCIChart(ref)
	if err != nil {
		return nil, err
	}

	// Extract
	files, err := extractFilesFromBuffer(chartBuffer)
	if err != nil {
		return nil, err
	}
	chartMetadata := h3chart.Metadata{}
	err = yaml.Unmarshal([]byte(files.Metadata), &chartMetadata)
	if err != nil {
		return nil, err
	}

	// Format Data
	chartVersion := models.ChartVersion{
		Version:    chartMetadata.Version,
		AppVersion: chartMetadata.AppVersion,
		Digest:     digest,
		URLs:       chartMetadata.Sources,
		Readme:     files.Readme,
		Values:     files.Values,
		Schema:     files.Schema,
	}

	maintainers := []chart.Maintainer{}
	for _, m := range chartMetadata.Maintainers {
		maintainers = append(maintainers, chart.Maintainer{
			Name:  m.Name,
			Email: m.Email,
			Url:   m.URL,
		})
	}

	// Encode repository names to store them in the database.
	encodedAppName := url.PathEscape(appName)

	return &models.Chart{
		ID:            path.Join(r.Name, encodedAppName),
		Name:          encodedAppName,
		Repo:          &models.Repo{Namespace: r.Namespace, Name: r.Name, URL: r.URL, Type: r.Type},
		Description:   chartMetadata.Description,
		Home:          chartMetadata.Home,
		Keywords:      chartMetadata.Keywords,
		Maintainers:   maintainers,
		Sources:       chartMetadata.Sources,
		Icon:          chartMetadata.Icon,
		Category:      chartMetadata.Annotations["category"],
		ChartVersions: []models.ChartVersion{chartVersion},
	}, nil
}

func chartImportWorker(repoURL *url.URL, r *OCIRegistry, chartJobs <-chan pullChartJob, resultChan chan pullChartResult) {
	for j := range chartJobs {
		log.WithFields(log.Fields{"name": j.AppName, "tag": j.Tag}).Debug("pulling chart")
		chart, err := pullAndExtract(repoURL, j.AppName, j.Tag, r.puller, r)
		resultChan <- pullChartResult{chart, err}
	}
}

// FilterIndex remove non chart tags
func (r *OCIRegistry) FilterIndex() {
	unfilteredTags := r.tags
	r.tags = map[string]TagList{}
	checktagJobs := make(chan checkTagJob, numWorkers)
	tagcheckRes := make(chan checkTagResult, numWorkers)
	var wg sync.WaitGroup

	// Process 10 tags at a time
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			tagCheckerWorker(r.ociCli, checktagJobs, tagcheckRes)
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(tagcheckRes)
	}()

	go func() {
		for _, appName := range r.repositories {
			for _, tag := range unfilteredTags[appName].Tags {
				checktagJobs <- checkTagJob{AppName: appName, Tag: tag}
			}
		}
		close(checktagJobs)
	}()

	// Start receiving tags
	for res := range tagcheckRes {
		if res.Error == nil {
			if res.isHelmChart {
				r.tags[res.AppName] = TagList{
					Name: unfilteredTags[res.AppName].Name,
					Tags: append(r.tags[res.AppName].Tags, res.Tag),
				}
			}
		} else {
			log.Errorf("failed to pull chart. Got %v", res.Error)
		}
	}

	// Order tags by semver
	for _, appName := range r.repositories {
		vs := make([]*semver.Version, len(r.tags[appName].Tags))
		for i, r := range r.tags[appName].Tags {
			v, err := semver.NewVersion(r)
			if err != nil {
				log.Errorf("Error parsing version: %s", err)
			}
			vs[i] = v
		}
		sort.Sort(sort.Reverse(semver.Collection(vs)))
		orderedTags := []string{}
		for _, v := range vs {
			orderedTags = append(orderedTags, v.String())
		}
		r.tags[appName] = TagList{
			Name: r.tags[appName].Name,
			Tags: orderedTags,
		}
	}
}

// Charts retrieve the list of charts exposed in the repo
func (r *OCIRegistry) Charts(fetchLatestOnly bool) ([]models.Chart, error) {
	result := map[string]*models.Chart{}
	repoURL, err := parseRepoURL(r.RepoInternal.URL)
	if err != nil {
		return nil, err
	}

	chartJobs := make(chan pullChartJob, numWorkers)
	chartResults := make(chan pullChartResult, numWorkers)
	var wg sync.WaitGroup
	// Process 10 charts at a time
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			chartImportWorker(repoURL, r, chartJobs, chartResults)
			wg.Done()
		}()
	}
	// When we know all workers have sent their data in chartChan, close it.
	go func() {
		wg.Wait()
		close(chartResults)
	}()

	log.Debugf("starting %d workers", numWorkers)
	go func() {
		for _, appName := range r.repositories {
			if fetchLatestOnly {
				chartJobs <- pullChartJob{AppName: appName, Tag: r.tags[appName].Tags[0]}
			} else {
				for _, tag := range r.tags[appName].Tags {
					chartJobs <- pullChartJob{AppName: appName, Tag: tag}
				}
			}
		}
		close(chartJobs)
	}()

	// Start receiving charts
	for res := range chartResults {
		if res.Error == nil {
			ch := res.Chart
			log.Debugf("received chart %s from channel", ch.ID)
			if r, ok := result[ch.ID]; ok {
				// Chart already exists, append version
				r.ChartVersions = append(result[ch.ID].ChartVersions, ch.ChartVersions...)
			} else {
				result[ch.ID] = ch
			}
		} else {
			log.Errorf("failed to pull chart. Got %v", res.Error)
		}
	}

	charts := []models.Chart{}
	for _, c := range result {
		charts = append(charts, *c)
	}
	return filterCharts(charts, r.filter)
}

// FetchFiles do nothing for the OCI case since they have been already fetched in the Charts() method
func (r *OCIRegistry) FetchFiles(name string, cv models.ChartVersion) (map[string]string, error) {
	return map[string]string{
		models.ValuesKey: cv.Values,
		models.ReadmeKey: cv.Readme,
		models.SchemaKey: cv.Schema,
	}, nil
}

func parseFilters(filters string) (*apprepov1alpha1.FilterRuleSpec, error) {
	filterSpec := &apprepov1alpha1.FilterRuleSpec{}
	if len(filters) > 0 {
		err := json.Unmarshal([]byte(filters), filterSpec)
		if err != nil {
			return nil, err
		}
	}
	return filterSpec, nil
}

func getHelmRepo(namespace, name, repoURL, authorizationHeader string, filter *apprepov1alpha1.FilterRuleSpec, netClient httpclient.Client) (Repo, error) {
	url, err := parseRepoURL(repoURL)
	if err != nil {
		log.WithFields(log.Fields{"url": repoURL}).WithError(err).Error("failed to parse URL")
		return nil, err
	}

	repoBytes, err := fetchRepoIndex(url.String(), authorizationHeader, netClient)
	if err != nil {
		return nil, err
	}

	return &HelmRepo{
		content: repoBytes,
		RepoInternal: &models.RepoInternal{
			Namespace:           namespace,
			Name:                name,
			URL:                 url.String(),
			AuthorizationHeader: authorizationHeader,
		},
		netClient: netClient,
		filter:    filter,
	}, nil
}

func getOCIRepo(namespace, name, repoURL, authorizationHeader string, filter *apprepov1alpha1.FilterRuleSpec, ociRepos []string, netClient *http.Client) (Repo, error) {
	url, err := parseRepoURL(repoURL)
	if err != nil {
		log.WithFields(log.Fields{"url": repoURL}).WithError(err).Error("failed to parse URL")
		return nil, err
	}
	headers := http.Header{}
	if authorizationHeader != "" {
		headers["Authorization"] = []string{authorizationHeader}
	}
	ociResolver := docker.NewResolver(docker.ResolverOptions{Headers: headers, Client: netClient})

	return &OCIRegistry{
		repositories: ociRepos,
		RepoInternal: &models.RepoInternal{Namespace: namespace, Name: name, URL: url.String(), AuthorizationHeader: authorizationHeader},
		puller:       &helm.OCIPuller{Resolver: ociResolver},
		ociCli:       &ociAPICli{authHeader: authorizationHeader, url: url, netClient: netClient},
		filter:       filter,
	}, nil
}

func fetchRepoIndex(url, authHeader string, cli httpclient.Client) ([]byte, error) {
	indexURL, err := parseRepoURL(url)
	if err != nil {
		log.WithFields(log.Fields{"url": url}).WithError(err).Error("failed to parse URL")
		return nil, err
	}
	indexURL.Path = path.Join(indexURL.Path, "index.yaml")
	return doReq(indexURL.String(), cli, map[string]string{"Authorization": authHeader})
}

func chartTarballURL(r *models.RepoInternal, cv models.ChartVersion) string {
	source := cv.URLs[0]
	if _, err := parseRepoURL(source); err != nil {
		// If the chart URL is not absolute, join with repo URL. It's fine if the
		// URL we build here is invalid as we can catch this error when actually
		// making the request
		u, _ := url.Parse(r.URL)
		u.Path = path.Join(u.Path, source)
		return u.String()
	}
	return source
}

type fileImporter struct {
	manager   assetManager
	netClient httpclient.Client
}

func (f *fileImporter) fetchFiles(charts []models.Chart, repo Repo) {
	iconJobs := make(chan models.Chart, numWorkers)
	chartFilesJobs := make(chan importChartFilesJob, numWorkers)
	var wg sync.WaitGroup

	log.Debugf("starting %d workers", numWorkers)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go f.importWorker(&wg, iconJobs, chartFilesJobs, repo)
	}

	// Enqueue jobs to process chart icons
	for _, c := range charts {
		iconJobs <- c
	}
	// Close the iconJobs channel to signal the worker pools to move on to the
	// chart files jobs
	close(iconJobs)

	// Iterate through the list of charts and enqueue the latest chart version to
	// be processed. Append the rest of the chart versions to a list to be
	// enqueued later
	var toEnqueue []importChartFilesJob
	for _, c := range charts {
		chartFilesJobs <- importChartFilesJob{c.Name, c.Repo, c.ChartVersions[0]}
		for _, cv := range c.ChartVersions[1:] {
			toEnqueue = append(toEnqueue, importChartFilesJob{c.Name, c.Repo, cv})
		}
	}

	// Enqueue all the remaining chart versions
	for _, cfj := range toEnqueue {
		chartFilesJobs <- cfj
	}
	// Close the chartFilesJobs channel to signal the worker pools that there are
	// no more jobs to process
	close(chartFilesJobs)

	// Wait for the worker pools to finish processing
	wg.Wait()
}

func (f *fileImporter) importWorker(wg *sync.WaitGroup, icons <-chan models.Chart, chartFiles <-chan importChartFilesJob, repo Repo) {
	defer wg.Done()
	for c := range icons {
		log.WithFields(log.Fields{"name": c.Name}).Debug("importing icon")
		if err := f.fetchAndImportIcon(c, repo.Repo()); err != nil {
			log.WithFields(log.Fields{"name": c.Name}).WithError(err).Error("failed to import icon")
		}
	}
	for j := range chartFiles {
		log.WithFields(log.Fields{"name": j.Name, "version": j.ChartVersion.Version}).Debug("importing readme and values")
		if err := f.fetchAndImportFiles(j.Name, repo, j.ChartVersion); err != nil {
			log.WithFields(log.Fields{"name": j.Name, "version": j.ChartVersion.Version}).WithError(err).Error("failed to import files")
		}
	}
}

func (f *fileImporter) fetchAndImportIcon(c models.Chart, r *models.RepoInternal) error {
	if c.Icon == "" {
		log.WithFields(log.Fields{"name": c.Name}).Info("icon not found")
		return nil
	}

	reqHeaders := make(map[string]string)
	reqHeaders["User-Agent"] = userAgent()
	if passCredentials || len(r.AuthorizationHeader) > 0 && isURLDomainEqual(c.Icon, r.URL) {
		reqHeaders["Authorization"] = r.AuthorizationHeader
	}

	reader, contentType, err := httpclient.GetStream(c.Icon, f.netClient, reqHeaders)
	if reader != nil {
		defer reader.Close()
	}
	if err != nil {
		return err
	}

	var img image.Image
	// if the icon is in any other format try to convert it to PNG
	if strings.Contains(contentType, "image/svg") {
		// if the icon is an SVG, it requires special processing
		icon, err := oksvg.ReadIconStream(reader)
		if err != nil {
			log.WithFields(log.Fields{"name": c.Name}).WithError(err).Error("failed to decode icon")
			return err
		}
		w, h := int(icon.ViewBox.W), int(icon.ViewBox.H)
		icon.SetTarget(0, 0, float64(w), float64(h))
		rgba := image.NewNRGBA(image.Rect(0, 0, w, h))
		icon.Draw(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())), 1)
		img = rgba
	} else {
		img, err = imaging.Decode(reader)
		if err != nil {
			log.WithFields(log.Fields{"name": c.Name}).WithError(err).Error("failed to decode icon")
			return err
		}
	}

	// TODO: make this configurable?
	resizedImg := imaging.Fit(img, 160, 160, imaging.Lanczos)
	var buf bytes.Buffer
	err = imaging.Encode(&buf, resizedImg, imaging.PNG)
	if err != nil {
		log.WithFields(log.Fields{"name": c.Name}).WithError(err).Error("failed to encode icon")
		return err
	}
	b := buf.Bytes()
	contentType = "image/png"

	return f.manager.updateIcon(models.Repo{Namespace: r.Namespace, Name: r.Name}, b, contentType, c.ID)
}

func (f *fileImporter) fetchAndImportFiles(name string, repo Repo, cv models.ChartVersion) error {
	r := repo.Repo()
	chartID := fmt.Sprintf("%s/%s", r.Name, name)
	chartFilesID := fmt.Sprintf("%s-%s", chartID, cv.Version)

	// Check if we already have indexed files for this chart version and digest
	if f.manager.filesExist(models.Repo{Namespace: r.Namespace, Name: r.Name}, chartFilesID, cv.Digest) {
		log.WithFields(log.Fields{"name": name, "version": cv.Version}).Debug("skipping existing files")
		return nil
	}
	log.WithFields(log.Fields{"name": name, "version": cv.Version}).Debug("fetching files")

	files, err := repo.FetchFiles(name, cv)
	if err != nil {
		return err
	}

	chartFiles := models.ChartFiles{ID: chartFilesID, Repo: &models.Repo{Name: r.Name, Namespace: r.Namespace, URL: r.URL}, Digest: cv.Digest}
	if v, ok := files[models.ReadmeKey]; ok {
		chartFiles.Readme = v
	} else {
		log.WithFields(log.Fields{"name": name, "version": cv.Version}).Info("README.md not found")
	}
	if v, ok := files[models.ValuesKey]; ok {
		chartFiles.Values = v
	} else {
		log.WithFields(log.Fields{"name": name, "version": cv.Version}).Info("values.yaml not found")
	}
	if v, ok := files[models.SchemaKey]; ok {
		chartFiles.Schema = v
	} else {
		log.WithFields(log.Fields{"name": name, "version": cv.Version}).Info("values.schema.json not found")
	}

	// inserts the chart files if not already indexed, or updates the existing
	// entry if digest has changed
	return f.manager.insertFiles(chartID, chartFiles)
}

// Check if two URL strings are in the same domain.
// Return true if so, and false otherwise or when an error occurs
func isURLDomainEqual(url1Str, url2Str string) bool {
	url1, err := url.ParseRequestURI(url1Str)
	if err != nil {
		return false
	}
	url2, err := url.ParseRequestURI(url2Str)
	if err != nil {
		return false
	}

	return url1.Scheme == url2.Scheme && url1.Host == url2.Host
}
