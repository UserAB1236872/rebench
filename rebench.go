package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var (
	speedTolPercent  = flag.Int("speedTol", 150, "Sets the percentage tolerance for a slower benchmark before returning a non-zero error status")
	recordTolPercent = flag.Int("recordTol", 70, "Sets the percentage tolerance for a faster benchmark before overwriting previous speed records")
	help             = flag.Bool("help", false, "Print instructions for the tool instead of running the program")
	quiet            = flag.Bool("q", false, "Squelches the log output")
	helpMsg          = `rebench [[-speedTol int -recordTol int -q] | -help]

The rebench program is used to track benchmarks across development. It may be difficult, unweidly, unwise, or just undesirable to unexport or otherwise move functions just to compare new benchmarks with old ones.

On the first run, this package will backup benchmarks from go test -bench in a hidden json file (hidden in the Unix sense meaning the file name begins with a "."). When run further times, it will compare the benchmark outputs with the previous bests. If the new benchmarks significantly underperform (controllable with the -speedTol flag), this program will exit with status 1. This status is also returned if old benchmarks are missing.

Additionally, if a new benchmark performs significantly better (controllable with -recordTol) it will overwrite the previous best.

It will also output a non-hidden file named bench_comparison.txt which breaks down the new benchmarks, the best benchmarks, and the value of newBench/oldBench.

A list of flags:

-speedTol int: Sets how much slower a benchmark must be in terms of percentages before exiting with a nonzero status. All benchmarks are still run if one fails. "In terms ofpercentages" means that newBenchmarkSpeed/oldBenchMarkSpeed > speedTol. Default is 150 percent

-recordTol int: Sets how much faster a benchmark must be before the previous record is overwitten in .bench_record.json (the comparison file). Works like -speedTol. The default is 70 percent.

-help: Prints this message and then exits.

-q: Quiet mode; mutes log output
`
)

func main() {
	flag.Parse()

	if *help {
		fmt.Println(helpMsg)
		os.Exit(0)
	}

	if *quiet {
		log.SetOutput(ioutil.Discard)
	}
	os.Exit(rebench(*speedTolPercent, *recordTolPercent))
}

//
func rebench(speedTolPercent, recordTolPercent int) int {
	record, err := runAndStoreBenches()
	if err != nil {
		log.Println(err, "aborting!")
		return -1
	}
	if len(record) == 0 {
		log.Println("Nothing to do! No benchmarks!")
		return 0
	}
	var gosrc string
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalln("can't get pwd, exiting:", err.Error())
	}

	speedTol := float64(speedTolPercent) / 100
	recordTol := float64(recordTolPercent) / 100

	for key, _ := range record {
		gosrc = findGosrc(pwd, key)
		if gosrc == "" {
			log.Fatalln("Cannot isolate go source directory (GOPATH/src) given the directory of invocation and go test -bench output. Perhaps you're using symbolic links? Aborting")
		}

		break
	}
	log.Println("Found gosrc (GOPATH/src) as", gosrc, "\n")

	var missing, tooSlow bool
	for pkgPath, benches := range record {
		log.Println("Working in package", pkgPath)
		err := os.Chdir(reform(gosrc, pkgPath))
		if err != nil {
			log.Println("Cannot enter the directory for the package", pkgPath, "("+gosrc+"/"+pkgPath+"), ignoring")
			continue
		}

		log.Println("Checking for and loading best benchmarks")
		// In the future may provide option to compare with the best,
		// or just the previous run
		oldBenches := unmarshallAndStoreBench(".bench_best.json")
		delta, oldBenches, m, ts := compare(oldBenches, benches, pkgPath, speedTol, recordTol)
		missing = missing || m
		tooSlow = tooSlow || ts
		backupMarshallAndStore(tabAlign(delta), benches, oldBenches)
		log.Println()
	}

	exitCode := 0
	if missing {
		log.Println("Old benchmarks were missing, flagging with non-zero return")
		exitCode = 1
	}

	if tooSlow {
		log.Println("New benchmarks are too slow, flagging with non-zero return")
		exitCode = 1
	}

	return exitCode
}

