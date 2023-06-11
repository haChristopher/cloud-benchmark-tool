package common

import (
	"math/rand"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

// FisherYatesShuffle shuffles an int array in-place to a random permutation
// cf
// https://en.wikipedia.org/wiki/Fisher–Yates_shuffle
// Durstenfeld, R. (July 1964). "Algorithm 235: Random permutation". Communications of the ACM. 7 (7): 420. doi:10.1145/364520.364540. S2CID 494994.
// Knuth, Donald E. (1969). Seminumerical algorithms. The Art of Computer Programming. Vol. 2. Reading, MA: Addison–Wesley. pp. 139–140. OCLC 85975465.
// Algorithm P (Shuffling)
func FisherYatesShuffle(listRef *[]int) {
	list := *listRef
	for i := len(list) - 1; i > 0; i-- {
		// random number from 0 to i (inclusive)
		j := rand.Intn(i + 1)
		// swap
		z := list[i]
		list[i] = list[j]
		list[j] = z
	}
}

// createExtendedPerm creates a randomly shuffled array, which contains each number
// in 0,...,n-1 exactly i times. This is to be used to shuffle i itSetup of n benchmarks.
func CreateExtendedPerm(n int, i int) *[]int {
	list := make([]int, n*i)
	for m := 0; m < n*i; m++ {
		list[m] = m % n
	}
	FisherYatesShuffle(&list)
	return &list
}

func SetEnvironmentVariables(envSlice []string) {
	log.Debugf("Setting environment variables %v", envSlice)
	for _, env := range envSlice {
		envSplit := strings.SplitN(env, "=", 2)
		if len(envSplit) != 2 {
			continue
		}

		err := os.Setenv(envSplit[0], envSplit[1])
		if err != nil {
			log.Fatalln(err)
		}
	}
	log.Debugf("Setting environment variables was succesful")
}

func RunCommands(commands []string, dir string) {
	log.Debugf("Running provided commands %v", commands)
	for _, command := range commands {
		log.Debugf("Running command: %s", command)
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorln(string(out))
			log.Fatalln(err)
		}
	}
}
