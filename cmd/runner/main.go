package main

import (
	"cloud-benchmark-tool/common"
	"context"
	"encoding/gob"
	"flag"
	"io"
	"io/fs"
	"math/rand"
	"net"
	"os/exec"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"

	"os"
	"time"
)

type (
	cmdArgs struct {
		Path                  string
		Tags                  string
		BasePackage           string
		Bed                   int
		Iterations            int
		Sr                    int
		OrchestratorIp        string
		BenchmarkListPort     string
		MeasurementReportPort string
		ProjectName           string
		BucketName            string
		GenPprof              bool
		Envs                  string
		Commands              string
		logfile               bool
	}
)

func parseArgs() (ca cmdArgs) {
	flag.StringVar(&(ca.Path), "path", "", "Path of the project under test to benchmark.") // Project is cloned by startup script and path passed here
	flag.StringVar(&(ca.Tags), "tags", "", "List fo Tags to run benchmark with.")
	flag.StringVar(&(ca.BasePackage), "base-package", "", "Base package name used for golang imports.")
	flag.IntVar(&(ca.Bed), "bed", 1, "Benchmark Execution Duration in seconds (single number, no unit).")
	flag.IntVar(&(ca.Iterations), "iterations", 1, "Number of iterations for a benchmark.")
	flag.IntVar(&(ca.Sr), "sr", 1, "Number of suite runs for the whole benchmark suite.")

	flag.StringVar(&(ca.OrchestratorIp), "orchestrator-ip", "127.0.0.1", "IP address of the orchestrator program to report results to.")
	flag.StringVar(&(ca.BenchmarkListPort), "benchmark-list-port", "5002", "Port, under which the orchestrator reports the list of benchmarks.")
	flag.StringVar(&(ca.MeasurementReportPort), "measurement-report-port", "5003", "Port, under which the orchestrator receives the benchmarking measurements.")

	flag.StringVar(&(ca.ProjectName), "project-name", "default", "Project of bucket to upload experiment pprof files to.")
	flag.StringVar(&(ca.BucketName), "bucket-name", "default", "Bucket to upload experiment pprof files to.")

	flag.BoolVar(&(ca.GenPprof), "generate-pprof", false, "Wether to generate pprof files or not.")
	flag.StringVar(&(ca.Envs), "envs", "", "List of environment variables to set.")
	flag.StringVar(&(ca.Commands), "commands", "", "List commands to execute before the benchmark in the project dir.")

	flag.BoolVar(&(ca.logfile), "logfile", true, "Wether to log to file.")

	flag.Parse()

	// Fix multiple slashes in path
	re := regexp.MustCompile(`/+`)
	ca.Path = re.ReplaceAllString(ca.Path, "/")

	return
}

const MEASUREMENT_BATCH_SIZE = 20

var hostname string = "runner"
var numExecutions int = 0

