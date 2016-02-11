/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package pwx

import (
	"os"
	"testing"

	pwxclient "github.com/libopenstorage/openstorage/api/client"
	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
	utiltesting "k8s.io/kubernetes/pkg/util/testing"
	"k8s.io/kubernetes/pkg/volume"
)

const pluginName = "kubernetes.io/pwx"

func newInitializedVolumePlugMgr(t *testing.T) (volume.VolumePluginMgr, string) {
	plugMgr := volume.VolumePluginMgr{}
	dir, err := utiltesting.MkTmpdir("pwx")
	assert.NoError(t, err)
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost(dir, nil, nil))
	return plugMgr, dir
}

func TestGetByName(t *testing.T) {
	assert := assert.New(t)
	plugMgr, _ := newInitializedVolumePlugMgr(t)

	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.NotNil(plug, "Can't find the plugin by name")
	assert.NoError(err)
}

func TestCanSupport(t *testing.T) {
	assert := assert.New(t)
	plugMgr, _ := newInitializedVolumePlugMgr(t)

	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.NoError(err)

	specs := map[*volume.Spec]bool{
		&volume.Spec{
			Volume: &api.Volume{
				VolumeSource: api.VolumeSource{
					Flocker: &api.PwxVolumeSource{},
				},
			},
		}: true,
		&volume.Spec{
			PersistentVolume: &api.PersistentVolume{
				Spec: api.PersistentVolumeSpec{
					PersistentVolumeSource: api.PersistentVolumeSource{
						Flocker: &api.PwxVolumeSource{},
					},
				},
			},
		}: true,
		&volume.Spec{
			Volume: &api.Volume{
				VolumeSource: api.VolumeSource{},
			},
		}: false,
	}

	for spec, expected := range specs {
		actual := plug.CanSupport(spec)
		assert.Equal(expected, actual)
	}
}

func TestGetPwxVolumeSource(t *testing.T) {
	assert := assert.New(t)

	p := pwxPlugin{}

	spec := &volume.Spec{
		Volume: &api.Volume{
			VolumeSource: api.VolumeSource{
				Pwx: &api.PwxVolumeSource{},
			},
		},
	}
	vs, ro := p.getPwxVolumeSource(spec)
	assert.False(ro)
	assert.Equal(spec.Volume.Pwx, vs)

	spec = &volume.Spec{
		PersistentVolume: &api.PersistentVolume{
			Spec: api.PersistentVolumeSpec{
				PersistentVolumeSource: api.PersistentVolumeSource{
					Pwx: &api.PwxVolumeSource{},
				},
			},
		},
	}
	vs, ro = p.getPwxVolumeSource(spec)
	assert.False(ro)
	assert.Equal(spec.PersistentVolume.Spec.Pwx, vs)
}

func TestNewBuilder(t *testing.T) {
	assert := assert.New(t)

	plugMgr, _ := newInitializedVolumePlugMgr(t)
	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.NoError(err)

	spec := &volume.Spec{
		Volume: &api.Volume{
			VolumeSource: api.VolumeSource{
				Pwx: &api.PwxVolumeSource{
					DatasetName: "something",
				},
			},
		},
	}

	_, err = plug.NewBuilder(spec, &api.Pod{}, volume.VolumeOptions{})
	assert.NoError(err)
}

func TestNewCleaner(t *testing.T) {
	assert := assert.New(t)

	p := pwxPlugin{}

	cleaner, err := p.NewCleaner("", types.UID(""))
	assert.Nil(cleaner)
	assert.NoError(err)
}

func TestIsReadOnly(t *testing.T) {
	b := &pwxBuilder{readOnly: true}
	assert.True(t, b.GetAttributes().ReadOnly)
}

func TestGetPath(t *testing.T) {
	const expectedPath = "/pwx/expected"

	assert := assert.New(t)

	b := pwxBuilder{pwx: &pwx{path: expectedPath}}
	assert.Equal(expectedPath, b.GetPath())
}

type mockPwxClient struct {
	datasetID, primaryUUID, path string
	datasetState                 *pwxclient.DatasetState
}

func newMockPwxClient(mockDatasetID, mockPrimaryUUID, mockPath string) *mockFlockerClient {
	return &mockPwxClient{
		datasetID:   mockDatasetID,
		primaryUUID: mockPrimaryUUID,
		path:        mockPath,
		datasetState: &flockerclient.DatasetState{
			Path:      mockPath,
			DatasetID: mockDatasetID,
			Primary:   mockPrimaryUUID,
		},
	}
}

func (m mockFlockerClient) CreateDataset(metaName string) (*flockerclient.DatasetState, error) {
	return m.datasetState, nil
}
func (m mockFlockerClient) GetDatasetState(datasetID string) (*flockerclient.DatasetState, error) {
	return m.datasetState, nil
}
func (m mockFlockerClient) GetDatasetID(metaName string) (string, error) {
	return m.datasetID, nil
}
func (m mockFlockerClient) GetPrimaryUUID() (string, error) {
	return m.primaryUUID, nil
}
func (m mockFlockerClient) UpdatePrimaryForDataset(primaryUUID, datasetID string) (*flockerclient.DatasetState, error) {
	return m.datasetState, nil
}

func TestSetUpAtInternal(t *testing.T) {
	const dir = "dir"
	mockPath := "expected-to-be-set-properly" // package var
	expectedPath := mockPath

	assert := assert.New(t)

	plugMgr, rootDir := newInitializedVolumePlugMgr(t)
	if rootDir != "" {
		defer os.RemoveAll(rootDir)
	}
	plug, err := plugMgr.FindPluginByName(flockerPluginName)
	assert.NoError(err)

	pod := &api.Pod{ObjectMeta: api.ObjectMeta{UID: types.UID("poduid")}}
	b := flockerBuilder{flocker: &flocker{pod: pod, plugin: plug.(*flockerPlugin)}}
	b.client = newMockFlockerClient("dataset-id", "primary-uid", mockPath)

	assert.NoError(b.SetUpAt(dir, nil))
	assert.Equal(expectedPath, b.flocker.path)
}
