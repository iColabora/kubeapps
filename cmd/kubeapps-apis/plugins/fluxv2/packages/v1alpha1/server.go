/*
Copyright © 2021 VMware
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
	"context"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/kube"
	apiext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	corev1 "github.com/kubeapps/kubeapps/cmd/kubeapps-apis/gen/core/packages/v1alpha1"
	"github.com/kubeapps/kubeapps/cmd/kubeapps-apis/gen/plugins/fluxv2/packages/v1alpha1"
	"github.com/kubeapps/kubeapps/cmd/kubeapps-apis/server"
	"github.com/kubeapps/kubeapps/pkg/agent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	log "k8s.io/klog/v2"
)

// Compile-time statement to ensure this service implementation satisfies the core packaging API
var _ corev1.PackagesServiceServer = (*Server)(nil)

type clientGetter func(context.Context) (dynamic.Interface, apiext.Interface, error)
type helmActionConfigGetter func(ctx context.Context, namespace string) (*action.Configuration, error)

// Server implements the fluxv2 packages v1alpha1 interface.
type Server struct {
	v1alpha1.UnimplementedFluxV2PackagesServiceServer

	// clientGetter is a field so that it can be switched in tests for
	// a fake client. NewServer() below sets this automatically with the
	// non-test implementation.
	clientGetter       clientGetter
	actionConfigGetter helmActionConfigGetter

	cache *NamespacedResourceWatcherCache
}

// NewServer returns a Server automatically configured with a function to obtain
// the k8s client config.
func NewServer(configGetter server.KubernetesConfigGetter) (*Server, error) {
	clientGetter := func(ctx context.Context) (dynamic.Interface, apiext.Interface, error) {
		if configGetter == nil {
			return nil, nil, status.Errorf(codes.Internal, "configGetter arg required")
		}
		// The Flux plugin currently supports interactions with the default (kubeapps)
		// cluster only:
		cluster := ""
		config, err := configGetter(ctx, cluster)
		if err != nil {
			return nil, nil, status.Errorf(codes.FailedPrecondition, fmt.Sprintf("unable to get config : %v", err))
		}
		dynamicClient, err := dynamic.NewForConfig(config)
		if err != nil {
			return nil, nil, status.Errorf(codes.FailedPrecondition, fmt.Sprintf("unable to get dynamic client : %v", err))
		}
		apiExtensions, err := apiext.NewForConfig(config)
		if err != nil {
			return nil, nil, status.Errorf(codes.FailedPrecondition, fmt.Sprintf("unable to get api extensions client : %v", err))
		}
		return dynamicClient, apiExtensions, nil
	}
	actionConfigGetter := func(ctx context.Context, namespace string) (*action.Configuration, error) {
		if configGetter == nil {
			return nil, status.Errorf(codes.Internal, "configGetter arg required")
		}
		// The Flux plugin currently supports interactions with the default (kubeapps)
		// cluster only:
		cluster := ""
		config, err := configGetter(ctx, cluster)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, fmt.Sprintf("unable to get config : %v", err))
		}

		restClientGetter := agent.NewConfigFlagsFromCluster(namespace, config)
		clientSet, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, fmt.Sprintf("unable to create kubernetes client : %v", err))
		}
		// TODO(mnelson): Update to allow different helm storage options.
		storage := agent.StorageForSecrets(namespace, clientSet)
		return &action.Configuration{
			RESTClientGetter: restClientGetter,
			KubeClient:       kube.New(restClientGetter),
			Releases:         storage,
			Log:              log.Infof,
		}, nil
	}
	repositoriesGvr := schema.GroupVersionResource{
		Group:    fluxGroup,
		Version:  fluxVersion,
		Resource: fluxHelmRepositories,
	}
	cacheConfig := cacheConfig{
		gvr:          repositoriesGvr,
		clientGetter: clientGetter,
		onAdd:        onAddOrModifyRepo,
		onModify:     onAddOrModifyRepo,
		onGet:        onGetRepo,
		onDelete:     onDeleteRepo,
	}
	cache, err := newCache(cacheConfig)
	if err != nil {
		return nil, err
	}
	return &Server{
		clientGetter:       clientGetter,
		actionConfigGetter: actionConfigGetter,
		cache:              cache,
	}, nil
}

// getDynamicClient returns a dynamic k8s client.
func (s *Server) getDynamicClient(ctx context.Context) (dynamic.Interface, error) {
	if s.clientGetter == nil {
		return nil, status.Errorf(codes.Internal, "server not configured with configGetter")
	}
	dynamicClient, _, err := s.clientGetter(ctx)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "unable to get client due to: %v", err)
	}
	return dynamicClient, nil
}

// ===== general note on error handling ========
// using fmt.Errorf vs status.Errorf in functions exposed as grpc:
//
// grpc itself will transform any error into a grpc status code (which is
// then translated into an http status via grpc gateway), so we'll need to
// be using status.Errorf(...) here, rather than fmt.Errorf(...), the former
// allowing you to specify a status code with the error which can be used
// for grpc and translated or http. Without doing this, the grpc status will
// be codes.Unknown which is translated to a 500. you might have a helper
// function that returns an error, then your actual handler function handles
// that error by returning a status.Errorf with the appropriate code

// GetPackageRepositories returns the package repositories based on the request.
// note that this func currently returns ALL repositories, not just those in 'ready' (reconciled) state
func (s *Server) GetPackageRepositories(ctx context.Context, request *v1alpha1.GetPackageRepositoriesRequest) (*v1alpha1.GetPackageRepositoriesResponse, error) {
	log.Infof("+fluxv2 GetPackageRepositories(request: [%v])", request)

	if request == nil || request.Context == nil {
		return nil, status.Errorf(codes.InvalidArgument, "no context provided")
	}

	if request.Context.Cluster != "" {
		return nil, status.Errorf(
			codes.Unimplemented,
			"not supported yet: request.Context.Cluster: [%v]",
			request.Context.Cluster)
	}

	repos, err := s.listReposInCluster(ctx, request.Context.Namespace)
	if err != nil {
		return nil, err
	}

	responseRepos := []*v1alpha1.PackageRepository{}
	for _, repoUnstructured := range repos.Items {
		repo, err := newPackageRepository(repoUnstructured.Object)
		if err != nil {
			return nil, err
		}
		responseRepos = append(responseRepos, repo)
	}
	return &v1alpha1.GetPackageRepositoriesResponse{
		Repositories: responseRepos,
	}, nil
}

// GetAvailablePackageSummaries returns the available packages based on the request.
// Note that currently packages are returned only from repos that are in a 'Ready'
// state. For the fluxv2 plugin, the request context namespace (the target
// namespace) is not relevant since charts from a repository in any namespace
//  accessible to the user are available to be installed in the target namespace.
func (s *Server) GetAvailablePackageSummaries(ctx context.Context, request *corev1.GetAvailablePackageSummariesRequest) (*corev1.GetAvailablePackageSummariesResponse, error) {
	log.Infof("+fluxv2 GetAvailablePackageSummaries(request: [%v])", request)

	// grpc compiles in getters for you which automatically return a default (empty) struct if the pointer was nil
	if request != nil && request.GetContext().GetCluster() != "" {
		return nil, status.Errorf(
			codes.Unimplemented,
			"not supported yet: request.Context.Cluster: [%v]",
			request.Context.Cluster)
	}

	pageSize := request.GetPaginationOptions().GetPageSize()
	pageOffset, err := pageOffsetFromPageToken(request.GetPaginationOptions().GetPageToken())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"unable to intepret page token %q: %v",
			request.GetPaginationOptions().GetPageToken(), err)
	}

	if s.cache == nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"server cache has not been properly initialized")
	}

	repos, err := s.cache.listKeys(request.GetFilterOptions().GetRepositories())
	if err != nil {
		return nil, err
	}

	cachedCharts, err := s.cache.fetchForMultiple(repos)
	if err != nil {
		return nil, err
	}

	packageSummaries, err := filterAndPaginateCharts(request.GetFilterOptions(), pageSize, pageOffset, cachedCharts)
	if err != nil {
		return nil, err
	}

	// Only return a next page token if the request was for pagination and
	// the results are a full page.
	nextPageToken := ""
	if pageSize > 0 && len(packageSummaries) == int(pageSize) {
		nextPageToken = fmt.Sprintf("%d", pageOffset+1)
	}
	return &corev1.GetAvailablePackageSummariesResponse{
		AvailablePackageSummaries: packageSummaries,
		NextPageToken:             nextPageToken,
		// TODO (gfichtenholt) Categories?
	}, nil
}

// GetAvailablePackageDetail returns the package metadata managed by the 'fluxv2' plugin
func (s *Server) GetAvailablePackageDetail(ctx context.Context, request *corev1.GetAvailablePackageDetailRequest) (*corev1.GetAvailablePackageDetailResponse, error) {
	log.Infof("+fluxv2 GetAvailablePackageDetail(request: [%v])", request)

	if request == nil || request.AvailablePackageRef == nil {
		return nil, status.Errorf(codes.InvalidArgument, "no request AvailablePackageRef provided")
	}

	packageRef := request.AvailablePackageRef
	// flux CRDs require a namespace, cluster-wide resources are not supported
	if packageRef.Context == nil || len(packageRef.Context.Namespace) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "AvailablePackageReference is missing required 'namespace' field")
	}

	unescapedChartID, err := getUnescapedChartID(packageRef.Identifier)
	if err != nil {
		return nil, err
	}
	packageIdParts := strings.Split(unescapedChartID, "/")

	// check if the repo has been indexed, stored in the cache and requested
	// package is part of it. Otherwise, there is a time window when this scenario can happen:
	// - GetAvailablePackageSummaries() may return {} while a ready repo is being indexed
	//   and said index is cached BUT
	// - GetAvailablePackageDetail() may return full package detail for one of the packages
	// in the repo
	name := types.NamespacedName{Namespace: packageRef.Context.Namespace, Name: packageIdParts[0]}
	ok, err := s.repoExistsInCache(name)
	if err != nil {
		return nil, err
	} else if !ok {
		return nil, status.Errorf(codes.NotFound, "no fully indexed repository [%s] has been found", name)
	}

	tarUrl, err, cleanUp := s.getChartTarball(ctx, packageIdParts[0], packageIdParts[1], packageRef.Context.Namespace, request.PkgVersion)
	if cleanUp != nil {
		defer cleanUp()
	}
	if err != nil {
		return nil, err
	}
	log.Infof("Found chart url: [%s] for chart [%s]", tarUrl, packageRef.Identifier)

	pkgDetail, err := availablePackageDetailFromTarball(packageRef.Identifier, tarUrl)
	if err != nil {
		return nil, err
	}

	// fix up a couple of fields that don't come from the chart tarball
	repoUrl, err := s.getRepoUrl(ctx, name)
	if err != nil {
		return nil, err
	}
	pkgDetail.RepoUrl = repoUrl
	pkgDetail.AvailablePackageRef.Context.Namespace = packageRef.Context.Namespace

	return &corev1.GetAvailablePackageDetailResponse{
		AvailablePackageDetail: pkgDetail,
	}, nil
}

// GetAvailablePackageVersions returns the package versions managed by the 'fluxv2' plugin
func (s *Server) GetAvailablePackageVersions(ctx context.Context, request *corev1.GetAvailablePackageVersionsRequest) (*corev1.GetAvailablePackageVersionsResponse, error) {
	log.Infof("+fluxv2 GetAvailablePackageVersions [%v]", request)

	if request.GetPkgVersion() != "" {
		return nil, status.Errorf(
			codes.Unimplemented,
			"not supported yet: request.GetPkgVersion(): [%v]",
			request.GetPkgVersion())
	}

	packageRef := request.GetAvailablePackageRef()
	namespace := packageRef.GetContext().GetNamespace()
	if namespace == "" || packageRef.GetIdentifier() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "required context or identifier not provided")
	}

	unescapedChartID, err := getUnescapedChartID(packageRef.Identifier)
	if err != nil {
		return nil, err
	}

	log.Infof("Requesting chart [%s] (latest version) in ns [%s]", unescapedChartID, namespace)
	packageIdParts := strings.Split(unescapedChartID, "/")
	repo := types.NamespacedName{Namespace: namespace, Name: packageIdParts[0]}
	chart, err := s.fetchChartFromCache(repo, packageIdParts[1])
	if err != nil {
		return nil, err
	} else if chart != nil {
		// found it
		return &corev1.GetAvailablePackageVersionsResponse{
			PackageAppVersions: packageAppVersionsSummary(chart.ChartVersions),
		}, nil
	} else {
		return nil, status.Errorf(codes.Internal, "unable to retrieve versions for chart: [%s]", packageRef.Identifier)
	}
}

// GetInstalledPackageSummaries returns the installed packages managed by the 'fluxv2' plugin
func (s *Server) GetInstalledPackageSummaries(ctx context.Context, request *corev1.GetInstalledPackageSummariesRequest) (*corev1.GetInstalledPackageSummariesResponse, error) {
	log.Infof("+fluxv2 GetInstalledPackageSummaries [%v]", request)
	pageSize := request.GetPaginationOptions().GetPageSize()
	pageOffset, err := pageOffsetFromPageToken(request.GetPaginationOptions().GetPageToken())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"unable to intepret page token %q: %v",
			request.GetPaginationOptions().GetPageToken(), err)
	}

	installedPkgSummaries, err := s.paginatedInstalledPkgSummaries(ctx, request.GetContext().GetNamespace(), pageSize, pageOffset)
	if err != nil {
		return nil, err
	}

	// Only return a next page token if the request was for pagination and
	// the results are a full page.
	nextPageToken := ""
	if pageSize > 0 && len(installedPkgSummaries) == int(pageSize) {
		nextPageToken = fmt.Sprintf("%d", pageOffset+1)
	}

	response := &corev1.GetInstalledPackageSummariesResponse{
		InstalledPackageSummaries: installedPkgSummaries,
		NextPageToken:             nextPageToken,
	}
	return response, nil
}

// GetInstalledPackageDetail returns the package metadata managed by the 'fluxv2' plugin
func (s *Server) GetInstalledPackageDetail(ctx context.Context, request *corev1.GetInstalledPackageDetailRequest) (*corev1.GetInstalledPackageDetailResponse, error) {
	log.Infof("+fluxv2 GetInstalledPackageDetail [%v]", request)

	if request == nil || request.InstalledPackageRef == nil {
		return nil, status.Errorf(codes.InvalidArgument, "no request InstalledPackageRef provided")
	}

	packageRef := request.InstalledPackageRef
	// flux CRDs require a namespace, cluster-wide resources are not supported
	if packageRef.Context == nil || len(packageRef.Context.Namespace) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "InstalledPackageReference is missing required 'namespace' field")
	}

	name := types.NamespacedName{Namespace: packageRef.Context.Namespace, Name: packageRef.Identifier}
	pkgDetail, err := s.installedPackageDetail(ctx, name)
	if err != nil {
		return nil, err
	}

	return &corev1.GetInstalledPackageDetailResponse{
		InstalledPackageDetail: pkgDetail,
	}, nil
}

// CreateInstalledPackage creates an installed package based on the request.
func (s *Server) CreateInstalledPackage(ctx context.Context, request *corev1.CreateInstalledPackageRequest) (*corev1.CreateInstalledPackageResponse, error) {
	log.Infof("+fluxv2 CreateInstalledPackage [%v]", request)

	if request == nil || request.AvailablePackageRef == nil {
		return nil, status.Errorf(codes.InvalidArgument, "no request AvailablePackageRef provided")
	}
	if request.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "no request Name provided")
	}
	if request.TargetContext == nil || request.TargetContext.Namespace == "" {
		return nil, status.Errorf(codes.InvalidArgument, "no request TargetContext namespace provided")
	}
	if request.TargetContext.Cluster != "" {
		return nil, status.Errorf(
			codes.Unimplemented,
			"not supported yet: request.TargetContext.Cluster: [%v]",
			request.TargetContext.Cluster)
	}

	targetName := types.NamespacedName{
		Name:      request.Name,
		Namespace: request.TargetContext.Namespace,
	}

	installedRef, err := s.newRelease(ctx, request.AvailablePackageRef, targetName)
	if err != nil {
		return nil, err
	}

	return &corev1.CreateInstalledPackageResponse{
		InstalledPackageRef: installedRef,
	}, nil
}
