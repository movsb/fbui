// Package elastic_cpu implements opt-in, system-wide CPU hotplug and frequency
// control for Linux handhelds. It does not change process affinity.
package elastic_cpu

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const root = "/sys/devices/system/cpu"
const sampleInterval = 3 * time.Second

type setting struct{ path, value string }
type policy struct {
	dir, governor  string
	min, max, idle int
}
type controller struct {
	cpus     []string
	policies []policy
	saved    []setting
	count    int
	quiet    time.Duration
}

// Start enables ... Stop waits for
// restoration; the caller must call it before leaving main. SIGKILL/power loss
// cannot run restoration. A failed write stops the controller and restores state.
func Start(gameRunning func() bool) func() {
	c, err := discover(root)
	if err != nil {
		log.Printf("弹性 CPU 未启用: %v", err)
		return func() {}
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		defer c.restore()
		ticker := time.NewTicker(sampleInterval)
		defer ticker.Stop()
		prev, err := readStats("/proc/stat")
		if err != nil {
			log.Printf("弹性 CPU: %v", err)
			return
		}
		wasGame := false
		lastCount := 0
		lastSample := time.Now()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				elapsed := now.Sub(lastSample)
				lastSample = now
				curr, err := readStats("/proc/stat")
				if err != nil {
					log.Printf("弹性 CPU: %v", err)
					return
				}
				game := gameRunning()
				mean, peak, valid := utilization(prev, curr)
				prev = curr
				if game && !wasGame {
					c.count = min(2, len(c.cpus))
					c.quiet = -10 * time.Second
				}
				if !game {
					c.count = 1
					c.quiet = 0
				} else if valid {
					c.step(mean, peak, elapsed)
				}
				if c.count != lastCount || game != wasGame {
					log.Printf("弹性 CPU: 核数=%d, 外部程序=%t", c.count, game)
					lastCount = c.count
				}
				if err := c.apply(game); err != nil {
					log.Printf("弹性 CPU 停止: %v", err)
					return
				}
				wasGame = game
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stop); <-done }) }
}

func readInt(path string) (int, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return 0, e
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}
func discover(base string) (*controller, error) {
	c := &controller{count: 1}
	save := func(path string) error {
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		c.saved = append(c.saved, setting{path, strings.TrimSpace(string(b))})
		return nil
	}
	// CPU0 stays online. Only contiguous, hotpluggable cores are managed.
	c.cpus = append(c.cpus, "")
	for i := 1; ; i++ {
		path := filepath.Join(base, fmt.Sprintf("cpu%d/online", i))
		if _, e := os.Stat(path); os.IsNotExist(e) {
			break
		}
		if e := save(path); e != nil {
			return nil, e
		}
		c.cpus = append(c.cpus, path)
	}
	dirs, _ := filepath.Glob(filepath.Join(base, "cpufreq/policy*"))
	if len(dirs) == 0 {
		dirs, _ = filepath.Glob(filepath.Join(base, "cpu0/cpufreq"))
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("未发现 CPUFreq policy")
	}
	for _, dir := range dirs {
		p := policy{dir: dir}
		for _, name := range []string{"scaling_governor", "scaling_min_freq", "scaling_max_freq"} {
			if e := save(filepath.Join(dir, name)); e != nil {
				return nil, e
			}
		}
		var err error
		if p.min, err = readInt(filepath.Join(dir, "cpuinfo_min_freq")); err != nil {
			return nil, err
		}
		if p.max, err = readInt(filepath.Join(dir, "scaling_max_freq")); err != nil {
			return nil, err
		}
		if p.max < p.min {
			return nil, fmt.Errorf("无效频率范围: %s", dir)
		}
		governors, e := os.ReadFile(filepath.Join(dir, "scaling_available_governors"))
		if e != nil {
			return nil, e
		}
		for _, preferred := range []string{"schedutil", "ondemand"} {
			for _, g := range strings.Fields(string(governors)) {
				if g == preferred && p.governor == "" {
					p.governor = g
				}
			}
		}
		if p.governor == "" {
			return nil, fmt.Errorf("%s 不支持 schedutil/ondemand", dir)
		}
		p.idle = p.min + (p.max-p.min)/3
		c.policies = append(c.policies, p)
	}
	return c, nil
}

func write(path, value string) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(b)) == value {
		return nil
	}
	if e = os.WriteFile(path, []byte(value), 0644); e != nil {
		return fmt.Errorf("%s: %w", path, e)
	}
	return nil
}
func (c *controller) apply(game bool) error {
	// Online before adjusting policy; frequency is shared by the policy's cores.
	for i := 1; i < c.count; i++ {
		if e := write(c.cpus[i], "1"); e != nil {
			return e
		}
	}
	for _, p := range c.policies {
		cap := p.idle
		if game {
			cap = p.max
		}
		for _, s := range []setting{
			{filepath.Join(p.dir, "scaling_min_freq"), strconv.Itoa(p.min)},
			{filepath.Join(p.dir, "scaling_max_freq"), strconv.Itoa(cap)},
			{filepath.Join(p.dir, "scaling_governor"), p.governor},
		} {
			if e := write(s.path, s.value); e != nil {
				return e
			}
		}
	}
	for i := len(c.cpus) - 1; i >= c.count; i-- {
		if e := write(c.cpus[i], "0"); e != nil {
			return e
		}
	}
	return nil
}
func (c *controller) restore() {
	// Re-enable cores so their policy interfaces are available during restoration.
	for _, path := range c.cpus[1:] {
		if e := write(path, "1"); e != nil {
			log.Printf("恢复 CPU: %v", e)
		}
	}
	for i := len(c.saved) - 1; i >= 0; i-- {
		s := c.saved[i]
		if e := write(s.path, s.value); e != nil {
			log.Printf("恢复 CPU: %v", e)
		}
	}
}
func (c *controller) step(mean, peak float64, elapsed time.Duration) {
	if mean >= .80 || peak >= .90 {
		c.count = min(c.count+1, len(c.cpus))
		c.quiet = -5 * time.Second
		return
	}
	// Shrink only when the remaining cores have <55% predicted load.
	if c.quiet < 0 {
		c.quiet = min(0, c.quiet+elapsed)
		return
	}
	if c.count > 1 && mean*float64(c.count)/float64(c.count-1) < .55 && peak < .70 {
		c.quiet += elapsed
		if c.quiet >= 10*time.Second {
			c.count--
			c.quiet = 0
		}
	} else {
		c.quiet = 0
	}
}

type stat struct{ busy, total uint64 }

func readStats(path string) (map[string]stat, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	out := map[string]stat{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 9 || !strings.HasPrefix(f[0], "cpu") || f[0] == "cpu" {
			continue
		}
		var s stat
		for i := 1; i <= 8; i++ {
			v, e := strconv.ParseUint(f[i], 10, 64)
			if e != nil {
				return nil, e
			}
			s.total += v
			if i != 4 && i != 5 {
				s.busy += v
			}
		}
		out[f[0]] = s
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有 CPU 统计")
	}
	return out, nil
}
func utilization(prev, curr map[string]stat) (mean, peak float64, valid bool) {
	// Skip hotplug transitions; newly online cores carry old cumulative counters.
	if len(prev) != len(curr) {
		return
	}
	for name, s := range curr {
		p, ok := prev[name]
		if !ok || s.total <= p.total || s.busy < p.busy {
			return 0, 0, false
		}
		u := float64(s.busy-p.busy) / float64(s.total-p.total)
		mean += u
		peak = max(peak, u)
	}
	return mean / float64(len(curr)), peak, true
}
