package elastic_cpu

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestElasticSteps(t *testing.T) {
	c := controller{cpus: make([]string, 4), count: 1}
	c.step(.85, .85, 500*time.Millisecond)
	if c.count != 2 {
		t.Fatal("must expand on saturation")
	}
	for range 29 {
		c.step(.1, .1, 500*time.Millisecond)
	}
	if c.count != 2 {
		t.Fatal("shrunk before cooldown and quiet window")
	}
	c.step(.1, .1, 500*time.Millisecond)
	if c.count != 1 {
		t.Fatal("must shrink after sustained idle")
	}
	c.count = 2
	c.quiet = 0
	for range 40 {
		c.step(.4, .5, 500*time.Millisecond)
	}
	if c.count != 2 {
		t.Fatal("remaining core would be overloaded")
	}
	c.step(.5, .95, 500*time.Millisecond)
	if c.count != 3 {
		t.Fatal("busy main thread must trigger expansion")
	}
}

func TestHotplugStats(t *testing.T) {
	p := map[string]stat{"cpu0": {20, 100}}
	n := map[string]stat{"cpu0": {100, 200}}
	mean, peak, ok := utilization(p, n)
	if !ok || mean != .8 || peak != .8 {
		t.Fatal(mean, peak, ok)
	}
	n["cpu1"] = stat{1000, 2000}
	if _, _, ok := utilization(p, n); ok {
		t.Fatal("must skip hotplug boundary")
	}
}

func TestApplyAndRestore(t *testing.T) {
	base := t.TempDir()
	put := func(path, value string) {
		t.Helper()
		p := filepath.Join(base, path)
		if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte(value), 0644); e != nil {
			t.Fatal(e)
		}
	}
	put("cpu1/online", "1")
	put("cpu2/online", "0")
	for k, v := range map[string]string{"scaling_governor": "performance", "scaling_available_governors": "performance ondemand", "scaling_min_freq": "1008000", "scaling_max_freq": "466962", "cpuinfo_max_freq": "2000000", "cpuinfo_min_freq": "200000"} {
		put("cpufreq/policy0/"+k, v)
	}
	c, e := discover(base)
	if e != nil {
		t.Fatal(e)
	}
	check := func(path, want string) {
		t.Helper()
		b, e := os.ReadFile(filepath.Join(base, path))
		if e != nil || string(b) != want {
			t.Fatalf("%s = %q (%v), want %q", path, b, e, want)
		}
	}
	if e := c.apply(false); e != nil {
		t.Fatal(e)
	}
	check("cpu1/online", "0")
	check("cpufreq/policy0/scaling_max_freq", "800000")
	check("cpufreq/policy0/scaling_min_freq", "200000")
	c.count = 2
	if e := c.apply(true); e != nil {
		t.Fatal(e)
	}
	check("cpu1/online", "1")
	check("cpufreq/policy0/scaling_max_freq", "2000000")
	check("cpufreq/policy0/scaling_min_freq", "1008000")
	// Rediscovery after a restart must not derive another lower idle cap.
	again, err := discover(base)
	if err != nil {
		t.Fatal(err)
	}
	if again.policies[0].idle != 800000 {
		t.Fatal("idle cap compounded after restart")
	}
	c.restore()
	check("cpu1/online", "1")
	check("cpu2/online", "0")
	check("cpufreq/policy0/scaling_max_freq", "466962")
	check("cpufreq/policy0/scaling_governor", "performance")
	check("cpufreq/policy0/scaling_min_freq", "1008000")
}

func TestSlowSamplingCooldown(t *testing.T) {
	c := controller{cpus: make([]string, 4), count: 1}
	c.step(.85, .85, sampleInterval)
	for range 5 {
		c.step(.1, .1, sampleInterval)
	}
	if c.count != 2 {
		t.Fatal("must retain cooldown and quiet period")
	}
	c.step(.1, .1, sampleInterval)
	if c.count != 1 {
		t.Fatal("slow sampling must not multiply cooldown duration")
	}
}
