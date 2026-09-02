package battery_info

import (
	"context"
	"embed"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/power_supply"
)

//go:embed *.html
var assets embed.FS

type row struct {
	section bool
	label   string
	value   string
}

type rowView struct {
	root  fbiw.Box
	label *fbiw.Text `css:".label"`
	value *fbiw.Text `css:".value"`
}

type Window struct {
	app     *fbiw.App
	doc     *fbiw.Document
	reader  *power_supply.Reader
	summary *fbiw.Text   `css:"#summary"`
	scroll  *fbiw.Scroll `css:"#fields"`
	empty   fbiw.Box     `css:"#empty"`
	status  *fbiw.Text   `css:"#status"`
	rows    []row
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	busy    bool
	pending bool
}

func New(app *fbiw.App, reader *power_supply.Reader) *Window {
	if reader == nil {
		reader = power_supply.NewReader("")
	}
	ctx, cancel := context.WithCancel(app.Context())
	w := &Window{app: app, reader: reader, ctx: ctx, cancel: cancel}
	w.doc = app.NewDesktop(assets, "window.html")
	w.doc.Bind(w)
	w.scroll.Listen(fbiw.StickDownEvent, w.handleEvents)
	w.scroll.Activate()
	w.refresh()
	return w
}

func (w *Window) setStatus(message string, warning bool) {
	w.status.SetText(message)
	if warning {
		w.status.ClassAdd("warning")
	} else {
		w.status.ClassRemove("warning")
	}
}

