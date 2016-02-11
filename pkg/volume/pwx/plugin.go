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
	"fmt"

	//pwxclient "github.com/libopenstorage/openstorage/api/client"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/strings"
	"k8s.io/kubernetes/pkg/volume"
	//volumeutil "k8s.io/kubernetes/pkg/volume/util"
)

const (
	pwxPluginName = "kubernetes.io/pwx"
)

func ProbeVolumePlugins() []volume.VolumePlugin {
	return []volume.VolumePlugin{&pwxPlugin{nil}}
}

type pwxPlugin struct {
	host volume.VolumeHost
}

func (p *pwxPlugin) Init(host volume.VolumeHost) error {
	p.host = host
	return nil
}

func (p *pwxPlugin) Name() string {
	return pwxPluginName
}

func (p *pwxPlugin) CanSupport(spec *volume.Spec) bool {
	return (spec.PersistentVolume != nil && spec.PersistentVolume.Spec.PwxDisk != nil) ||
		(spec.Volume != nil && spec.Volume.PwxDisk != nil)
}

func (p *pwxPlugin) NewBuilder(spec *volume.Spec, pod *api.Pod, _ volume.VolumeOptions) (volume.Builder, error) {
	// Inject real implementations here, test through the internal function.
	return p.newBuilderInternal(spec, pod.UID, &PWXDiskUtil{}, p.host.GetMounter())
}

func (p *pwxPlugin) newBuilderInternal(spec *volume.Spec, podUID types.UID, manager pwxManager, mounter mount.Interface) (volume.Builder, error) {
	var readOnly bool

	var px *api.PwxVolumeSource
	if spec.Volume != nil && spec.Volume.PwxDisk != nil {
		px = spec.Volume.PwxDisk
		readOnly = px.ReadOnly
	} else {
		px = spec.PersistentVolume.Spec.PwxDisk
		readOnly = spec.ReadOnly
	}

	volid := px.VolumeID
	fsType := px.FSType

	return &pwxBuilder{
		pwxDisk: &pwxDisk{
			podUID:   podUID,
			volName:  spec.Name(),
			VolumeID: volid,
			mounter:  mounter,
			manager:  manager,
			plugin:   p,
		},
		fsType:      fsType,
		readOnly:    readOnly,
		diskMounter: &mount.SafeFormatAndMount{mounter, exec.New()}}, nil
}

func (p *pwxPlugin) NewCleaner(volName string, podUID types.UID) (volume.Cleaner, error) {
	// Inject real implementations here, test through the internal function.
	return p.newCleanerInternal(volName, podUID, &PWXDiskUtil{}, p.host.GetMounter())
}

func (p *pwxPlugin) newCleanerInternal(volName string, podUID types.UID, manager pwxManager, mounter mount.Interface) (volume.Cleaner, error) {
	return &pwxDiskCleaner{&pwxDisk{
		podUID:  podUID,
		volName: volName,
		manager: manager,
		mounter: mounter,
		plugin:  p,
	}}, nil
}

// Unmounts the bind mount, and detaches the disk only if the PD
// resource was the last reference to that disk on the kubelet.
func (c *pwxDiskCleaner) TearDown() error {
	return c.TearDownAt(c.GetPath())
}

// Unmounts the bind mount, and detaches the disk only if the PD
// resource was the last reference to that disk on the kubelet.
func (c *pwxDiskCleaner) TearDownAt(dir string) error {
	return nil
}

func (p *pwxPlugin) NewProvisioner(options volume.VolumeOptions) (volume.Provisioner, error) {
	return p.newProvisionerInternal(options, &PWXDiskUtil{})
}

func (p *pwxPlugin) newProvisionerInternal(options volume.VolumeOptions, manager pwxManager) (volume.Provisioner, error) {
	return &pwxDiskProvisioner{
		pwxDisk: &pwxDisk{
			manager: manager,
			plugin:  p,
		},
		options: options,
	}, nil
}

