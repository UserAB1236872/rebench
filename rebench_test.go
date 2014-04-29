package main

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func cd(t *testing.T) string {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Cannot get pwd %v", err)
	}
	err = os.Chdir(reform(pwd, "testpackage"))
	if err != nil {
		t.Fatalf("Cannot cd into test package")
	}

	return pwd
}

// From https://gist.github.com/elazarl/5507969
func cp(dst, src string, t *testing.T) error {
	s, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		t.Fatal(err)
	}
	return d.Close()
}

func cleanup(top string) {
	os.Remove(".bench_results.json.old")
	os.Remove(".bench_results.json")
	os.Remove(".bench_best.json.old")
	os.Remove(".bench_best.json")
	os.Remove("bench_comparison.txt")
	os.Remove(".bench_comparison.txt.old")

	if err := os.Chdir(top); err != nil {
		panic(err)
	}
}

func TestEmpty(t *testing.T) {
	top := cd(t)
	defer cleanup(top)

	code := rebench(150, 70)
	if code != 0 {
		t.Errorf("Program returned non-zero exit code for valid invocation")
	}

	result := unmarshallAndStoreBench(".bench_results.json")
	if len(result) != 2 {
		t.Fatalf("Wrong number of results %v", result)
	}

	result = unmarshallAndStoreBench(".bench_best.json")
	if len(result) != 2 {
		t.Fatalf("Wrong number of best results %v", result)
	}
}

func TestRealBenchIsSlower(t *testing.T) {
	top := cd(t)
	defer cleanup(top)
	cp(".bench_best.json", reform(top, "testpackage", ".mockoutputs", "obviously_faster.json"), t)

	code := rebench(150, 70)
	if code == 0 {
		t.Errorf("Program returned good exit code when best benchmark is obviously faster")
	}

	best := unmarshallAndStoreBench(".bench_best.json")

	if best["BenchmarkSleep"] != 500 || best["BenchmarkSleep2"] != 10000 {
		t.Errorf("Either read or wrote best benchmarks incorrectly %v", best)
	}
}

func TestRealBenchIsFaster(t *testing.T) {
	top := cd(t)
	defer cleanup(top)
	cp(".bench_best.json", reform(top, "testpackage", ".mockoutputs", "2xslower.json"), t)

	code := rebench(150, 70)
	if code != 0 {
		t.Errorf("Program returned bad exit code when real benchmark is obviously faster")
	}

	result := unmarshallAndStoreBench(".bench_results.json")

	best := unmarshallAndStoreBench(".bench_best.json")

	if best["BenchmarkSleep"] != result["BenchmarkSleep"] || best["BenchmarkSleep2"] != result["BenchmarkSleep2"] {
		t.Errorf("New best benchmarks don't match real bests (should have been overwritten due to speed)")
	}
}

func TestRealBenchHasMore(t *testing.T) {
	top := cd(t)
	defer cleanup(top)
	cp(".bench_best.json", reform(top, "testpackage", ".mockoutputs", "missing.json"), t)

	code := rebench(150, 70)
	if code != 0 {
		t.Errorf("Program returned bad exit code when real benchmark has more benchmarks than best")
	}

	result := unmarshallAndStoreBench(".bench_results.json")
	best := unmarshallAndStoreBench(".bench_best.json")

	if len(best) != 2 || best["BenchmarkSleep2"] != result["BenchmarkSleep2"] {
		t.Errorf("Missing benchmark is either not written or written incorrectly")
	}
}

func TestRealBenchMissing(t *testing.T) {
	top := cd(t)
	defer cleanup(top)
	cp(".bench_best.json", reform(top, "testpackage", ".mockoutputs", "toomany.json"), t)

	code := rebench(150, 70)
	if code == 0 {
		t.Errorf("Program returned good exit code when real benchmark is missing benchmarks")
	}

	result := unmarshallAndStoreBench(".bench_results.json")
	if len(result) != 2 {
		t.Errorf("Current result erroneously wrote output from best file that shouldn't be there")
	}
	best := unmarshallAndStoreBench(".bench_best.json")

	if len(best) != 3 {
		t.Errorf("Didn't write missing benchmark back out")
	}
}