func (w *Window) queueRefresh() {
	w.mu.Lock()
	if w.busy {
		w.pending = true
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	w.refresh()
}

func (w *Window) refresh() {
	w.mu.Lock()
	if w.busy {
		w.pending = true
		w.mu.Unlock()
		return
	}
	w.busy = true
	w.mu.Unlock()
	w.app.Async(func() { w.setStatus("正在读取电源信息…", false) })
	go func() {
		supplies, err := w.reader.List()
		w.app.Async(func() {
			if w.ctx.Err() != nil {
				return
			}
			w.render(supplies)
			if err != nil {
				w.setStatus("部分数据读取失败："+err.Error(), true)
			} else {
				w.setStatus("X 刷新 · B 返回", false)
			}
		})
		w.mu.Lock()
		w.busy = false
		pending := w.pending
		w.pending = false
		w.mu.Unlock()
		if pending && w.ctx.Err() == nil {
			w.refresh()
		}
	}()
}

func (w *Window) render(supplies []power_supply.Supply) {
	w.rows = buildRows(supplies)
	w.summary.SetText(buildSummary(supplies))
	w.scroll.SetItems(len(w.rows), func() (fbiw.Box, *rowView) {
		view := fbiw.Unmarshal[rowView](w.doc, `
<block padding="0 10" align=middle border-width=3>
	<inline font-size=small>
		<text class="label"></text>
		<spacer></spacer>
		<text class="value mono"></text>
	</inline>
</block>
`)
		return view.root, view
	}, func(view *rowView, index int) {
		item := w.rows[index]
		view.label.SetText(item.label)
		view.value.SetText(item.value)
		if item.section {
			view.root.ClassAdd("section")
		} else {
			view.root.ClassRemove("section")
		}
	})
	w.scroll.SetProp("display", fmt.Sprint(len(supplies) > 0))
	w.empty.SetProp("display", fmt.Sprint(len(supplies) == 0))
}

func (w *Window) handleEvents(event *fbiw.Event) {
	switch event.Stick.Name {
	case fbiw.B:
		w.cancel()
		w.doc.Close()
	case fbiw.X:
		w.queueRefresh()
	}
}

var groups = []struct {
	name string
	keys []string
}{
	{"概览", []string{
		"POWER_SUPPLY_NAME",
		"POWER_SUPPLY_TYPE",
		"POWER_SUPPLY_PRESENT",
		"POWER_SUPPLY_ONLINE",
		"POWER_SUPPLY_STATUS",
		"POWER_SUPPLY_HEALTH",
		"POWER_SUPPLY_CAPACITY",
		"POWER_SUPPLY_CAPACITY_LEVEL",
		"POWER_SUPPLY_CAPACITY_ALERT_MIN",
	}},
	{"电气", []string{
		"POWER_SUPPLY_VOLTAGE_NOW",
		"POWER_SUPPLY_VOLTAGE_MIN_DESIGN",
		"POWER_SUPPLY_VOLTAGE_MAX_DESIGN",
		"POWER_SUPPLY_CURRENT_NOW",
		"POWER_SUPPLY_CURRENT_AVG",
		"POWER_SUPPLY_POWER_NOW",
		"POWER_SUPPLY_CHARGE_NOW",
		"POWER_SUPPLY_CHARGE_COUNTER",
		"POWER_SUPPLY_CHARGE_FULL",
		"POWER_SUPPLY_CHARGE_FULL_DESIGN",
		"POWER_SUPPLY_ENERGY_NOW",
		"POWER_SUPPLY_ENERGY_FULL",
		"POWER_SUPPLY_ENERGY_FULL_DESIGN",
		"POWER_SUPPLY_CONSTANT_CHARGE_CURRENT",
		"POWER_SUPPLY_INPUT_CURRENT_LIMIT",
	}},
	{"温度", []string{
		"POWER_SUPPLY_TEMP",
		"POWER_SUPPLY_TEMP_ALERT_MIN",
		"POWER_SUPPLY_TEMP_ALERT_MAX",
		"POWER_SUPPLY_TEMP_AMBIENT",
		"POWER_SUPPLY_TEMP_AMBIENT_ALERT_MIN",
		"POWER_SUPPLY_TEMP_AMBIENT_ALERT_MAX",
	}},
	{"时间", []string{
		"POWER_SUPPLY_TIME_TO_EMPTY_NOW",
		"POWER_SUPPLY_TIME_TO_FULL_NOW",
	}},
	{"标识", []string{
		"POWER_SUPPLY_MANUFACTURER",
		"POWER_SUPPLY_MODEL_NAME",
		"POWER_SUPPLY_SERIAL_NUMBER",
		"POWER_SUPPLY_TECHNOLOGY",
		"POWER_SUPPLY_CYCLE_COUNT",
	}},
}

func buildRows(supplies []power_supply.Supply) []row {
	var rows []row
	for _, supply := range supplies {
		title := supply.Name
		if supply.Type != "" {
			title += " · " + power_supply.Format("POWER_SUPPLY_TYPE", supply.Type)
		}
		rows = append(rows, row{section: true, label: title})
		if supply.Error != "" {
			rows = append(rows, row{label: "读取错误", value: supply.Error})
		}
		for _, group := range groups {
			start := len(rows)
			rows = append(rows, row{section: true, label: group.name})
			for _, key := range group.keys {
				if value, ok := supply.Values[key]; ok {
					rows = append(rows, row{label: power_supply.Label(key), value: power_supply.Format(key, value)})
				}
			}
			if len(rows) == start+1 {
				rows = rows[:start]
			}
		}
		keys := make([]string, 0, len(supply.Values))
		for key := range supply.Values {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if len(keys) > 0 {
			rows = append(rows, row{section: true, label: "原始数据"})
			for _, key := range keys {
				rows = append(rows, row{label: key, value: supply.Values[key]})
			}
		}
	}
	return rows
}

func buildSummary(supplies []power_supply.Supply) string {
	battery, ok := power_supply.FirstBattery(supplies)
	if !ok {
		if len(supplies) == 0 {
			return "未发现电池或外部供电设备"
		}
		return fmt.Sprintf("未发现电池 · 已发现 %d 个外部供电设备", len(supplies))
	}
	var parts []string
	for _, field := range []struct{ key, label string }{
		{"capacity", "电量"},
		{"status", "状态"},
		{"health", "健康"},
		{"temp", "温度"},
		{"voltage_now", "电压"},
		{"time_to_empty_now", "预计耗尽"},
		{"time_to_full_now", "预计充满"},
	} {
		if value, found := battery.Value(field.key); found {
			key := "POWER_SUPPLY_" + strings.ToUpper(field.key)
			parts = append(parts, field.label+"："+power_supply.Format(key, value))
		}
	}
	if len(parts) == 0 {
		return battery.Name
	}
	return strings.Join(parts, "    ")
}
