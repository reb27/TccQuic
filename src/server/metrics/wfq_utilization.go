package metrics

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type WFQUtil struct {
	mu      sync.Mutex
	file    *os.File
	w       *csv.Writer
	ticker  *time.Ticker
	stop    chan struct{}
	classes []ClassInt

	weights    map[ClassInt]float64 // pesos alvo normalizados
	rawWeights map[ClassInt]float64 // pesos brutos (não normalizados)
	bytes      map[ClassInt]int64   // bytes por janela
	adapted    bool                 // houve recálculo dinâmico nesta janela?
}

var wfqOnce sync.Once
var wfqInst *WFQUtil
var pendingWFQWeights map[ClassInt]float64

func cloneWeights(weights map[ClassInt]float64) map[ClassInt]float64 {
	cloned := make(map[ClassInt]float64, len(weights))
	for k, v := range weights {
		cloned[k] = v
	}
	return cloned
}

func applyWFQWeightsLocked(u *WFQUtil, weights map[ClassInt]float64) {
	u.weights = map[ClassInt]float64{}
	u.rawWeights = map[ClassInt]float64{}
	var sum float64
	for _, c := range u.classes {
		sum += weights[c]
	}
	if sum <= 0 {
		return
	}
	for _, c := range u.classes {
		u.weights[c] = weights[c] / sum
		u.rawWeights[c] = weights[c]
	}
}

func StartWFQUtilWriter(csvPath string, classes []ClassInt, interval time.Duration) {
	wfqOnce.Do(func() {
		if csvPath == "" {
			csvPath = "/tmp/server_scheduler_test/wfq_utilization.csv"
		}
		_ = os.MkdirAll(filepath.Dir(csvPath), 0o755)
		f, _ := os.OpenFile(csvPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		w := csv.NewWriter(f)
		if st, _ := f.Stat(); st != nil && st.Size() == 0 {
			_ = w.Write([]string{
				"ts", "w_low", "w_medium", "w_high",
				"share_low", "share_medium", "share_high",
				"err_low", "err_medium", "err_high", "mae",
				"bytes_low", "bytes_medium", "bytes_high",
				"raw_w_low", "raw_w_medium", "raw_w_high",
				"ratio_low", "ratio_medium", "ratio_high",
				"adapted",
			})
			w.Flush()
		}
		wfqInst = &WFQUtil{
			file:       f,
			w:          w,
			ticker:     time.NewTicker(interval),
			stop:       make(chan struct{}),
			classes:    classes,
			weights:    map[ClassInt]float64{},
			rawWeights: map[ClassInt]float64{},
			bytes:      map[ClassInt]int64{},
		}
		if len(pendingWFQWeights) > 0 {
			wfqInst.mu.Lock()
			applyWFQWeightsLocked(wfqInst, pendingWFQWeights)
			wfqInst.mu.Unlock()
		}
		go wfqInst.loop()
	})
}

func StopWFQUtilWriter() {
	if wfqInst == nil {
		return
	}
	close(wfqInst.stop)
	wfqInst.ticker.Stop()
	wfqInst.w.Flush()
	_ = wfqInst.file.Close()
	wfqInst = nil
}

// defina os pesos WFQ (ex.: {low:1, medium:2, high:3})
func SetWFQWeights(weights map[ClassInt]float64) {
	pendingWFQWeights = cloneWeights(weights)
	if wfqInst == nil {
		return
	}
	wfqInst.mu.Lock()
	applyWFQWeightsLocked(wfqInst, weights)
	wfqInst.mu.Unlock()
}

// SetWFQAdapted sinaliza se houve recálculo dinâmico de pesos nesta janela.
// Chamada por recalcWFQWeights() a cada rounding.
func SetWFQAdapted(v bool) {
	if wfqInst == nil {
		return
	}
	wfqInst.mu.Lock()
	wfqInst.adapted = v
	wfqInst.mu.Unlock()
}

// chame quando uma requisição COMPLETA enviar bytes>0
func RecordBytesForWFQ(class ClassInt, bytes int) {
	if wfqInst == nil || bytes <= 0 {
		return
	}
	wfqInst.mu.Lock()
	wfqInst.bytes[class] += int64(bytes)
	wfqInst.mu.Unlock()
}

func (u *WFQUtil) loop() {
	for {
		select {
		case <-u.stop:
			return
		case now := <-u.ticker.C:
			u.mu.Lock()
			var total int64
			for _, c := range u.classes {
				total += u.bytes[c]
			}
			obs := map[ClassInt]float64{}
			for _, c := range u.classes {
				if total > 0 {
					obs[c] = float64(u.bytes[c]) / float64(total)
				} else {
					obs[c] = 0
				}
			}
			err := map[ClassInt]float64{}
			var mae float64
			for _, c := range u.classes {
				e := math.Abs(obs[c] - u.weights[c])
				err[c] = e
				mae += e
			}
			mae /= float64(len(u.classes))

			// Calcular razão throughput/weight por classe
			ratio := map[ClassInt]float64{}
			for _, c := range u.classes {
				if u.rawWeights[c] > 0 {
					ratio[c] = float64(u.bytes[c]) / u.rawWeights[c]
				} else {
					ratio[c] = 0
				}
			}

			adaptedStr := "0"
			if u.adapted {
				adaptedStr = "1"
			}
			rec := []string{
				now.Format(time.RFC3339Nano),
				f614(u.weights[0]), f614(u.weights[1]), f614(u.weights[2]),
				f614(obs[0]), f614(obs[1]), f614(obs[2]),
				f614(err[0]), f614(err[1]), f614(err[2]),
				f614(mae),
				strconv.FormatInt(u.bytes[0], 10),
				strconv.FormatInt(u.bytes[1], 10),
				strconv.FormatInt(u.bytes[2], 10),
				f614(u.rawWeights[0]), f614(u.rawWeights[1]), f614(u.rawWeights[2]),
				f614(ratio[0]), f614(ratio[1]), f614(ratio[2]),
				adaptedStr,
			}
			_ = u.w.Write(rec)
			u.w.Flush()
			for k := range u.bytes {
				u.bytes[k] = 0
			}
			u.adapted = false // resetar após gravar
			u.mu.Unlock()
		}
	}
}

func f614(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
