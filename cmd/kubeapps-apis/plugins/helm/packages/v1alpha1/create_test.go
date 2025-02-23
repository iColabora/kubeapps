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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kubeapps/kubeapps/cmd/apprepository-controller/pkg/apis/apprepository/v1alpha1"
	corev1 "github.com/kubeapps/kubeapps/cmd/kubeapps-apis/gen/core/packages/v1alpha1"
	plugins "github.com/kubeapps/kubeapps/cmd/kubeapps-apis/gen/core/plugins/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateInstalledPackage(t *testing.T) {
	testCases := []struct {
		name               string
		request            *corev1.CreateInstalledPackageRequest
		expectedResponse   *corev1.CreateInstalledPackageResponse
		expectedStatusCode codes.Code
		expectedRelease    *release.Release
	}{
		{
			name: "creates the installed package from repo without credentials",
			request: &corev1.CreateInstalledPackageRequest{
				AvailablePackageRef: &corev1.AvailablePackageReference{
					Context: &corev1.Context{
						Namespace: globalPackagingNamespace,
					},
					Identifier: "bitnami/apache",
				},
				TargetContext: &corev1.Context{
					Namespace: "default",
				},
				Name: "my-apache",
				PkgVersionReference: &corev1.VersionReference{
					Version: "1.18.3",
				},
				Values: "{\"foo\": \"bar\"}",
			},
			expectedResponse: &corev1.CreateInstalledPackageResponse{
				InstalledPackageRef: &corev1.InstalledPackageReference{
					Context: &corev1.Context{
						Cluster:   "default",
						Namespace: "default",
					},
					Identifier: "my-apache",
					Plugin:     GetPluginDetail(),
				},
			},
			expectedStatusCode: codes.OK,
			expectedRelease: &release.Release{
				Name: "my-apache",
				Info: &release.Info{
					Description: "Install complete",
					Status:      release.StatusDeployed,
				},
				Chart: &chart.Chart{
					Metadata: &chart.Metadata{Name: "apache"},
					Values:   map[string]interface{}{},
				},
				Config:    map[string]interface{}{"foo": "bar"},
				Version:   1,
				Namespace: "default",
			},
		},
		{
			name: "returns invalid if available package ref invalid",
			request: &corev1.CreateInstalledPackageRequest{
				AvailablePackageRef: &corev1.AvailablePackageReference{
					Context: &corev1.Context{
						Namespace: globalPackagingNamespace,
					},
					Identifier: "not-a-valid-identifier",
				},
			},
			expectedStatusCode: codes.InvalidArgument,
		},
	}

	ignoredUnexported := cmpopts.IgnoreUnexported(
		corev1.CreateInstalledPackageResponse{},
		corev1.InstalledPackageReference{},
		corev1.Context{},
		plugins.Plugin{},
		chart.Chart{},
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authorized := true
			actionConfig := newActionConfigFixture(t, tc.request.GetTargetContext().GetNamespace(), nil)
			server, _, cleanup := makeServer(t, authorized, actionConfig, &v1alpha1.AppRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bitnami",
					Namespace: globalPackagingNamespace,
				},
			})
			defer cleanup()

			response, err := server.CreateInstalledPackage(context.Background(), tc.request)

			if got, want := status.Code(err), tc.expectedStatusCode; got != want {
				t.Fatalf("got: %+v, want: %+v, err: %+v", got, want, err)
			}

			// Verify the expected response (our contract to the caller).
			if got, want := response, tc.expectedResponse; !cmp.Equal(got, want, ignoredUnexported) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(want, got, ignoredUnexported))
			}

			if tc.expectedRelease != nil {
				// Verify the expected request was made to Helm (our contract to the helm lib).
				releases, err := actionConfig.Releases.Driver.List(func(*release.Release) bool { return true })
				if err != nil {
					t.Fatalf("%+v", err)
				}
				if got, want := len(releases), 1; got != want {
					t.Fatalf("got: %d, want: %d", got, want)
				}

				ignoredFields := cmpopts.IgnoreFields(release.Info{}, "FirstDeployed", "LastDeployed")
				if got, want := releases[0], tc.expectedRelease; !cmp.Equal(got, want, ignoredUnexported, ignoredFields) {
					t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(want, got, ignoredUnexported, ignoredFields))
				}
			}
		})
	}
}