// Compares old benchmarks and new benchmarks. If any old benchmarks are no longer present, it will return a false bool. Same if any benchmarks became noticeably slower (specified by
// the argument speedTol). It will also record a new best if the new benchmark is faster than the specified recordTol and write it as the new best.
//
// May need to be rewritten to compare more things in the future.
func compare(oldBenches, benches map[string]uint64, pkgPath string, speedTol, recordTol float64) (delta string, bestBenches map[string]uint64, missing bool, tooSlow bool) {
	delta = "Benchmark Name\tNew Speed\tBest Speed\tFactor (New/Old)\n"
	if oldBenches != nil {
		var firstMissing bool
		// Missing comparison
		for key, speed := range oldBenches {
			if _, ok := benches[key]; !ok {
				if !firstMissing {
					log.Print("Old benchmarks appear to be missing, is this intentional? List of missing benchmarks: ")
					firstMissing = true
					missing = true
				}
				log.Print(key + " ")
				delta += fmt.Sprintf("%s\tMISSING\t%d\tN/A\n", key, speed)
			}
		}
		log.Println()

		// Speed comparison
		for benchName, speed := range benches {
			if oldSpeed, ok := oldBenches[benchName]; !ok {
				delta += fmt.Sprintf("%s\t%d\tMISSING\tN/A\n", benchName, speed)
				log.Println("Benchmark", benchName, "appears to be new. Not comparing speed, but logging as new best for this benchmark.")
				oldBenches[benchName] = speed
				continue
			} else {
				factor := float64(speed) / float64(oldSpeed)
				delta += fmt.Sprintf("%s\t%d\t%d\t%f\n", benchName, speed, oldSpeed, factor)
				if factor > speedTol {
					log.Println("Benchmark", benchName, "reports a speed", factor, "as fast as the old version. This is slower than expected")
					tooSlow = true
				} else if factor < recordTol {
					oldBenches[benchName] = speed
					log.Println("Benchmark", benchName, "reports a speed", factor, "as fast as the old version. This is a new record according to your threshold!")
				}
			}
		}
	} else {
		log.Println("No best benchmarks on record for this package, recording all current benchmarks (if any) as new best.")
		oldBenches = make(map[string]uint64, len(benches))
		for key, speed := range benches {
			delta += fmt.Sprintf("%s\t%d\tNO FILE\tN/A\n", key, speed)
			oldBenches[key] = speed
		}
	}

	return delta, oldBenches, missing, tooSlow
}

// Goes through the 4-column delta and records the max character word in each column
// Then it pads each column with exactly len(word in this column)-len(max word in this column)+4 spaces
// (that is, the next column always starts at 4 spaces after the largest word in that column)
//
// Could easily be, and probably will be, generalized for any string with a uniform number of columns
func tabAlign(delta string) string {
	rows := strings.Split(delta, "\n")

	max := [4]int{}
	for _, row := range rows {
		cols := strings.Split(row, "\t")
		if len(cols) != 4 {
			continue
		}

		for i, str := range cols {
			max[i] = intMax(max[i], len(str))
		}
	}

	aligned := make([]string, len(rows))
	for r, row := range rows {
		cols := strings.Split(row, "\t")
		if len(cols) != 4 {
			continue
		}

		str := cols[0]
		for i := 0; i < len(cols)-1; i++ {
			str += strings.Repeat(" ", max[i]-len(cols[i])+4)
			str += cols[i+1]
		}
		aligned[r] = str
	}

	return strings.Join(aligned, "\n")
}

func intMax(a, b int) int {
	if a > b {
		return a
	}

	return b
}

