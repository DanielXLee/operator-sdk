// Copyright 2018 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	apimanifests "github.com/operator-framework/api/pkg/manifests"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/operator-sdk/internal/olm/operator"
	"github.com/operator-framework/operator-sdk/internal/olm/operator/packagemanifests"
	"github.com/operator-framework/operator-sdk/internal/util/k8sutil"
)

const (
	defaultTimeout = 2 * time.Minute

	defaultOperatorName    = "memcached-operator"
	defaultOperatorVersion = "0.0.2"
)

var (
	kubeconfigPath = os.Getenv(k8sutil.KubeConfigEnvVar)
)

// TODO(estroz): rewrite these in the style of e2e tests (ginkgo/gomega + scaffold a project for each scenario).

func TestOLMIntegration(t *testing.T) {
	if image, ok := os.LookupEnv(imageEnvVar); ok && image != "" {
		testImageTag = image
	}

	t.Run("PackageManifestsBasic", PackageManifestsBasic)
	t.Run("PackageManifestsOwnNamespace", PackageManifestsOwnNamespace)
	t.Run("PackageManifestsMultiplePackages", PackageManifestsMultiplePackages)
}

func PackageManifestsOwnNamespace(t *testing.T) {

	csvConfig := CSVTemplateConfig{
		OperatorName:    defaultOperatorName,
		Version:         defaultOperatorVersion,
		TestImageTag:    testImageTag,
		ReplacesCSVName: "",
		CRDKeys: []DefinitionKey{
			{
				Kind:  "Memcached",
				Name:  "memcacheds.cache.example.com",
				Group: "cache.example.com",
				Versions: []apiextv1beta1.CustomResourceDefinitionVersion{
					{Name: "v1alpha1", Storage: true, Served: true},
				},
			},
		},
		InstallModes: []operatorsv1alpha1.InstallMode{
			{Type: operatorsv1alpha1.InstallModeTypeOwnNamespace, Supported: true},
			{Type: operatorsv1alpha1.InstallModeTypeSingleNamespace, Supported: false},
			{Type: operatorsv1alpha1.InstallModeTypeMultiNamespace, Supported: false},
			{Type: operatorsv1alpha1.InstallModeTypeAllNamespaces, Supported: false},
		},
	}
	tmp, cleanup := mkTempDirWithCleanup(t, "")
	defer cleanup()

	channels := []apimanifests.PackageChannel{
		{Name: "alpha", CurrentCSVName: fmt.Sprintf("%s.v%s", defaultOperatorName, defaultOperatorVersion)},
	}
	manifestsDir := filepath.Join(tmp, defaultOperatorName)
	err := writeOperatorManifests(manifestsDir, csvConfig)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}
	err = writePackageManifest(manifestsDir, defaultOperatorName, channels)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}

	cfg := &operator.Configuration{KubeconfigPath: kubeconfigPath}
	assert.NoError(t, cfg.Load())
	i := packagemanifests.NewInstall(cfg)
	i.PackageManifestsDirectory = manifestsDir
	i.Version = defaultOperatorVersion
	i.InstallMode = operator.InstallMode{
		InstallModeType:  operatorsv1alpha1.InstallModeTypeOwnNamespace,
		TargetNamespaces: []string{"default"},
	}

	// Cleanup.
	defer func() {
		if err := doUninstall(t, kubeconfigPath); err != nil {
			t.Fatal(err)
		}
	}()

	// Deploy operator.
	assert.NoError(t, doInstall(i))
}

func PackageManifestsBasic(t *testing.T) {

	csvConfig := CSVTemplateConfig{
		OperatorName:    defaultOperatorName,
		Version:         defaultOperatorVersion,
		TestImageTag:    testImageTag,
		ReplacesCSVName: "",
		CRDKeys: []DefinitionKey{
			{
				Kind:  "Memcached",
				Name:  "memcacheds.cache.example.com",
				Group: "cache.example.com",
				Versions: []apiextv1beta1.CustomResourceDefinitionVersion{
					{Name: "v1alpha1", Storage: true, Served: true},
				},
			},
		},
		InstallModes: []operatorsv1alpha1.InstallMode{
			{Type: operatorsv1alpha1.InstallModeTypeOwnNamespace, Supported: false},
			{Type: operatorsv1alpha1.InstallModeTypeSingleNamespace, Supported: false},
			{Type: operatorsv1alpha1.InstallModeTypeMultiNamespace, Supported: false},
			{Type: operatorsv1alpha1.InstallModeTypeAllNamespaces, Supported: true},
		},
	}
	tmp, cleanup := mkTempDirWithCleanup(t, "")
	defer cleanup()

	channels := []apimanifests.PackageChannel{
		{Name: "alpha", CurrentCSVName: fmt.Sprintf("%s.v%s", defaultOperatorName, defaultOperatorVersion)},
	}
	manifestsDir := filepath.Join(tmp, defaultOperatorName)
	err := writeOperatorManifests(manifestsDir, csvConfig)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}
	err = writePackageManifest(manifestsDir, defaultOperatorName, channels)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}
	cfg := &operator.Configuration{KubeconfigPath: kubeconfigPath}
	assert.NoError(t, cfg.Load())
	i := packagemanifests.NewInstall(cfg)
	i.PackageManifestsDirectory = manifestsDir
	i.Version = defaultOperatorVersion

	// Remove operator before deploy
	assert.Error(t, doUninstall(t, kubeconfigPath))

	// Deploy operator
	assert.NoError(t, doInstall(i))
	// Fail to deploy operator after deploy
	assert.Error(t, doInstall(i))

	// Remove operator after deploy
	assert.NoError(t, doUninstall(t, kubeconfigPath))
	// Remove operator after removal
	assert.Error(t, doUninstall(t, kubeconfigPath))
}

