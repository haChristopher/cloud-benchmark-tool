package common

import (
	"context"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	computepb "google.golang.org/genproto/googleapis/cloud/compute/v1"
)

func UploadBytes(toUpload []byte, fileKey string, gcpProjectName string, gcpBucketName string, gclient *storage.Client, ctx context.Context) {
	wc := gclient.Bucket(gcpBucketName).Object(fileKey).NewWriter(ctx)
	wc.ContentType = "text/plain"
	// make this file public readable
	// wc.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	wc.Metadata = map[string]string{
		"x-goog-project-id": gcpProjectName,
	}

	log.Debugln("Uploading data to bucket")
	i, err := wc.Write(toUpload)
	if err != nil {
		log.Fatalln(err)
	}
	err = wc.Close()
	if err != nil {
		log.Fatalln(err)
	}
	log.Debugf("Wrote %d\n", i)
	log.Debugln("Finished uploading data to bucket")
}

func CreateInstance(name string, orchestratorName string, gcpProjectName string, gcpRegion string, gcpZone string, gcpBucketName string, gcpImageName string, gclient *compute.InstancesClient, ctx context.Context) {
	log.Debugln("Creating instance " + name)
	instance := GenerateNewInstance(name, orchestratorName, gcpProjectName, gcpRegion, gcpZone, gcpBucketName, gcpImageName)

	req := computepb.InsertInstanceRequest{
		InstanceResource: instance,
		Project:          gcpProjectName,
		Zone:             gcpZone,
	}

	op, err := gclient.Insert(ctx, &req)
	if err != nil {
		log.Fatalln(err)

	}

	err = op.Wait(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	log.Debugln("Finished creating instance " + name)
}

func ShutdownAllInstances(toShutdown *[]string, gcpProjectName string, gcpZone string, gclient *compute.InstancesClient, ctx context.Context) {
	log.Debugln("Removing all instances")

	listReq := computepb.ListInstancesRequest{
		Project: gcpProjectName,
		Zone:    gcpZone,
	}

	it := gclient.List(ctx, &listReq)

	for {
		instance, err := it.Next()
		if err == iterator.Done {
			log.Debugln(err)
			break
		} else if err != nil {
			log.Fatalln(err)
		}

		// Only shut down instances in list
		if contains(*instance.Name, toShutdown) {
			log.Debugln("Removing instance " + *instance.Name)
			delReq := computepb.DeleteInstanceRequest{
				Instance: *instance.Name,
				Project:  gcpProjectName,
				Zone:     gcpZone,
			}
			op, err := gclient.Delete(ctx, &delReq)
			if err != nil {
				log.Fatalln(err)
			}

			err = op.Wait(ctx)
			if err != nil {
				log.Fatalln(err)
			}
			log.Debugln("Finished removing instance " + *instance.Name)
		}
	}
	log.Debugln("Finished removing all instances")
}

func contains(elem string, list *[]string) bool {
	for i := 0; i < len(*list); i++ {
		if elem == (*list)[i] {
			return true
		}
	}
	return false
}