// Just file i/o. Backs up all files it can in <filename>.old (hiding it if not hidden by prepending ".")
// Then it marshalls the data and writes it in the corresponding file.
//
// This should avoid scribbling in directories with no benchmarks
func backupMarshallAndStore(delta string, benches map[string]uint64, newBest map[string]uint64) {
	if _, err := os.Stat(".bench_results.json"); !os.IsNotExist(err) {
		os.Remove(".bench_results.json.old")
		log.Println("Backing up .bench_results.json in .bench_results.json.old")
		err = os.Rename(".bench_results.json", ".bench_results.json.old")
		if err != nil {
			log.Println("Could not back up benchmarks file, overwriting if possible")
		}
	}

	if _, err := os.Stat(".bench_best.json"); !os.IsNotExist(err) {
		log.Println("Backing up .bench_best.json in .bench_best.json.old")
		err = os.Remove(".bench_best.json.old")
		err = os.Rename(".bench_best.json", ".bench_best.json.old")
		if err != nil {
			log.Println("Could not back up best benchmarks file, overwriting if possible")
		}
	}

	if _, err := os.Stat("bench_comparison.txt"); !os.IsNotExist(err) {
		log.Println("Backing up bench_comparison.txt in .bench_comparison.txt.old")
		os.Remove(".bench_comparison.txt.old")
		err = os.Rename("bench_comparison.txt", ".bench_comparison.txt.old")
		if err != nil {
			log.Println("Could not back up comparison file, overwriting if possible")
		}

	}

	if len(benches) > 0 {
		out, err := json.Marshal(benches)
		if err != nil {
			log.Println("Couldn't marshall benchmarks as json")
		} else {
			err = ioutil.WriteFile(".bench_results.json", out, 0666)
			if err != nil {
				log.Println("Couldn't write benchmark results in current directory")
			}
		}
	}

	if len(newBest) > 0 {
		out, err := json.Marshal(newBest)
		if err != nil {
			log.Println("Couldn't marshall benchmarks as json")
		} else {
			err = ioutil.WriteFile(".bench_best.json", out, 0666)
			if err != nil {
				log.Println("Couldn't write benchmark results in current directory")
			}
		}
	}

	if len(benches) > 0 || len(newBest) > 0 {
		err := ioutil.WriteFile("bench_comparison.txt", []byte(delta), 0666)
		if err != nil {
			log.Println("Could not write benchmark comparisons file")
		}
	}
}

func findGosrc(pwd, pkgName string) string {
	path := convertPath(pkgName)

	index := strings.LastIndex(pwd, path)
	if index <= 1 {
		return ""
	}

	// index-1 also lops off the terminating / (or \ on Windows)
	return pwd[:index-1]
}

func runAndStoreBenches() (map[string]map[string]uint64, error) {

	log.Println("Running go test -bench=. -run=lksadfjalsdjfalskdfjalskdf ./...")

	// -run=lksadfjalsdjfalskdfjalskdf makes it... incredibly unlikely that the tool will run any tests
	// I know of no way to outright inform "go test" to outright not run any TestXxx functions.
	gotest := exec.Command("go", "test", "-bench=.", "-run=lksadfjalsdjfalskdfjalskdf", "./...")
	out, err := gotest.Output()
	if err != nil {
		log.Println("go test returned with non-zero return value, aborting")
		return nil, errors.New("Problem running go test")
	}

	outstr := string(out)

	benches := strings.Split(outstr, "\n")

	record := make(map[string]map[string]uint64)
	curr := make(map[string]uint64)
	log.Println("Parsing the results of go test...")
	for _, line := range benches {
		result := strings.Split(line, "\t")

		for i, word := range result {
			result[i] = strings.TrimSpace(word)
		}

		if len(result) < 3 || result[0] == "?" {
			continue
		}

		if strings.HasPrefix(result[0], "Benchmark") {
			time := strings.TrimRight(result[2], " ns/op")
			t, err := strconv.ParseUint(time, 10, 64)
			if err != nil {
				log.Println("could not properly convert benchmark time into uint64: ", err.Error())
				return nil, errors.New("Couldn't convert benchmark time to uint64")
			}

			curr[result[0]] = t
		} else if result[0] == "ok" {
			record[result[1]] = curr
			curr = make(map[string]uint64)
		}
	}

	return record, nil
}

func unmarshallAndStoreBench(fileName string) map[string]uint64 {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		log.Println("previous benchmark file does not exist for current directory")
		return nil
	}

	raw, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Println("cannot open", fileName, "for current benchmark directory")
		return nil
	}

	out := make(map[string]uint64)
	err = json.Unmarshal(raw, &out)
	if err != nil {
		log.Printf("cannot unmarshall json for file %s because: %v\n", fileName, err)
		return nil
	}

	return out
}