func PackageManifestsMultiplePackages(t *testing.T) {

	operatorVersion1 := defaultOperatorVersion
	operatorVersion2 := "0.0.3"
	csvConfigs := []CSVTemplateConfig{
		{
			OperatorName:    defaultOperatorName,
			Version:         operatorVersion1,
			TestImageTag:    testImageTag,
			ReplacesCSVName: "",
			CRDKeys: []DefinitionKey{
				{
					Kind:  "Memcached",
					Name:  "memcacheds.cache.example.com",
					Group: "cache.example.com",
					Versions: []apiextv1beta1.CustomResourceDefinitionVersion{
						{Name: "v1alpha1", Storage: true, Served: true},
					},
				},
			},
			InstallModes: []operatorsv1alpha1.InstallMode{
				{Type: operatorsv1alpha1.InstallModeTypeOwnNamespace, Supported: false},
				{Type: operatorsv1alpha1.InstallModeTypeSingleNamespace, Supported: false},
				{Type: operatorsv1alpha1.InstallModeTypeMultiNamespace, Supported: false},
				{Type: operatorsv1alpha1.InstallModeTypeAllNamespaces, Supported: true},
			},
		},
		{
			OperatorName:    defaultOperatorName,
			Version:         operatorVersion2,
			TestImageTag:    testImageTag,
			ReplacesCSVName: fmt.Sprintf("%s.v%s", defaultOperatorName, operatorVersion1),
			CRDKeys: []DefinitionKey{
				{
					Kind:  "Memcached",
					Name:  "memcacheds.cache.example.com",
					Group: "cache.example.com",
					Versions: []apiextv1beta1.CustomResourceDefinitionVersion{
						{Name: "v1alpha1", Storage: false, Served: true},
						{Name: "v1alpha2", Storage: true, Served: true},
					},
				},
			},
			InstallModes: []operatorsv1alpha1.InstallMode{
				{Type: operatorsv1alpha1.InstallModeTypeOwnNamespace, Supported: false},
				{Type: operatorsv1alpha1.InstallModeTypeSingleNamespace, Supported: false},
				{Type: operatorsv1alpha1.InstallModeTypeMultiNamespace, Supported: false},
				{Type: operatorsv1alpha1.InstallModeTypeAllNamespaces, Supported: true},
			},
		},
	}

	tmp, cleanup := mkTempDirWithCleanup(t, "")
	defer cleanup()

	channels := []apimanifests.PackageChannel{
		{Name: "stable", CurrentCSVName: fmt.Sprintf("%s.v%s", defaultOperatorName, operatorVersion2)},
		{Name: "alpha", CurrentCSVName: fmt.Sprintf("%s.v%s", defaultOperatorName, operatorVersion1)},
	}
	manifestsDir := filepath.Join(tmp, defaultOperatorName)
	for _, config := range csvConfigs {
		err := writeOperatorManifests(manifestsDir, config)
		if err != nil {
			os.RemoveAll(tmp)
			t.Fatal(err)
		}
	}
	err := writePackageManifest(manifestsDir, defaultOperatorName, channels)
	if err != nil {
		os.RemoveAll(tmp)
		t.Fatal(err)
	}
	cfg := &operator.Configuration{KubeconfigPath: kubeconfigPath}
	assert.NoError(t, cfg.Load())
	i := packagemanifests.NewInstall(cfg)
	i.PackageManifestsDirectory = manifestsDir
	i.Version = operatorVersion2

	// Deploy operator
	assert.NoError(t, doInstall(i))
	// Remove operator after deploy
	assert.NoError(t, doUninstall(t, kubeconfigPath))
}

func doUninstall(t *testing.T, kubeconfigPath string) error {
	cfg := &operator.Configuration{KubeconfigPath: kubeconfigPath}
	assert.NoError(t, cfg.Load())
	uninstall := operator.NewUninstall(cfg)
	uninstall.DeleteAll = true
	uninstall.DeleteOperatorGroupNames = []string{operator.SDKOperatorGroupName}
	uninstall.Package = defaultOperatorName
	uninstall.Logf = logrus.Infof

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := uninstall.Run(ctx); err != nil {
		return err
	}
	return waitForPackageManifestConfigMapDeletion(ctx, cfg, defaultOperatorName)
}

type installer interface {
	Run(context.Context) (*operatorsv1alpha1.ClusterServiceVersion, error)
}

func doInstall(i installer) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := i.Run(ctx)
	return err
}

func waitForPackageManifestConfigMapDeletion(ctx context.Context, cfg *operator.Configuration, packageName string) error {
	cfgmaps := corev1.ConfigMapList{}
	opts := []client.ListOption{
		client.InNamespace(cfg.Namespace),
		client.MatchingLabels{"owner": "operator-sdk", "package-name": packageName},
	}
	return wait.PollImmediateUntil(250*time.Millisecond, func() (bool, error) {
		if err := cfg.Client.List(ctx, &cfgmaps, opts...); err != nil {
			return false, err
		}
		return len(cfgmaps.Items) == 0, nil
	}, ctx.Done())
}
