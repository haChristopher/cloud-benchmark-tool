package main

import (
	"cloud-benchmark-tool/common"
	"context"
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/storage"
	"github.com/BurntSushi/toml"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"math/rand"

	"os"
	"time"
)

type (
	configFile struct {
		Name           string
		Path           string
		ProjUri        string
		Tags           []string
		Commands       []string
		Envs           []string
		Zone           string
		Region         string
		BasePackage    string
		GCPProject     string
		GCPBucket      string
		GCPImage       string
		GcpDiskSize    int
		GcpMachineType string
		GenPprof       bool
		Bed            int
		It             int
		Sr             int
		Ir             int
	}

	cmdArgs struct {
		CleanDB               bool
		RunLocal              bool
		CredentialsFile       string
		ConfigFile            string
		SqliteFile            string
		BenchRegex            string
		InstanceName          string
		Ip                    string
		BenchmarkListPort     string
		MeasurementReportPort string
	}

	setup struct {
		Bed        int
		Iterations int
		Sr         int
		Ir         int
		Mu         sync.Mutex
	}

	irPosCounter struct {
		IrPos int
		Mu    sync.Mutex
	}
)

var wg sync.WaitGroup
var wgIrResults sync.WaitGroup
var currSetup setup
var currIrPos irPosCounter

func parseArgs() cmdArgs {
	var ca cmdArgs
	flag.BoolVar(&(ca.CleanDB), "clean-db", false, "Clean database, i.e., drop all tables related to benchmark data collection.")
	flag.BoolVar(&(ca.RunLocal), "local", false, "Runs locally without creating instances, connecting to local runners.")
	flag.StringVar(&(ca.CredentialsFile), "credentials", "creds.json", "Path to the credentials.json for GCP.")
	flag.StringVar(&(ca.ConfigFile), "configFile", "configFile.toml", "Path to the configFile.toml file.")
	flag.StringVar(&(ca.SqliteFile), "db", "database.db", "Path to the sqlite3 database file.")
	flag.StringVar(&(ca.InstanceName), "instance-name", "orchestrator", "GCP instance name of the orchestrator, so that it does not shut itself down.")
	flag.StringVar(&(ca.BenchRegex), "bench", ".", "Regex to restrict benchmarks to run, default is run all.")

	flag.StringVar(&(ca.Ip), "ip", "127.0.0.1", "IP address of this node.")
	flag.StringVar(&(ca.BenchmarkListPort), "benchmark-list-port", "5000", "Port, under which the orchestrator reports the list of benchmarks.")
	flag.StringVar(&(ca.MeasurementReportPort), "measurement-report-port", "5001", "Port, under which the orchestrator receives the benchmarking measurements.")

	flag.Parse()

	// [] TODO fix multiple slashes in paths
	return ca
}

