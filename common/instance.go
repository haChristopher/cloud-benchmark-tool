package common

import (
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

func GenerateNewInstance(
	name string,
	orchestratorName string,
	gcpProjectName string,
	gcpRegion string,
	gcpZone string,
	gcpBucketName string,
	gcpImageName string,
	gcpInstanceDiskSize int,
	gcpMachineType string,
) *computepb.Instance {
	newInstance := computepb.Instance{
		CanIpForward: FalsePointer(),
		Disks: []*computepb.AttachedDisk{
			{
				AutoDelete: TruePointer(),
				Boot:       TruePointer(),
				DeviceName: StringPointer(name),
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					DiskSizeGb:  Int64Pointer(int64(gcpInstanceDiskSize)),
					DiskType:    StringPointer("projects/" + gcpProjectName + "/zones/" + gcpZone + "/diskTypes/pd-balanced"),
					SourceImage: StringPointer("projects/" + gcpProjectName + "/global/images/" + gcpImageName),
				},
				Mode: StringPointer("READ_WRITE"),
				Type: StringPointer("PERSISTENT"),
			},
		},
		NetworkInterfaces: []*computepb.NetworkInterface{
			{
				AccessConfigs: []*computepb.AccessConfig{
					{
						Name:        StringPointer("External NAT"),
						NetworkTier: StringPointer("Premium"),
					},
				},
				StackType:  StringPointer("IPV4_ONLY"),
				Subnetwork: StringPointer("projects/" + gcpProjectName + "/regions/" + gcpRegion + "/subnetworks/default"),
			},
		},
		MachineType: StringPointer("projects/" + gcpProjectName + "/zones/" + gcpZone + "/machineTypes/" + gcpMachineType),
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{
				{
					Key:   StringPointer("startup-script-url"),
					Value: StringPointer("https://storage.googleapis.com/" + gcpBucketName + "/" + orchestratorName + "/startup.sh"),
				},
			},
		},
		ServiceAccounts: []*computepb.ServiceAccount{
			{
				Email: StringPointer("994134327751-compute@developer.gserviceaccount.com"),
				Scopes: []string{
					"https://www.googleapis.com/auth/devstorage.full_control",
				},
			},
		},
		Name: StringPointer(name),
		Zone: StringPointer("projects/" + gcpProjectName + "/zones/europe-west3-c"),
	}
	return &newInstance
}

func FalsePointer() *bool {
	a := false
	return &a
}

func TruePointer() *bool {
	a := true
	return &a
}

func StringPointer(v string) *string {
	return &v
}

func Int64Pointer(i int64) *int64 {
	return &i
}
