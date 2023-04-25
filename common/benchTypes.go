package common

import (
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	benchparser "golang.org/x/tools/benchmark/parse"
)

type (
	Measurement struct {
		N       int
		NsPerOp float64
		BedPos  int
		ItPos   int
		SrPos   int
		Tag     string
	}

	Benchmark struct {
		Name        string
		NameRegexp  string
		Package     string
		ProjectPath string
		Measurement []Measurement
	}
)

// MaskNameRegexp turns a benchmark name, e.g., BenchmarkNumberOne/20, into a regular expression,
// which will never accidentally execute other benchmarks.
// For some reason, '/' has a special meaning in "go test -bench x" and the beginning (^) and end ($)
// have to be marked in every substring around a '/'.
//
// Every substring around '/' seems to be treated like its own regular expression. As far as I can tell,
// this behavior is not documented. See https://github.com/golang/go/blob/master/src/testing/match.go at func splitRegexp.
func MaskNameRegexp(name string) string {
	nameQuoted := regexp.QuoteMeta(name)
	nameSplit := strings.Split(nameQuoted, "/")
	nameRegexp := ""

	for i := 0; i < len(nameSplit)-1; i++ {
		nameRegexp = nameRegexp + "^" + nameSplit[i] + "$/"
	}
	nameRegexp = nameRegexp + "^" + nameSplit[len(nameSplit)-1] + "$" // last iteration without '/'

	return nameRegexp
}

func (bench *Benchmark) RunBenchmark(bed int, itPos int, srPos int, pprof bool, tag string) error {
	cmd := exec.Command("go", "clean", "--cache")

	_, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%#v: error while running go clean --cache.", cmd.Args)
	}

	sRun := strconv.Itoa(srPos)
	iter := strconv.Itoa(itPos)

	// var pprofCmd [4]string
	// if pprof {
	// 	pprofCmd = [4]string{"-memprofile", bench.Name + ".out", "-cpuprofile", bench.Name + ".out"}
	// }

	// Create directories for pprof output
	err = os.MkdirAll("proj/cpu", os.ModePerm)
	if err != nil {
		log.Println(err)
	}

	err = os.MkdirAll("proj/mem", os.ModePerm)
	if err != nil {
		log.Println(err)
	}

	for i := 0; i < bed; i++ {
		// each iteration on this level is 1s of benchtime, repeat until bed is reached
		// go tool pprof -nodecount=3000 --nodefraction=0.0 --edgefraction=0.0 -dot cpu.pprof > pprof.dot
		// , "-memprofile", "mem/"+bench.Name+"_"+iter+"_"+tag+"_"+sRun+".out"
		cmd := exec.Command("go", "test", "-benchtime", "1s", "-bench", bench.NameRegexp, bench.Package, "-cpuprofile", "cpu/"+bench.Name+"_"+iter+"_"+sRun+"_"+tag+".out")
		cmd.Dir = bench.ProjectPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Wrapf(err, "%#v: output: %s", cmd.Args, out)
		}

		// split output into lines
		lines := strings.Split(string(out), "\n")

		// parse output
		for j := 0; j < len(lines); j++ {
			isBench, err := regexp.MatchString("^Benchmark", lines[j])
			if err != nil {
				return errors.Wrapf(err, "%#v: output: %s", cmd.Args, out)
			}

			if isBench {
				b, err := benchparser.ParseLine(lines[j])
				if err != nil {
					return errors.Wrapf(err, "%#v: output: %s", cmd.Args, out)
				}

				// save new measurement
				newMsrmnt := Measurement{
					N:       b.N,
					NsPerOp: b.NsPerOp,
					BedPos:  i + 1,
					ItPos:   itPos,
					SrPos:   srPos,
					Tag:     tag,
				}

				bench.Measurement = append(bench.Measurement, newMsrmnt)
			}
		}
	}

	return nil
}
