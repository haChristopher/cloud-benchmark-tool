# uOpTime

Also called μOpTime

Benutze RMAD als Instabilitätsmaß (oder anderes relatives Streuungsmaß in % des Mittelwerts (mean oder median)).

Grenze zur Instabilität: RMAD > 0.01 (1% des Mittels Abweichung)

Probiere unterschiedliche Schätzansätze für CV, RMAD und RCIW

Absolute Minimalkonfiguration:
- IR: nicht anfassen
- SR: 1
- It: 3
- BED: 1s


finde config mit ir 1, verifiziere mit ir 2 und 3


# Setup

Create google cloud Project
Create Service Account
Create Bucket
Create Disk Image with git and golang installed
Copy config file and create own

Its important that the compute engine default service account has the following roles:
- Storage Access (to load the startup script from the bucket)

gp mod

# Run Orchestrator

```
make all
```

```
./build/orchestrator --configFile configFile.toml --credentials master-thesis-benchmark-d7f8df1edc74.json --benchmark-list-port 5002 --measurement-report-port 5003 --instance-name operator-main --ip 10.156.0.13 --clean-db
```

# Debugging

For debugging the startup script of the VMs, connect to them using ssh and run the following command:
```
sudo journalctl -u google-startup-scripts.service -f
```

Run Startup script:
```
    sudo google_metadata_script_runner startup
```

# Image Creation

```
sudo apt install graphviz gv
```

go test -benchtime 1s -bench ^BenchmarkCreateBuildInfo$ ./expfmt -memprofile BenchmarkCreateBuildInfo.out -cpuprofile BenchmarkCreateBuildInfo.out


# Configuration

Changing go version, if gvm is installed on the image, you can use the commands config variable
```
commands=["gvm install go1.18", "gvm use go1.18 --default"]
```

Setting environment variables for project setup and benchmark execution
```
envs=["GO111MODULE=on"]
```

# Running Locally

Orchestrator:
```
./build/orchestrator -local --configFile config-roaring.toml
```

Runner only:
```
CGO_ENABLED=0 go build -o cmd/orchestrator/build/ -v cloud-benchmark-tool/cmd/runner
```