func main() {
	// Seed rand with current time (running with no seed gives deterministic results)
	rand.Seed(time.Now().UnixNano())

	// Create log file
	f, fileCreationErr := os.OpenFile("./log-orchestrator.txt", os.O_WRONLY|os.O_CREATE, 0755)
	if fileCreationErr != nil {
		panic(1)
	}

	// Initiate logging
	log.SetOutput(f)
	log.SetLevel(log.DebugLevel)

	ca := parseArgs()

	var cfg configFile
	_, err := toml.DecodeFile(ca.ConfigFile, &cfg)

	if err != nil {
		log.Errorln(err)
		panic(err)
	}

	log.Debugf("Finished reading %s", ca.ConfigFile)
	log.Debugln(cfg)

	// Set envs and run commands
	common.SetEnvironmentVariables(cfg.Envs)
	common.RunCommands(cfg.Commands, cfg.Path)

	// TODO: write to and read from DB
	// --- Connect to DB (sqlite) ---
	ConnectToDB(DbConfig{
		Type: "sqlite",
		Uri:  ca.SqliteFile,
	}, ca.CleanDB) // assigns the global variable db
	defer CloseDB()
	// --- Finish connect to DB ---

	// TODO: read benchmarks from db, if no forced reread or db clean
	log.Debugf("Begin collecting benchmarks of %s", cfg.Name)
	benchmarks, err := CollectBenchmarks(cfg.Name, cfg.Path, cfg.BasePackage, cfg.Tags, ca.BenchRegex)
	if err != nil {
		log.Fatalln(err)
	}

	log.Debugf("Finished collecting benchmarks of %s", cfg.Name)
	log.Debugf("Found %d benchmarks: %+v", len(*benchmarks), *benchmarks)

	// Remove non performance benchmarks with Size or Memory in the name
	for i := 0; i < len(*benchmarks); i++ {
		if strings.Contains((*benchmarks)[i].Name, "BenchmarkSize") || strings.Contains((*benchmarks)[i].Name, "BenchmarkMemory") {
			log.Info("Removing benchmark non perf benchmark: ", (*benchmarks)[i].Name)
			*benchmarks = append((*benchmarks)[:i], (*benchmarks)[i+1:]...)
			i--
		}
	}

	/********** Start server endpoints ************/
	// Sending Benchmarks
	quitSend := make(chan bool, 1)
	inSend, err := net.Listen("tcp", ":5002")
	if err != nil {
		log.Fatalln(err)
	}
	go sendBenchmarkHandler(benchmarks, &inSend, quitSend)

	// Recevie Measurements
	quitRecv := make(chan bool, 1)
	inRecv, err := net.Listen("tcp", ":5003")
	if err != nil {
		log.Fatalln(err)
	}
	go readMeasurementHandler(&inRecv, quitRecv)

	/********** Start Cloud Instances ************/
	ctx := context.Background()
	creds := option.WithCredentialsFile(ca.CredentialsFile)

	// open gcp clients
	gclientStorage, err := storage.NewClient(ctx, creds)
	if err != nil {
		log.Fatalln(err)
	}
	defer gclientStorage.Close()
	gclientCompute, err := compute.NewInstancesRESTClient(ctx, creds)
	if err != nil {
		log.Fatalln(err)
	}
	defer gclientCompute.Close()

	// start setup
	currSetup.Mu.Lock()
	currSetup.Bed = cfg.Bed
	currSetup.Iterations = cfg.It
	currSetup.Sr = cfg.Sr
	currSetup.Ir = cfg.Ir
	currSetup.Mu.Unlock()
	currIrPos.Mu.Lock()
	currIrPos.IrPos = 0
	currIrPos.Mu.Unlock()

	// RUN EXPERIMENT
	currSetup.Mu.Lock()
	script := generateStartupScript(
		cfg.ProjUri,
		cfg.Tags,
		cfg.BasePackage,
		currSetup.Bed,
		currSetup.Iterations,
		currSetup.Sr,
		ca.Ip,
		ca.BenchmarkListPort,
		ca.MeasurementReportPort,
		cfg.GCPProject,
		cfg.GCPBucket,
		cfg.GenPprof,
		cfg.Envs,
		cfg.Commands,
	)
	instances := currSetup.Ir

	log.Debugf("Experiment Start\nSetup: BED = %d, It = %d, SR = %d, IR = %d", currSetup.Bed, currSetup.Iterations, currSetup.Sr, currSetup.Ir)
	currSetup.Mu.Unlock()
	/*fT, _ := os.Create("tmp")
	fT.Write(script)
	fT.Chmod(0777)
	fT.Close()*/
	currIrPos.Mu.Lock()
	currIrPos.IrPos = 0
	currIrPos.Mu.Unlock()

	// upload startup script to bucket
	fileKey := ca.InstanceName + "/startup.sh"
	common.UploadBytes(script, fileKey, cfg.GCPProject, cfg.GCPBucket, gclientStorage, ctx)

	listOfInstances := make([]string, 3)

	// Skip instance creation when running locally
	if !ca.RunLocal {
		for j := 0; j < instances; j++ {
			name := fmt.Sprintf("%s-instance-%d", ca.InstanceName, j)
			common.CreateInstance(name,
				ca.InstanceName,
				cfg.GCPProject,
				cfg.Region,
				cfg.Zone,
				cfg.GCPBucket,
				cfg.GCPImage,
				cfg.GcpDiskSize,
				cfg.GcpMachineType,
				gclientCompute,
				ctx,
			)
			listOfInstances = append(listOfInstances, name)
			wgIrResults.Add(1)
		}
		log.Debugln(listOfInstances)
	} else {
		// Wait for results of 1 local instance
		wgIrResults.Add(1)
	}

	// wait for results
	wgIrResults.Wait()

	// Wait 10 seconds for logfiles to be uploaded to bucket then shutdown instances
	time.Sleep(10 * time.Second)
	common.ShutdownAllInstances(&listOfInstances, cfg.GCPProject, cfg.Zone, gclientCompute, ctx)

	// END EXPERIMENT

	// Only end when Crtl+C is pressed
	//c := make(chan os.Signal, 1)   // create channel on os.Signal
	//signal.Notify(c, os.Interrupt) // notify channel on Crtl+C
	//for sig := range c {           // wait and block until notify
	//	fmt.Println(sig.String())
	//	quitSend <- true
	//	quitRecv <- true
	//	break // break and end program after notify
	//}
	quitSend <- true
	quitRecv <- true
	inSend.Close()
	inRecv.Close()
	CloseMeasurementQueue()
	wg.Wait()
	close(quitSend)
	close(quitRecv)
	log.Debugln("Finished experiment")
}