// Abstract interface to pwx operations.
type pwxManager interface {
	// Creates a volume
	CreateVolume(provisioner *pwxDiskProvisioner) (volumeID string, volumeSizeGB int, err error)
}

// pwxDisk volumes are disk resources provided by Portworx
// that are attached to the kubelet's host machine and exposed to the pod.
type pwxDisk struct {
	volName string
	podUID  types.UID
	// Unique identifier of the Volume, used to find the disk resource in the provider.
	VolumeID string
	// Specifies the partition to mount
	partition string
	// Utility interface that provides API calls to the provider to attach/detach disks.
	manager pwxManager
	// Mounter interface that provides system calls to mount the global path to the pod local path.
	mounter mount.Interface
	plugin  *pwxPlugin
	volume.MetricsNil
}

type pwxBuilder struct {
	*pwxDisk
	// Filesystem type, optional.
	fsType string
	// Specifies whether the disk will be attached as read-only.
	readOnly bool
	// diskMounter provides the interface that is used to mount the actual block device.
	diskMounter *mount.SafeFormatAndMount
}

var _ volume.Builder = &pwxBuilder{}

func (b *pwxBuilder) GetAttributes() volume.Attributes {
	return volume.Attributes{
		ReadOnly:        b.readOnly,
		Managed:         false,
		SupportsSELinux: false,
	}
}

func (b *pwxBuilder) SetUp(fsGroup *int64) error {
	return b.SetUpAt(b.GetPath(), fsGroup)
}

func (b *pwxBuilder) SetUpAt(dir string, fsGroup *int64) error {
	//c, err := pwxclient.NewDriverClient("pxd")

	fmt.Sprintf("string: %v", dir)
	fmt.Sprintf("fsGroup: %v", fsGroup)

	/*
		if b.client == nil {
			c, err := pwxclient.NewDriverClient("pxd")
			if err != nil {
				return err
			}
			b.client = c
		}
	*/
	return nil
}

func (pd *pwxDisk) GetPath() string {
	name := pwxPluginName
	return pd.plugin.host.GetPodVolumeDir(pd.podUID, strings.EscapeQualifiedNameForDisk(name), pd.volName)
}

type pwxDiskCleaner struct {
	*pwxDisk
}

var _ volume.Cleaner = &pwxDiskCleaner{}

type pwxDiskProvisioner struct {
	*pwxDisk
	options volume.VolumeOptions
}

var _ volume.Provisioner = &pwxDiskProvisioner{}

func (c *pwxDiskProvisioner) Provision(pv *api.PersistentVolume) error {
	volumeID, sizeGB, err := c.manager.CreateVolume(c)
	if err != nil {
		return err
	}
	pv.Spec.PwxDisk.VolumeID = volumeID
	pv.Spec.Capacity = api.ResourceList{
		api.ResourceName(api.ResourceStorage): resource.MustParse(fmt.Sprintf("%dGi", sizeGB)),
	}
	return nil
}

func (c *pwxDiskProvisioner) NewPersistentVolumeTemplate() (*api.PersistentVolume, error) {
	// Provide dummy api.PersistentVolume.Spec, it will be filled in
	// gcePersistentDiskProvisioner.Provision()
	return &api.PersistentVolume{
		ObjectMeta: api.ObjectMeta{
			GenerateName: "pwx-",
			Labels:       map[string]string{},
			Annotations: map[string]string{
				"kubernetes.io/createdby": "pwx",
			},
		},
		Spec: api.PersistentVolumeSpec{
			Capacity: api.ResourceList{
				api.ResourceName(api.ResourceStorage): c.options.Capacity,
			},
			PersistentVolumeSource: api.PersistentVolumeSource{
				PwxDisk: &api.PwxVolumeSource{
					VolumeID: "00000000-0000-0000000-0000-000000",
					FSType:   "ext4",
					ReadOnly: false,
				},
			},
		},
	}, nil
}