func main() {
	// Seed rand with current time (running with no seed gives deterministic results)
	rand.Seed(time.Now().UnixNano())

	// Set hostname
	name, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	hostname = name

	// Parse cmd arguments
	ca := parseArgs()

	if ca.GenPprof {
		// Create folders for cpu and mem pprof files
		err := os.MkdirAll("cpu", os.ModePerm)
		if err != nil {
			log.Println(err)
		}

		err = os.MkdirAll("mem", os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}

	// Create log file
	var f *os.File
	log.SetOutput(os.Stdout)

	// Initiate logging
	if ca.logfile {
		f, err = os.OpenFile("./"+hostname+"-log.txt", os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			log.Println(err)
			panic(1)
		}
		log.SetOutput(f)
	}
	log.SetLevel(log.DebugLevel)

	// Receive benchmarks from orchestrator
	log.Debug("Reading benchmarks from orchestrator")
	benchmarks := readBenchmarks(ca.Path, ca.OrchestratorIp, ca.BenchmarkListPort)
	log.Debug(benchmarks)
	log.Debug("Finished reading benchmarks")

	// Log tags
	log.Debug("Tags to use: ", ca.Tags)
	tags := strings.Split(ca.Tags, ",")
	log.Debugf("Tags for this run: %v", tags)

	// Set Envs
	envs := strings.Split(ca.Envs, ",")
	common.SetEnvironmentVariables(envs)

	// Run Commands
	log.Debug("Commands to run: ", ca.Commands)
	commands := strings.Split(ca.Commands, ",")
	common.RunCommands(commands, ca.Path)

	// Run benchmarks
	for i := 1; i <= ca.Sr; i++ {
		start := time.Now()
		log.Infof("Begin Suite Run %d of %d", i, ca.Sr)
		order := *common.CreateExtendedPerm(len(*benchmarks), ca.Iterations)
		itCounts := make([]int, len(*benchmarks))
		log.Debugf("Order of this run: %v", order)

		for j := 0; j < len(order); j++ {
			curr := order[j]
			itCounts[curr]++

			for _, tag := range shuffle(tags) {
				// execute current benchmark
				log.Debugf("Executing %s with iteration %d of %d on tag: %s", (*benchmarks)[curr].Name, itCounts[curr], ca.Iterations, tag)

				// First take is already initital checked out
				if len(tags) > 1 {
					log.Debug("Checking out tag: ", tag)
					gitCheckout := exec.Command("git", "checkout", "tags/"+tag)
					gitCheckout.Dir = (*benchmarks)[curr].ProjectPath
					_, gitCheckoutErr := gitCheckout.CombinedOutput()

					if gitCheckoutErr != nil {
						log.Debug(gitCheckoutErr)
					}
				}

				if (*benchmarks)[curr].Failing {
					log.Info("Skipping previously failing benchmark: ", (*benchmarks)[curr].Name, " on tag: ", tag)
					continue
				}

				// Run benchmark
				err := (*benchmarks)[curr].RunBenchmark(ca.Bed, itCounts[curr], i, tag, ca.GenPprof)
				if err != nil {
					log.Debug(err)
				}
				numExecutions++
			}

			if numExecutions > MEASUREMENT_BATCH_SIZE {
				log.Debug("Sending measurements to orchestrator and clearing measurements: ", numExecutions)
				sendMeasurements(benchmarks, ca.OrchestratorIp, ca.MeasurementReportPort)
				clearBenchmarkMeasurements(benchmarks)
				numExecutions = 0
			}

		}
		elapsed := time.Since(start)
		log.Debugf("Finished Suite Run %d of %d", i, ca.Sr)
		log.Debugf("Running on suite run took: %s", elapsed)
	}

	if ca.GenPprof {
		log.Debug("Uploading pprof files to bucket")
		uploadFilesToBucket("cpu/", ca.ProjectName, ca.BucketName)
	}

	log.Debug("Sending measurements to orchestrator and clearing measurements")
	sendMeasurements(benchmarks, ca.OrchestratorIp, ca.MeasurementReportPort)
	log.Debug("Sending done signal to orchestrator")
	sendDoneSignal(ca.OrchestratorIp, ca.MeasurementReportPort)
	log.Debug("Finished sending measurements")

	// Close and upload log file to bucket
	if ca.logfile && f != nil {
		log.Debug("Closing log file and uploading log file to bucket")
		log.SetOutput(os.Stdout)
		fileCloseErr := f.Close()
		if fileCloseErr != nil {
			log.Debug(fileCloseErr)
		}
		uploadFilesToBucket(f.Name(), ca.ProjectName, ca.BucketName)
	}

}

func shuffle(slice []string) []string {
	rand.Shuffle(len(slice), func(i, j int) { slice[i], slice[j] = slice[j], slice[i] })
	return slice
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
		if len((*benchmarks)[i].Measurement) != 0 {
			encoder.Encode((*benchmarks)[i])
		}
	}
	conn.Close()
}

func sendDoneSignal(ip string, port string) {
	conn, err := net.Dial("tcp", ip+":"+port)
	if err != nil {
		log.Fatal(err)
	}
	encoder := gob.NewEncoder(conn)
	encoder.Encode(common.Benchmark{Name: "alldone"})
	conn.Close()
}

func clearBenchmarkMeasurements(benchmarks *[]common.Benchmark) {
	N := len(*benchmarks)
	for i := 0; i < N; i++ {
		(*benchmarks)[i].Measurement = make([]common.Measurement, 0)
	}
}

// Uploads a single file or directory to a google cloud bucket
func uploadFilesToBucket(path string, gcpProjectName string, gcpBucketName string) {
	var items []fs.DirEntry
	fileInfo, err := os.Stat(path)
	if err != nil {
		log.Warnf("Could not get stat for path: %s", path)
		return
	}

	// Check if single file or directory
	if fileInfo.IsDir() {
		items, _ = os.ReadDir(path)
	} else {
		items = append(items, fs.FileInfoToDirEntry(fileInfo))
	}

	if len(items) == 0 {
		log.Warnf("No files found to upload in path: %s", path)
		return
	}

	// open gcp storage client use default credentials (orchestrator should grant access)
	ctx := context.Background()
	gclientStorage, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	defer gclientStorage.Close()

	for _, item := range items {
		log.Debug("Uploading file: ", item.Name())

		full_path := path
		if item.IsDir() {
			full_path = full_path + item.Name()
		}

		bytes, err := os.ReadFile(full_path)
		if err != nil {
			log.Warnf("Could not read file %s and upload it to bucket in path: %s", item.Name(), path)
			continue
		}

		key := "exp6goquery" + "/" + hostname + "/" + time.Now().Format("01-02-2006") + "_" + item.Name()
		common.UploadBytes(bytes, key, gcpProjectName, gcpBucketName, gclientStorage, ctx)
	}
}