func sendBenchmarks(benchmarks *[]common.Benchmark, conn net.Conn) {
	// TODO should be ok without wait group
	wg.Add(1)
	defer wg.Done()
	encoder := gob.NewEncoder(conn)
	N := len(*benchmarks)
	log.Debugln("Sending benchmarks to instance")
	for i := 0; i < N; i++ {
		encoder.Encode((*benchmarks)[i])
	}
	conn.Close()
	log.Debugln("Finished sending benchmarks")
}

func sendBenchmarkHandler(benchmarks *[]common.Benchmark, in *net.Listener, quit <-chan bool) {
Loop:
	for {
		select {
		case <-quit:
			break Loop
		default:
			conn, err := (*in).Accept()
			if err != nil {
				log.Errorln(err)
				continue
			}
			go sendBenchmarks(benchmarks, conn)
		}
	}
}

func readMeasurements(conn net.Conn) {
	benchmarks := make([]common.Benchmark, 0, 10)

	dec := gob.NewDecoder(conn)
	log.Debugln("Receiving measurements from instance")
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

		if b.Name == "alldone" {
			log.Debugln("Received all done")
			wgIrResults.Done()
			break
		}

		benchmarks = append(benchmarks, b)
	}
	log.Debugln("Finished receiving measurements")

	// record measurements in db
	currSetup.Mu.Lock()
	bedSetup := currSetup.Bed
	itSetup := currSetup.Iterations
	srSetup := currSetup.Sr
	irSetup := currSetup.Ir
	currSetup.Mu.Unlock()
	currIrPos.Mu.Lock()
	currIrPos.IrPos = currIrPos.IrPos + 1
	irPos := currIrPos.IrPos
	currIrPos.Mu.Unlock()

	for i := 0; i < len(benchmarks); i++ {
		RecordMeasurement(&benchmarks[i], bedSetup, itSetup, srSetup, irSetup, irPos, &wg)
	}
	log.Debugln("Finished adding measurements into queue")
}

func readMeasurementHandler(in *net.Listener, quit <-chan bool) {
Loop:
	for {
		select {
		case <-quit:
			break Loop
		default:
			conn, err := (*in).Accept()
			if err != nil {
				log.Errorln(err)
				continue
			}
			go readMeasurements(conn)
		}
	}
}
