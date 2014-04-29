package testpackage_test

import (
	"testing"
	"time"
)

// Benchmarks on a Sleep of seconds length should roughly be dominated by
// the time we sleep (+/- some startup time). So easy to test our program
// with it. A bit slow, granted
func BenchmarkSleep(b *testing.B) {
	for i := 0; i < b.N; i++ {
		time.Sleep(1 * time.Second)
	}
}

func BenchmarkSleep2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		time.Sleep(5 * time.Second)
	}
}
