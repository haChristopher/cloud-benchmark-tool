package main

import (
	"cloud-benchmark-tool/common"
	"context"
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"regexp"

	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"

	"os"
	"time"
)

type (
	cmdArgs struct {
		Path                  string
		BasePackage           string
		Bed                   int
		Iterations            int
		Sr                    int
		OrchestratorIp        string
		BenchmarkListPort     string
		MeasurementReportPort string
		ProjectName           string
		BucketName            string
	}
)

func parseArgs() (ca cmdArgs) {
	flag.StringVar(&(ca.Path), "path", "", "Path of the project under test to benchmark.") // Project is cloned by startup script and path passed here
	flag.StringVar(&(ca.BasePackage), "base-package", "", "Base package name used for golang imports.")
	flag.IntVar(&(ca.Bed), "bed", 1, "Benchmark Execution Duration in seconds (single number, no unit).")
	flag.IntVar(&(ca.Iterations), "iterations", 1, "Number of iterations for a benchmark.")
	flag.IntVar(&(ca.Sr), "sr", 1, "Number of suite runs for the whole benchmark suite.")

	flag.StringVar(&(ca.OrchestratorIp), "orchestrator-ip", "127.0.0.1", "IP address of the orchestrator program to report results to.")
	flag.StringVar(&(ca.BenchmarkListPort), "benchmark-list-port", "5000", "Port, under which the orchestrator reports the list of benchmarks.")
	flag.StringVar(&(ca.MeasurementReportPort), "measurement-report-port", "5001", "Port, under which the orchestrator receives the benchmarking measurements.")

	flag.StringVar(&(ca.ProjectName), "project-name", "default", "Project of bucket to upload experiment pprof files to.")
	flag.StringVar(&(ca.BucketName), "bucket-name", "default", "Bucket to upload experiment pprof files to.")

	flag.Parse()

	// Fix multiple slashes in path
	re := regexp.MustCompile(`/+`)
	ca.Path = re.ReplaceAllString(ca.Path, "/")

	return
}

func main() {
	// Seed rand with current time (running with no seed gives deterministic results)
	rand.Seed(time.Now().UnixNano())

	// parse cmd arguments
	ca := parseArgs()

	// Create log file
	f, err := os.OpenFile("./log.txt", os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		panic(1)
	}

	// Initiate logging
	//log.SetOutput(os.Stdout)
	log.SetOutput(f)
	log.SetLevel(log.DebugLevel)

	// Receive benchmarks from orchestrator
	log.Debug("Reading benchmarks from orchestrator")
	benchmarks := readBenchmarks(ca.Path, ca.OrchestratorIp, ca.BenchmarkListPort)
	log.Debug(benchmarks)
	log.Debug("Finished reading benchmarks")

	// Run benchmarks
	for i := 1; i <= ca.Sr; i++ {
		log.Debugf("Begin Suite Run %d of %d", i, ca.Sr)
		order := *common.CreateExtendedPerm(len(*benchmarks), ca.Iterations)
		itCounts := make([]int, len(*benchmarks))
		log.Debugf("Order of this run: %v", order)
		for j := 0; j < len(order); j++ {
			curr := order[j]
			itCounts[curr]++
			// execute current benchmark
			log.Debugf("Executing %s with iteration %d of %d", (*benchmarks)[curr].Name, itCounts[curr], ca.Iterations)
			err := (*benchmarks)[curr].RunBenchmark(ca.Bed, itCounts[curr], i, true)
			if err != nil {
				log.Debug(err)
			}
		}
		log.Debugf("Finished Suite Run %d of %d", i, ca.Sr)
	}

	// Upload pprof files to bucket
	log.Debug("Uploading pprof files to bucket")
	uploadPprofFilesToBucket("proj/cpu/", ca.ProjectName, ca.BucketName)
	uploadPprofFilesToBucket("cpu/", ca.ProjectName, ca.BucketName)

	// Send benchmarks with measurement results back
	log.Debug("Sending measurements to orchestrator")
	sendMeasurements(benchmarks, ca.OrchestratorIp, ca.MeasurementReportPort)
	log.Debug("Finished sending measurements")
}

func readBenchmarks(projPath string, ip string, port string) *[]common.Benchmark {
	benchmarks := make([]common.Benchmark, 0, 10)

	conn, err := net.Dial("tcp", ip+":"+port)
	if err != nil {
		log.Fatalln(err)
	}
	dec := gob.NewDecoder(conn)
	for {
		var b common.Benchmark
		err := dec.Decode(&b)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				log.Fatalln(err)
			}
		}
		// Rewrite project path
		b.ProjectPath = projPath

		benchmarks = append(benchmarks, b)
	}

	return &benchmarks
}

func sendMeasurements(benchmarks *[]common.Benchmark, ip string, port string) {
	conn, err := net.Dial("tcp", ip+":"+port)
	if err != nil {
		log.Fatal(err)
	}
	encoder := gob.NewEncoder(conn)
	N := len(*benchmarks)
	for i := 0; i < N; i++ {
		encoder.Encode((*benchmarks)[i])
	}
	conn.Close()
}

func uploadPprofFilesToBucket(path string, gcpProjectName string, gcpBucketName string) {
	items, _ := ioutil.ReadDir(path)

	// check if there is an item
	if len(items) == 0 {
		log.Warnf("No files found to upload in path: %s", path)
		return
	}

	// open gcp storage client use default credentials (orchestrator should granta access)
	ctx := context.Background()
	gclientStorage, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	defer gclientStorage.Close()

	for _, item := range items {
		fmt.Println(item.Name())
		bytes, err := ioutil.ReadFile(item.Name())

		if err != nil {
			log.Warnf("Could not read file %s and upload it to bucket", item.Name())
			continue
		}

		// use from common
		key := "runner/" + item.Name()
		common.UploadBytes(bytes, key, gcpProjectName, gcpBucketName, gclientStorage, ctx)
	}
}
