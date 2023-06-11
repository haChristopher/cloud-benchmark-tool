package main

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed build/runner
var runnerBytes []byte

func generateStartupScript(
	projUri string,
	tags []string,
	basePackage string,
	bed int,
	iterations int,
	sr int,
	orchestratorIp string,
	benchListPort string,
	msrmntReportPort string,
	projectName string,
	bucketName string,
	genPprof bool,
	envs []string,
	commands []string,
) []byte {

	scriptFormatString := `#!/bin/bash

echo "Running startup script ..."
export PATH=$PATH:/usr/local/go/bin
LOGFILE=startup.log

# define the tasks that need to be done with the extracted content
run_benchmark_runner() {
    cd $WORK_DIR
    chmod +x runner
    git clone %s proj
	git config --global --add safe.directory '*'
	cd proj
	git fetch --all --tags
	git checkout tags/%s
	cd ..
    ./runner -path $WORK_DIR/proj -logfile -tags %s -base-package %s -bed %d -iterations %d -sr %d -orchestrator-ip %s -benchmark-list-port %s -measurement-report-port %s -project-name %s -bucket-name %s -generate-pprof=%t -envs %s -commands %s
    # do something with the extracted content
}

WORK_DIR=/tmp
export HOME=/tmp

# line number where payload starts
PAYLOAD_LINE=$(awk '/^__PAYLOAD_BEGINS__/ { print NR + 1; exit 0; }' $0)

# extract the embedded file
tail -n +${PAYLOAD_LINE} $0 >> $WORK_DIR/runner

# perform actions with the extracted content
run_benchmark_runner >& $LOGFILE

exit 0
__PAYLOAD_BEGINS__
`
	return append([]byte(fmt.Sprintf(
		scriptFormatString,
		projUri,
		tags[0],
		strings.Join(tags, ","),
		basePackage,
		bed,
		iterations,
		sr,
		orchestratorIp,
		benchListPort,
		msrmntReportPort,
		projectName,
		bucketName,
		genPprof,
		strings.Join(envs, ","),
		strings.Join(commands, ","))), runnerBytes...)
}
